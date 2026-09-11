package schemamigrator

// no interface, just migrates the migration file to the connection string or the .db file

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

type dialect int // creating enums like this

const (
	sqliteDialect   dialect = iota // gets 0
	postgresDialect                // will get 1
)

type SchemaMigrator struct {
	connStr       string
	migrationFile string
	dialect       dialect
	db            *sql.DB
	mu            sync.Mutex // one migration at a time per migrator
}

func NewSchemaMigrator(c string, m string) (*SchemaMigrator, error) {
	d := resolveDialect(c)

	if d == sqliteDialect {
		// if db file doesnt exist, make one with that fileName
		if _, err := os.Stat(c); os.IsNotExist(err) {
			f, err := os.Create(c)
			if err != nil {
				return nil, fmt.Errorf("creating sqlite db file: %w", err)
			}
			f.Close()
		}
	}

	db, err := sql.Open(driverName(d), c)
	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}

	// ping the connection string, or the db file
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging db: %w", err)
	}

	return &SchemaMigrator{
		connStr:       c,
		migrationFile: m,
		dialect:       d,
		db:            db,
	}, nil
}

// postgres conn strings are a url or a
// key=value dsn, anything else we treat as a sqlite file path
func resolveDialect(connStr string) dialect {
	if strings.HasPrefix(connStr, "postgres://") || strings.HasPrefix(connStr, "postgresql://") || strings.Contains(connStr, "host=") {
		return postgresDialect
	}
	return sqliteDialect
}

func driverName(d dialect) string {
	if d == postgresDialect {
		return "postgres"
	}
	return "sqlite3"
}

func (sm *SchemaMigrator) MigrateSchema() error {
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
		if sm.dialect == postgresDialect {
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
	case sqliteDialect:
		query = "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'"
	case postgresDialect:
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
