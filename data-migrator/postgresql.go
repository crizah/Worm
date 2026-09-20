package datamigrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	datawriter "github.com/crizah/Worm/data-writer"
	"github.com/crizah/Worm/schema"
	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
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
	writer     datawriter.DW            // the writer interface, dont store this in here lol

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

func NewPostgresDM(db *sql.DB, conn string, stateDb *sql.DB, sc []*schema.Table, limit int, w datawriter.DW) (*PostgresDataMigrator, error) {
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

	p := &PostgresDataMigrator{db: db, connStr: conn, tables: t, tableIndex: indexTable, limit: limit, colMap: cMap, writer: w, stateDb: stateDb}

	return p, nil
}

func (p *PostgresDataMigrator) CreateSnapshot(ctx context.Context) error {

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

	if err := p.persistSnapshot(ctx); err != nil {
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

	err = p.seedBatchState(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (p *PostgresDataMigrator) persistSnapshot(ctx context.Context) error {
	_, err := p.stateDb.ExecContext(ctx,
		`INSERT INTO capture_snapshot_state (slot_name, snapshot_id, lsn, created_at) VALUES (?, ?, ?, ?)`,
		p.slotName, p.snapshotId, p.lsn.String(), time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (p *PostgresDataMigrator) seedBatchState(ctx context.Context) error {
	if p.snapshotTx == nil {
		return fmt.Errorf("SeedBatchState called before CreateSnapshot")
	}

	for _, table := range p.tables {
		var totalRows int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s;", table)
		if err := p.snapshotTx.QueryRowContext(ctx, query).Scan(&totalRows); err != nil {
			return fmt.Errorf("counting rows in %s: %w", table, err)
		}

		index := p.tableIndex[table]
		var cols []string
		for _, col := range index.Columns {
			cols = append(cols, col.Name)
		}

		if err := p.persistBatchStatus(ctx, table, "pending", strings.Join(cols, ", "), 0, totalRows); err != nil {
			return fmt.Errorf("seeding batch state for %s: %w", table, err)
		}
	}
	return nil
}

func (p *PostgresDataMigrator) Backfill(ctx context.Context) error {
	// go table by table (sorted order)
	// start with your checkpoint (keyset pagination)
	// get all that paginated data in memory
	// decode it (normalise it)
	// hand it off to the writer, which encodes + writes + persists its own checkpoint on success
	// continue by moving to the next page using the cursor the writer just confirmed

	for _, table := range p.tables {
		var lastValsJSON string
		var indexColumnComma string
		var rowsDone int
		var totalRows int
		var status string

		q := `SELECT last_index_values, index_columns, rows_done, total_rows, status FROM capture_batch_state WHERE table_name = ?`
		if err := p.stateDb.QueryRowContext(ctx, q, table).Scan(&lastValsJSON, &indexColumnComma, &rowsDone, &totalRows, &status); err != nil {
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
			batch, err := p.resumeBackfill(ctx, table, rowsDone, indexColumnComma, lastVals)
			if err != nil {
				return err
			}
			batch.IndexColumns = indexColumns
			// writer encodes + writes to target + persists rows_done and last_index_values on success
			lv, err := p.writer.Write(ctx, batch, p.colMap, 1)
			if err != nil {
				return err
			}
			lastVals = lv
			rowsDone += len(batch.Rows)
		}

		if err := p.markTableDone(ctx, table); err != nil {
			return err
		}
	}
	return nil
}

func (p *PostgresDataMigrator) StreamData(ctx context.Context) error {
	// takes a replConn (doesnt have to be the same one as the backfill)
	// read the last recorded lns from state db
	// do a START_REPLICATION SLOT <slotName> LOGICAL <lsn>
	var slotName string
	var lsn string
	q := `SELECT slot_name, lsn FROM capture_snapshot_stage`
	if err := p.stateDb.QueryRowContext(ctx, q).Scan(&slotName, &lsn); err != nil {
		return fmt.Errorf("reading lsn state : %s", err.Error())
	}

	// resue the same replConn
	if p.replConn.IsClosed() {
		// TODO: make a new one
	}

	startLSN, err := pglogrepl.ParseLSN(lsn)
	if err != nil {
		return fmt.Errorf("parsing lsn %q: %w", lsn, err)
	}

	err = pglogrepl.StartReplication(ctx, p.replConn, slotName, startLSN, pglogrepl.StartReplicationOptions{
		Mode: pglogrepl.LogicalReplication,
		PluginArgs: []string{
			"proto_version '1'",
			"publication_names 'worm_pub'", // TODO: this publication doesn't get created anywhere yet - pgoutput will fail without it existing first
		},
	})
	if err != nil {
		return fmt.Errorf("starting replication: %w", err)
	}

	relations := map[uint32]*pglogrepl.RelationMessage{}
	receivedLSN := startLSN
	nextStandbyUpdate := time.Now().Add(10 * time.Second)

	for {
		if time.Now().After(nextStandbyUpdate) {
			if err := pglogrepl.SendStandbyStatusUpdate(ctx, p.replConn, pglogrepl.StandbyStatusUpdate{WALWritePosition: receivedLSN}); err != nil {
				return fmt.Errorf("sending standby status update: %w", err)
			}
			nextStandbyUpdate = time.Now().Add(10 * time.Second)
		}

		recvCtx, cancel := context.WithDeadline(ctx, nextStandbyUpdate)
		rawMsg, err := p.replConn.ReceiveMessage(recvCtx)
		cancel()
		if err != nil {
			if pgconn.Timeout(err) {
				continue // just means its time to loop around and send a standby status update
			}
			return fmt.Errorf("receiving replication message: %w", err)
		}

		cd, ok := rawMsg.(*pgproto3.CopyData)
		if !ok {
			continue // notices/other protocol chatter we dont care about
		}
		if len(cd.Data) == 0 {
			continue
		}

		switch cd.Data[0] {
		case 'k': // primary keepalive
			pkm, err := pglogrepl.ParsePrimaryKeepaliveMessage(cd.Data[1:])
			if err != nil {
				return fmt.Errorf("parsing keepalive: %w", err)
			}
			if pkm.ReplyRequested {
				nextStandbyUpdate = time.Time{} // force a status update on the next loop iteration
			}

		case 'w': // XLogData - an actual change
			xld, err := pglogrepl.ParseXLogData(cd.Data[1:])
			if err != nil {
				return fmt.Errorf("parsing xlog data: %w", err)
			}
			receivedLSN = xld.WALStart

			msg, err := pglogrepl.Parse(xld.WALData)
			if err != nil {
				return fmt.Errorf("parsing pgoutput message: %w", err)
			}

			switch m := msg.(type) {
			case *pglogrepl.RelationMessage:
				// pgoutput sends one of these before the first change on a relation,
				// and again whenever that relation's shape changes - cache it, every
				// insert/update/delete after this only carries the relation's ID.
				relations[m.RelationID] = m

			case *pglogrepl.InsertMessage:
				rel, ok := relations[m.RelationID]
				if !ok {
					return fmt.Errorf("insert for unknown relation id %d - missing Relation message", m.RelationID)
				}
				batch, err := p.decodeTuple(rel, m.Tuple)
				if err != nil {
					return fmt.Errorf("decoding insert on %s: %w", rel.RelationName, err)
				}
				batch.IndexColumns = indexColumnsFor(p.tableIndex[rel.RelationName])
				// a genuine new row inserted twice (e.g. after a resume) has the same PK
				// both times, so INSERT OR IGNORE's idempotency is exactly what we want here too
				if _, err := p.writer.Write(ctx, batch, p.colMap, 1); err != nil {
					return fmt.Errorf("writing streamed insert for %s: %w", rel.RelationName, err)
				}

			case *pglogrepl.UpdateMessage, *pglogrepl.DeleteMessage:
				// TODO: get the actual prev values and do an update or delete
			case *pglogrepl.CommitMessage:
				if err := p.persistStreamLSN(m.CommitLSN); err != nil {
					return fmt.Errorf("persisting stream lsn: %w", err)
				}
			}
		}
	}
}

// decodeTuple turns one pgoutput tuple into a single-row Batch, decoding pgoutput's
// text-format column values per the column's normalized schema.Type. This is a
// different decode path from p.decode() - that one handles values already typed by
// lib/pq's driver (bool, time.Time, ...), these arrive as raw text bytes instead.
func (p *PostgresDataMigrator) decodeTuple(rel *pglogrepl.RelationMessage, tuple *pglogrepl.TupleData) (*schema.Batch, error) {
	cols := make([]string, len(rel.Columns))
	row := make([]any, len(rel.Columns))

	for i, col := range tuple.Columns {
		colName := rel.Columns[i].Name
		cols[i] = colName

		switch col.DataType {
		case 'n': // null
			row[i] = nil
		case 'u': // unchanged toast value - not sent, nothing we can do but leave it nil
			row[i] = nil
		case 't': // text-formatted value
			decoded, err := p.decodeText(p.colMap[rel.RelationName+colName], col.Data)
			if err != nil {
				return nil, err
			}
			row[i] = decoded
		}
	}

	return &schema.Batch{
		Table:   rel.RelationName,
		Columns: cols,
		Rows:    [][]any{row},
	}, nil
}

func (p *PostgresDataMigrator) decodeText(t schema.Type, raw []byte) (any, error) {
	s := string(raw)
	switch t.(type) {
	case schema.BoolType:
		return s == "t", nil
	case schema.IntegerType:
		return strconv.ParseInt(s, 10, 64)
	case schema.TimeType:
		return time.Parse("2006-01-02 15:04:05.999999-07", s)
	default: // uuid/text/enum/json are already fine as strings
		return s, nil
	}
}

func indexColumnsFor(idx *schema.Index) []string {
	names := make([]string, len(idx.Columns))
	for i, c := range idx.Columns {
		names[i] = c.Name
	}
	return names
}

func (p *PostgresDataMigrator) persistStreamLSN(lsn pglogrepl.LSN) error {
	_, err := p.stateDb.Exec(`UPDATE capture_snapshot_stage SET lsn = ? WHERE slot_name = ?`, lsn.String(), p.slotName)
	return err
}
func (p *PostgresDataMigrator) markTableDone(ctx context.Context, table string) error {
	_, err := p.stateDb.ExecContext(ctx,
		`UPDATE capture_batch_state SET status = 'done', updated_at = ? WHERE table_name = ?`,
		time.Now().UTC().Format(time.RFC3339), table,
	)
	return err
}

func (p *PostgresDataMigrator) decode(ctx context.Context, t schema.Type, raw any) (any, error) {
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

func (p *PostgresDataMigrator) resumeBackfill(ctx context.Context, tableName string, rowsDone int, indexColumns string, lastVals []any) (*schema.Batch, error) {
	var rows *sql.Rows
	var err error

	if rowsDone == 0 {
		// this is the first page

		// lock state db to have status = in_progress
		if _, err := p.stateDb.ExecContext(ctx,
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
			decoded, err := p.decode(ctx, colType, rowValues[i])
			if err != nil {
				return nil, err
			}
			rowValues[i] = decoded
		}

		batch.Rows = append(batch.Rows, rowValues)
	}
	return batch, rows.Err()
}

func (p *PostgresDataMigrator) persistBatchStatus(ctx context.Context, t string, s string, indexColumns string, rowsDone int, totalRows int) error {
	indexName := p.tableIndex[t].Name
	_, err := p.stateDb.ExecContext(ctx,
		`INSERT INTO capture_batch_state (table_name, status, index_columns, last_index_values, index_name, rows_done, total_rows, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t, s, indexColumns, "", indexName, rowsDone, totalRows, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}
