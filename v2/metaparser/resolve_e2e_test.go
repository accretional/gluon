package metaparser_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/accretional/gluon/v2/metaparser"
	pb "github.com/accretional/gluon/v2/pb"
)

// TestResolveAndComposeE2E drives the two new streaming RPCs over the gRPC
// wire: ResolveDependencies (server-streaming) expands a Starlark build file
// into dependency grammar documents, and EBNF (client-streaming) merges the
// local grammar with those documents — the dependency's real production
// overriding the local placeholder of the same name.
func TestResolveAndComposeE2E(t *testing.T) {
	dir := t.TempDir()
	depLang := filepath.Join(dir, "dep-lang")
	if err := os.MkdirAll(depLang, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depLang, "seam.ebnf"), []byte(`Seam = "real-a" | "real-b" ;`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bzl := filepath.Join(dir, "grammar_deps.bzl")
	body := `GRAMMAR_DEPS = [struct(name = "dep", grammar_srcs = "dep-lang", external_rules = ["Seam"])]` + "\n"
	if err := os.WriteFile(bzl, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	client, teardown := startServer(t)
	defer teardown()
	ctx := context.Background()

	// 1. ResolveDependencies — collect the streamed dependency documents.
	rs, err := client.ResolveDependencies(ctx, &pb.ResolveRequest{BuildFile: bzl})
	if err != nil {
		t.Fatalf("ResolveDependencies: %v", err)
	}
	var depDocs []*pb.DocumentDescriptor
	for {
		doc, err := rs.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("ResolveDependencies Recv: %v", err)
		}
		depDocs = append(depDocs, doc)
	}
	if len(depDocs) != 1 || depDocs[0].GetName() != "dep" {
		t.Fatalf("dependency docs = %+v, want one named dep", depDocs)
	}

	// 2. EBNF — send local grammar (with placeholder Seam) then the dep docs.
	local := metaparser.WrapString(`Root = "<r>" , Seam , "</r>" ; Seam = "placeholder" ;`)
	local.Name = "local"
	es, err := client.EBNF(ctx)
	if err != nil {
		t.Fatalf("EBNF open: %v", err)
	}
	if err := es.Send(local); err != nil {
		t.Fatalf("EBNF Send local: %v", err)
	}
	for _, d := range depDocs {
		if err := es.Send(d); err != nil {
			t.Fatalf("EBNF Send dep: %v", err)
		}
	}
	gd, err := es.CloseAndRecv()
	if err != nil {
		t.Fatalf("EBNF CloseAndRecv: %v", err)
	}

	// 3. The dependency's Seam overrode the local placeholder; root stays first.
	if gd.GetRules()[0].GetName() != "Root" {
		t.Errorf("root not first: %v", ruleNames(gd))
	}
	seam := ruleByName(gd, "Seam")
	if seam == nil {
		t.Fatal("Seam rule missing after compose")
	}
	terms := terminalsOf(seam)
	if !contains(terms, "real-a") || !contains(terms, "real-b") {
		t.Errorf("Seam not overridden by dependency over the wire: terminals=%v", terms)
	}
	if contains(terms, "placeholder") {
		t.Errorf("Seam still carries local placeholder over the wire: terminals=%v", terms)
	}
}
