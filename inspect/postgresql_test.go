package inspect

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/crizah/Worm/querier"
	"github.com/crizah/Worm/schema"
	"github.com/joho/godotenv"
)

var testData *schema.Schema

func TestPInspect(t *testing.T) {
	godotenv.Load()
	conn := os.Getenv("DB_CONN")

	q, err := querier.NewPQuerier(conn)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}

	ctx := context.Background()
	pi := NewPInspector(q)
	sc, err := pi.Inspect(ctx)

	if sc.DbName != testData.DbName {
		t.Errorf("Not the right db name, got %s", sc.DbName)
		return
	}

	expected := testData.Tables
	got := sc.Tables

	tableMap := checkStuff(t, expected, got, func(c *schema.Table) string {
		return c.Name
	})
	if tableMap == nil {
		t.Errorf("some error")
		return
	}

	for e, g := range tableMap {
		if !testTables(t, e, g) {
			t.Errorf("Error in table %s", e.Name)
			return
		}

	}

}

func checkStuff[T any](t *testing.T, a []*T, b []*T, getKey func(*T) string) map[*T]*T {

	if len(a) != len(b) {
		t.Errorf("Expected size %d got size %d", len(a), len(b))
		// not sure of the syntax here
		return nil
	}

	gotMap := make(map[string]*T)
	for _, col := range b {
		key := getKey(col)
		gotMap[key] = col
	}

	totalMap := make(map[*T]*T)

	for _, col := range a {
		key := getKey(col)
		got, ok := gotMap[key]
		if !ok {
			t.Errorf("Expected ColumnName %s to exist", key)
			return nil

		}
		totalMap[col] = got
	}

	return totalMap

}

func testColumns(t *testing.T, e *schema.Column, g *schema.Column) bool {
	if e.Name != g.Name {
		t.Errorf("Expected Name %s got name %s", e.Name, g.Name)
		return false
	}

	if e.Type != g.Type {
		if !reflect.DeepEqual(e.Type, g.Type) {
			// %T prints the underlying type
			// %+v prints the struct values
			t.Errorf("Expected Type %T(%+v) got Type %T(%+v)", e.Type, e.Type, g.Type, g.Type)
			return false
		}

	}
	return true

}

func testIndexes(t *testing.T, e *schema.Index, g *schema.Index) bool {
	// already checked names, check columns
	expected := e.Columns
	got := g.Columns

	colMap := checkStuff(t, expected, got, func(c *schema.Column) string {
		return c.Name
	})
	if colMap == nil {
		return false
	}

	for ee, gg := range colMap {
		if !testColumns(t, ee, gg) {
			t.Errorf("Error in Column %s", ee.Name)
			return false
		}

	}

	if e.IsPK != g.IsPK {
		t.Errorf("Error in IsPk expected %t ", e.IsPK)
		return false
	}

	if e.IsUnique != g.IsUnique {
		t.Errorf("Error in IsUnique expected %t ", e.IsUnique)
		return false
	}

	return true

}

func testFks(t *testing.T, e *schema.ForeignKey, g *schema.ForeignKey) bool {
	// TODO: make this function
	// if i call check table isndie here, is it going to be an infinite loop lol?
	return false
}

func testTables(t *testing.T, e *schema.Table, g *schema.Table) bool {

	expected := e.Columns
	got := g.Columns

	colMap := checkStuff(t, expected, got, func(c *schema.Column) string {
		return c.Name
	})
	if colMap == nil {
		return false
	}

	for ee, gg := range colMap {
		if !testColumns(t, ee, gg) {
			t.Errorf("Error in Column %s", ee.Name)
			return false
		}

	}

	// and the same for fks and indexes

	// Indexes
	ex := e.Indexes
	gott := g.Indexes

	indexMap := checkStuff(t, ex, gott, func(c *schema.Index) string {
		return c.Name
	})
	if indexMap == nil {
		return false
	}

	for ee, gg := range indexMap {
		if !testIndexes(t, ee, gg) {
			t.Errorf("Error in Index %s", ee.Name)
			return false
		}

	}

	// Fks
	ek := e.FKs
	gk := g.FKs

	fkMap := checkStuff(t, ek, gk, func(c *schema.ForeignKey) string {
		return c.Name
	})
	if fkMap == nil {
		return false
	}

	for ee, gg := range fkMap {
		if !testFks(t, ee, gg) {
			t.Errorf("Error in Index %s", ee.Name)
			return false
		}

	}

	// test PK
	if !testIndexes(t, e.PK, g.PK) {
		t.Errorf("Error in PK %s", e.Name)
		return false
	}

	return true

}
