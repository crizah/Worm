package datamigrator

import (
	"context"
)

type DM interface {
	CreateSnapshot(ctx context.Context) error
	Migrate(ctx context.Context) error
	Resume(ctx context.Context) error
}
