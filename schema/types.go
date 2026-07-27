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
		Val    string
		Values *[]string // enum values

	}
)

func (t *BoolType) t()
func (t *IntegerType) t()
func (t *UUIDType) t()
func (t *TimeType) t()
func (t *EnumType) t()
