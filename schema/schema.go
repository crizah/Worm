package schema

// defines the universal go structs that each query maps to

type Schema struct {
	DbName string
	Tables []*Table
}
type Table struct {
	Name    string
	Columns []*Column
	PK      *Index
	FKs     []*ForeignKey
	Indexes []*Index
}

type Column struct {
	Name       string
	Type       Type
	IsNullable bool
	IsPK       bool
	IsUnique   bool // NOTE: not used, stub
	Default    Expr
}

type Index struct {
	Name       string
	Columns    []*Column
	IsPK       bool
	IsUnique   bool
	IsPartial  bool
	Predicates []*Predicate
}

type Predicate struct {
	Column      *Column
	ColumnValue Expr
	Operator    OperatorOption
}

type ForeignKey struct {
	Name       string
	RefTable   *Table
	RefColumns []*Column
	Columns    []*Column

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

type OperatorOption string

const (
	EQUALS   OperatorOption = "="
	LEQUAL   OperatorOption = "<="
	GEQUAL   OperatorOption = ">="
	NOTEQUAL OperatorOption = "!="
	LESSER   OperatorOption = "<"
	GREATER  OperatorOption = ">"
)

// Expressions
type Expr interface {
	expr()
}

type RawExpr struct {
	// for defaults which are: "hello", "false", 1 , enums
	// can be integres, boolean, enums etc, let the values always be string tho
	Val     string
	ExpType Type
}

type MethodExpr struct {
	// for defaults that are methods eg: "now()", "gen_random_uuid()""
	Name string // eg: "now"
	Args []Expr // empty, but can have args
}

func (e RawExpr) expr()    {}
func (e MethodExpr) expr() {}
