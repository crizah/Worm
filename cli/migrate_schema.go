package cli

import (
	"database/sql"
	"fmt"

	emitter "github.com/crizah/Worm/emmit"
	"github.com/crizah/Worm/inspect"
	"github.com/crizah/Worm/querier"
	"github.com/crizah/Worm/schema"
	schemamigrator "github.com/crizah/Worm/schema-migrator"
	"github.com/spf13/cobra"
)

var migrateSchemaCmd = &cobra.Command{
	Use:   "migrate-schema",
	Short: "Migrate schema from the source db to the target db",
	Long:  `migrates your schema`,
	RunE:  runMigrateSchema,
}

func init() {
	rootCmd.AddCommand(migrateSchemaCmd)

	// TODO: flags and then add them to context:
	// migrateSchemaCmd.Flags().StringVar(&sourceConnStr, "source", "", "source db connection string")
	// migrateSchemaCmd.Flags().StringVar(&targetConnStr, "target", "", "target db connection string")
}

func runMigrateSchema(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	stateDb, ok := ctx.Value(stateDBKey).(*sql.DB)
	if !ok {
		return fmt.Errorf("state db missing from context")
	}
	sourceDb, ok := ctx.Value(sourceDBKey).(*sql.DB)
	if !ok {
		return fmt.Errorf("source db missing from context")
	}
	targetDB, ok := ctx.Value(targetDBKey).(*sql.DB)
	if !ok {
		return fmt.Errorf("target db missing from context")
	}

	sourceDialect, ok := ctx.Value(sourceDialectKey).(schema.Dialect)
	if !ok {
		return fmt.Errorf("source dialect missing from context")
	}

	targetDialect, ok := ctx.Value(targetDialectKey).(schema.Dialect)
	if !ok {
		return fmt.Errorf("target dialect missing from context")
	}

	// create the state db tables
	_, err := stateDb.Exec(`
		CREATE TABLE IF NOT EXISTS capture_snapshot_state (
			slot_name   TEXT PRIMARY KEY,
			snapshot_id TEXT NOT NULL,
			lsn         TEXT NOT NULL,
			created_at  TEXT NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("error creating state db table: %s", err.Error())
	}

	// create the state tracking table for batches
	// status tracks the status of the table itself, pending, in progress, done
	// index_name contaisn the index we are using for this table
	// index_columns contain comma seperated column names we use as int his index
	// last_index_values stores comma seperated values in the form of a json blob {columnName1: last_value, columnName2: last_value}
	// rows_done stores how many rows are finished for the table and total_rows contains total rows to do
	// updated_at tracks last update on this table row
	_, err = stateDb.Exec(`
		CREATE TABLE IF NOT EXISTS capture_batch_state (
		    table_name TEXT PRIMARY KEY,
			status     TEXT,
			index_columns TEXT,
			last_index_values    TEXT,
			index_name TEXT,
			rows_done INTEGER,
			total_rows INTEGER,
			updated_at TIMESTAMP
		)
	`)

	if err != nil {
		return fmt.Errorf("error creating state db table: %s", err.Error())
	}

	// table order + index names it uses for data and resume to read back from
	_, err = stateDb.Exec(`
		CREATE TABLE IF NOT EXISTS capture_plan_tables (
			table_name      TEXT PRIMARY KEY,
			ordinal         INTEGER NOT NULL,
			index_name      TEXT NOT NULL,
			index_columns   TEXT NOT NULL,
			index_is_unique INTEGER NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("error creating state db table: %s", err.Error())
	}

	// column name to column type for decoder encoder in data migrater to do. plan.go resolves this
	_, err = stateDb.Exec(`
		CREATE TABLE IF NOT EXISTS capture_plan_columns (
			table_name  TEXT NOT NULL,
			column_name TEXT NOT NULL,
			type_kind   TEXT NOT NULL,
			enum_name   TEXT,
			enum_values TEXT,
			PRIMARY KEY (table_name, column_name)
		)
	`)
	if err != nil {
		return fmt.Errorf("error creating state db table: %s", err.Error())
	}

	// captures the stage name we are at so we can ensure a schema-migrater->data-migrater->resume-data flow
	// delete this db is u want to start from scratch (i.e migrate new schema)
	_, err = stateDb.Exec(`
		CREATE TABLE IF NOT EXISTS capture_stage (
		    id INTEGER PRIMARY KEY,
			stage INTEGER
		)
	`)
	if err != nil {
		return fmt.Errorf("error creating state db table: %s", err.Error())
	}

	var sch *schema.Schema
	var inspecter inspect.Inspector

	// querier -> inspecter -> emitter -> schema-migrator flow

	switch sourceDialect {
	case schema.PostgresDialect:
		// querier
		q, err := querier.NewPQuerier(sourceDb)
		if err != nil {
			return fmt.Errorf("connecting: %s", err.Error())
		}
		// inspecter
		inspecter = inspect.NewPInspector(q)

	case schema.SqliteDialect:
		// TODO: sqlite, for later
	}

	// TODO: move the running of querier into querier??? why tf is it in inspecter???
	sch, err = inspecter.Inspect(ctx) // runs the querier, adds shit to sc
	if err != nil {

		return fmt.Errorf("inspect: %s", err.Error())
	}

	// migration file
	dir := "./.data/"
	fileName := fmt.Sprintf("%s-migration.sql", sch.DbName)

	// emitter is interface independent, and so is schema migrater
	emm := emitter.NewSqlEmitter(sch)

	_, err = emm.Emitt(ctx, dir, fileName)
	if err != nil {
		return fmt.Errorf("error emitting %s", err.Error())
	}
	sm, err := schemamigrator.NewSchemaMigrator(targetDB, targetDialect, dir+fileName)
	if err != nil {
		return fmt.Errorf("error creating schema migrater %s", err.Error())
	}
	err = sm.MigrateSchema(ctx)
	if err != nil {
		return fmt.Errorf("error migrating schema %s", err.Error())
	}

	err = persistStage(ctx, stateDb, 1)
	if err != nil {
		return fmt.Errorf("persisting stage: %s", err.Error())
	}

	// schema migration done yaya
	fmt.Print("schema migration done")

	if err := persistPlanTables(ctx, stateDb, sch.Tables); err != nil {
		return fmt.Errorf("persisting plan tables: %s", err.Error())
	}
	if err := persistPlanColumns(ctx, stateDb, sch.Tables); err != nil {
		return fmt.Errorf("persisting plan columns: %s", err.Error())
	}

	fmt.Print("saved to state db")

	return nil
}
