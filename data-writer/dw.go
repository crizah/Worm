package datawriter

import (
	"context"

	"github.com/crizah/Worm/schema"
)

type DW interface {
	Write(ctx context.Context, b *schema.Batch, colMap map[string]schema.Type, indexColumns []string) ([]any, error)
	encode(ctx context.Context, t schema.Type, v any) (any, error)
}
