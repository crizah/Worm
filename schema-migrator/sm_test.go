package schemamigrator

import (
	"testing"
)

func TestSchemaMigratorxx(t *testing.T) {
	sm, err := NewSchemaMigrator("yay.db", "../emmit/sqlite-migration-test.sql")
	if err != nil {
		t.Fatalf("error creating migrator: %v", err)
	}

	err = sm.MigrateSchema()
	if err != nil {
		t.Fatalf("error migrating schema: %v", err)
	}

}
