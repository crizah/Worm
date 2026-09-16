package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	emitter "github.com/crizah/Worm/emmit"
	"github.com/crizah/Worm/inspect"
	"github.com/crizah/Worm/querier"
	"github.com/crizah/Worm/schema"
	schemamigrator "github.com/crizah/Worm/schema-migrator"
	"github.com/joho/godotenv"
)

func main() {
	// rough code of how i want things to fit together
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

	// querier -> inspecter -> emitter -> schema-migrater
	switch sourceDialect {
	case 0:
		// postgres
		// querier
		q, err := querier.NewPQuerier(sourceDbConn) // pings the db
		if err != nil {
			log.Fatalf("connecting: %v", err)
			return
		}

		// inspecter
		pi := inspect.NewPInspector(q)
		// TODO: move the running of querier into querier??? why tf is it in inspecter???
		schema, err = pi.Inspect(ctx) // runs the querier, adds shit to sc
		if err != nil {
			log.Fatalf("inspect: %v", err)
			return
		}
	case 1:
		// sqlite, for later
	}

	// emitter is interface independent, and so is schema migrater
	emm := emitter.NewSqlEmitter(schema)
	dir := "./"
	fileName := fmt.Sprintf("%s-migration.sql", schema.DbName)
	_, err = emm.Emitt(dir, fileName)
	if err != nil {
		log.Fatalf("error emitting %s", err.Error())
		return
	}
	sm, err := schemamigrator.NewSchemaMigrator(targetDbConn, targetDialect, dir+fileName)
	if err != nil {
		log.Fatalf("error creating schema migrater %s", err.Error())
		return
	}
	err = sm.MigrateSchema()
	if err != nil {
		log.Fatalf("error migrating schema %s", err.Error())
	}

	// schema migration done yaya
	fmt.Print("schema migration done")

}
