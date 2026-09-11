package inspect

import (
	"context"
	"regexp"
	"strings"

	"github.com/crizah/Worm/querier"
	"github.com/crizah/Worm/schema"
)

type Inspector interface {
	Inspect(ctx context.Context) (*schema.Schema, error)
	buildFk(fk querier.FkRow, refTable *schema.Table) *schema.ForeignKey
}

// splits a partial index predicate on its top level AND/OR into one raw side per predicate
var predicateSplitRe = regexp.MustCompile(`\s*(AND|OR)\s*`)

func buildIndexes(ind []querier.IndexRow, cols []*schema.Column) ([]*schema.Index, *schema.Index) {
	// Note: both ind and cols are already table specific

	// group IndexName -> []schema.Columns
	indexMap := make(map[string][]*schema.Column)
	// maps if this index is unqiue or not via index name
	isUnique := make(map[string]bool)
	predicateMap := make(map[string][]*schema.Predicate) // maps index name to Predicates if exists
	columnMap := make(map[string]*schema.Column)         // maps columnName to column

	for _, c := range cols {
		columnMap[c.Name] = c
	}

	var pk string
	for _, i := range ind {
		// per index name, group the columns
		var groupedColumn []*schema.Column
		if i.IsPrimaryKey {
			pk = i.Name
		}
		// get the predicates
		if i.IsPartial {
			// composite partial indexes have multiple rows (one per column), only parse once per index name
			if _, done := predicateMap[i.Name]; !done {
				// format:
				// (event_type = 'MISSED_DEADLINE'::text) AND (superseded = false)
				// each side of AND is one predicate (could also be OR)
				s := *i.PartialPredicate
				parts := predicateSplitRe.Split(s, -1)
				joiners := predicateSplitRe.FindAllString(s, -1) // gets the "AND", "OR" in order

				var pred []*schema.Predicate
				for i, p := range parts {
					//  this is in the format columnName = 'VALUE'::<enum_name>, (if enum type)
					// or columnName = <value> if non enum type
					//
					// get the column name, if the column type is enum type, do the enum type extraction
					// i.e:
					//
					// parts := strings.Split(val, "::")
					// cleanVal := strings.Trim(parts[0], "'")
					//
					// otherwise, do normal extraction
					//
					pp := strings.Split(p, "=")         // NOTE: hardcoding = operator here, can support more later on
					cName := strings.Trim(pp[0], " ()") // remove whitespace and brackets as well
					column := columnMap[cName]

					predicate := &schema.Predicate{
						Column:   column,
						Operator: schema.EQUALS,
					}

					// do the joins
					if i == 0 {
						// first one gets empty
						predicate.PredicateJoin = schema.EmptyOp
					} else {
						joiner := strings.TrimSpace(joiners[i-1])

						if joiner == "AND" {
							predicate.PredicateJoin = schema.AndOp
						} else if joiner == "OR" {
							predicate.PredicateJoin = schema.OrOp
						}
					}

					cc, isEnum := column.Type.(schema.EnumType)
					if isEnum {
						ppp := strings.Split(pp[1], "::")
						enumVal := strings.Trim(ppp[0], " '") // remove whitespace then quotes
						predicate.ColumnValue = &schema.RawExpr{
							Val:     enumVal,
							ExpType: cc,
						}
					} else {
						vvv := strings.Split(pp[1], "::")          // strip the type cast suffix if present, eg 'owner@company.com'::text
						nonEnumVal := strings.Trim(vvv[0], " ()'") // remove whitespace, brackets and quotes as well
						predicate.ColumnValue = &schema.RawExpr{
							Val:     nonEnumVal,
							ExpType: column.Type,
						}
					}
					pred = append(pred, predicate)
				}
				predicateMap[i.Name] = pred
			}
		}

		isUnique[i.Name] = i.IsUnique
		for _, cname := range cols {
			if cname.Name == i.ColumnName {
				groupedColumn = append(groupedColumn, cname)
			}
		}
		indexMap[i.Name] = append(indexMap[i.Name], groupedColumn...) // multi column indexes have multiple rows

	}

	var ans []*schema.Index
	var Pk *schema.Index
	for name, columns := range indexMap { // iterate the map to avoid duplicates
		isPart := len(predicateMap[name]) > 0
		index := &schema.Index{
			Name:       name,
			Columns:    columns,
			IsPK:       pk == name,
			IsUnique:   isUnique[name],
			IsPartial:  isPart,
			Predicates: predicateMap[name],
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
		col.IsUnique = pk[c.Name] == 2

		t, _ := postmap[c.DataType]
		e, isEnum := t.(schema.EnumType)
		if isEnum {
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
		// normalise the val if its a bool
		if b, isBool := dataType.(schema.BoolType); isBool {
			// NOTE: POSTGRES SPECIFIC HERE, we need to normalise the value
			if b.Val == "true" {
				return &schema.RawExpr{
					Val:     "true",
					ExpType: dataType,
				}
			} else {
				return &schema.RawExpr{
					Val:     "false",
					ExpType: dataType,
				}
			}

		}
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
