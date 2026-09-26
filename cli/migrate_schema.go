package cli

import (
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

	// TODO: flags, e.g.:
	// migrateSchemaCmd.Flags().StringVar(&sourceConnStr, "source", "", "source db connection string")
	// migrateSchemaCmd.Flags().StringVar(&targetConnStr, "target", "", "target db connection string")
}

func runMigrateSchema(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	_ = ctx // TODO: pass into querier / inspecter / emitter / schema-migrator calls

	// TODO: migrate-schema logic goes here (this is the
	// querier -> inspecter -> emitter -> schema-migrator flow
	// currently sketched out in main.go).

	return nil
}
