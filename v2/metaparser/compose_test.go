package metaparser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/accretional/gluon/v2/metaparser"
	pb "github.com/accretional/gluon/v2/pb"
)

// terminalsOf collects every terminal literal in a rule's body (recursing into
// scoper bodies), for asserting which definition survived a merge.
func terminalsOf(rule *pb.RuleDescriptor) []string {
	var out []string
	var walk func(ps []*pb.Production)
	walk = func(ps []*pb.Production) {
		for _, p := range ps {
			switch k := p.GetKind().(type) {
			case *pb.Production_Terminal:
				out = append(out, k.Terminal)
			case *pb.Production_Scoper:
				walk(k.Scoper.GetBody())
			}
		}
	}
	walk(rule.GetExpressions())
	return out
}

func ruleByName(gd *pb.GrammarDescriptor, name string) *pb.RuleDescriptor {
	for _, r := range gd.GetRules() {
		if r.GetName() == name {
			return r
		}
	}
	return nil
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// TestParseEBNFStream_OverrideAndDedupe covers the merge policy: a later
// document's rule overrides an earlier placeholder of the same name, identical
// rules deduplicate, and the first document keeps its root position + names the
// grammar.
func TestParseEBNFStream_OverrideAndDedupe(t *testing.T) {
	local := metaparser.WrapString(`Root = "<x>" , Seam , "</x>" ; Seam = "?" ; digit = "0" ... "9" ;`)
	local.Name = "local"
	dep := metaparser.WrapString(`Seam = "a" | "b" ; digit = "0" ... "9" ;`)
	dep.Name = "dep"

	gd, err := metaparser.ParseEBNFStream([]*pb.DocumentDescriptor{local, dep})
	if err != nil {
		t.Fatalf("ParseEBNFStream: %v", err)
	}

	if gd.GetName() != "local" {
		t.Errorf("merged grammar name = %q, want first document's name \"local\"", gd.GetName())
	}
	if len(gd.GetRules()) == 0 || gd.GetRules()[0].GetName() != "Root" {
		t.Errorf("root rule not first: got %v", ruleNames(gd))
	}

	// digit is defined identically in both — must appear exactly once.
	var digitCount int
	for _, r := range gd.GetRules() {
		if r.GetName() == "digit" {
			digitCount++
		}
	}
	if digitCount != 1 {
		t.Errorf("digit rule count = %d, want 1 (identical dedupe)", digitCount)
	}

	// Seam must be the dependency's definition (a|b), not the local placeholder "?".
	seam := ruleByName(gd, "Seam")
	if seam == nil {
		t.Fatal("Seam rule missing")
	}
	terms := terminalsOf(seam)
	if !contains(terms, "a") || !contains(terms, "b") {
		t.Errorf("Seam not overridden by dependency: terminals=%v (want a,b)", terms)
	}
	if contains(terms, "?") {
		t.Errorf("Seam still has local placeholder terminal %q: terminals=%v", "?", terms)
	}
}

func ruleNames(gd *pb.GrammarDescriptor) []string {
	var out []string
	for _, r := range gd.GetRules() {
		out = append(out, r.GetName())
	}
	return out
}

// TestResolveAndCompose is the end-to-end resolver path: a Starlark build file
// points at a dependency grammar directory; ResolveDependencyDocs streams that
// grammar as a DocumentDescriptor; composing it after a local document yields a
// merged grammar carrying both grammars' rules.
func TestResolveAndCompose(t *testing.T) {
	dir := t.TempDir()
	depLang := filepath.Join(dir, "dep-lang")
	if err := os.MkdirAll(depLang, 0o755); err != nil {
		t.Fatal(err)
	}
	// Two files, to confirm concatenation across the dependency's sources.
	if err := os.WriteFile(filepath.Join(depLang, "a.ebnf"), []byte(`Foo = "foo" ;`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depLang, "b.ebnf"), []byte(`Bar = "bar" ;`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bzl := filepath.Join(dir, "grammar_deps.bzl")
	body := `GRAMMAR_DEPS = [struct(name = "dep", grammar_srcs = "dep-lang", external_rules = ["Foo"])]` + "\n"
	if err := os.WriteFile(bzl, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	depDocs, err := metaparser.ResolveDependencyDocs(bzl)
	if err != nil {
		t.Fatalf("ResolveDependencyDocs: %v", err)
	}
	if len(depDocs) != 1 {
		t.Fatalf("got %d dependency docs, want 1", len(depDocs))
	}
	if depDocs[0].GetName() != "dep" {
		t.Errorf("dep doc name = %q, want dep", depDocs[0].GetName())
	}

	// Compose: local grammar first (root), then the dependency docs.
	local := metaparser.WrapString(`Root = "<r>" , Foo , Bar , "</r>" ;`)
	local.Name = "local"
	docs := append([]*pb.DocumentDescriptor{local}, depDocs...)
	gd, err := metaparser.ParseEBNFStream(docs)
	if err != nil {
		t.Fatalf("ParseEBNFStream: %v", err)
	}
	for _, want := range []string{"Root", "Foo", "Bar"} {
		if ruleByName(gd, want) == nil {
			t.Errorf("merged grammar missing rule %q; have %v", want, ruleNames(gd))
		}
	}
	if gd.GetRules()[0].GetName() != "Root" {
		t.Errorf("root not first after compose: %v", ruleNames(gd))
	}
}
