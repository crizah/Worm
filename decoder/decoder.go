package decoder

import "github.com/crizah/Worm/schema"

// decodes the actual data types into canonical types
type ValueDecoder interface {
	Decode(t schema.Type, raw any) (any, error)
}

// this only exists for a reference, the type itself isnt used anywhere
type canonDataType string

const (
	BOOLTYPE   = "BOOL"
	STRINGTYPE = "STRING"
	INTTYPE    = "INT"
	TIMETYPE   = "TIME"
)

var schemaCanonMap = map[schema.Type]canonDataType{
	schema.BoolType{}:    BOOLTYPE,
	schema.JSONType{}:    STRINGTYPE,
	schema.UUIDType{}:    STRINGTYPE,
	schema.TextType{}:    STRINGTYPE,
	schema.EnumType{}:    STRINGTYPE,
	schema.TimeType{}:    TIMETYPE,
	schema.IntegerType{}: INTTYPE,
}
