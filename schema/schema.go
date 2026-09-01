package schema

// defines the go structs we compare againts on a diff

type Schema struct {
	DbName string
	Tables []Table
}
type Table struct {
	Name    string
	Columns []Column
	PK      Index
	FKs     []ForeignKey
	Indexes []*Index
}

type Column struct {
	Name       string
	Type       Type
	IsNullable bool // if column is not null
	IsPK       bool // if column is pk
	Default    Expr
	FKs        []ForeignKey
}

// Index
type Index struct {
	Name    string
	Table   *Table
	Columns []Column
	isPK    bool
}

// PKs

// FKs

type ForeignKey struct {
	RefTable   *Table
	RefColumns []*Column

	OnUpdate ReferenceOption
	OnDelete ReferenceOption
}

type ReferenceOption string

const (
	NoAction   ReferenceOption = "NO ACTION"
	Restrict   ReferenceOption = "RESTRICT"
	Cascade    ReferenceOption = "CASCADE"
	SetNull    ReferenceOption = "SET NULL"
	SetDefault ReferenceOption = "SET DEFAULT"
)

// Expressions
type Expr interface {
	expr()
}

type RawExpr struct {
	// for defaults which are: "hello", "false", 1 , enums
	// can be integres, boolean, enums etc, but value will always be in string
	Val     string
	ExpType Type
}

type MethodExpr struct {
	// for defaults that are methods eg: "now()", "gen_random_uuid()""
	Name string // Now
	Args []Expr // empty, but can have args
}

func (e RawExpr) expr()    {}
func (e MethodExpr) expr() {}
