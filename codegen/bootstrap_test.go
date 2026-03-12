package codegen

import (
	"os/exec"
	"strings"
	"testing"
)

func skipIfNoBuildTools(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not found")
	}
	for _, bin := range []string{"protoc", "protoc-gen-go", "protoc-gen-go-grpc"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not found", bin)
		}
	}
}

// TestBootstrapSimple runs the full bootstrap on a simple service interface.
func TestBootstrapSimple(t *testing.T) {
	skipIfNoBuildTools(t)

	src := `package kvstore

import "context"

type Key struct {
	Name      string
	Namespace string
}

type Value struct {
	Data    []byte
	Version int32
}

type KVStore interface {
	Get(ctx context.Context, key *Key) (*Value, error)
	Put(ctx context.Context, key *Key, val *Value) error
	Delete(ctx context.Context, key *Key) error
	Ping() error
}
`
	result, err := Bootstrap("test/kvstore", src)
	if err != nil {
		t.Fatal(err)
	}

	if !result.CompileOK {
		for name, content := range result.Package.Files {
			if strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".proto") {
				t.Logf("=== %s ===\n%s", name, content)
			}
		}
		t.Fatalf("compile failed: %v", result.CompileError)
	}

	// Log all generated files
	for name, content := range result.Package.Files {
		if strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".proto") {
			t.Logf("=== %s ===\n%s", name, content)
		}
	}

	// Verify package structure
	expectFiles := []string{
		"pb/kvstore.proto",
		"kv_store_server.go",
		"main.go",
		"go.mod",
	}
	for _, f := range expectFiles {
		if _, ok := result.Package.Files[f]; !ok {
			t.Errorf("missing file: %s (have: %v)", f, fileKeys(result.Package.Files))
		}
	}

	// Verify server uses pb types
	serverGo := result.Package.Files["kv_store_server.go"]
	for _, want := range []string{
		"pb.UnimplementedKVStoreServer",
		"*pb.Key",
		"*pb.Value",
		"package main",
	} {
		if !strings.Contains(serverGo, want) {
			t.Errorf("server should contain %q", want)
		}
	}

	// Verify main.go wires up the server
	mainGo := result.Package.Files["main.go"]
	for _, want := range []string{
		"pb.RegisterKVStoreServer",
		"NewKVStoreServer()",
		"grpc.NewServer()",
		"net.Listen",
	} {
		if !strings.Contains(mainGo, want) {
			t.Errorf("main.go should contain %q", want)
		}
	}

	// Verify round-trip
	if !result.RoundTripOK {
		t.Error("round-trip verification failed")
		if result.RoundTrip != nil {
			for _, s := range result.RoundTrip.Structs {
				t.Logf("  struct: %s", s.Name)
			}
			for _, f := range result.RoundTrip.Functions {
				t.Logf("  func: %s", f.Name)
			}
		}
	}
}

// TestBootstrapComplex runs bootstrap on a more complex API with multiple
// param types, return types, and methods.
func TestBootstrapComplex(t *testing.T) {
	skipIfNoBuildTools(t)

	src := `package userapi

import "context"

type User struct {
	ID    string
	Name  string
	Email string
	Age   int32
}

type CreateUserRequest struct {
	Name  string
	Email string
	Age   int32
}

type UpdateUserRequest struct {
	ID    string
	Name  string
	Email string
}

type ListUsersRequest struct {
	Limit  int32
	Offset int32
	Filter string
}

type ListUsersResponse struct {
	Users []User
	Total int32
}

type UserService interface {
	CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error)
	GetUser(ctx context.Context, id string) (*User, error)
	UpdateUser(ctx context.Context, req *UpdateUserRequest) (*User, error)
	DeleteUser(ctx context.Context, id string) error
	ListUsers(ctx context.Context, req *ListUsersRequest) (*ListUsersResponse, error)
	Health() error
}
`
	result, err := Bootstrap("test/userapi", src)
	if err != nil {
		t.Fatal(err)
	}

	if !result.CompileOK {
		for name, content := range result.Package.Files {
			if strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".proto") {
				t.Logf("=== %s ===\n%s", name, content)
			}
		}
		t.Fatalf("compile failed: %v", result.CompileError)
	}

	if !result.RoundTripOK {
		t.Error("round-trip verification failed")
	}

	// Verify specific expectations
	rt := result.RoundTrip
	if rt == nil {
		t.Fatal("round-trip is nil")
	}

	// Should find the server struct and constructor
	foundServer := false
	foundConstructor := false
	for _, s := range rt.Structs {
		if s.Name == "UserServiceServer" {
			foundServer = true
		}
	}
	for _, f := range rt.Functions {
		if f.Name == "NewUserServiceServer" {
			foundConstructor = true
		}
	}

	if !foundServer {
		t.Error("round-trip should find UserServiceServer struct")
	}
	if !foundConstructor {
		t.Error("round-trip should find NewUserServiceServer constructor")
	}

	// Verify all methods are present
	methodNames := make(map[string]bool)
	for _, f := range rt.Functions {
		methodNames[f.Name] = true
	}
	for _, want := range []string{"CreateUser", "GetUser", "UpdateUser", "DeleteUser", "ListUsers", "Health"} {
		if !methodNames[want] {
			t.Errorf("round-trip missing method: %s", want)
		}
	}
}

// TestBootstrapMultipleInterfaces tests bootstrap with multiple service
// interfaces in one package.
func TestBootstrapMultipleInterfaces(t *testing.T) {
	skipIfNoBuildTools(t)

	src := `package multi

import "context"

type Item struct {
	ID   string
	Name string
}

type Query struct {
	Filter string
	Limit  int32
}

type ItemService interface {
	GetItem(ctx context.Context, id string) (*Item, error)
	CreateItem(ctx context.Context, name string) (*Item, error)
}

type SearchService interface {
	Search(ctx context.Context, q *Query) (*Item, error)
	Ping() error
}
`
	result, err := Bootstrap("test/multi", src)
	if err != nil {
		t.Fatal(err)
	}

	if !result.CompileOK {
		for name, content := range result.Package.Files {
			if strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".proto") {
				t.Logf("=== %s ===\n%s", name, content)
			}
		}
		t.Fatalf("compile failed: %v", result.CompileError)
	}

	// Should have generated files for both services
	pkg := result.Package
	if _, ok := pkg.Files["item_service_server.go"]; !ok {
		t.Error("missing item_service_server.go")
	}
	if _, ok := pkg.Files["search_service_server.go"]; !ok {
		t.Error("missing search_service_server.go")
	}

	// Both should be registered in main.go
	mainGo := pkg.Files["main.go"]
	if !strings.Contains(mainGo, "pb.RegisterItemServiceServer") {
		t.Error("main.go should register ItemService")
	}
	if !strings.Contains(mainGo, "pb.RegisterSearchServiceServer") {
		t.Error("main.go should register SearchService")
	}

	// Single unified proto in pb/
	if _, ok := pkg.Files["pb/multi.proto"]; !ok {
		t.Error("missing pb/multi.proto")
	}
}

// TestBootstrapRoundTrip verifies the round-trip: generated code can be
// re-analyzed and the structure matches what we expect.
func TestBootstrapRoundTrip(t *testing.T) {
	skipIfNoBuildTools(t)

	src := `package echo

import "context"

type EchoRequest struct {
	Message string
}

type EchoResponse struct {
	Message string
	Echo    string
}

type EchoService interface {
	Echo(ctx context.Context, req *EchoRequest) (*EchoResponse, error)
}
`
	result, err := Bootstrap("test/echo", src)
	if err != nil {
		t.Fatal(err)
	}

	if !result.CompileOK {
		t.Fatalf("compile failed: %v", result.CompileError)
	}

	rt := result.RoundTrip
	if rt == nil {
		t.Fatal("round-trip is nil")
	}

	// Should find the server struct
	structNames := make(map[string]bool)
	for _, s := range rt.Structs {
		structNames[s.Name] = true
	}

	for _, want := range []string{
		"EchoServiceServer",
	} {
		if !structNames[want] {
			t.Errorf("round-trip missing struct: %s (have: %v)", want, structNames)
		}
	}

	// Verify the Echo method exists on the server
	for _, f := range rt.Functions {
		if f.Name == "Echo" && f.RecvType != "" {
			if !f.HasContext {
				t.Error("Echo method should have context")
			}
		}
	}
}

// TestBootstrapPingOnly tests the minimal case: an interface with only
// parameterless methods.
func TestBootstrapPingOnly(t *testing.T) {
	skipIfNoBuildTools(t)

	src := `package health

type HealthChecker interface {
	Ping() error
	Ready() error
}
`
	result, err := Bootstrap("test/health", src)
	if err != nil {
		t.Fatal(err)
	}

	if !result.CompileOK {
		for name, content := range result.Package.Files {
			if strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".proto") {
				t.Logf("=== %s ===\n%s", name, content)
			}
		}
		t.Fatalf("compile failed: %v", result.CompileError)
	}
}

// TestBootstrapNoInterface verifies that bootstrap returns an error when
// there are no interfaces to onboard.
func TestBootstrapNoInterface(t *testing.T) {
	src := `package plain

type Config struct {
	Host string
	Port int
}
`
	_, err := Bootstrap("test/plain", src)
	if err == nil {
		t.Error("expected error for source with no interfaces")
	}
}

// TestWritePackage verifies WritePackage produces all expected files.
func TestWritePackage(t *testing.T) {
	src := `package example

import "context"

type Req struct {
	Name string
}

type Resp struct {
	Value string
}

type Svc interface {
	Do(ctx context.Context, req *Req) (*Resp, error)
}
`
	info, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}

	bundles, err := onboardPackageInfo("example", info)
	if err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	pkg, err := WritePackage("test/example", "example", info, bundles, tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	expectedFiles := []string{
		"pb/example.proto",
		"svc_server.go",
		"main.go",
		"go.mod",
	}

	for _, name := range expectedFiles {
		content, ok := pkg.Files[name]
		if !ok {
			t.Errorf("missing file: %s (have: %v)", name, fileKeys(pkg.Files))
			continue
		}
		if len(content) == 0 {
			t.Errorf("empty file: %s", name)
		}
	}

	// Server file should use pb types
	serverGo := pkg.Files["svc_server.go"]
	if !strings.Contains(serverGo, "pb.UnimplementedSvcServer") {
		t.Error("server should embed pb.UnimplementedSvcServer")
	}

	// go.mod should have real deps
	goMod := pkg.Files["go.mod"]
	if !strings.Contains(goMod, "google.golang.org/grpc") {
		t.Error("go.mod should require grpc")
	}
	if !strings.Contains(goMod, "google.golang.org/protobuf") {
		t.Error("go.mod should require protobuf")
	}
}

func fileKeys(m map[string]string) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestFormatGeneratedFile verifies gofmt on generated code.
func TestFormatGeneratedFile(t *testing.T) {
	src := `package main

func   main(  )  {
}
`
	formatted, err := FormatGeneratedFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(formatted, "  ") {
		t.Errorf("formatted code should not have extra spaces:\n%s", formatted)
	}
}
