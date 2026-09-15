package datawriter

import "github.com/crizah/Worm/schema"

type DM interface {
	Write(b *schema.Batch, colMap map[string]schema.Type, indexColumns []string) ([]any, error)
	encode(t schema.Type, v any) (any, error)
}
