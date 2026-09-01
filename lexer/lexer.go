package lexer

import (
	"strings"

	"github.com/crizah/Worm/token"
)

// omitts tokens
type Lexer struct {
	input string
	curr  int  // prev char
	read  int  // peek char
	ch    byte // current char being processed
}

func New(input string) *Lexer {
	l := &Lexer{
		input: input,
	}
	l.readChar()
	return l

}

func (l *Lexer) NextToken() *token.Token {
	// skips whitespace and other bullshit to give up the next actual token
	tok := &token.Token{}
	switch l.ch {
	case '(':
		tok = token.NewToken(token.LPAREN, l.ch)
	case ')':
		tok = token.NewToken(token.RPAREN, l.ch)
	case ',':
		tok = token.NewToken(token.COMMA, l.ch)
	case ';':
		tok = token.NewToken(token.SEMICOLON, l.ch)
	case 0:
		tok.Literal = ""
		tok.Type = token.EOF
	case '\'':
		// read until closing ' and then string
		s := ""
		l.readChar()
		for l.ch != '\'' {
			s += string(l.ch)
			l.readChar()
		}
		tok.Literal = s
		tok.Type = token.STRING
	default:
		// if its letter, read the entire word, try to match with one of our keywords
		// if is digits -> INT
		//
		if isLetter(l.ch) { // can be keyword or identifier

			tok.Literal = l.readIdentifier() // function reads the string until non letter and then returns its literal
			tok.Type = l.getType(tok.Literal)
			return tok

		}
		if isWhitespace(l.ch) {
			// skip all whitespace
			for isWhitespace(l.ch) {
				l.readChar()
			}
			return l.NextToken()
		}

		if isNumber(l.ch) {
			tok.Literal = l.readNumber()
			tok.Type = token.INT
			return tok
		} else {
			// couldnt be anything else
			tok = token.NewToken(token.ILLEGAL, l.ch)
			return tok
		}

	}
	l.readChar()
	return tok

}

func (l *Lexer) readNumber() string {
	s := ""
	for isNumber(l.ch) {
		s += string(l.ch)
		l.readChar()
	}
	return s

}
func isNumber(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func (l *Lexer) readChar() {
	// this is to update pointers of the lexer

	if l.read >= len(l.input) {
		l.ch = 0 // end of input
	} else {
		l.ch = l.input[l.read]
	}
	l.curr = l.read
	l.read++
}

func (l *Lexer) readIdentifier() string {
	// moves the actual positions of the lexer as well
	s := ""
	for isLetter(l.ch) {
		s += string(l.ch) // append the character to the string
		l.readChar()      // move to the next character
	}

	// if the next is whitespace, reached the end of the word
	if isWhitespace(l.ch) {
		return s

	}

	// if its not a whitespace, weird syntax, could be number but return error i think
	return s

}
func isWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func isLetter(ch byte) bool {
	return ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z')
}

var m = map[string]token.TokenType{
	"CREATE":     token.CREATE,
	"TABLE":      token.TABLE,
	"ALTER":      token.ALTER,
	"ADD":        token.ADD,
	"DROP":       token.DROP,
	"COLUMN":     token.COLUMN,
	"PRIMARY":    token.PRIMARY,
	"KEY":        token.KEY,
	"FOREIGN":    token.FOREIGN,
	"REFERENCE":  token.REFERENCES,
	"NOT":        token.NOT,
	"NULL":       token.NULL,
	"DEFAULT":    token.DEFAULT,
	"UNIQUE":     token.UNIQUE,
	"CHECK":      token.CHECK,
	"CONSTRAINT": token.CONSTRAINT,
	"IF":         token.IF,
	"EXIST":      token.EXIST,
	"ON":         token.ON,
	"CASCADE":    token.CASCADE,
	"RESTRICT":   token.RESTRICT,
}

func (l *Lexer) getType(lit string) token.TokenType {
	// uppercase the literal and see of it matches our keywords
	// if yes, return that keyword
	// otheerwise return ident
	upper := strings.ToUpper(lit)
	if tok, ok := m[upper]; ok {
		return tok
	}

	return token.IDENT

}
