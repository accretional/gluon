package builddep

import "testing"

const sample = `
GRAMMAR_DEPS = [
    struct(
        name           = "css",
        grammar_srcs   = "../proto-css/lang",
        proto          = "css.proto",
        proto_include  = "../proto-css/proto",
        proto_package  = "css",
        go_package     = "github.com/accretional/proto-css/proto/pb/css",
        external_rules = ["CssStyleSheet", "MediaQueryListType"],
    ),
]
`

func TestParse(t *testing.T) {
	deps, err := Parse("grammar_deps.bzl", []byte(sample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("got %d deps, want 1", len(deps))
	}
	d := deps[0]
	if d.Name != "css" {
		t.Errorf("Name = %q, want css", d.Name)
	}
	if d.Proto != "css.proto" {
		t.Errorf("Proto = %q, want css.proto", d.Proto)
	}
	if d.ProtoPackage != "css" {
		t.Errorf("ProtoPackage = %q, want css", d.ProtoPackage)
	}
	if d.GoPackage != "github.com/accretional/proto-css/proto/pb/css" {
		t.Errorf("GoPackage = %q", d.GoPackage)
	}
	if len(d.ExternalRules) != 2 || d.ExternalRules[0] != "CssStyleSheet" || d.ExternalRules[1] != "MediaQueryListType" {
		t.Errorf("ExternalRules = %v", d.ExternalRules)
	}
}

func TestParse_NoDeps(t *testing.T) {
	if _, err := Parse("x.bzl", []byte("X = 1\n")); err == nil {
		t.Fatal("expected error for missing GRAMMAR_DEPS")
	}
}
