package codegen

import (
	"testing"
)

func TestTransformAlreadyGRPC(t *testing.T) {
	// Interface already follows gRPC convention — should pass through unchanged
	src := `package example

import "context"

type SearchRequest struct {
	Query string
}

type SearchResponse struct {
	Items []string
}

type Searcher interface {
	Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error)
}
`
	info, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}

	result := TransformInterface(info.Interfaces[0], info.Structs)
	if len(result.Messages) != 0 {
		t.Errorf("expected no generated messages for already-gRPC interface, got %d", len(result.Messages))
	}
	if result.Interface.Name != "Searcher" {
		t.Errorf("interface name = %q", result.Interface.Name)
	}
	if len(result.Interface.Methods) != 1 {
		t.Fatalf("methods = %d", len(result.Interface.Methods))
	}
	m := result.Interface.Methods[0]
	if !m.HasContext || !m.HasError {
		t.Error("transformed method should have context and error")
	}
}

func TestTransformMultipleParams(t *testing.T) {
	src := `package example

type Store interface {
	Set(key string, value string) error
}
`
	info, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}

	result := TransformInterface(info.Interfaces[0], nil)

	// Should generate SetRequest with Key and Value fields
	if len(result.Messages) == 0 {
		t.Fatal("expected generated messages")
	}

	foundReq := false
	for _, msg := range result.Messages {
		if msg.Name == "SetRequest" {
			foundReq = true
			if len(msg.Fields) != 2 {
				t.Errorf("SetRequest fields = %d, want 2", len(msg.Fields))
			}
			if msg.Fields[0].Name != "Key" {
				t.Errorf("field 0 = %q, want Key", msg.Fields[0].Name)
			}
			if msg.Fields[1].Name != "Value" {
				t.Errorf("field 1 = %q, want Value", msg.Fields[1].Name)
			}
		}
	}
	if !foundReq {
		t.Error("SetRequest not found in messages")
	}
}

func TestTransformNoParams(t *testing.T) {
	src := `package example

type Pinger interface {
	Ping() error
}
`
	info, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}

	result := TransformInterface(info.Interfaces[0], nil)

	// Should generate Nothing for both request and response
	m := result.Interface.Methods[0]
	if !m.HasContext {
		t.Error("transformed Ping should have context")
	}
	if !m.HasError {
		t.Error("transformed Ping should have error")
	}
}

func TestTransformMultipleResults(t *testing.T) {
	src := `package example

import "context"

type Fetcher interface {
	Fetch(ctx context.Context, url string) (string, int, error)
}
`
	info, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}

	result := TransformInterface(info.Interfaces[0], nil)

	// Should generate FetchRequest (for url param) and FetchResponse (for string, int results)
	var foundReq, foundResp bool
	for _, msg := range result.Messages {
		switch msg.Name {
		case "FetchRequest":
			foundReq = true
			if len(msg.Fields) != 1 {
				t.Errorf("FetchRequest fields = %d", len(msg.Fields))
			}
		case "FetchResponse":
			foundResp = true
			if len(msg.Fields) != 2 {
				t.Errorf("FetchResponse fields = %d", len(msg.Fields))
			}
		}
	}
	if !foundReq {
		t.Error("FetchRequest not generated")
	}
	if !foundResp {
		t.Error("FetchResponse not generated")
	}
}

func TestTransformPreservesExistingTypes(t *testing.T) {
	src := `package example

import "context"

type GetRequest struct {
	ID string
}

type GetResponse struct {
	Name string
}

type Getter interface {
	Get(ctx context.Context, req *GetRequest) (*GetResponse, error)
	List() error
}
`
	info, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}

	result := TransformInterface(info.Interfaces[0], info.Structs)

	// Get should pass through (already gRPC-compatible)
	// List should get Nothing generated for request and response
	// But Nothing should only appear once even though it's used for both
	if result.Interface.Methods[0].Name != "Get" {
		t.Error("first method should be Get")
	}
	if result.Interface.Methods[1].Name != "List" {
		t.Error("second method should be List")
	}
}

func TestGenerateMessageDecls(t *testing.T) {
	messages := []StructInfo{
		{
			Name: "FooRequest",
			Fields: []FieldInfo{
				{Name: "Name", TypeStr: "string"},
			},
		},
		{
			Name: "FooResponse",
		},
	}

	// Should use TypeExpr, but we need real exprs for this to work.
	// Let's test with a real parsed source.
	src := `package example

type Foo interface {
	Do(name string, count int) (string, error)
}
`
	info, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}

	result := TransformInterface(info.Interfaces[0], nil)
	decls := GenerateMessageDecls(result.Messages)
	if len(decls) == 0 {
		t.Error("expected generated declarations")
	}

	// Verify we can format them
	for _, d := range decls {
		if d == nil {
			t.Error("nil declaration")
		}
	}

	_ = messages // used above for documentation
}
