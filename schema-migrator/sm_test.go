package schemamigrator

import (
	"context"
	"testing"
)

func TestSchemaMigratorxx(t *testing.T) {
	sm, err := NewSchemaMigrator("yay.db", 1, "../emmit/sqlite-migration-test.sql")
	if err != nil {
		t.Fatalf("error creating migrator: %v", err)
	}
	ctx := context.Background()

	err = sm.MigrateSchema(ctx)
	if err != nil {
		t.Fatalf("error migrating schema: %v", err)
	}

}
