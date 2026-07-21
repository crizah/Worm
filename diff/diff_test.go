package diff

import (
	"testing"

	"github.com/crizah/Worm/lcs"
	"github.com/crizah/Worm/token"
)

func TestDiff(t *testing.T) {
	tests := []struct {
		inputFile  string
		editedFile string
		diff       []DiffOperation
	}{
		{"../token/tests/users.sql", "../token/tests/users_modified.sql", []DiffOperation{
			{Op: KEEP, Literal: "CREATE TABLE IF NOT EXIST users("},
			{Op: KEEP, Literal: "id UUID PRIMARY KEY,"},
			{Op: ADD, Literal: "username VARCHAR NOT NULL UNIQUE,"},
			{Op: ADD, Literal: "phone_number VARCHAR"},
			{Op: DEL, Literal: "username VARCHAR NOT NULL UNIQUE"},
			{Op: KEEP, Literal: ");"},
			{Op: KEEP, Literal: "CREATE"},
		}},
	}

	for i, test := range tests {
		// tokenise the files
		tokenA, err := token.TokeniseFile(test.inputFile)
		if err != nil {
			t.Fatalf("error %s", err.Error())
		}
		tokenB, err := token.TokeniseFile(test.editedFile)
		if err != nil {
			t.Fatalf("error %s", err.Error())
		}

		lcs := lcs.LCSLines(tokenA, tokenB)

		diff := GetDiffTokens(tokenA, tokenB, lcs)

		if len(diff) != len(test.diff) {
			t.Fatalf("tests[%d] - length mismatch. expected=%d, got=%d", i, len(test.diff), len(diff))

		}

		for j, d := range diff {
			if d.Op != test.diff[j].Op {
				t.Fatalf("tests[%d] - mismatch at index %d. expected=%s , got=%s", i, j, test.diff[j].Op, d.Op)
			}
			if d.Literal != test.diff[j].Literal {
				t.Fatalf("tests[%d] - mismatch at index %d. expected=%s , got=%s", i, j, test.diff[j].Literal, d.Literal)

			}
		}

	}

}
