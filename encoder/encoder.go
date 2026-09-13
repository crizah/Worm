package encoder

import "github.com/crizah/Worm/schema"

// encodes the normalised data types into native db types
type Encoder interface {
	Encode(t schema.Type, v any) (any, error)
}
