package diff

import (
	"testing"

	"github.com/crizah/Worm/diff"
)

func TestLCS(t *testing.T) {

	tests := []struct {
		start    string
		edited   string
		expected string
	}{
		{"BMOAL", "BLOA", "BOA"},
		{"let inicio = 10", "let inicio = 125", "let inicio = 1"},
	}

	for i, test := range tests {
		got := diff.LCS(test.start, test.edited)
		if got != test.expected {
			t.Fatalf("tests[%d]- wrong. expected=%s, got=%s", i, test.expected, got)
		}
	}

}
