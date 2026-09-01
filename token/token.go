package token

type TokenType string

type Token struct {
	Type    TokenType
	Literal string
}

const (
	// keywords
	CREATE     = "CREATE"
	TABLE      = "TABLE"
	ALTER      = "ALTER"
	ADD        = "ADD"
	DROP       = "DROP"
	COLUMN     = "COLUMN"
	PRIMARY    = "PRIMARY"
	KEY        = "KEY"
	FOREIGN    = "FOREIGN"
	REFERENCES = "REFERENCE"
	NOT        = "NOT"
	NULL       = "NULL"
	DEFAULT    = "DEFAULT"
	UNIQUE     = "UNIQUE"
	CHECK      = "CHECK"
	CONSTRAINT = "CONSTRAINT"
	IF         = "IF"
	EXIST      = "EXIST"
	ON         = "ON"
	CASCADE    = "CASCADE"
	RESTRICT   = "RESTRICT"

	// symbols
	LPAREN    = "("
	RPAREN    = ")"
	COMMA     = ","
	SEMICOLON = ";"

	// ERRORS
	ILLEGAL = "ILLEGAL" // token we have not defined
	EOF     = "EOF"     // signifies end of file

	// IDENTIFIERS
	IDENT = "IDENT" // users, id, username, UUID, Timestampz, etc

	// LITERALS
	INT    = "INT"    // 255
	STRING = "STRING" // 'hello'

)

func NewToken(t TokenType, ch byte) *Token {
	return &Token{
		Type:    t,
		Literal: string(ch),
	}

}
