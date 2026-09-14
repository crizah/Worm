package capture

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresCapture struct {
	db         *sql.DB  // our postgres db connection
	connStr    string   // our postgres conn string, needed to create replication connection
	snapshotId string   // returned snapshot id
	stateDb    *sql.DB  // state db to track snapshot
	tables     []string // table names sorted that we get from emitter

	slotName string // current slot name
	lsn      pglogrepl.LSN
	replConn *pgconn.PgConn // kept open on purpose: closing it invalidates the exported snapshot
}

func NewPostgresCapture(conn string, stateDb string, t []string) (*PostgresCapture, error) {
	// setup and ping postgres db
	db, err := sql.Open("postgres", conn)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}
	// ping
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging postgres db: %w", err)
	}

	p := &PostgresCapture{db: db, connStr: conn, tables: t}

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

	// create the state tracking table
	_, err = p.stateDb.Exec(`
		CREATE TABLE IF NOT EXISTS capture_state (
			slot_name   TEXT PRIMARY KEY,
			snapshot_id TEXT NOT NULL,
			lsn         TEXT NOT NULL,
			created_at  TEXT NOT NULL
		)
	`)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (p *PostgresCapture) CreateSnapshot() error {
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

func (p *PostgresCapture) persistSnapshot() error {
	_, err := p.stateDb.Exec(
		`INSERT INTO capture_state (slot_name, snapshot_id, lsn, created_at) VALUES (?, ?, ?, ?)`,
		p.slotName, p.snapshotId, p.lsn.String(), time.Now().UTC().Format(time.RFC3339),
	)
	return err
}
