package cli

import (
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
	_ = ctx // TODO: pass into data-migrator / data-writer calls

	// TODO: migrate-data logic goes here (CreateSnapshot + Backfill,
	// currently sketched out in main.go).
	// TODO: have a check here for if we have any state db entry, this command shouldnt run, it should be resume instead

	return nil
}
