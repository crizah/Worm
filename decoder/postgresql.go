package decoder

import (
	"github.com/crizah/Worm/schema"
)

type PostgresDecoder struct {
}

func (PostgresDecoder) Decode(t schema.Type, raw any) (any, error) {
	switch t.(type) {
	case schema.BoolType:
		return raw, nil // postgres already gives u bool
	case schema.JSONType:
		if b, ok := raw.([]byte); ok {
			return string(b), nil
		}
		return raw, nil
	case schema.UUIDType, schema.TextType, schema.EnumType:
		if b, ok := raw.([]byte); ok {
			return string(b), nil
		}
		return raw, nil
	case schema.TimeType:
		return raw, nil // already time.Time
	default:
		return raw, nil
	}

	// do the actual decoding here
}
