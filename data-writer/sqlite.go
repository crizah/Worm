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

func NewSqliteDW(db *sql.DB, stateConn *sql.DB) (*SQLiteDataMigrator, error) {
	return &SQLiteDataMigrator{
		targetDb: db,
		stateDb:  stateConn,
	}, nil

}
func (sq *SQLiteDataMigrator) Write(ctx context.Context, b *schema.Batch, colMap map[string]schema.Type, t int) ([]any, error) {
	switch t {
	case 1:
		// insert
		lastVals, err := sq.writeInsert(ctx, b, colMap)
		if err != nil {
			return nil, err
		}
		return lastVals, nil

	case 2:
		// update
		err := sq.writeUpdate(ctx, b, colMap)
		if err != nil {
			return nil, err
		}
		return nil, nil

	case 3:
		// delete
		err := sq.writeDelete(ctx, b, colMap)
		if err != nil {
			return nil, err
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("unrecognsied number %d", t)

	}
}
func (sq *SQLiteDataMigrator) writeUpdate(ctx context.Context, b *schema.Batch, colMap map[string]schema.Type) error {
	// updates existing rows, doesnt write to state db
	if len(b.Rows) == 0 {
		return nil
	}

	tx, err := sq.targetDb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning target tx: %w", err)
	}
	defer tx.Rollback()

	// handles possibly change values of unique indexes as well (diff syntax, but the below one gets the gist)
	// UPDATE table_name
	//  SET column_1 = v.column_1,
	//  columne_2 = v.column_2
	//  FROM (VALUES
	//   (val1, 'val2', 'val3', val4),
	//   (val1, 'val2', 'val3', val4)
	//  ) AS v(index1, index2,column1, column2)
	//  WHERE table.index1 = index1
	//   AND table.index2= index2;

	oldAliasNames := make([]string, len(b.IndexColumns))
	for i, idx := range b.IndexColumns {
		oldAliasNames[i] = "old_" + idx
	}
	aliasCols := append(append([]string{}, oldAliasNames...), b.Columns...)
	rowPlaceholder := "(" + strings.TrimSuffix(strings.Repeat("?,", len(aliasCols)), ",") + ")"

	setClauses := make([]string, len(b.Columns))
	for i, col := range b.Columns {
		setClauses[i] = fmt.Sprintf("%s = a.%s", col, col)
	}

	whereClauses := make([]string, len(b.IndexColumns))
	for i, idx := range b.IndexColumns {
		whereClauses[i] = fmt.Sprintf("%s.%s = a.%s", b.Table, idx, oldAliasNames[i])
	}

	valuePlaceholders := make([]string, len(b.Rows))
	var args []any
	for i, row := range b.Rows {
		valuePlaceholders[i] = rowPlaceholder

		for j, idxVal := range b.PrevVals[i] {
			encoded, err := sq.encode(ctx, colMap[b.Table+b.IndexColumns[j]], idxVal)
			if err != nil {
				return fmt.Errorf("encoding %s.%s: %w", b.Table, b.IndexColumns[j], err)
			}
			args = append(args, encoded)
		}
		for j, val := range row {
			encoded, err := sq.encode(ctx, colMap[b.Table+b.Columns[j]], val)
			if err != nil {
				return fmt.Errorf("encoding %s.%s: %w", b.Table, b.Columns[j], err)
			}
			args = append(args, encoded)
		}
	}

	updateQuery := fmt.Sprintf(
		"UPDATE %s SET %s FROM (VALUES %s) AS a(%s) WHERE %s",
		b.Table,
		strings.Join(setClauses, ", "),
		strings.Join(valuePlaceholders, ", "),
		strings.Join(aliasCols, ", "),
		strings.Join(whereClauses, " AND "),
	)

	if _, err := tx.ExecContext(ctx, updateQuery, args...); err != nil {
		return fmt.Errorf("updating %s: %w", b.Table, err)
	}

	return tx.Commit()
}

func (sq *SQLiteDataMigrator) writeDelete(ctx context.Context, b *schema.Batch, colMap map[string]schema.Type) error {
	// deletes existing rows, doesnt write to state db - only PrevVals matters here,
	// there's no new row data on a delete
	if len(b.PrevVals) == 0 {
		return nil
	}

	tx, err := sq.targetDb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning target tx: %w", err)
	}
	defer tx.Rollback()

	// DELETE FROM cityd WHERE (locId, subId) IN (VALUES (?,?), (?,?), (?,?))
	tuplePlaceholder := "(" + strings.TrimSuffix(strings.Repeat("?,", len(b.IndexColumns)), ",") + ")"

	valuePlaceholders := make([]string, len(b.PrevVals))
	var args []any
	for i, prevVal := range b.PrevVals {
		valuePlaceholders[i] = tuplePlaceholder
		for j, idxVal := range prevVal {
			encoded, err := sq.encode(ctx, colMap[b.Table+b.IndexColumns[j]], idxVal)
			if err != nil {
				return fmt.Errorf("encoding %s.%s: %w", b.Table, b.IndexColumns[j], err)
			}
			args = append(args, encoded)
		}
	}

	deleteQuery := fmt.Sprintf(
		"DELETE FROM %s WHERE (%s) IN (VALUES %s)",
		b.Table,
		strings.Join(b.IndexColumns, ", "),
		strings.Join(valuePlaceholders, ", "),
	)

	if _, err := tx.ExecContext(ctx, deleteQuery, args...); err != nil {
		return fmt.Errorf("deleting from %s: %w", b.Table, err)
	}

	return tx.Commit()
}

func (sq *SQLiteDataMigrator) writeInsert(ctx context.Context, b *schema.Batch, colMap map[string]schema.Type) ([]any, error) {
	// inserts new rows AND writes to state db
	if len(b.Rows) == 0 {
		return nil, nil
	}

	// begin transaction
	tx, err := sq.targetDb.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning target tx: %w", err)
	}
	defer tx.Rollback()
	indexColumns := b.IndexColumns
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
