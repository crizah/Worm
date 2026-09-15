package datamigrator

import (
	"context"
	"database/sql"
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
	limit      int                      // the batch limit of how many rows to process at a time
	colMap     map[string]schema.Type   // stores the column type mapped to tableName+columnName
	writer     datawriter.DM            // the writer interface

	slotName string // current slot name
	lsn      pglogrepl.LSN
	replConn *pgconn.PgConn // kept open on purpose: closing it invalidates the exported snapshot
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

	// insert all initial table values
	for _, table := range p.tables {
		// add all the initial values to the state db
		// get number of rows in table
		var total_rows int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s;", table)
		err := p.db.QueryRow(query).Scan(&total_rows)
		if err != nil {
			return nil, err
		}

		index := p.tableIndex[table]

		var builder strings.Builder
		for i, col := range index.Columns {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(col.Name)
		}
		cols := builder.String()

		err = p.persistBatchStatus(table, "pending", cols, 0, total_rows)
	}

	if err != nil {
		return nil, err
	}
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

	return nil
}

func (p *PostgresDataMigrator) persistSnapshot() error {
	_, err := p.stateDb.Exec(
		`INSERT INTO capture_snapshot_state (slot_name, snapshot_id, lsn, created_at) VALUES (?, ?, ?, ?)`,
		p.slotName, p.snapshotId, p.lsn.String(), time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (p *PostgresDataMigrator) Backfill() error {
	// go table by table (sorted order)
	// start with your checkpoint (keyset pagination)
	// get all that paginated data in memory
	// decode it (normalise it)
	// hnad it off to the writer (go routine based on target db type)
	// the go routine decodes and writes, returns back conformation to this func (configure a tll otherwise error handeling is always explicit so this shouldnt be an issue)
	// write to local db after this
	// continue by moving checkpoint

	// TODO: see if we have a valid replication connection, if not, make one and re add all the voletile things
	// like tables, colMap, etc

	for _, table := range p.tables {
		var last_vals string
		var column_names string
		var rows_done int
		var total_rows int
		var status string

		q := fmt.Sprintf("SELECT last_index_values, index_columns, rows_done, total_rows, status from capture_batch_status where table_name= '%s'", table)
		err := p.db.QueryRow(q).Scan(&last_vals, &column_names, &rows_done, &total_rows, &status)
		if err != nil {
			return err
		}
		if status == "done" {
			continue
		}

		for rows_done < total_rows {
			rd, b, err := p.resumeBackfill(table, rows_done, column_names, last_vals)
			if err != nil {
				return err
			}

			// send it to writer, it mutates state db on success
			err = p.writer.Write(b, p.colMap)
			if err != nil {
				return err
			}

			rows_done = rd // continue the loop

		}
		// write to state db status done

	}
	return nil
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

func (p *PostgresDataMigrator) resumeBackfill(tableName string, rows_done int, column_names string, last_vals string) (int, *schema.Batch, error) {
	query := ""
	// TODO: lock state db to have status = in_progress
	if rows_done == 0 {
		// this is the first insertion
		// columnNames is alreday comma seperated, so this should be fine
		query = fmt.Sprintf("SELECT * from %s ORDER BY %s ASC limit %d", tableName, column_names, p.limit)

	} else {
		// we are resuming
		query = fmt.Sprintf("SELECT * from %s WHERE (%s) > (%s) ORDER BY %s LIMIT %d", column_names, last_vals, p.limit)
	}
	rows, err := p.db.Query(query)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return 0, nil, err
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
			return 0, nil, err
		}

		for i, colName := range cols {
			val := rowValues[i]
			colType := p.colMap[tableName+colName]

			row_val, err := p.decode(colType, val)
			if err != nil {
				return 0, nil, err
			}
			rowValues[i] = row_val
		}

		batch.Rows = append(batch.Rows, rowValues)
	}
	return len(batch.Rows), batch, nil
}

func (p *PostgresDataMigrator) persistBatchStatus(t string, s string, index_columns string, rows_done int, total_rows int) error {
	_, err := p.stateDb.Exec(
		`INSERT INTO capture_batch_state (table_name, status, index_columns, last_index_values
		, index_name, rows_done, total_rows, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t, s, index_columns, rows_done, total_rows, time.Now().UTC().Format(time.RFC3339),
	)
	return err

}
