package querier

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type PostgresQuerier struct {
	db *sqlx.DB
}

func NewPQuerier(conn string) (*PostgresQuerier, error) {
	// ping at startup
	db, err := sqlx.Connect("postgres", conn)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}

	p := &PostgresQuerier{db: db}
	return p, nil

}

func executeSelect[T any](db *sqlx.DB, query string, schemaName string) ([]T, error) {
	// generics cant me methods unless struct itself is generic
	var ans []T
	if err := db.Select(&ans, query, schemaName); err != nil {
		return nil, err
	}
	return ans, nil
}
func (p *PostgresQuerier) Columns(ctx context.Context) ([]ColumnRow, error) {
	exec := `
	SELECT table_name, column_name, ordinal_position, data_type, udt_name,
         is_nullable, column_default,
         character_maximum_length, numeric_precision, numeric_scale
  FROM information_schema.columns
  WHERE table_schema = $1
  ORDER BY table_name, ordinal_position;
	`
	// udt_name is enum name when data_type is USER DEFINED
	ans, err := executeSelect[ColumnRow](p.db, exec, "public") // hardcoded for now
	if err != nil {
		return nil, err
	}

	return ans, nil

}

func (p *PostgresQuerier) Tables(ctx context.Context) ([]string, error) {
	exec := `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = $1
		  AND table_type = 'BASE TABLE'
		ORDER BY table_name;
		`

	ans, err := executeSelect[string](p.db, exec, "public") // hardcoded for now
	if err != nil {
		return nil, err
	}

	return ans, nil

}

func (p *PostgresQuerier) Constraints(cts context.Context) ([]ConstraintRow, error) {
	exec := `
 SELECT tc.table_name, tc.constraint_name, tc.constraint_type,
         kcu.column_name, kcu.ordinal_position
  FROM information_schema.table_constraints tc
  JOIN information_schema.key_column_usage kcu
    ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
  WHERE tc.table_schema = $1
    AND tc.constraint_type IN ('PRIMARY KEY', 'UNIQUE')
  ORDER BY tc.table_name, tc.constraint_name, kcu.ordinal_position;

	`

	ans, err := executeSelect[ConstraintRow](p.db, exec, "public") // hardcoded for now
	if err != nil {
		return nil, err
	}

	return ans, nil

}

func (p *PostgresQuerier) ForeignKeys(ctx context.Context) ([]FkRow, error) {
	exec := `
 SELECT tc.table_name, kcu.column_name, tc.constraint_name,
         ccu.table_name AS ref_table_name, ccu.column_name AS ref_column_name,
         rc.update_rule, rc.delete_rule
  FROM information_schema.table_constraints tc
  JOIN information_schema.key_column_usage kcu
    ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
  JOIN information_schema.referential_constraints rc
    ON tc.constraint_name = rc.constraint_name AND tc.table_schema = rc.constraint_schema
  JOIN information_schema.constraint_column_usage ccu
    ON rc.unique_constraint_name = ccu.constraint_name AND rc.unique_constraint_schema = ccu.constraint_schema
  WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_schema = $1
  ORDER BY tc.table_name, kcu.ordinal_position;
`
	ans, err := executeSelect[FkRow](p.db, exec, "public") // hardcoded for now
	if err != nil {
		return nil, err
	}

	return ans, nil
}

func (p *PostgresQuerier) Enums(ctx context.Context) (map[string][]string, error) {
	exec := `SELECT t.typname AS enum_name, e.enumlabel AS value, e.enumsortorder
 FROM pg_type t
 JOIN pg_enum e ON t.oid = e.enumtypid
 JOIN pg_namespace n ON n.oid = t.typnamespace
 WHERE n.nspname = $1
 ORDER BY t.typname, e.enumsortorder;
`
	rows, err := executeSelect[EnumRow](p.db, exec, "public") // hardcoded for now
	if err != nil {
		return nil, err
	}

	enums := make(map[string][]string)
	for _, r := range rows {
		enums[r.EnumName] = append(enums[r.EnumName], r.Value)
	}
	return enums, nil
}
