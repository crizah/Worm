package inspect

import (
	"context"

	"github.com/crizah/Worm/querier"
	"github.com/crizah/Worm/schema"
)

type Inspector interface {
	buildTable(tableName string, columns []schema.Column, constraints []schema.Constraints, fks []schema.ForeignKey, enums []schema.EnumType) (schema.Table, error)
	buildColumn(columns []querier.ColumnRow) ([]schema.Column, error)
	buildFK(fks []querier.FkRow)
	Inspect(ctx context.Context, q querier.Querier) (schema.Schema, error)
}
