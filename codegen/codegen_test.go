package codegen

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestAnalyzeSource verifies we can parse and analyze Go source.
func TestAnalyzeSource(t *testing.T) {
	src := `package example

type Config struct {
	Host string
	Port int
	Debug bool
}

type Server struct {
	config Config
}

func NewServer(cfg Config) *Server {
	return &Server{config: cfg}
}

func (s *Server) Start() error {
	return nil
}

func (s *Server) Stop() error {
	return nil
}
`
	info, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}

	if info.Name != "example" {
		t.Errorf("package name = %q", info.Name)
	}
	if len(info.Structs) != 2 {
		t.Fatalf("expected 2 structs, got %d", len(info.Structs))
	}

	// Check Config struct
	cfg := info.Structs[0]
	if cfg.Name != "Config" {
		t.Errorf("first struct = %q", cfg.Name)
	}
	if len(cfg.Fields) != 3 {
		t.Errorf("Config fields = %d", len(cfg.Fields))
	}

	// Check Server struct has methods
	srv := info.Structs[1]
	if srv.Name != "Server" {
		t.Errorf("second struct = %q", srv.Name)
	}
	if len(srv.Methods) != 2 {
		t.Errorf("Server methods = %d, want 2", len(srv.Methods))
	}

	// Check function info
	if len(info.Functions) != 3 { // NewServer, Start, Stop
		t.Errorf("functions = %d, want 3", len(info.Functions))
	}

	// Check Start has error result
	for _, f := range info.Functions {
		if f.Name == "Start" {
			if !f.HasError {
				t.Error("Start should have error result")
			}
			if f.RecvType != "*Server" {
				t.Errorf("Start recv = %q", f.RecvType)
			}
		}
	}
}

// TestAnalyzeInterface verifies interface analysis works.
func TestAnalyzeInterface(t *testing.T) {
	src := `package example

import "context"

type Request struct {
	Query string
	Limit int
}

type Response struct {
	Items []string
	Total int
}

type SearchService interface {
	Search(ctx context.Context, req *Request) (*Response, error)
	Count(ctx context.Context) (int, error)
	Ping() error
}
`
	info, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}

	if len(info.Interfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(info.Interfaces))
	}

	iface := info.Interfaces[0]
	if iface.Name != "SearchService" {
		t.Errorf("interface name = %q", iface.Name)
	}
	if len(iface.Methods) != 3 {
		t.Fatalf("expected 3 methods, got %d", len(iface.Methods))
	}

	// Check Search method
	search := iface.Methods[0]
	if search.Name != "Search" {
		t.Errorf("method 0 = %q", search.Name)
	}
	if !search.HasContext {
		t.Error("Search should have context")
	}
	if !search.HasError {
		t.Error("Search should have error")
	}

	// Check Ping — no context
	ping := iface.Methods[2]
	if ping.HasContext {
		t.Error("Ping should not have context")
	}
}

// TestGenerateProto generates a proto definition from analyzed Go types.
func TestGenerateProto(t *testing.T) {
	src := `package example

import "context"

type SearchRequest struct {
	Query string
	Limit int32
	Offset int32
}

type SearchResponse struct {
	Items []string
	Total int32
}

type SearchService interface {
	Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error)
	Ping() error
}
`
	info, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}

	proto := GenerateProto("example", "example/pb", info.Interfaces[0], info.Structs)
	t.Log(proto)

	// Verify proto contains expected elements
	for _, want := range []string{
		`syntax = "proto3"`,
		"message SearchRequest",
		"message SearchResponse",
		"service SearchService",
		"rpc Search(SearchRequest) returns (SearchResponse)",
		"rpc Ping(Nothing) returns (Nothing)",
		"string query",
		"int32 limit",
		"repeated string items",
	} {
		if !strings.Contains(proto, want) {
			t.Errorf("proto missing %q:\n%s", want, proto)
		}
	}
}

// TestAnalyzeAstkit is the bootstrap test: analyze astkit's own source files
// using astkit, and generate code from what we find.
func TestAnalyzeAstkit(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "../astkit", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	pkg, ok := pkgs["astkit"]
	if !ok {
		t.Fatal("astkit package not found")
	}

	// Analyze each file and collect all info
	allStructs := make(map[string]*StructInfo)
	allFuncs := 0

	for _, f := range pkg.Files {
		info := AnalyzeFile(f, fset)
		for i := range info.Structs {
			s := &info.Structs[i]
			if existing, ok := allStructs[s.Name]; ok {
				existing.Methods = append(existing.Methods, s.Methods...)
			} else {
				allStructs[s.Name] = s
			}
		}
		allFuncs += len(info.Functions)
	}

	t.Logf("Found %d struct types, %d exported functions in astkit", len(allStructs), allFuncs)

	// Verify we found key types
	for _, name := range []string{"File", "Func", "Struct", "Source", "StructTag", "TagBuilder", "TypeSpecWrapper"} {
		if _, ok := allStructs[name]; !ok {
			t.Errorf("missing struct: %s", name)
		}
	}

	// Verify Func has methods
	funcInfo := allStructs["Func"]
	if len(funcInfo.Methods) == 0 {
		t.Error("Func should have methods")
	}
	t.Logf("Func has %d methods", len(funcInfo.Methods))

	// Verify Struct has methods
	structInfo := allStructs["Struct"]
	if len(structInfo.Methods) == 0 {
		t.Error("Struct should have methods")
	}
	t.Logf("Struct has %d methods", len(structInfo.Methods))
}

