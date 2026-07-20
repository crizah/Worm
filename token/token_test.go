package token

import "testing"

func TestTokenString(t *testing.T) {
	tests := []struct {
		input  string
		size   int
		output []string
	}{
		{"hello", 1, []string{"hello"}},
		{`hello
		hi
		how are you today`, 3, []string{"hello", "hi", "how are you today"}},
	}

	for i, test := range tests {
		got := tokeniseString(test.input)

		if len(got) != test.size {
			t.Fatalf("tests[%d]- wrong size. expected=%d, got=%d", i, test.size, len(got))
		}

		for j, str := range got {
			if str != test.output[j] {
				t.Fatalf("tests[%d]- wrong. expected=%s, got=%s", i, test.output[j], str)
			}

		}
	}

}

func TestTokenFile(t *testing.T) {
	tests := []struct {
		input  string
		output []string
	}{
		{"./tests/users.sql", []string{"CREATE TABLE IF NOT EXIST users(",
			"id UUID PRIMARY KEY,", "username VARCHAR NOT NULL UNIQUE", ");", "CREATE"}},
	}

	for i, test := range tests {
		got, err := TokeniseFile(test.input)
		if err != nil {
			t.Fatalf("tests[%d]- error %s", i, err.Error())

		}

		for j, str := range got {
			if str != test.output[j] {
				t.Fatalf("tests[%d]- wrong. expected=%s, got=%s", i, test.output[j], str)
			}
		}

	}
}
