package cli

import (
	"database/sql"
	"fmt"

	datamigrator "github.com/crizah/Worm/data-migrator"
	datawriter "github.com/crizah/Worm/data-writer"
	"github.com/crizah/Worm/schema"
	"github.com/spf13/cobra"
)

var migrateResumeCmd = &cobra.Command{
	Use:   "migrate-resume",
	Short: "Resume a previously interrupted migration using state db",
	Long:  `resumes the data migration from wher it was last left off`,
	RunE:  runMigrateResume,
}

func init() {
	rootCmd.AddCommand(migrateResumeCmd)

	// TODO: flags, e.g.:
	// migrateResumeCmd.Flags().StringVar(&stateDbPath, "state-db", "./.data/state.db", "path to state db")
}

func runMigrateResume(cmd *cobra.Command, args []string) error {
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

	claimed, err := claimStage(ctx, stateDb, []int{2, 3}, 3)
	if err != nil {
		return fmt.Errorf("claiming stage: %w", err)
	}
	if !claimed {
		return fmt.Errorf("wrong command bro. run migrate-schema then migrate-data first")
	}

	// get the shit
	stuff, err := getShit(ctx, stateDb)
	if err != nil {
		return fmt.Errorf("error getting shit: %w", err)
	}

	// rebuild that bitch up, we dont need to make any connectiosn, resume takes care of allat
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
	err = dataMigrater.Resume(ctx)
	if err != nil {
		return fmt.Errorf("error resuming: %w", err)
	}

	fmt.Printf("yayay its in sync. you can resume later on again after a while to sync again")
	return nil
}
