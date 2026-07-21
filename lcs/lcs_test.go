package lcs

import (
	"testing"

	"github.com/crizah/Worm/token"
)

func TestLCSstring(t *testing.T) {

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

func TestLCSfiles(t *testing.T) {
	tests := []struct {
		a   string
		b   string
		lcs []string
	}{
		{"../token/tests/users.sql", "../token/tests/users_modified.sql", []string{
			"CREATE TABLE IF NOT EXIST users(", "id UUID PRIMARY KEY,",
			");", "CREATE",
		}},
	}

	for i, test := range tests {
		tokenA, err := token.TokeniseFile(test.a)
		if err != nil {
			t.Fatalf("error %s", err.Error())
		}
		tokenB, err := token.TokeniseFile(test.b)
		if err != nil {
			t.Fatalf("error %s", err.Error())
		}

		got := LCSLines(tokenA, tokenB)
		if len(got) != len(test.lcs) {
			t.Fatalf("tests[%d] - length mismatch. expected=%d, got=%d", i, len(test.lcs), len(got))
		}

		for j, str := range got {
			t.Log(str)
			if str != test.lcs[j] {
				t.Fatalf("tests[%d] - mismatch at index %d. expected=%s, got=%s", i, j, test.lcs[j], str)
			}
		}

	}
}
