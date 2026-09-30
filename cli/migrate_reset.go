package cli

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/crizah/Worm/schema"
	schemamigrator "github.com/crizah/Worm/schema-migrator"
	"github.com/spf13/cobra"
)

var migrateResetCmd = &cobra.Command{
	Use:   "migrate-reset",
	Short: "tears down worm's own state so a fresh migrate-schema can run again",
	Long: `Drops worm's replication slot(s) and publication on the source
postgres db, wipes the target db's tables, and clears worm's own
bookkeeping tables in the state db. Run this before running migrate-schema
again against the same source/target - migrate-schema/migrate-data will
fail (or silently stop replicating) on a second run without it: replication
slots are never dropped on their own, CREATE PUBLICATION isn't schema-scoped
so DROP SCHEMA doesn't touch it, and capture_plan_tables/capture_plan_columns/
capture_batch_state all have table_name as their primary key.`,
	RunE: runMigrateReset,
}

func init() {
	rootCmd.AddCommand(migrateResetCmd)
}

func runMigrateReset(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	stateDb, ok := ctx.Value(stateDBKey).(*sql.DB)
	if !ok {
		return fmt.Errorf("state db missing from context")
	}
	sourceDb, ok := ctx.Value(sourceDBKey).(*sql.DB)
	if !ok {
		return fmt.Errorf("source db missing from context")
	}
	sourceDialect, ok := ctx.Value(sourceDialectKey).(schema.Dialect)
	if !ok {
		return fmt.Errorf("source dialect missing from context")
	}
	targetDB, ok := ctx.Value(targetDBKey).(*sql.DB)
	if !ok {
		return fmt.Errorf("target db missing from context")
	}
	targetDialect, ok := ctx.Value(targetDialectKey).(schema.Dialect)
	if !ok {
		return fmt.Errorf("target dialect missing from context")
	}

	if sourceDialect == schema.PostgresDialect {
		if err := resetPostgresSource(ctx, sourceDb, stateDb); err != nil {
			return fmt.Errorf("resetting source: %w", err)
		}
	}

	sm, err := schemamigrator.NewSchemaMigrator(targetDB, targetDialect, "")
	if err != nil {
		return fmt.Errorf("creating schema migrator: %w", err)
	}
	if err := sm.Reset(ctx); err != nil {
		return fmt.Errorf("wiping target: %w", err)
	}

	if err := clearStateDb(ctx, stateDb); err != nil {
		return fmt.Errorf("clearing state db: %w", err)
	}

	fmt.Print("reset done - run migrate-schema to start fresh")
	return nil
}

func resetPostgresSource(ctx context.Context, sourceDb *sql.DB, stateDb *sql.DB) error {
	slotName, err := recordedSlotName(ctx, stateDb)
	if err != nil {
		return fmt.Errorf("reading recorded slot: %w", err)
	}
	if slotName != "" {
		if err := dropReplicationSlot(ctx, sourceDb, slotName); err != nil {
			return err
		}
	}

	if _, err := sourceDb.ExecContext(ctx, `DROP PUBLICATION IF EXISTS worm_pub`); err != nil {
		return fmt.Errorf("dropping publication: %w", err)
	}
	return nil
}

func recordedSlotName(ctx context.Context, stateDb *sql.DB) (string, error) {
	exists, err := tableExists(ctx, stateDb, "capture_snapshot_state")
	if err != nil || !exists {
		return "", err // fresh state db, migrate-schema never ran - nothing to drop
	}

	var slotName string
	err = stateDb.QueryRowContext(ctx, `SELECT slot_name FROM capture_snapshot_state`).Scan(&slotName)
	if err == sql.ErrNoRows {
		return "", nil // table exists but CreateSnapshot never ran
	}
	if err != nil {
		return "", err
	}
	return slotName, nil
}

func dropReplicationSlot(ctx context.Context, sourceDb *sql.DB, name string) error {
	if _, err := sourceDb.ExecContext(ctx, `SELECT pg_drop_replication_slot($1)`, name); err != nil {
		return fmt.Errorf("dropping slot %s (still active? stop migrate-resume/migrate-data first): %w", name, err)
	}
	return nil
}

func clearStateDb(ctx context.Context, stateDb *sql.DB) error {
	tables := []string{
		"capture_snapshot_state",
		"capture_batch_state",
		"capture_plan_tables",
		"capture_plan_columns",
		"capture_stage",
	}
	for _, t := range tables {
		exists, err := tableExists(ctx, stateDb, t)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if _, err := stateDb.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", t)); err != nil {
			return fmt.Errorf("clearing %s: %w", t, err)
		}
	}
	return nil
}

func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var n string
	err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
