package inspect

import (
	"context"

	"github.com/crizah/Worm/schema"
)

type Inspector interface {
	Inspect(ctx context.Context) (*schema.Schema, error)
}
