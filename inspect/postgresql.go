package inspect

import (
	"context"
	"strings"
	"sync"

	"github.com/crizah/Worm/querier"
	"github.com/crizah/Worm/schema"
	"golang.org/x/sync/errgroup"
)

type PInspector struct {
	q querier.PostgresQuerier
}

func NewPInspector(q querier.PostgresQuerier) *PInspector {
	return &PInspector{
		q: q,
	}
}

func (p *PInspector) Inspect(ctx context.Context) (*schema.Schema, error) {
	var wg sync.WaitGroup
	wg.Add(6)
	// if any error out, we stop all
	var tables []string
	var cols []querier.ColumnRow
	var cons []querier.ConstraintRow
	var fks []querier.FkRow
	var enums map[string][]string
	var indexes []querier.IndexRow

	g, gCtx := errgroup.WithContext(ctx) // if one fails, all fail

	g.Go(func() error {

		var err error
		tables, err = p.q.Tables(gCtx)
		return err
	})

	g.Go(func() error {
		var err error
		cols, err = p.q.Columns(gCtx)
		return err
	})

	g.Go(func() error {
		var err error
		cons, err = p.q.Constraints(gCtx)
		return err
	})

	g.Go(func() error {
		var err error
		fks, err = p.q.ForeignKeys(gCtx)
		return err
	})

	g.Go(func() error {
		var err error
		enums, err = p.q.Enums(gCtx)
		return err
	})

	g.Go(func() error {
		var err error
		indexes, err = p.q.Indexes(gCtx)
		return err
	})

	wg.Wait()
	if err := g.Wait(); err != nil {
		return nil, err
	}

	groupedCols := groupBy(cols, func(c querier.ColumnRow) string {
		return c.TableName
	})

	groupedCons := groupBy(cons, func(c querier.ConstraintRow) string {
		return c.TableName
	})
	groupedFks := groupBy(fks, func(c querier.FkRow) string {
		return c.TableName
	})

	groupedIndexes := groupBy(indexes, func(c querier.IndexRow) string {
		return c.TableName
	})

	s := &schema.Schema{}
	for _, t := range tables {
		columns := buildColumns(t, groupedCols[t], enums, groupedCons[t])

		// tableFks:= buildFks(groupedFks[t]) // we need to build all tables before we can build the fks
		indexes, pk := buildIndexes(groupedIndexes[t], columns)

		table := schema.Table{
			Name:    t,
			Columns: columns, // TODO: doesnt have fk yet and unique constraint (remove fk from column level to only table level)
			PK:      pk,
			Indexes: indexes,
		}

		s.Tables = append(s.Tables, table)
	}
	return s, nil
}

func buildIndexes(ind []querier.IndexRow, cols []schema.Column) ([]*schema.Index, *schema.Index) {
	// group IndexName -> []schema.Columns
	indexMap := make(map[string][]schema.Column)
	var pk string
	for _, i := range ind {
		// per index name, group the columns
		var groupedColumn []schema.Column
		if i.IsPrimaryKey {
			pk = i.Name
		}
		for _, cname := range cols {
			if cname.Name == i.ColumnName {
				groupedColumn = append(groupedColumn, cname)
			}
		}
		indexMap[i.Name] = groupedColumn

	}

	var ans []*schema.Index
	var Pk *schema.Index
	for _, i := range ind {
		index := &schema.Index{
			Name:    i.Name,
			Columns: indexMap[i.Name],
			IsPK:    pk == i.Name,
		}
		if index.IsPK {
			Pk = index
		}
		ans = append(ans, index)
	}
	return ans, Pk

}

// type Table struct {
// 	Name    string
// 	Columns []Column
// 	PK      *Index
// 	FKs     []ForeignKey
// 	Indexes []*Index
// }

// func buildFks(fks []querier.FkRow)([]schema.ForeignKey){
// 	var ans []schema.ForeignKey
// 	for _, f := range fks{
// 		k := schema.ForeignKey{
// 			RefTable: ,

// 		}

// 	}
// }
// type FkRow struct {
// 	TableName      string `db:"table_name"`
// 	ColumnName     string `db:"column_name"`
// 	ConstraintName string `db:"constraint_name"`
// 	RefTableName   string `db:"ref_table_name"`
// 	RefColumnName  string `db:"ref_column_name"`
// 	UpdateRule     string `db:"update_rule"`
// 	DeleteRule     string `db:"delete_rule"`
// }

// type ForeignKey struct {
// 	RefTable   *Table
// 	RefColumns []*Column
// 	Columns    []*Column

// 	OnUpdate ReferenceOption
// 	OnDelete ReferenceOption
// }

func groupBy[T any](items []T, getKey func(T) string) map[string][]T {
	// cant do a T.TableName on a generic type, so we pass a function that returns the table name instead
	grouped := make(map[string][]T)

	for _, item := range items {
		key := getKey(item)
		grouped[key] = append(grouped[key], item)
	}

	return grouped

}

func buildColumns(tableName string, cols []querier.ColumnRow, enums map[string][]string, con []querier.ConstraintRow) []schema.Column {
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
	var ans []schema.Column
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

		ans = append(ans, col)
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

// 	type Column struct {
// 		Name       string 111
// 		Type       Type 111
// 		IsNullable bool 111
// 		IsPK       bool 111
// 		Default    Expr 111
// 	}
