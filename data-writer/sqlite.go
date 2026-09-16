package datawriter

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/crizah/Worm/schema"
)

type SQLiteDataMigrator struct {
	targetDb *sql.DB
	stateDb  *sql.DB // same state db the migrator writes snapshot/batch state to
}

func NewSqliteDW(targetConn string, stateConn *sql.DB) (*SQLiteDataMigrator, error) {
	db, err := sql.Open("sqlite3", targetConn)
	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}

	// ping the connection string, or the db file
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging db: %w", err)
	}
	return &SQLiteDataMigrator{
		targetDb: db,
		stateDb:  stateConn,
	}, nil

}
func (sq *SQLiteDataMigrator) Write(ctx context.Context, b *schema.Batch, colMap map[string]schema.Type, indexColumns []string) ([]any, error) {
	if len(b.Rows) == 0 {
		return nil, nil
	}

	// begin transaction
	tx, err := sq.targetDb.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning target tx: %w", err)
	}
	defer tx.Rollback()

	rowPlaceholder := "(" + strings.TrimSuffix(strings.Repeat("?,", len(b.Columns)), ",") + ")" // adds placeholders based on len of columns
	// insert or ignore because of resume design
	insertPrefix := fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) VALUES ", b.Table, strings.Join(b.Columns, ", "))
	var args []any
	var placeHolders string

	for i, row := range b.Rows {
		placeHolders += rowPlaceholder
		if i != len(b.Rows)-1 {
			placeHolders += ", "
		}

		for j, val := range row {
			// encode back the data to sqlite format
			encoded, err := sq.encode(ctx, colMap[b.Table+b.Columns[j]], val)
			if err != nil {
				return nil, fmt.Errorf("encoding %s.%s: %w", b.Table, b.Columns[j], err)
			}
			args = append(args, encoded)
		}
	}

	// batch insert
	stmt := insertPrefix + placeHolders
	if _, err := tx.Exec(stmt, args...); err != nil {
		return nil, fmt.Errorf("inserting into %s: %w", b.Table, err)
	}

	// get last vals
	lastRow := b.Rows[len(b.Rows)-1]

	// only get the rows corresponding to our index columns
	var indexes []int
	for _, col := range indexColumns {
		for i, c := range b.Columns {
			if col == c {
				indexes = append(indexes, i)
			}
		}
	}

	// get the lastVals based on these indexes
	var lastVals []any
	for _, i := range indexes {
		lastVals = append(lastVals, lastRow[i])
	}
	lastValsJSON, err := json.Marshal(lastVals)
	if err != nil {
		return nil, fmt.Errorf("encoding checkpoint for %s: %w", b.Table, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing target tx: %w", err)
	}

	// checkpoint is only written after the target commit succeeds
	_, err = sq.stateDb.Exec(
		`UPDATE capture_batch_state
		 SET rows_done = rows_done + ?, last_index_values = ?, status = 'in_progress', updated_at = ?
		 WHERE table_name = ?`,
		len(b.Rows), string(lastValsJSON), time.Now().UTC().Format(time.RFC3339), b.Table,
	)
	if err != nil {
		return nil, fmt.Errorf("persisting checkpoint for %s: %w", b.Table, err)
	}

	return lastVals, nil
}

func (sq *SQLiteDataMigrator) encode(ctx context.Context, t schema.Type, v any) (any, error) {
	switch t.(type) {
	case schema.BoolType:
		if b, _ := v.(bool); b {
			return int64(1), nil
		}
		return int64(0), nil
	case schema.TimeType:
		if tm, ok := v.(time.Time); ok {
			return tm.Format(time.RFC3339), nil
		}
		return v, nil
	default:
		return v, nil // json/uuid/text/enum are already canonical strings
	}
}
