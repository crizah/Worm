package main

// compares row counts between SOURCE_CONN_STR and TARGET_CONN_STR, table by
// table. doesn't hardcode table names - discovers them per side and diffs
// the two lists as well as the counts, so a missing table shows up too.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"

	"github.com/crizah/Worm/schema"
	"github.com/crizah/Worm/utils"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	godotenv.Load()

	sourceDialect, err := schema.ResolveEnum(os.Getenv("SOURCE_DB"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "source dialect: %s\n", err)
		os.Exit(1)
	}
	targetDialect, err := schema.ResolveEnum(os.Getenv("TARGET_DB"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "target dialect: %s\n", err)
		os.Exit(1)
	}

	sourceDb, err := utils.PingDB(sourceDialect, os.Getenv("SOURCE_CONN_STR"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "source db: %s\n", err)
		os.Exit(1)
	}
	targetDb, err := utils.PingDB(targetDialect, os.Getenv("TARGET_CONN_STR"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "target db: %s\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	sourceCounts, err := tableCounts(ctx, sourceDb, sourceDialect)
	if err != nil {
		fmt.Fprintf(os.Stderr, "counting source tables: %s\n", err)
		os.Exit(1)
	}
	targetCounts, err := tableCounts(ctx, targetDb, targetDialect)
	if err != nil {
		fmt.Fprintf(os.Stderr, "counting target tables: %s\n", err)
		os.Exit(1)
	}

	names := make(map[string]struct{})
	for t := range sourceCounts {
		names[t] = struct{}{}
	}
	for t := range targetCounts {
		names[t] = struct{}{}
	}
	var sorted []string
	for t := range names {
		sorted = append(sorted, t)
	}
	sort.Strings(sorted)

	mismatch := false
	fmt.Printf("%-25s %12s %12s  %s\n", "table", "source", "target", "status")
	for _, t := range sorted {
		sc, sok := sourceCounts[t]
		tc, tok := targetCounts[t]

		status := "ok"
		switch {
		case !sok:
			status = "missing in source"
		case !tok:
			status = "missing in target"
		case sc != tc:
			status = "MISMATCH"
		}
		if status != "ok" {
			mismatch = true
		}
		fmt.Printf("%-25s %12d %12d  %s\n", t, sc, tc, status)
	}

	if mismatch {
		os.Exit(1)
	}
	fmt.Println("all counts match")
}

func tableCounts(ctx context.Context, db *sql.DB, d schema.Dialect) (map[string]int, error) {
	tables, err := listTables(ctx, db, d)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(tables))
	for _, t := range tables {
		var c int
		if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", t)).Scan(&c); err != nil {
			return nil, fmt.Errorf("counting %s: %w", t, err)
		}
		counts[t] = c
	}
	return counts, nil
}

func listTables(ctx context.Context, db *sql.DB, d schema.Dialect) ([]string, error) {
	var query string
	switch d {
	case schema.PostgresDialect:
		query = "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'"
	case schema.SqliteDialect:
		query = "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'"
	default:
		return nil, fmt.Errorf("unknown dialect")
	}

	rows, err := db.QueryContext(ctx, query)
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
