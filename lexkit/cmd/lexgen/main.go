// Command lexgen parses EBNF grammar files using lexkit and generates
// textproto GrammarDescriptor files. It also validates the Go grammar
// against golang.org/x/exp/ebnf as a cross-check.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/exp/ebnf"

	pb "github.com/accretional/gluon/pb"

	"github.com/accretional/gluon/lexkit"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: lexgen <lexkit_dir>\n")
		os.Exit(1)
	}
	dir := os.Args[1]

	type grammar struct {
		name      string
		input     string
		output    string
		lexFn     func() *pb.LexDescriptor
		validator func(string) error
	}

	grammars := []grammar{
		{
			name:   "Go",
			input:  filepath.Join(dir, "go_ebnf.txt"),
			output: filepath.Join(dir, "go_grammar.textproto"),
			lexFn:  lexkit.GoLex,
			validator: func(src string) error {
				return validateGoEBNF(src)
			},
		},
		{
			name:   "Protocol Buffers",
			input:  filepath.Join(dir, "proto_ebnf.txt"),
			output: filepath.Join(dir, "proto_grammar.textproto"),
			lexFn:  lexkit.ProtoLex,
		},
		{
			name:   "EBNF",
			input:  filepath.Join(dir, "ebnf.txt"),
			output: filepath.Join(dir, "ebnf_grammar.textproto"),
			lexFn:  lexkit.EBNFLex,
			validator: func(src string) error {
				return validateSelfDescribing(src)
			},
		},
	}

	exitCode := 0
	for _, g := range grammars {
		fmt.Printf("=== %s ===\n", g.name)

		src, err := os.ReadFile(g.input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  error reading %s: %v\n", g.input, err)
			exitCode = 1
			continue
		}

		lex := g.lexFn()
		gd, err := lexkit.Parse(string(src), lex)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  error parsing %s: %v\n", g.input, err)
			exitCode = 1
			continue
		}

		fmt.Printf("  parsed %d productions\n", len(gd.Productions))
		for i, p := range gd.Productions {
			fmt.Printf("    [%d] %s\n", i+1, p.Name)
		}

		textproto := lexkit.ToTextproto(gd)
		if err := os.WriteFile(g.output, []byte(textproto), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "  error writing %s: %v\n", g.output, err)
			exitCode = 1
			continue
		}
		fmt.Printf("  wrote %s (%d bytes)\n", g.output, len(textproto))

		// Run validator if available
		if g.validator != nil {
			if err := g.validator(string(src)); err != nil {
				fmt.Printf("  validation: FAILED — %v\n", err)
			} else {
				fmt.Printf("  validation: PASSED\n")
			}
		}

		fmt.Println()
	}

	// Self-hosting test: load the EBNF grammar textproto and use its
	// LexDescriptor to re-parse ebnf.txt. This validates that the
	// textproto can serve as its own lexer configuration.
	ebnfTextproto := filepath.Join(dir, "ebnf_grammar.textproto")
	ebnfSource := filepath.Join(dir, "ebnf.txt")

	fmt.Println("=== Self-hosting: ebnf_grammar.textproto → ebnf.txt ===")
	if err := validateSelfHosting(ebnfTextproto, ebnfSource); err != nil {
		fmt.Printf("  FAILED: %v\n", err)
		exitCode = 1
	} else {
		fmt.Println("  PASSED")
	}

	os.Exit(exitCode)
}

// validateSelfHosting loads a GrammarDescriptor textproto, extracts its
// LexDescriptor, and uses it to re-parse the original EBNF source. It
// then verifies the re-parsed productions match the textproto's.
func validateSelfHosting(textprotoPath, sourcePath string) error {
	// Load the grammar textproto
	gd, err := lexkit.LoadGrammar(textprotoPath)
	if err != nil {
		return fmt.Errorf("loading textproto: %w", err)
	}
	if gd.Lex == nil {
		return fmt.Errorf("textproto has no lex descriptor")
	}

	// Read the EBNF source
	src, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("reading source: %w", err)
	}

	// Re-parse using the loaded LexDescriptor
	reparsed, err := lexkit.Parse(string(src), gd.Lex)
	if err != nil {
		return fmt.Errorf("re-parse with loaded lex: %w", err)
	}

	// Compare production counts
	if len(reparsed.Productions) != len(gd.Productions) {
		return fmt.Errorf("production count mismatch: textproto=%d, re-parsed=%d",
			len(gd.Productions), len(reparsed.Productions))
	}

	// Compare production names and raw expressions
	for i, got := range reparsed.Productions {
		want := gd.Productions[i]
		gotRaw := lexkit.TokenToRaw(got.Token)
		wantRaw := lexkit.TokenToRaw(want.Token)
		if got.Name != want.Name {
			return fmt.Errorf("production[%d] name: got %q, want %q", i, got.Name, want.Name)
		}
		if gotRaw != wantRaw {
			return fmt.Errorf("production[%d] %q raw mismatch:\n  got:  %q\n  want: %q",
				i, got.Name, gotRaw, wantRaw)
		}
	}

	fmt.Printf("  re-parsed %d productions from loaded LexDescriptor, all match\n",
		len(reparsed.Productions))
	return nil
}

// validateGoEBNF parses the Go EBNF grammar using golang.org/x/exp/ebnf
// as a cross-validation. The x/exp/ebnf package uses Go's EBNF variant
// (= and . as definition/termination, no commas).
func validateGoEBNF(src string) error {
	// x/exp/ebnf.Parse expects the grammar without // comments.
	lines := strings.Split(src, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		cleaned = append(cleaned, line)
	}
	cleanSrc := strings.Join(cleaned, "\n")

	grammar, err := ebnf.Parse("go_ebnf.txt", strings.NewReader(cleanSrc))
	if err != nil {
		return fmt.Errorf("x/exp/ebnf parse: %w", err)
	}

	if err := ebnf.Verify(grammar, "SourceFile"); err != nil {
		return fmt.Errorf("x/exp/ebnf verify: %w", err)
	}

	return nil
}

// validateSelfDescribing checks that the EBNF-of-EBNF grammar is
// self-consistent: it should parse and produce the expected set of
// meta-productions (Syntax, Production, Expression, etc.).
func validateSelfDescribing(src string) error {
	gd, err := lexkit.Parse(src, lexkit.EBNFLex())
	if err != nil {
		return fmt.Errorf("self-parse failed: %w", err)
	}

	expected := map[string]bool{
		"Syntax": false, "Production": false, "Expression": false,
		"Term": false, "Factor": false, "Group": false,
		"Option": false, "Repetition": false,
	}
	for _, p := range gd.Productions {
		if _, ok := expected[p.Name]; ok {
			expected[p.Name] = true
		}
	}

	var missing []string
	for name, found := range expected {
		if !found {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing expected meta-productions: %v", missing)
	}

	return nil
}
