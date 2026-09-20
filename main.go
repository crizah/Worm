package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	datamigrator "github.com/crizah/Worm/data-migrator"
	datawriter "github.com/crizah/Worm/data-writer"
	emitter "github.com/crizah/Worm/emmit"
	"github.com/crizah/Worm/inspect"
	"github.com/crizah/Worm/querier"
	"github.com/crizah/Worm/schema"
	schemamigrator "github.com/crizah/Worm/schema-migrator"
	"github.com/crizah/Worm/utils"
	"github.com/joho/godotenv"
)

func main() {
	// rough code of how i want things to fit together
	// TODO: make this into a command line tool lol
	// get these values from a yaml file later on
	godotenv.Load()
	sourceDbType := os.Getenv("SOURCE_DB")
	sourceDialect, err := schema.ResolveEnum(sourceDbType)
	if err != nil {
		log.Fatalf("source db dialect %s", err.Error())
		return
	}

	targetDbType := os.Getenv("TARGET_DB")
	targetDialect, err := schema.ResolveEnum(targetDbType)
	if err != nil {
		log.Fatalf("target db dialect %s", err.Error())
		return
	}
	sourceDbConn := os.Getenv("SOURCE_CONN_STR")
	if sourceDbConn == "" {
		log.Fatalf("empty connection string for source db")
		return
	}

	sConnD := schema.ResolveDialect(sourceDbConn)
	if sConnD != sourceDialect {
		log.Fatalf("type mismatch, source db of type %d, conn str of type %d", sourceDialect, sConnD)
		return
	}
	targetDbConn := os.Getenv("TARGET_CONN_STR")

	// dont let target db be empty even if its sqlite
	tConnD := schema.ResolveDialect(targetDbConn) // if its sqlite, it should have the entire local path
	if tConnD != targetDialect {
		log.Fatalf("type mismatch, target db of type %d, conn str of type %d", targetDialect, tConnD)
		return
	}

	if tConnD == 1 {
		// sqlite, check the validity of the connection path
		file := filepath.Base(targetDbConn) // "xxx.db"

		// we didnt check for .db in resolve dialect
		if filepath.Ext(file) != ".db" {
			log.Fatalf("connection string needs to be a .db file")
		}
	}
	ctx := context.Background()
	var schema *schema.Schema
	var inspecter inspect.Inspector

	stateDbPath := "./.data/state.db"
	// TODO: setup state db here itself create state db
	// setup and ping sqlite state db
	// if db file doesnt exist, make one with that fileName
	if _, err := os.Stat(stateDbPath); os.IsNotExist(err) {
		f, err := os.Create(stateDbPath)
		if err != nil {
			log.Fatalf("creating sqlite db file: %s", err.Error())
			return
		}
		f.Close()
	}

	stateDb, err := utils.PingDB(1, stateDbPath)
	if err != nil {
		log.Fatalf("error reaching state db: %s", err.Error())
		return
	}

	// set up the tables in stateDb

	// create the state tracking table for snapshots
	_, err = stateDb.Exec(`
		CREATE TABLE IF NOT EXISTS capture_snapshot_state (
			slot_name   TEXT PRIMARY KEY,
			snapshot_id TEXT NOT NULL,
			lsn         TEXT NOT NULL,
			created_at  TEXT NOT NULL
		)
	`)
	if err != nil {
		log.Fatalf("error creating state db table: %s", err.Error())
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
		log.Fatalf("error creating state db table: %s", err.Error())
	}

	sourceDb, err := utils.PingDB(sourceDialect, sourceDbConn)
	if err != nil {
		log.Fatalf("error reaching source db: %s", err.Error())
		return
	}
	targetDb, err := utils.PingDB(targetDialect, targetDbConn)
	if err != nil {
		log.Fatalf("error reaching target db: %s", err.Error())
		return
	}

	// migration file
	dir := "./.data/"
	fileName := fmt.Sprintf("%s-migration.sql", schema.DbName)

	// querier -> inspecter -> emitter -> schema-migrater
	switch sourceDialect {
	case 0:
		// postgres
		// querier
		q, err := querier.NewPQuerier(sourceDb) // pings the db
		if err != nil {
			log.Fatalf("connecting: %s", err.Error())
			return
		}

		// inspecter
		inspecter = inspect.NewPInspector(q)

	case 1:
		// sqlite, for later
	}

	// TODO: move the running of querier into querier??? why tf is it in inspecter???
	schema, err = inspecter.Inspect(ctx) // runs the querier, adds shit to sc
	if err != nil {
		log.Fatalf("inspect: %s", err.Error())
		return
	}

	// emitter is interface independent, and so is schema migrater
	emm := emitter.NewSqlEmitter(schema)

	_, err = emm.Emitt(ctx, dir, fileName)
	if err != nil {
		log.Fatalf("error emitting %s", err.Error())
		return
	}
	sm, err := schemamigrator.NewSchemaMigrator(targetDb, targetDialect, dir+fileName)
	if err != nil {
		log.Fatalf("error creating schema migrater %s", err.Error())
		return
	}
	err = sm.MigrateSchema(ctx)
	if err != nil {
		log.Fatalf("error migrating schema %s", err.Error())
	}

	// schema migration done yaya
	fmt.Print("schema migration done")

	// data migrater belongs as per source connection type
	// data writer belongs as per target connection type
	var dataMigrater datamigrator.DM
	var dataWriter datawriter.DW

	switch targetDialect {
	case 0:
	// postgres
	case 1:
		dataWriter, err = datawriter.NewSqliteDW(targetDb, stateDb)
		if err != nil {
			log.Fatalf("error making datawriter %s", err.Error())
			return
		}
	}

	switch sourceDialect {
	case 0:
		// postgres
		dataMigrater, err = datamigrator.NewPostgresDM(sourceDb, sourceDbConn, stateDb, schema.Tables, 500, dataWriter)
	case 1:
		// sqlite
	}
	err = dataMigrater.CreateSnapshot(ctx)
	if err != nil {
		log.Fatalf("error creating snapshot %s", err.Error())
		return
	}
	err = dataMigrater.Backfill(ctx)
	if err != nil {
		log.Fatalf("error backfilling data %s", err.Error())
		return
	}

}
