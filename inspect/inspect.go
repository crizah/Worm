package inspect

import (
	"context"
	"strings"

	"github.com/crizah/Worm/querier"
	"github.com/crizah/Worm/schema"
)

type Inspector interface {
	Inspect(ctx context.Context) (*schema.Schema, error)
	buildFk(fk querier.FkRow, refTable *schema.Table) *schema.ForeignKey
}

func buildIndexes(ind []querier.IndexRow, cols []*schema.Column) ([]*schema.Index, *schema.Index) {
	// Note: both ind and cols are already table specific

	// group IndexName -> []schema.Columns
	indexMap := make(map[string][]*schema.Column)
	// maps if this index is unqiue or not via index name
	isUnique := make(map[string]bool)

	var pk string
	for _, i := range ind {
		// per index name, group the columns
		var groupedColumn []*schema.Column
		if i.IsPrimaryKey {
			pk = i.Name
		}
		isUnique[i.Name] = i.IsUnique
		for _, cname := range cols {
			if cname.Name == i.ColumnName {
				groupedColumn = append(groupedColumn, cname)
			}
		}
		indexMap[i.Name] = append(indexMap[i.Name], groupedColumn...) // multi column indexes

	}

	var ans []*schema.Index
	var Pk *schema.Index
	for name, columns := range indexMap { // iterate the map to avoid duplicates
		index := &schema.Index{
			Name:     name,
			Columns:  columns,
			IsPK:     pk == name,
			IsUnique: isUnique[name],
		}
		if index.IsPK {
			Pk = index
		}
		ans = append(ans, index)
	}
	return ans, Pk

}

func groupBy[T any](items []T, getKey func(T) string) map[string][]T {
	// cant do a T.TableName on a generic type, so we pass a function that returns the table name instead
	grouped := make(map[string][]T)

	for _, item := range items {
		key := getKey(item)
		grouped[key] = append(grouped[key], item)
	}

	return grouped

}

func buildColumns(cols []querier.ColumnRow, enums map[string][]string, con []querier.ConstraintRow, columnMap map[string]*schema.Column) []*schema.Column {
	pk := make(map[string]int) // 1 means PK, 2 means unique
	for _, c := range con {
		switch c.ConstraintType {
		case "PRIMARY KEY":
			pk[c.ColumnName] = 1
		case "UNIQUE":
			pk[c.ColumnName] = 2
		default:
			// will never happen lol
			pk[c.ColumnName] = 0
		}
	}
	var ans []*schema.Column
	for _, c := range cols {
		col := schema.Column{Name: c.Name}
		col.IsNullable = c.IsNullable != "NO"
		col.IsPK = pk[c.Name] == 1

		t, _ := postmap[c.DataType]
		if e, isEnum := t.(schema.EnumType); isEnum {
			e.Name = c.UdtName
			e.Values = enums[e.Name]
			t = e // must reassign
		}
		col.Type = t

		col.Default = buildDefaultExp(t, c.DefaultExpr)
		columnMap[c.TableName+"."+col.Name] = &col

		ans = append(ans, &col)
	}
	return ans
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
