package main

import (
	"context"
	"database/sql"
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

	stateDb, err := sql.Open("sqlite3", stateDbPath)
	if err != nil {
		log.Fatalf("opening db: %s", err.Error())
		return
	}

	// querier -> inspecter -> emitter -> schema-migrater
	switch sourceDialect {
	case 0:
		// postgres
		// querier
		q, err := querier.NewPQuerier(sourceDbConn) // pings the db
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
	dir := "./.data/"
	fileName := fmt.Sprintf("%s-migration.sql", schema.DbName)
	_, err = emm.Emitt(ctx, dir, fileName)
	if err != nil {
		log.Fatalf("error emitting %s", err.Error())
		return
	}
	sm, err := schemamigrator.NewSchemaMigrator(targetDbConn, targetDialect, dir+fileName)
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

	// ping  db file
	if err := stateDb.Ping(); err != nil {
		log.Fatalf("pinging sqlDb: %s", err.Error())
		return
	}
	switch targetDialect {
	case 0:
	// postgres
	case 1:
		dataWriter, err = datawriter.NewSqliteDW(targetDbConn, stateDb)
		if err != nil {
			log.Fatalf("error making datawriter %s", err.Error())
			return
		}
	}

	switch sourceDialect {
	case 0:
		// postgres
		dataMigrater, err = datamigrator.NewPostgresDM(sourceDbConn, stateDb, schema.Tables, 500, dataWriter)
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
