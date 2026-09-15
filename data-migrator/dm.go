package datamigrator

import "github.com/crizah/Worm/schema"

type DM interface {
	CreateSnapshot() error
	Backfill() error
	decode(t schema.Type, raw any) (any, error)
}
