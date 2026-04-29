package lexkit

import (
	"strings"
	"testing"
)

// TestParseASTRequiresFullInput locks in the invariant that ParseAST
// errors when the start rule consumes only a prefix of the input.
// Trailing content past the last terminal is rejected — partial-parse
// callers should write a different grammar, not silently succeed.
func TestParseASTRequiresFullInput(t *testing.T) {
	const ebnf = `
		ipv4_address = octet , "." , octet , "." , octet , "." , octet ;
		octet = digit | digit , digit | digit , digit , digit ;
		digit = "0" | "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9" ;
	`
	gd, err := Parse(ebnf, EBNFLex())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"clean", "1.2.3.4", false},
		{"trailing junk", "1.2.3.4junk", true},
		{"trailing dot", "1.2.3.4.", true},
		{"prefix only", "1.2.3", true},
		{"trailing whitespace", "1.2.3.4 ", false}, // syntactic-mode WS skip after the last terminal is fine
		// Trailing comment markers are NOT silently consumed —
		// comments are an EBNF-source convention; treating them as
		// trailing-skip in user input would silently swallow
		// malformed input, which is what fuzzing turned up.
		{"trailing ebnf comment open", "1.2.3.4(*", true},
		{"trailing c block comment", "1.2.3.4/*", true},
		{"trailing line comment", "1.2.3.4//", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseAST(tc.input, "ipv4", "ipv4_address", gd)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseAST(%q) err = %v, wantErr=%v", tc.input, err, tc.wantErr)
			}
			if tc.wantErr && err != nil && !strings.Contains(err.Error(), "unconsumed") &&
				!strings.Contains(err.Error(), "did not match") {
				t.Fatalf("ParseAST(%q) err = %v, want 'unconsumed' or 'did not match'", tc.input, err)
			}
		})
	}
}
