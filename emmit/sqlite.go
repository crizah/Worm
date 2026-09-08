package emitter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crizah/Worm/schema"
)

type SQLiteEmitter struct {
	Schema  *schema.Schema
	dirPath string
}

func NewSqlEmitter(sch *schema.Schema, path string) *SQLiteEmitter {
	t := sortTables(sch.Tables)
	sch.Tables = t
	return &SQLiteEmitter{
		Schema:  sch,
		dirPath: path, // path to write the migration file
	}
}

func (e *SQLiteEmitter) Emitt() error {
	// writes migration files
	var stmts []string
	for _, t := range e.Schema.Tables {
		stmts = append(stmts, e.buildTable(t))
	}

	// build indexes after all the tables (i think we NEED to do it this way, cos() we arent sorting based on contraints, just fk dependency)
	for _, t := range e.Schema.Tables {
		for _, i := range t.Indexes {
			if i.IsPK {
				// we already did this while building the tables itself
				continue
			}
			var cols []string
			for _, col := range i.Columns {
				cols = append(cols, col.Name)
			}

			isUnique := ""
			if i.IsUnique {
				isUnique = "UNIQUE "
			}

			stmts = append(stmts, "CREATE %s INDEX %s ON %s (%s)", isUnique, i.Name, t.Name, strings.Join(cols, ","))
		}
	}

	if err := os.MkdirAll(e.dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	fullPath := filepath.Join(e.dirPath, "migration.sql")

	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	defer file.Close()
	for _, line := range stmts {
		if _, err := file.WriteString(line + "\n"); err != nil {
			return fmt.Errorf("failed to write to file: %w", err)
		}
	}

	return nil
}

func (e *SQLiteEmitter) buildTable(t *schema.Table) string {
	// just emitt strings ig??
	var cols []string
	for _, c := range t.Columns {

		col := e.buildColumns(c)
		cols = append(cols, col)
	}

	if t.PK != nil {
		// find the columns that have the Pks
		var pkCols []string
		for _, col := range t.PK.Columns {
			pkCols = append(pkCols, col.Name)
		}
		cols = append(cols, fmt.Sprintf(" PRIMARY KEY (%s)", strings.Join(pkCols, ", ")))
	}

	// build fks
	for _, fk := range t.FKs {
		var local, ref []string
		for _, c := range fk.Columns {
			local = append(local, c.Name)
		}
		for _, c := range fk.RefColumns {
			ref = append(ref, c.Name)
		}
		cols = append(cols, fmt.Sprintf("  FOREIGN KEY (%s) REFERENCES %s(%s) ON DELETE %s ON UPDATE %s",
			strings.Join(local, ", "), fk.RefTable.Name, strings.Join(ref, ", "), fk.OnDelete, fk.OnUpdate))
	}

	// create all table and then create indexes after that

	stmt := fmt.Sprintf("CREATE TABLE %s (\n%s\n);", t.Name, strings.Join(cols, ",\n"))
	return stmt

}

func (e *SQLiteEmitter) buildColumns(c *schema.Column) string {
	// TODO: FK is left in here

	notNull := ""
	if c.IsNullable {
		notNull = "NOT NULL"
	}

	typeValue := sqliteTypeMap[c.Type]

	defaultValue := ""
	if r, ok := c.Default.(*schema.RawExpr); ok {
		// raw exp
		// depending on values, emit with or without qoutes

		// handle enums
		// status TEXT DEFAULT 'INVITED' CHECK(status IN ('INVITED', 'ACTIVE', 'SUSPENDED'))
		en, ok := r.ExpType.(schema.EnumType)
		if ok {
			quoted := make([]string, len(en.Values))
			for i, v := range en.Values {
				quoted[i] = "'" + v + "'"
			}
			currValue := "'" + en.Name + "'"
			defaultValue = fmt.Sprintf(" DEFAULT %s CHECK(%s IN (%s))", currValue, c.Name, strings.Join(quoted, ", "))
		}

		if typeValue == "INTEGER" {
			defaultValue = "DEFAULT" + r.Val
		} else {
			defaultValue = "DEFAULT '" + r.Val + "'"
		}

	}
	if m, ok := c.Default.(*schema.MethodExpr); ok {
		// method exp
		// fuck args for now

		method := schemaFuncsToSqliteDefaults[m.Name]
		if method == "NULL" {
			// we dont have an equivelent default for this
			// log it
			// TODO: LOG THIS SOMEWHERE
		} else {
			defaultValue = "DEFAULT" + method
		}

	}

	ans := fmt.Sprintf("%s %s %s %s", c.Name, typeValue, notNull, defaultValue)
	return ans

}

func sortTables(tables []*schema.Table) []*schema.Table {
	// build adj map, i.e for every table, what tables depend on it
	// build incoming arr, i.e for every table, how many tables depend on this guy
	// sort arr, add to queue all that have 0 and remove dependency one by one, if the dependency for any guy becoumes 0
	// add to ans
	// cyclic guys will be all thats left, add them as is

	// TODO: since multiple fks in the same table can ref to the same refTable, handle that, but i think it should be fine
	var ans []*schema.Table
	adjMap := make(map[*schema.Table][]*schema.Table)
	incoming := make(map[*schema.Table]int) // a map is way better i dont think a vector will work

	for _, t := range tables {
		incoming[t] = 0
	}

	for _, t := range tables {
		for _, fk := range t.FKs {
			incoming[t]++
			adjMap[fk.RefTable] = append(adjMap[fk.RefTable], t)
		}
	}

	var q []*schema.Table
	for _, t := range tables {
		if incoming[t] == 0 {
			q = append(q, t)
		}
	}

	for !(len(q) == 0) {
		t := q[0] // front
		q = q[1:] // pop
		ans = append(ans, t)

		// for all the guys that this giuy references
		for _, n := range adjMap[t] {
			incoming[n]--

			if incoming[n] == 0 {
				// no more outgoing, add to queue
				q = append(q, n)
			}
		}

	}

	// only guys left will be cyclic guys, append them as is
	if len(ans) != len(tables) {
		for _, t := range tables {
			if incoming[t] > 0 {
				ans = append(ans, t)
			}
		}

	}

	return ans

}

var sqliteTypeMap = map[schema.Type]string{
	// sqlite doenst have boolean, so map it to integer
	schema.BoolType{}:    "INTEGER",
	schema.IntegerType{}: "INTEGER",

	// same with other fleshed out types, sqlite doesnt have them, all text
	schema.UUIDType{}: "TEXT",
	schema.TextType{}: "TEXT",
	schema.TimeType{}: "TEXT",
	schema.JSONType{}: "TEXT",
	schema.EnumType{}: "TEXT",
}

// should do mapping on the default functions we have from schema to sqlite
// i think let the schema have the same names for functions as postgres, and then if needed, we can map
// to specified languages

var schemaFuncsToSqliteDefaults = map[string]string{
	// NULL means there isnt any counterpart, omitt as warning to the user
	"now":                   "CURRENT_TIMESTAMP",
	"transaction_timestamp": "CURRENT_TIMESTAMP",
	"statement_timestamp":   "CURRENT_TIMESTAMP",
	"clock_timestamp":       "CURRENT_TIMESTAMP",
	"gen_random_uuid":       "NULL",
	"uuid_generate_v4":      "NULL",
	"nextval":               "NULL", // SQLite uses INTEGER PRIMARY KEY AUTOINCREMENT instead
	"inet_client_addr":      "NULL",
	"random":                "random()", // Note: PG returns 0.0-1.0, SQLite returns a 64-bit int
	"md5":                   "NULL",
}
