package querier

// tests against test data on a real scratch postgres container
import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

// docker run --rm -d --name worm-pg -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:16
// psql "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable" -f testdata/schema.sql

func TestPostQuerier(t *testing.T) {
	godotenv.Load()
	conn := os.Getenv("DB_CONN")

	q, err := NewPQuerier(conn)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}

	ctx := context.Background()

	t.Run("Tables", func(t *testing.T) {
		tables, err := q.Tables(ctx)
		if err != nil {
			t.Fatalf("Tables: %v", err)
		}
		want := []string{"organizations", "users"}
		if !equalStrings(tables, want) {
			t.Fatalf("Tables = %v, want %v", tables, want)
		}
	})

	t.Run("Columns", func(t *testing.T) {
		cols, err := q.Columns(ctx)
		if err != nil {
			t.Fatalf("Columns: %v", err)
		}
		byKey := columnsByKey(cols)

		id, ok := byKey["organizations.id"]
		if !ok {
			t.Fatalf("missing column organizations.id")
		}
		if id.DataType != "uuid" {
			t.Errorf("organizations.id data_type = %q, want uuid", id.DataType)
		}
		if id.IsNullable != "NO" {
			t.Errorf("organizations.id is_nullable = %q, want NO", id.IsNullable)
		}
		if id.DefaultExpr == nil || !strings.Contains(*id.DefaultExpr, "gen_random_uuid") {
			t.Errorf("organizations.id default = %v, want to contain gen_random_uuid", id.DefaultExpr)
		}

		name := byKey["organizations.name"]
		if name.IsNullable != "NO" {
			t.Errorf("organizations.name is_nullable = %q, want NO", name.IsNullable)
		}

		domain := byKey["organizations.domain"]
		if domain.IsNullable != "YES" {
			t.Errorf("organizations.domain is_nullable = %q, want YES (UNIQUE alone doesn't imply NOT NULL)", domain.IsNullable)
		}

		attendance := byKey["organizations.attendance_enabled"]
		if attendance.DataType != "boolean" {
			t.Errorf("organizations.attendance_enabled data_type = %q, want boolean", attendance.DataType)
		}
		if attendance.DefaultExpr == nil || !strings.Contains(*attendance.DefaultExpr, "false") {
			t.Errorf("organizations.attendance_enabled default = %v, want to contain false", attendance.DefaultExpr)
		}

		orgID := byKey["users.org_id"]
		if orgID.IsNullable != "NO" {
			t.Errorf("users.org_id is_nullable = %q, want NO", orgID.IsNullable)
		}

		role, ok := byKey["users.role"]
		if !ok {
			t.Fatalf("missing column users.role")
		}
		if role.DataType != "USER-DEFINED" {
			t.Errorf("users.role data_type = %q, want USER-DEFINED", role.DataType)
		}
		if role.UdtName != "user_role" {
			t.Errorf("users.role udt_name = %q, want user_role", role.UdtName)
		}
	})

	t.Run("Constraints", func(t *testing.T) {
		cons, err := q.Constraints(ctx)
		if err != nil {
			t.Fatalf("Constraints: %v", err)
		}

		var pks, uniques int
		for _, c := range cons {
			switch c.ConstraintType {
			case "PRIMARY KEY":
				pks++
			case "UNIQUE":
				uniques++
			default:
				t.Errorf("unexpected constraint_type %q on %s.%s", c.ConstraintType, c.TableName, c.ConstraintName)
			}
		}
		// organizations.id, users.id
		if pks != 2 {
			t.Errorf("PRIMARY KEY row count = %d, want 2", pks)
		}
		// organizations.domain (1 col) + users(org_id, email) (2 cols) = 3 key_column_usage rows
		if uniques != 3 {
			t.Errorf("UNIQUE row count = %d, want 3", uniques)
		}
	})

	t.Run("ForeignKeys", func(t *testing.T) {
		fks, err := q.ForeignKeys(ctx)
		if err != nil {
			t.Fatalf("ForeignKeys: %v", err)
		}
		if len(fks) != 1 {
			t.Fatalf("ForeignKeys count = %d, want 1", len(fks))
		}
		fk := (fks)[0]
		if fk.TableName != "users" || fk.ColumnName != "org_id" {
			t.Errorf("fk local side = %s.%s, want users.org_id", fk.TableName, fk.ColumnName)
		}
		if fk.RefTableName != "organizations" || fk.RefColumnName != "id" {
			t.Errorf("fk ref side = %s.%s, want organizations.id", fk.RefTableName, fk.RefColumnName)
		}
		if fk.DeleteRule != "CASCADE" {
			t.Errorf("fk delete_rule = %q, want CASCADE", fk.DeleteRule)
		}
		if fk.UpdateRule != "NO ACTION" {
			t.Errorf("fk update_rule = %q, want NO ACTION", fk.UpdateRule)
		}
	})

	t.Run("Enums", func(t *testing.T) {
		enums, err := q.Enums(ctx)
		if err != nil {
			t.Fatalf("Enums: %v", err)
		}
		got, ok := enums["user_role"]
		if !ok {
			t.Fatalf("enums = %v, missing user_role", enums)
		}
		want := []string{"admin", "member", "viewer"}
		if !equalStrings(got, want) {
			t.Errorf("user_role values = %v, want %v", got, want)
		}
	})
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func columnsByKey(cols []ColumnRow) map[string]ColumnRow {
	m := make(map[string]ColumnRow, len(cols))
	for _, c := range cols {
		m[c.TableName+"."+c.Name] = c
	}
	return m
}
