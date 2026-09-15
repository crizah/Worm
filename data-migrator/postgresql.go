package datamigrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	datawriter "github.com/crizah/Worm/data-writer"
	"github.com/crizah/Worm/schema"
	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresDataMigrator struct {
	db         *sql.DB                  // our postgres db connection
	connStr    string                   // our postgres conn string, needed to create replication connection
	snapshotId string                   // returned snapshot id
	stateDb    *sql.DB                  // state db to track snapshot
	tables     []string                 // table names sorted that we get from emitter
	tableIndex map[string]*schema.Index // the index we use for pagination, mapped to its table
	limit      int                      // the batch limit of how many rows to process at a time, dont let it exceed 999
	colMap     map[string]schema.Type   // stores the column type mapped to tableName+columnName
	writer     datawriter.DM            // the writer interface

	slotName string // current slot name
	lsn      pglogrepl.LSN
	replConn *pgconn.PgConn // kept open on purpose: closing it invalidates the exported snapshot

	snapshotTx *sql.Tx // the transaction for the snapshot
}

func getIndex(t *schema.Table) (*schema.Index, error) {
	// gives us the appropriate index to do pagination with
	// priority: pk -> unique (not null)
	// if neither exist, fail. TODO: have this faliure in the inspecter phase itself\
	for _, index := range t.Indexes {
		if index.IsPK {
			return index, nil
		}
	}

	// we didnt find a pk, check for non nullable unique contraints
	for _, index := range t.Indexes {
		if index.IsUnique {
			// check if all the columns assocoated with it are not nullable
			flag := false
			for _, col := range index.Columns {
				if col.IsNullable {
					flag = true
					break
				}
			}
			if !flag {
				// this index is valid
				return index, nil
			}
		}
	}
	// cant have any index
	return nil, fmt.Errorf("Didnt find any valid index") // TODO: again, have this failure exist on inspecter phase itself
}

func NewPostgresDM(conn string, stateDb string, sc []*schema.Table, limit int, w datawriter.DM) (*PostgresDataMigrator, error) {
	// setup and ping postgres db
	db, err := sql.Open("postgres", conn)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}
	// ping
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging postgres db: %w", err)
	}
	var t []string
	var indexTable = make(map[string]*schema.Index) // maps table name to the index we use
	cMap := make(map[string]schema.Type)
	for _, table := range sc {
		t = append(t, table.Name)
		index, err := getIndex(table)

		if err != nil {
			return nil, err
		}
		for _, col := range table.Columns {
			s := table.Name + col.Name
			cMap[s] = col.Type
		}

		indexTable[table.Name] = index
	}

	p := &PostgresDataMigrator{db: db, connStr: conn, tables: t, tableIndex: indexTable, limit: limit, colMap: cMap, writer: w}

	// setup and ping sqlite state db
	// if db file doesnt exist, make one with that fileName
	if _, err := os.Stat(stateDb); os.IsNotExist(err) {
		f, err := os.Create(stateDb)
		if err != nil {
			return nil, fmt.Errorf("creating sqlite db file: %w", err)
		}
		f.Close()
	}

	sqlDb, err := sql.Open("sqlite3", stateDb)
	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}

	// ping  db file
	if err := sqlDb.Ping(); err != nil {
		return nil, fmt.Errorf("pinging sqlDb: %w", err)
	}
	p.stateDb = sqlDb

	// create the state tracking table for snapshots
	_, err = p.stateDb.Exec(`
		CREATE TABLE IF NOT EXISTS capture_snapshot_state (
			slot_name   TEXT PRIMARY KEY,
			snapshot_id TEXT NOT NULL,
			lsn         TEXT NOT NULL,
			created_at  TEXT NOT NULL
		)
	`)

	// create the state tracking table for batches
	// status tracks the status of the table itself, pending, in progress, done
	// index_name contaisn the index we are using for this table
	// index_columns contain comma seperated column names we use as int his index
	// last_index_values stores comma seperated values in the form of a json blob {columnName1: last_value, columnName2: last_value}
	// rows_done stores how many rows are finished for the table and total_rows contains total rows to do
	// updated_at tracks last update on this table row
	_, err = p.stateDb.Exec(`
		CREATE TABLE IF NOT EXISTS capture_batch_state (
		    table_name TEXT PRIMARY KEY,
			status     TEXT,
			index_columns TEXT,
			last_index_values    TEXT,
			index_name TEXT,
			rows_done INTEGER,
			total_rows INTEGER,
			updated_at TIMESTAMP
		)
	`)

	return p, nil
}

func (p *PostgresDataMigrator) CreateSnapshot() error {
	ctx := context.Background()

	// CREATE_REPLICATION_SLOT isnt sql, it needs a connection opened in replication mode, lib/pq cant do this
	cfg, err := pgconn.ParseConfig(p.connStr)
	if err != nil {
		return fmt.Errorf("parsing connection string: %w", err)
	}
	cfg.RuntimeParams["replication"] = "database"

	replConn, err := pgconn.ConnectConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("opening replication connection: %w", err)
	}

	p.slotName = fmt.Sprintf("worm_slot_%d", time.Now().Unix())

	result, err := pglogrepl.CreateReplicationSlot(ctx, replConn, p.slotName, "pgoutput", pglogrepl.CreateReplicationSlotOptions{
		Temporary:      false,
		SnapshotAction: "EXPORT_SNAPSHOT",
		Mode:           pglogrepl.ReplicationMode(pglogrepl.LogicalReplication),
	})
	if err != nil {
		replConn.Close(ctx)
		return fmt.Errorf("creating replication slot: %w", err)
	}

	lsn, err := pglogrepl.ParseLSN(result.ConsistentPoint)
	if err != nil {
		replConn.Close(ctx)
		return fmt.Errorf("parsing consistent point %q: %w", result.ConsistentPoint, err)
	}

	// dont close replConn: the exported snapshot below is only valid while this connection stays open and idle
	p.replConn = replConn
	p.lsn = lsn
	p.snapshotId = result.SnapshotName

	if err := p.persistSnapshot(); err != nil {
		return fmt.Errorf("persisting snapshot state: %w", err)
	}

	// pin one dedicated, read-only tx to this
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return fmt.Errorf("beginning snapshot read tx: %w", err)
	}
	if _, err := tx.Exec(fmt.Sprintf("SET TRANSACTION SNAPSHOT '%s'", p.snapshotId)); err != nil {
		tx.Rollback()
		return fmt.Errorf("setting transaction snapshot: %w", err)
	}
	p.snapshotTx = tx

	err = p.seedBatchState()
	if err != nil {
		return err
	}

	return nil
}

func (p *PostgresDataMigrator) persistSnapshot() error {
	_, err := p.stateDb.Exec(
		`INSERT INTO capture_snapshot_state (slot_name, snapshot_id, lsn, created_at) VALUES (?, ?, ?, ?)`,
		p.slotName, p.snapshotId, p.lsn.String(), time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (p *PostgresDataMigrator) seedBatchState() error {
	if p.snapshotTx == nil {
		return fmt.Errorf("SeedBatchState called before CreateSnapshot")
	}

	for _, table := range p.tables {
		var totalRows int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s;", table)
		if err := p.snapshotTx.QueryRow(query).Scan(&totalRows); err != nil {
			return fmt.Errorf("counting rows in %s: %w", table, err)
		}

		index := p.tableIndex[table]
		var cols []string
		for _, col := range index.Columns {
			cols = append(cols, col.Name)
		}

		if err := p.persistBatchStatus(table, "pending", strings.Join(cols, ", "), 0, totalRows); err != nil {
			return fmt.Errorf("seeding batch state for %s: %w", table, err)
		}
	}
	return nil
}

func (p *PostgresDataMigrator) Backfill() error {
	// go table by table (sorted order)
	// start with your checkpoint (keyset pagination)
	// get all that paginated data in memory
	// decode it (normalise it)
	// hand it off to the writer, which encodes + writes + persists its own checkpoint on success
	// continue by moving to the next page using the cursor the writer just confirmed

	// TODO: see if we have a valid replication connection, if not, make one and re add all the voletile things
	// like tables, colMap, etc

	for _, table := range p.tables {
		var lastValsJSON string
		var indexColumnComma string
		var rowsDone int
		var totalRows int
		var status string

		q := `SELECT last_index_values, index_columns, rows_done, total_rows, status FROM capture_batch_state WHERE table_name = ?`
		if err := p.stateDb.QueryRow(q, table).Scan(&lastValsJSON, &indexColumnComma, &rowsDone, &totalRows, &status); err != nil {
			return fmt.Errorf("reading batch state for %s: %w", table, err)
		}
		if status == "done" {
			continue
		}
		indexColumns := strings.Split(indexColumnComma, ", ")

		var lastVals []any
		if lastValsJSON != "" {
			if err := json.Unmarshal([]byte(lastValsJSON), &lastVals); err != nil {
				return fmt.Errorf("decoding last index values for %s: %w", table, err)
			}
		}
		for rowsDone < totalRows {
			batch, err := p.resumeBackfill(table, rowsDone, indexColumnComma, lastVals)
			if err != nil {
				return err
			}
			// writer encodes + writes to target + persists rows_done and last_index_values on success
			lv, err := p.writer.Write(batch, p.colMap, indexColumns)
			if err != nil {
				return err
			}
			lastVals = lv
			rowsDone += len(batch.Rows)
		}

		if err := p.markTableDone(table); err != nil {
			return err
		}
	}
	return nil
}

func (p *PostgresDataMigrator) markTableDone(table string) error {
	_, err := p.stateDb.Exec(
		`UPDATE capture_batch_state SET status = 'done', updated_at = ? WHERE table_name = ?`,
		time.Now().UTC().Format(time.RFC3339), table,
	)
	return err
}

func (p *PostgresDataMigrator) decode(t schema.Type, raw any) (any, error) {
	switch t.(type) {
	case schema.BoolType:
		return raw, nil // postgres already gives u bool
	case schema.JSONType:
		if b, ok := raw.([]byte); ok {
			return string(b), nil
		}
		return raw, nil
	case schema.UUIDType, schema.TextType, schema.EnumType:
		if b, ok := raw.([]byte); ok {
			return string(b), nil
		}
		return raw, nil
	case schema.TimeType:
		return raw, nil // already time.Time
	default:
		return raw, nil
	}

}

func (p *PostgresDataMigrator) resumeBackfill(tableName string, rowsDone int, indexColumns string, lastVals []any) (*schema.Batch, error) {
	var rows *sql.Rows
	var err error

	if rowsDone == 0 {
		// this is the first page

		// lock state db to have status = in_progress
		if _, err := p.stateDb.Exec(
			`UPDATE capture_batch_state SET status = 'in_progress', updated_at = ? WHERE table_name = ?`,
			time.Now().UTC().Format(time.RFC3339), tableName,
		); err != nil {
			return nil, err
		}

		query := fmt.Sprintf("SELECT * FROM %s ORDER BY %s ASC LIMIT %d", tableName, indexColumns, p.limit)
		rows, err = p.snapshotTx.Query(query)
	} else {
		// otherwise, make a query with pagination with lastVals
		var placeholders string
		for i, _ := range lastVals {
			placeholders += fmt.Sprintf("$%d", i+1) // $1, $2, $3...
			if i != len(lastVals)-1 {
				placeholders += ", "
			}
		}
		query := fmt.Sprintf("SELECT * FROM %s WHERE (%s) > (%s) ORDER BY %s LIMIT %d",
			tableName, indexColumns, placeholders, indexColumns, p.limit)
		rows, err = p.snapshotTx.Query(query, lastVals...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	batch := &schema.Batch{
		Table:   tableName,
		Columns: cols,
	}

	for rows.Next() {
		rowValues := make([]any, len(cols))

		rowPointers := make([]any, len(cols))
		for i := range rowValues {
			rowPointers[i] = &rowValues[i]
		}

		if err := rows.Scan(rowPointers...); err != nil {
			return nil, err
		}

		for i, colName := range cols {
			colType := p.colMap[tableName+colName]
			decoded, err := p.decode(colType, rowValues[i])
			if err != nil {
				return nil, err
			}
			rowValues[i] = decoded
		}

		batch.Rows = append(batch.Rows, rowValues)
	}
	return batch, rows.Err()
}

func (p *PostgresDataMigrator) persistBatchStatus(t string, s string, indexColumns string, rowsDone int, totalRows int) error {
	indexName := p.tableIndex[t].Name
	_, err := p.stateDb.Exec(
		`INSERT INTO capture_batch_state (table_name, status, index_columns, last_index_values, index_name, rows_done, total_rows, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t, s, indexColumns, "", indexName, rowsDone, totalRows, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}
