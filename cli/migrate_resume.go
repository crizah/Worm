package cli

import (
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
	_ = ctx // TODO: read capture_batch_state / capture_snapshot_state and resume from there

	// TODO: migrate-resume logic goes here.
	// make the public connection,
	// read from the state db the things needed to build the migrater struct (table order, index columns etc)
	// rebuild everytime

	return nil
}
