package schema

// make it an interface because value

type Type interface {
	t()
}

type (
	BoolType struct {
		Val string // all of these val fields are stubs and dead, the actual value is stores inside the expression value
	}

	IntegerType struct {
		Val string
	}
	UUIDType struct {
		Val string
	}

	TimeType struct {
		Val string
	}

	EnumType struct {
		Name   string   // these arent dead tho, this stores the enum name
		Values []string // enum values
	}

	TextType struct {
		Val string
	}

	JSONType struct {
		Val string
	}
)

func (t BoolType) t()    {}
func (t IntegerType) t() {}
func (t UUIDType) t()    {}
func (t TimeType) t()    {}
func (t EnumType) t()    {}
func (t TextType) t()    {}
func (t JSONType) t()    {}
