package emitter

import (
	"context"
	"os"
	"testing"

	"github.com/crizah/Worm/inspect"
	"github.com/crizah/Worm/querier"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

var expectedSortTable = []string{"organizations", "users"}
var expectedStatements = []string{
	"CREATE TABLE organizations (\nid TEXT NOT NULL DEFAULT NUHHUH,\nname TEXT NOT NULL ,\ndomain TEXT  ,\nattendance_enabled INTEGER NOT NULL DEFAULT 0,\ncreated_at TEXT  DEFAULT CURRENT_TIMESTAMP,\nPRIMARY KEY (id)\n);",
	"CREATE TABLE users (\nid TEXT NOT NULL DEFAULT NUHHUH,\norg_id TEXT NOT NULL ,\nemail TEXT NOT NULL ,\nrole TEXT NOT NULL DEFAULT 'member' CHECK(role IN ('admin', 'member', 'viewer')),\ncreated_at TEXT  DEFAULT CURRENT_TIMESTAMP,\nPRIMARY KEY (id),\nFOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE ON UPDATE NO ACTION\n);",
	"CREATE UNIQUE INDEX organizations_domain_key ON organizations (domain);",
	"CREATE INDEX idx_users_org_id ON users (org_id);",
	"CREATE UNIQUE INDEX users_org_id_email_key ON users (org_id,email);",
}

func TestSqLiteEmitter(t *testing.T) {
	godotenv.Load()
	conn := os.Getenv("DB_CONN")

	q, err := querier.NewPQuerier(conn)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}

	ctx := context.Background()
	pi := inspect.NewPInspector(q)
	sc, err := pi.Inspect(ctx)

	emm := NewSqlEmitter(sc, "./", "sqlite-migration-test.sql")
	stmts := emm.Emitt()

	err = writeFile(emm.dirPath, emm.fileName, stmts)
	if err != nil {
		t.Fatalf("error writing file: %v", err)
	}
	for i, tab := range sc.Tables {
		if !testSortTables(t, expectedSortTable[i], tab.Name) {
			t.Errorf("error sorting tables")
			return
		}
	}

	for i, s := range stmts {
		if !testStatements(t, expectedStatements[i], s) {
			t.Errorf("error on stmts")
			return

		}
	}

}
func testStatements(t *testing.T, e string, g string) bool {
	if e != g {
		t.Errorf("expected: %s got: %s", e, g)
		return false
	}
	return true
}

func testSortTables(t *testing.T, e string, g string) bool {
	if e != g {
		t.Errorf("expected name: %s got name: %s", e, g)
		return false
	}
	return true
}
