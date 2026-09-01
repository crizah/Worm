package schema

// make it an interface because value

type Type interface {
	t()
}

type (
	BoolType struct {
		Val string
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
		Name   string
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
