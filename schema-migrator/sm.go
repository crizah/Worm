package schemamigrator

// no interface, just migrates the migration file to the connection string or the .db file

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/crizah/Worm/schema"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

type SchemaMigrator struct {
	migrationFile string
	dialect       schema.Dialect
	db            *sql.DB
	mu            sync.Mutex // one migration at a time per migrator
}

func NewSchemaMigrator(db *sql.DB, d schema.Dialect, m string) (*SchemaMigrator, error) {

	return &SchemaMigrator{
		migrationFile: m,
		dialect:       d,
		db:            db,
	}, nil
}

func (sm *SchemaMigrator) MigrateSchema(ctx context.Context) error {
	// acquire lock
	sm.mu.Lock()
	defer sm.mu.Unlock()

	data, err := os.ReadFile(sm.migrationFile)
	if err != nil {
		return fmt.Errorf("reading migration file: %w", err)
	}

	// seperate the migration file via ; (order preservation is crucial)
	stmts := splitStatements(string(data))

	// start transaction
	tx, err := sm.db.Begin()
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() // no-op once committed

	// wipe the entire db state of the connection string
	if err := sm.wipe(tx); err != nil {
		return fmt.Errorf("wiping db state: %w", err)
	}

	// run each statement
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("running statement %q: %w", stmt, err)
		}
	}

	// once everything ran, commit and end transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
	// release lock happens via defer sm.mu.Unlock() above
}

func splitStatements(migration string) []string {
	rawStmts := strings.Split(migration, ";")

	var stmts []string
	for _, s := range rawStmts {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		stmts = append(stmts, s)
	}
	return stmts
}

func (sm *SchemaMigrator) wipe(tx *sql.Tx) error {
	tables, err := sm.listTables(tx)
	if err != nil {
		return fmt.Errorf("listing tables: %w", err)
	}

	for _, t := range tables {
		drop := fmt.Sprintf("DROP TABLE IF EXISTS %s", t)
		if sm.dialect == schema.PostgresDialect {
			// postgres enforces fks, cascade so drop order doesnt matter
			drop += " CASCADE"
		}
		if _, err := tx.Exec(drop); err != nil {
			return fmt.Errorf("dropping table %s: %w", t, err)
		}
	}
	return nil
}

func (sm *SchemaMigrator) listTables(tx *sql.Tx) ([]string, error) {
	var query string
	switch sm.dialect {
	case schema.SqliteDialect:
		query = "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'"
	case schema.PostgresDialect:
		query = "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'"
	default:
		return nil, fmt.Errorf("unknown dialect")
	}

	rows, err := tx.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}
