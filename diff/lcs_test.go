package diff

import (
	"testing"
)

func TestLCS(t *testing.T) {

	tests := []struct {
		start    string
		edited   string
		expected string
	}{
		{"BMOAL", "BLOA", "BOA"},
		{"let inicio = 10", "let inicio = 125", "let inicio = 1"},
		{"xyz", "abc", ""},
		{`CREATE TABLE users (
		id UUID PRIMARY KEY
		);`, `CREATE TABLE users (
		id UUID PRIMARY KEY,
		username VARCHAR NOT NULL UNIQUE
		);`, `CREATE TABLE users (
		id UUID PRIMARY KEY
		);`},
	}

	for i, test := range tests {
		got := LCSstring(test.start, test.edited)
		if got != test.expected {
			t.Fatalf("tests[%d]- wrong. expected=%s, got=%s", i, test.expected, got)
		}
	}

}

func TestLCSModified(t *testing.T) {

}
