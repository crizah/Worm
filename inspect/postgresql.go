package inspect

import (
	"context"
	"fmt"
	"strings"

	"github.com/crizah/Worm/querier"
	"github.com/crizah/Worm/schema"
)

func Inspect(ctx context.Context, q querier.Querier) (*schema.Schema, error) {
	tables, err := q.Tables(ctx)
	if err != nil {
		return nil, err
	}
	cols, err := q.Columns(ctx)
	if err != nil {
		return nil, err
	}
	cons, err := q.Constraints(ctx)
	// both cons and fks have foreign keys, what is that about
	if err != nil {
		return nil, err
	}
	fks, err := q.ForeignKeys(ctx)
	if err != nil {
		return nil, err
	}
	enums, err := q.Enums(ctx)
	if err != nil {
		return nil, err
	}

	groupedCols := groupBy(cols, func(c querier.ColumnRow) string {
		return c.TableName
	})

	groupedCons := groupBy(cons, func(c querier.ConstraintRow) string {
		return c.TableName
	})

	s := &schema.Schema{}
	for _, t := range tables {
		columns, err := buildColumns(t, groupedCols[t], enums, groupedCons[t])

		// s.Tables = append(s.Tables, table)
	}
	return s, nil
}

func groupBy[T any](items []T, getKey func(T) string) map[string][]T {
	// cant do a T.TableName on a generic type, so we pass a function that returns the table name instead
	// groups by tableName
	grouped := make(map[string][]T)

	for _, item := range items {
		key := getKey(item)
		grouped[key] = append(grouped[key], item)
	}

	return grouped

}

func buildColumns(tableName string, cols []querier.ColumnRow, enums map[string][]string, con []querier.ConstraintRow) ([]schema.Column, error) {
	pk := make(map[string]int) // 1 means PK, 2 means FK, 0 means idk
	for _, c := range con {
		if c.ConstraintType == "PRIMARY KEY" {
			pk[c.ColumnName] = 1
		}
	}
	var ans []schema.Column
	for _, c := range cols {
		col := schema.Column{Name: c.Name}
		col.IsNullable = c.IsNullable != "NO"
		col.IsPK = pk[c.Name] == 1

		t, ok := postmap[c.DataType]
		if !ok {
			return nil, fmt.Errorf("invalid type %s", c.DataType)
		}
		if e, isEnum := t.(schema.EnumType); isEnum {
			e.Name = c.UdtName
			e.Values = enums[e.Name]
			t = e // must reassign
		}
		col.Type = t

		col.Default = buildDefaultExp(t, c.DefaultExpr)

		ans = append(ans, col)
	}

	return ans, nil
}

func buildDefaultExp(dataType schema.Type, def *string) schema.Expr {
	if def == nil {
		// simce this is nil, should we add a nil expr type in expr interface?
		return nil
	}

	val := *def
	// get enums out of the way
	if _, isEnum := dataType.(schema.EnumType); isEnum {
		// this is in the format 'VALUE'::<enum_name>, so strip the enum name as ExpType will already have that
		parts := strings.Split(val, "::")
		cleanVal := strings.Trim(parts[0], "'") // get rid of the quotes
		return &schema.RawExpr{
			Val:     cleanVal,
			ExpType: dataType,
		}

	}

	// if it has parenthesis, its an method expression
	idx := strings.IndexByte(val, '(')
	if idx == -1 {
		// no paranthesis, its a raw exp
		return &schema.RawExpr{
			Val:     val,
			ExpType: dataType,
		}
	}

	name := val[:idx] // everything before this index
	endIdx := strings.IndexByte(val, ')')
	// can have args, strip them off the commas
	argsStr := val[idx+1 : endIdx]
	var args []schema.Expr

	if strings.TrimSpace(argsStr) != "" {
		rawArgs := strings.Split(argsStr, ",")
		for _, rawArg := range rawArgs {
			argStr := strings.TrimSpace(rawArg)
			argVal := argStr

			args = append(args, &schema.RawExpr{
				Val: argVal,
				// TODO: extract expression type here as well
			})
		}
	}
	return &schema.MethodExpr{
		Name: name,
		Args: args,
	}

}

var postmap = map[string]schema.Type{
	"uuid":                        schema.UUIDType{},
	"boolean":                     schema.BoolType{},
	"text":                        schema.TextType{},
	"bigint":                      schema.IntegerType{},
	"integer":                     schema.IntegerType{},
	"date":                        schema.TimeType{},
	"timestamp with time zone":    schema.TimeType{},
	"timestamp without time zone": schema.TimeType{},
	"jsonb":                       schema.JSONType{},
	"json":                        schema.JSONType{},
	"USER-DEFINED":                schema.EnumType{},
}

// 	type Column struct {
// 		Name       string 111
// 		Type       Type 111
// 		IsNullable bool 111
// 		IsPK       bool 111
// 		Default    Expr 111
// 		FKs        *[]ForeignKey
// 	}
