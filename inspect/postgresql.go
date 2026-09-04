package inspect

import (
	"context"
	"sync"

	"github.com/crizah/Worm/querier"
	"github.com/crizah/Worm/schema"
	"golang.org/x/sync/errgroup"
)

type PInspector struct {
	q *querier.PostgresQuerier
}

func NewPInspector(q *querier.PostgresQuerier) *PInspector {
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
	var dbName string

	g, gCtx := errgroup.WithContext(ctx) // if one fails, all fail

	g.Go(func() error {

		var err error
		tables, err = p.q.Tables(gCtx)
		return err
	})

	g.Go(func() error {
		var err error
		dbName, err = p.q.DbName(ctx)
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
	groupedIndexes := groupBy(indexes, func(c querier.IndexRow) string {
		return c.TableName
	})

	s := &schema.Schema{DbName: dbName}
	tableMap := make(map[string]*schema.Table)
	columnMap := make(map[string]*schema.Column) // maps tableName.columnName
	for _, t := range tables {
		columns := buildColumns(groupedCols[t], enums, groupedCons[t], columnMap)
		indexes, pk := buildIndexes(groupedIndexes[t], columns)

		table := schema.Table{
			Name:    t,
			Columns: columns,
			Indexes: indexes,
			PK:      pk,
		}
		tableMap[table.Name] = &table

		s.Tables = append(s.Tables, &table)
	}

	// once we have built all the tables, only then build the fks
	allFks := make(map[string]*schema.ForeignKey) // have a map of tableName.columnName

	for _, fk := range fks {
		k := fk.TableName + "." + fk.ColumnName
		sf, ok := allFks[k]
		if !ok {
			// if this doesnt exist yet, we need to make a new fk
			localTable := tableMap[fk.TableName]
			refTable := tableMap[fk.RefTableName]

			sf = p.buildFk(fk, refTable)
			allFks[k] = sf
			localTable.FKs = append(localTable.FKs, sf)
		}
		// then build the []columns and []ref columns
		sf.Columns = append(sf.Columns, columnMap[fk.TableName+"."+fk.ColumnName])
		sf.RefColumns = append(sf.RefColumns, columnMap[fk.RefTableName+"."+fk.RefColumnName])
	}

	return s, nil
}

func (p *PInspector) buildFk(fk querier.FkRow, refTable *schema.Table) *schema.ForeignKey {
	return &schema.ForeignKey{
		Name:     fk.Name,
		RefTable: refTable,
		OnUpdate: schema.ReferenceOption(fk.UpdateRule), // map cleanly, only for postgres, for sql we need a map
		// thats why this is a method
		OnDelete: schema.ReferenceOption(fk.DeleteRule),
	}

}
