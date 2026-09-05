package inspect

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/crizah/Worm/querier"
	"github.com/crizah/Worm/schema"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

var testData *schema.Schema

// all this data is global right now, cant run on a diff test suite
var tableVisited = make(map[string]struct{}) // maps if this table is already visited by either testTable or testFks

// expected shape of querier/testdata/schema.sql after going through the querier + inspector
func init() {
	orgID := &schema.Column{Name: "id", Type: schema.UUIDType{}}
	orgName := &schema.Column{Name: "name", Type: schema.TextType{}}
	orgDomain := &schema.Column{Name: "domain", Type: schema.TextType{}}
	orgAttendance := &schema.Column{Name: "attendance_enabled", Type: schema.BoolType{}}
	orgCreatedAt := &schema.Column{Name: "created_at", Type: schema.TimeType{}}

	orgPK := &schema.Index{
		Name:     "organizations_pkey",
		Columns:  []*schema.Column{orgID},
		IsPK:     true,
		IsUnique: true,
	}
	orgDomainUnique := &schema.Index{
		Name:     "organizations_domain_key",
		Columns:  []*schema.Column{orgDomain},
		IsUnique: true,
	}

	organizations := &schema.Table{
		Name:    "organizations",
		Columns: []*schema.Column{orgID, orgName, orgDomain, orgAttendance, orgCreatedAt},
		PK:      orgPK,
		Indexes: []*schema.Index{orgPK, orgDomainUnique},
	}

	userID := &schema.Column{Name: "id", Type: schema.UUIDType{}}
	userOrgID := &schema.Column{Name: "org_id", Type: schema.UUIDType{}}
	userEmail := &schema.Column{Name: "email", Type: schema.TextType{}}
	userRole := &schema.Column{Name: "role", Type: schema.EnumType{Name: "user_role", Values: []string{"admin", "member", "viewer"}}}
	userCreatedAt := &schema.Column{Name: "created_at", Type: schema.TimeType{}}

	usersPK := &schema.Index{
		Name:     "users_pkey",
		Columns:  []*schema.Column{userID},
		IsPK:     true,
		IsUnique: true,
	}
	usersOrgEmailUnique := &schema.Index{
		Name:     "users_org_id_email_key",
		Columns:  []*schema.Column{userOrgID, userEmail},
		IsUnique: true,
	}
	usersOrgIDIndex := &schema.Index{
		Name:    "idx_users_org_id",
		Columns: []*schema.Column{userOrgID},
	}

	users := &schema.Table{
		Name:    "users",
		Columns: []*schema.Column{userID, userOrgID, userEmail, userRole, userCreatedAt},
		PK:      usersPK,
		Indexes: []*schema.Index{usersPK, usersOrgEmailUnique, usersOrgIDIndex},
		FKs: []*schema.ForeignKey{
			{
				Name:       "users_org_id_fkey",
				RefTable:   organizations,
				RefColumns: []*schema.Column{orgID},
				Columns:    []*schema.Column{userOrgID},
				OnUpdate:   schema.NoAction,
				OnDelete:   schema.Cascade,
			},
		},
	}

	testData = &schema.Schema{
		DbName: "worm_dev",
		Tables: []*schema.Table{organizations, users},
	}
}

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
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}

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
		var zero T
		t.Errorf("Error in %T Expected size %d got size %d", zero, len(a), len(b))
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

	if !reflect.DeepEqual(e.Type, g.Type) {
		// %T prints the underlying type
		// %+v prints the struct values
		t.Errorf("Expected Type %T(%+v) got Type %T(%+v)", e.Type, e.Type, g.Type, g.Type)
		return false
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
	// ref columns
	expected := e.RefColumns
	got := g.RefColumns

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
	// columns
	expected = e.Columns
	got = g.Columns

	colMap = checkStuff(t, expected, got, func(c *schema.Column) string {
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

	// references
	if e.OnUpdate != g.OnUpdate {
		t.Errorf("Error onUpdate expected %s, got %s", e.OnUpdate, g.OnUpdate)
		return false
	}

	if e.OnDelete != g.OnDelete {
		t.Errorf("Error onDelete expected %s, got %s", e.OnDelete, g.OnDelete)
		return false
	}

	// ref tables

	if e.RefTable != nil && g.RefTable != nil {
		if !testTables(t, e.RefTable, g.RefTable) {
			return false
		}
	}

	return true
}

func testTables(t *testing.T, e *schema.Table, g *schema.Table) bool {
	_, ok := tableVisited[e.Name]
	if ok {
		// must be true right if the cycle is continuing?
		return true
	}
	// we wont have a circular dependency, so i think this should work with just a visited array
	tableVisited[e.Name] = struct{}{} // mark visited

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
