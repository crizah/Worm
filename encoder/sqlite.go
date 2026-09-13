package encoder

import (
	"time"

	"github.com/crizah/Worm/schema"
)

type SQLiteEncoder struct{}

func (SQLiteEncoder) Encode(t schema.Type, v any) (any, error) {
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
