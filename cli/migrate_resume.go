package cli

import (
	"database/sql"
	"fmt"

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

	var stage int
	var err error
	q := `SELECT stage from capture_stage`
	err = stateDb.QueryRowContext(ctx, q).Scan(&stage)
	if err != nil {
		return fmt.Errorf("error querying state db")
	}
	if stage != 2 || stage != 3 {
		return fmt.Errorf("wrong command bro. run migrate and then data %d", stage)
	}

	// persist state here itself
	err = persistStage(ctx, stateDb, 3)
	if err != nil {
		fmt.Errorf("persisting stage: %s", err.Error())
	}

	// make the public connection,
	// read from the state db the things needed to build the migrater struct (table order, index columns etc)
	// rebuild everytime

	return nil
}
