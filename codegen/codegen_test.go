package codegen

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
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

// TestGenerateServiceImpl generates a Go server implementation from an interface.
func TestGenerateServiceImpl(t *testing.T) {
	src := `package example

import "context"

type Request struct {
	Name string
}

type Response struct {
	Message string
}

type Greeter interface {
	Greet(ctx context.Context, req *Request) (*Response, error)
	Health() error
}
`
	info, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}

	code, err := GenerateServiceImpl("example", info.Interfaces[0])
	if err != nil {
		t.Fatal(err)
	}
	t.Log(code)

	for _, want := range []string{
		"GreeterServer",
		"UnimplementedGreeterServer",
		"NewGreeterServer",
		"func (s *GreeterServer) Greet",
		"func (s *GreeterServer) Health",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("code missing %q:\n%s", want, code)
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

// TestBootstrapCompile is the full bootstrap: analyze Go source, generate a
// complete server implementation, write it to a temp file, and compile it.
func TestBootstrapCompile(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go binary not found")
	}

	// Define a service interface
	src := `package main

import "context"

type GetRequest struct {
	ID string
}

type GetResponse struct {
	Name string
	Value string
}

type ListRequest struct {
	Prefix string
	Limit int32
}

type ListResponse struct {
	Items []string
}

type KVStore interface {
	Get(ctx context.Context, req *GetRequest) (*GetResponse, error)
	List(ctx context.Context, req *ListRequest) (*ListResponse, error)
	Delete(ctx context.Context, req *GetRequest) error
	Ping() error
}
`
	info, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}

	// Generate proto definition
	proto := GenerateProto("kvstore", "kvstore/pb", info.Interfaces[0], info.Structs)
	t.Logf("Generated proto:\n%s", proto)

	// Generate server implementation
	impl, err := GenerateServiceImpl("main", info.Interfaces[0])
	if err != nil {
		t.Fatal(err)
	}

	// Build a compilable program from the generated code
	fullSrc := `package main

import "context"

type GetRequest struct {
	ID string
}

type GetResponse struct {
	Name string
	Value string
}

type ListRequest struct {
	Prefix string
	Limit int32
}

type ListResponse struct {
	Items []string
}

type UnimplementedKVStoreServer struct{}

` + impl + `

// Prove the generated code is usable
var _ = context.Background

func main() {
	s := NewKVStoreServer()
	_ = s
}
`
	t.Logf("Generated source:\n%s", fullSrc)

	// Write to temp dir and compile
	tmpDir := t.TempDir()
	mainFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(mainFile, []byte(fullSrc), 0644); err != nil {
		t.Fatal(err)
	}

	// Initialize a go module in the temp dir
	modInit := exec.Command(goBin, "mod", "init", "test/bootstrap")
	modInit.Dir = tmpDir
	if out, err := modInit.CombinedOutput(); err != nil {
		t.Fatalf("go mod init failed: %v\n%s", err, out)
	}

	// Compile
	build := exec.Command(goBin, "build", "-o", "/dev/null", ".")
	build.Dir = tmpDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("COMPILE FAILED: %v\n%s\n\nGenerated source:\n%s", err, out, fullSrc)
	}

	t.Log("Bootstrap compile succeeded — generated code compiles!")
}

// TestBootstrapFromAstkit analyzes astkit's Struct wrapper type, generates
// a gRPC-style service from its methods, and verifies it compiles.
func TestBootstrapFromAstkit(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go binary not found")
	}

	// Parse astkit's struct.go to get Struct type methods
	fset := token.NewFileSet()
	structFile, err := parser.ParseFile(fset, "../astkit/struct.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	info := AnalyzeFile(structFile, fset)

	var structType *StructInfo
	for i := range info.Structs {
		if info.Structs[i].Name == "Struct" {
			structType = &info.Structs[i]
			break
		}
	}
	if structType == nil {
		t.Fatal("Struct type not found")
	}

	t.Logf("Struct has %d fields, %d methods", len(structType.Fields), len(structType.Methods))
	for _, m := range structType.Methods {
		t.Logf("  method: %s (params: %d, results: %d)", m.Name, len(m.Params), len(m.Results))
	}

	// Build an interface from the Struct methods that are "service-like"
	// (take simple params and return simple results)
	iface := InterfaceInfo{Name: "StructService"}
	for _, m := range structType.Methods {
		// Skip methods that take ast.Expr or other complex params
		// Keep simple query methods
		switch m.Name {
		case "HasField", "FieldNames", "ExportAllFields":
			iface.Methods = append(iface.Methods, m)
		}
	}

	if len(iface.Methods) == 0 {
		t.Skip("no suitable methods found")
	}

	// Generate proto definition
	proto := GenerateProto("structsvc", "structsvc/pb", iface, nil)
	t.Logf("Generated proto from astkit.Struct:\n%s", proto)

	// Generate server implementation
	impl, err := GenerateServiceImpl("main", iface)
	if err != nil {
		t.Fatal(err)
	}

	// Build compilable program
	fullSrc := `package main

type UnimplementedStructServiceServer struct{}

` + impl + `

func main() {
	s := NewStructServiceServer()
	_ = s
}
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(fullSrc), 0644); err != nil {
		t.Fatal(err)
	}

	modInit := exec.Command(goBin, "mod", "init", "test/astkit-bootstrap")
	modInit.Dir = tmpDir
	if out, err := modInit.CombinedOutput(); err != nil {
		t.Fatalf("go mod init: %v\n%s", err, out)
	}

	build := exec.Command(goBin, "build", "-o", "/dev/null", ".")
	build.Dir = tmpDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("COMPILE FAILED: %v\n%s\n\nGenerated source:\n%s", err, out, fullSrc)
	}

	t.Log("Astkit bootstrap compile succeeded!")
}
