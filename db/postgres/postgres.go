package postgres

import (
	"database/sql"
	"fmt"

	"github.com/crizah/Worm/errors"
)

type Postgres struct {
	db *sql.DB
}

const POSTGRES_ERROR = "Postgres: %s, Error %w"

func New(conn string) (*Postgres, error) {
	db, err := sql.Open("postgres", conn)
	if err != nil {
		return nil, fmt.Errorf(POSTGRES_ERROR, errors.CONNECTION_ERROR, err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf(POSTGRES_ERROR, errors.CONNECTION_ERROR, err)
	}
	backend := &Postgres{db: db}
	err = backend.migrate()
	if err != nil {
		return nil, err
	}
	return backend, nil
}

func (pg *Postgres) migrate() error {
	// add the worm table into the connected db
	statement := "xyz" // get postgres.sql here
	_, err := pg.db.Exec(statement)
	if err != nil {
		return fmt.Errorf(POSTGRES_ERROR, errors.MIGRATION_ERROR, err)
	}
	return nil

}

func (pg *Postgres) setVersion(version string, description string) error {
	// set worm_version
	statement := "xyz$1, $2"
	_, err := pg.db.Exec(statement, version, description)
	if err != nil {
		return fmt.Errorf(POSTGRES_ERROR, errors.MIGRATION_ERROR, err)
	}
	return nil

}
