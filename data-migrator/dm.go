package datamigrator

import (
	"context"

	"github.com/crizah/Worm/schema"
)

type DM interface {
	CreateSnapshot(ctx context.Context) error
	Backfill(ctx context.Context) error
	decode(ctx context.Context, t schema.Type, raw any) (any, error) // has to exist
}
