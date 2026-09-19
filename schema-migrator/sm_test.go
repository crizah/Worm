package schemamigrator

import (
	"context"
	"testing"

	"github.com/crizah/Worm/utils"
)

func TestSchemaMigratorxx(t *testing.T) {
	db, err := utils.PingDB(1, "yay.db")
	if err != nil {
		t.Fatalf("pinging err %s", err.Error())
	}

	sm, err := NewSchemaMigrator(db, 1, "../emmit/sqlite-migration-test.sql")
	if err != nil {
		t.Fatalf("error creating migrator: %v", err)
	}
	ctx := context.Background()

	err = sm.MigrateSchema(ctx)
	if err != nil {
		t.Fatalf("error migrating schema: %v", err)
	}

}
