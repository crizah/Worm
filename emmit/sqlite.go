package emitter

import (
	"fmt"
	"strings"

	"github.com/crizah/Worm/schema"
)

type SQLiteEmitter struct {
	Schema   *schema.Schema
	dirPath  string
	fileName string
}

func NewSqlEmitter(sch *schema.Schema, path string, filename string) *SQLiteEmitter {
	t := sortTables(sch.Tables)
	sch.Tables = t
	return &SQLiteEmitter{
		Schema:   sch,
		dirPath:  path, // path to write the migration file
		fileName: filename,
	}
}

func (e *SQLiteEmitter) Emitt() []string {
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

			stmts = append(stmts, fmt.Sprintf("CREATE %sINDEX %s ON %s (%s);", isUnique, i.Name, t.Name, strings.Join(cols, ",")))
		}
	}
	return stmts
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
		cols = append(cols, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(pkCols, ", ")))
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
		cols = append(cols, fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s(%s) ON DELETE %s ON UPDATE %s",
			strings.Join(local, ", "), fk.RefTable.Name, strings.Join(ref, ", "), fk.OnDelete, fk.OnUpdate))
	}

	// create all table and then create indexes after that

	stmt := fmt.Sprintf("CREATE TABLE %s (\n%s\n);", t.Name, strings.Join(cols, ",\n"))
	return stmt

}

func (e *SQLiteEmitter) buildColumns(c *schema.Column) string {
	// TODO: FK is left in here

	notNull := ""
	if !c.IsNullable {
		notNull = "NOT NULL"
	}

	var typeValue string
	// enum type is unhashable becaiuse of slice
	if _, isEnum := c.Type.(schema.EnumType); isEnum {
		typeValue = "TEXT"
	} else {
		typeValue = sqliteTypeMap[c.Type]
	}

	// check if the column was boolean type, we need to map false->1 and true-> 0 in defaults if it has one
	isBool := false
	if _, isb := c.Type.(schema.BoolType); isb {
		isBool = true

	}

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
			currValue := "'" + r.Val + "'"
			defaultValue = fmt.Sprintf("DEFAULT %s CHECK(%s IN (%s))", currValue, c.Name, strings.Join(quoted, ", "))
		} else {
			// over here, check if the integer type was actually boolean in postgres, and emit 1 or 0 according to that in defaults
			if typeValue == "INTEGER" {
				if isBool {
					if r.Val == "true" {
						defaultValue = "DEFAULT 1"
					} else {
						defaultValue = "DEFAULT 0"
					}
				} else {
					// normal integer, without quotes
					defaultValue = "DEFAULT " + r.Val
				}

			} else {
				// TEXT type with quotes
				defaultValue = "DEFAULT '" + r.Val + "'"
			}

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
			// TODO: BUILD DEFAULT METHOD FOR THE SPECIFIC GUY HERE
			defaultValue = "DEFAULT NUHHUH"
		} else {
			defaultValue = "DEFAULT " + method
		}

	}

	ans := fmt.Sprintf("%s %s %s %s", c.Name, typeValue, notNull, defaultValue)
	return ans

}

var sqliteGenRandomUUID = `(
        lower(hex(randomblob(4))) || '-' ||
        lower(hex(randomblob(2))) || '-4' ||
        substr(lower(hex(randomblob(2))),2) || '-' ||
        substr('89ab',abs(random()) % 4 + 1, 1) ||
        substr(lower(hex(randomblob(2))),2) || '-' ||
        lower(hex(randomblob(6)))
    )`

var sqliteTypeMap = map[schema.Type]string{
	// sqlite doenst have boolean, so map it to integer
	schema.BoolType{}:    "INTEGER",
	schema.IntegerType{}: "INTEGER",

	// same with other fleshed out types, sqlite doesnt have them, all text
	schema.UUIDType{}: "TEXT",
	schema.TextType{}: "TEXT",
	schema.TimeType{}: "TEXT",
	schema.JSONType{}: "TEXT",
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
	"gen_random_uuid":       sqliteGenRandomUUID,
	"uuid_generate_v4":      "NULL",
	"nextval":               "NULL", // SQLite uses INTEGER PRIMARY KEY AUTOINCREMENT instead
	"inet_client_addr":      "NULL",
	"random":                "random()", // Note: PG returns 0.0-1.0, SQLite returns a 64-bit int
	"md5":                   "NULL",
}
