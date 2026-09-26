package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/crizah/Worm/schema"
	"github.com/crizah/Worm/utils"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "worm",
	Short: "Worm migrates schema and data between databases",
	Long:  `what it said above`,

	// before the start of every command, load up the env variables and open the state db
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		godotenv.Load()
		sourceDbType := os.Getenv("SOURCE_DB")
		sourceDialect, err := schema.ResolveEnum(sourceDbType)
		if err != nil {
			return fmt.Errorf("source db dialect %s", err.Error())
		}

		targetDbType := os.Getenv("TARGET_DB")
		targetDialect, err := schema.ResolveEnum(targetDbType)
		if err != nil {
			return fmt.Errorf("target db dialect %s", err.Error())
		}
		sourceDbConn := os.Getenv("SOURCE_CONN_STR")
		if sourceDbConn == "" {
			log.Fatalf("empty connection string for source db")
			return fmt.Errorf("empty connection string for source db")
		}

		sConnD := schema.ResolveDialect(sourceDbConn)
		if sConnD != sourceDialect {

			return fmt.Errorf("type mismatch, source db of type %d, conn str of type %d", sourceDialect, sConnD)
		}
		targetDbConn := os.Getenv("TARGET_CONN_STR")

		// dont let target db be empty even if its sqlite
		tConnD := schema.ResolveDialect(targetDbConn) // if its sqlite, it should have the entire local path
		if tConnD != targetDialect {

			return fmt.Errorf("type mismatch, target db of type %d, conn str of type %d", targetDialect, tConnD)
		}

		if tConnD == 1 {
			// sqlite, check the validity of the connection path
			file := filepath.Base(targetDbConn) // "xxx.db"

			// we didnt check for .db in resolve dialect
			if filepath.Ext(file) != ".db" {
				return fmt.Errorf("connection string needs to be a .db file")
			}
		}

		// create/open the state db at the start of every command, but do the actual create table during schema migration
		stateDbPath := "./.data/state.db"
		if _, err := os.Stat(stateDbPath); os.IsNotExist(err) {
			f, err := os.Create(stateDbPath)
			if err != nil {

				return fmt.Errorf("creating sqlite db file: %s", err.Error())
			}
			f.Close()
		}

		stateDb, err := utils.PingDB(1, stateDbPath) // pass this guy through via context (remove from structs bodies then)
		if err != nil {

			return fmt.Errorf("error reaching state db: %s", err.Error())
		}

		return nil
	},
}

// Execute is the single entrypoint main.go should call. It builds one
// context for this process invocation and hands it to whichever
// subcommand was matched, via cmd.Context() inside that command's RunE.
//
// signal.NotifyContext wires SIGINT/SIGTERM into ctx.Done() instead of
// letting the runtime kill the process outright, so long-running work
// gets a chance to notice cancellation and unwind. That only works for
// code that actually checks ctx.Done() / uses the ...Context(ctx, ...)
// variants of your DB calls — see the notes left in each command stub.
func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
