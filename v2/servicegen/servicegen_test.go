package servicegen

import (
	"strings"
	"testing"
)

func TestFormatHTMLSpec(t *testing.T) {
	got, err := Format(Spec{
		Package:   "html",
		Import:    "html.proto",
		GoPackage: "github.com/accretional/proto-html/proto/pb/htmlservice;htmlservicepb",
		Root:      "HtmlDocument",
		RootField: "document",
	})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	for _, want := range []string{
		`syntax = "proto3";`,
		"package html;",
		`import "html.proto";`,
		`option go_package = "github.com/accretional/proto-html/proto/pb/htmlservice;htmlservicepb";`,
		"service HtmlService {",
		"rpc Render(RenderRequest) returns (RenderResponse);",
		"rpc RenderStream(RenderRequest) returns (stream RenderChunk);",
		"rpc Parse(ParseRequest) returns (ParseResponse);",
		"HtmlDocument document = 1;",
		"string html = 1;",
		`import "google/protobuf/any.proto";`,
		"google.protobuf.Any node = 2;",
		"string type = 2;",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n---\n%s", want, got)
		}
	}
}

func TestFormatDefaults(t *testing.T) {
	got, err := Format(Spec{
		Package:   "css",
		Import:    "css.proto",
		GoPackage: "example.com/proto-css/proto/pb/cssservice;cssservicepb",
		Root:      "CssStyleSheet",
	})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	for _, want := range []string{
		"service CssService {", // Title(pkg)+Service
		"CssStyleSheet root = 1;", // RootField default
		"string css = 1;",         // TextField default = package
		"// The CSS text to parse.", // Lang default = upper(package)
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n---\n%s", want, got)
		}
	}
}

func TestFormatRequiredFields(t *testing.T) {
	if _, err := Format(Spec{Package: "html"}); err == nil {
		t.Fatal("expected error for missing required fields")
	}
}
