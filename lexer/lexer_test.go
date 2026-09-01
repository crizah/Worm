package lexer

import (
	"testing"

	"github.com/crizah/Worm/token"
)

func TestLexer(t *testing.T) {
	input := `create TABLE IF NOT EXISTS users(
    id UUID PRIMARY KEY,
    username VARCHAR NOT NULL UNIQUE,
    role VARCHAR DEFAULT 'admin',
    score INT DEFAULT 100,
    meow_kitty INT DEFAULT 100,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    is_active BOOLEAN DEFAULT FALSE
	);`

	tests := []struct {
		expectedToken   token.TokenType
		expectedLiteral string
	}{
		{token.CREATE, "create"},
		{token.TABLE, "TABLE"},
		{token.IF, "IF"},
		{token.NOT, "NOT"},
		{token.EXISTS, "EXISTS"},
		{token.IDENT, "users"},
		{token.LPAREN, "("},
		{token.IDENT, "id"},
		{token.IDENT, "UUID"},
		{token.PRIMARY, "PRIMARY"},
		{token.KEY, "KEY"},
		{token.COMMA, ","},
		{token.IDENT, "username"},
		{token.IDENT, "VARCHAR"},
		{token.NOT, "NOT"},
		{token.NULL, "NULL"},
		{token.UNIQUE, "UNIQUE"},
		{token.COMMA, ","},
		{token.IDENT, "role"},
		{token.IDENT, "VARCHAR"},
		{token.DEFAULT, "DEFAULT"},
		{token.STRING, "admin"},
		{token.COMMA, ","},
		{token.IDENT, "score"},
		{token.IDENT, "INT"},
		{token.DEFAULT, "DEFAULT"},
		{token.INT, "100"},
		{token.COMMA, ","},
		{token.IDENT, "meow_kitty"},
		{token.IDENT, "INT"},
		{token.DEFAULT, "DEFAULT"},
		{token.INT, "100"},
		{token.COMMA, ","},
		{token.IDENT, "created_at"},
		{token.IDENT, "TIMESTAMPTZ"},
		{token.DEFAULT, "DEFAULT"},
		{token.IDENT, "NOW"},
		{token.LPAREN, "("},
		{token.RPAREN, ")"},
		{token.COMMA, ","},
		{token.IDENT, "is_active"},
		{token.IDENT, "BOOLEAN"},
		{token.DEFAULT, "DEFAULT"},
		{token.FALSE, "FALSE"},
		{token.RPAREN, ")"},
		{token.SEMICOLON, ";"},
		{token.EOF, ""},
	}
	l := New(input)

	for i, x := range tests {
		tok := l.NextToken()

		if tok.Type != x.expectedToken {
			t.Fatalf("tests[%d]- tokentype wrong. expected=%q, got=%q", i, x.expectedToken, tok.Type)
		}

		if tok.Literal != x.expectedLiteral {
			t.Fatalf("tests[%d]- literal wrong. expected=%q, got=%q", i, x.expectedLiteral, tok.Literal)
		}
	}

}
