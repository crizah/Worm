package querier

import "context"

// interface for querier that queries the live dbs

type Querier interface {
	Tables(ctx context.Context) (*[]string, error)
	Columns(ctx context.Context) (*[]columnRow, error)
	Constraints(ctx context.Context) (*[]constraintRow, error)
	ForeignKeys(ctx context.Context) (*[]fkRow, error)
	Enums(ctx context.Context) (map[string][]string, error) // maps enum name to values
}

type constraintRow struct {
	TableName      string `db:"table_name"`
	ConstraintName string `db:"constraint_name"`
	ConstraintType string `db:"constraint_type"`
}

type enumRow struct {
	EnumName string `db:"enum_name"`
	Value    string `db:"value"`
	SortPos  int    `db:"enumsortorder"`
}

type fkRow struct {
	TableName      string `db:"table_name"`
	ColumnName     string `db:"column_name"`
	ConstraintName string `db:"constraint_name"`
	RefTableName   string `db:"ref_table_name"`
	RefColumnName  string `db:"ref_column_name"`
	UpdateRule     string `db:"update_rule"`
	DeleteRule     string `db:"delete_rule"`
}

type columnRow struct {
	TableName    string  `db:"table_name"`
	Name         string  `db:"column_name"`
	Ordinal      int     `db:"ordinal_position"`
	DataType     string  `db:"data_type"`
	UdtName      string  `db:"udt_name"`
	IsNullable   string  `db:"is_nullable"`
	DefaultExpr  *string `db:"column_default"`
	CharMaxLen   *int    `db:"character_maximum_length"`
	NumericPrec  *int    `db:"numeric_precision"`
	NumericScale *int    `db:"numeric_scale"`
}
