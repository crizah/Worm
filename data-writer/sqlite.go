package datawriter

import (
	"database/sql"
	"time"

	"github.com/crizah/Worm/schema"
)

type SQLiteDataMigrator struct {
	targetDb *sql.DB
	stateDb  *sql.DB // get the state db from the same place backfill gets its db, the parent functoion where this will all fit together
}

func (sq *SQLiteDataMigrator) Write(b *schema.Batch, colMap map[string]schema.Type) error {
	// start transaction
	for i, c := range b.Columns {
		cType := colMap[b.Table+c]
		encoded, err := sq.encode(cType, b.Rows[i])
		if err != nil {
			return err
		}

		// write to the db
	}
	// write to state db
	// commit target db and state db at the same time, last_val in state db is b[b.size()-1]

	return nil
}

func (sq *SQLiteDataMigrator) encode(t schema.Type, v any) (any, error) {
	switch t.(type) {
	case schema.BoolType:
		if b, _ := v.(bool); b {
			return int64(1), nil
		}
		return int64(0), nil
	case schema.TimeType:
		if tm, ok := v.(time.Time); ok {
			return tm.Format(time.RFC3339), nil
		}
		return v, nil
	default:
		return v, nil // json/uuid/text/enum are already canonical strings
	}
}
