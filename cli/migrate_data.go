package cli

import (
	"database/sql"
	"fmt"

	datamigrator "github.com/crizah/Worm/data-migrator"
	datawriter "github.com/crizah/Worm/data-writer"
	"github.com/crizah/Worm/schema"
	"github.com/spf13/cobra"
)

var migrateDataCmd = &cobra.Command{
	Use:   "migrate-data",
	Short: "Migrate data from the source db to the target db",
	Long:  `one shot command to migrate the entire data`,
	RunE:  runMigrateData,
}

func init() {
	rootCmd.AddCommand(migrateDataCmd)

	// TODO: flags, e.g.:
	// migrateDataCmd.Flags().StringVar(&sourceConnStr, "source", "", "source db connection string")
	// migrateDataCmd.Flags().StringVar(&targetConnStr, "target", "", "target db connection string")
	// migrateDataCmd.Flags().IntVar(&batchSize, "batch-size", 500, "rows per batch")
}

func runMigrateData(cmd *cobra.Command, args []string) error {
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

	sourceDbConn, ok := ctx.Value(sourceDBConnKey).(string)
	if !ok {
		return fmt.Errorf("source db connection not in context")
	}

	claimed, err := claimStage(ctx, stateDb, []int{1}, 2)
	if err != nil {
		return fmt.Errorf("claiming stage: %s", err.Error())
	}
	if !claimed {
		return fmt.Errorf("wrong command bro. run resume or schema migrater")
	}

	// get the shit
	stuff, err := getShit(ctx, stateDb)
	if err != nil {
		return fmt.Errorf("error getting shit %s", err.Error())
	}

	// data migrater belongs as per source connection type
	// data writer belongs as per target connection type
	var dataMigrater datamigrator.DM
	var dataWriter datawriter.DW

	switch targetDialect {
	case schema.PostgresDialect:
		// TODO: postgres data writer
	case schema.SqliteDialect:
		dataWriter, err = datawriter.NewSqliteDW(targetDB, stateDb)
		if err != nil {
			return fmt.Errorf("error making datawriter %s", err.Error())
		}
	}
	switch sourceDialect {
	case schema.PostgresDialect:
		dataMigrater, err = datamigrator.NewPostgresDM(sourceDb, sourceDbConn, stateDb, stuff.Tables, stuff.ColMap, stuff.IndexMap, 500, dataWriter)
	case schema.SqliteDialect:
		// TODO: sqlite source
	}
	err = dataMigrater.CreateSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("error creating snapshot %s", err.Error())
	}
	err = dataMigrater.Migrate(ctx)
	if err != nil {
		return fmt.Errorf("error migrating data %s", err.Error())
	}

	fmt.Printf("data migration done yayaya")

	return nil
}
