package utils

import (
	"database/sql"
	"fmt"

	"github.com/crizah/Worm/schema"
)

func PingDB(t schema.Dialect, str string) (*sql.DB, error) {
	// opens and pings the db, returns the db conn
	d := schema.DriverName(t)
	db, err := sql.Open(d, str)
	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}

	// ping the connection string, or the db file
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging db: %w", err)
	}
	return db, nil

}
