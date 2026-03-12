package codegen

import (
	"os/exec"
	"strings"
	"testing"
)

// TestBootstrapSimple runs the full bootstrap on a simple service interface.
func TestBootstrapSimple(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not found")
	}

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
		t.Fatalf("compile failed: %v", result.CompileError)
	}

	// Log all generated files
	for name, content := range result.Package.Files {
		if strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".proto") {
			t.Logf("=== %s ===\n%s", name, content)
		}
	}

	// Verify round-trip
	if !result.RoundTripOK {
		t.Error("round-trip verification failed")
		if result.RoundTrip != nil {
			t.Logf("Round-trip structs: %d, functions: %d",
				len(result.RoundTrip.Structs), len(result.RoundTrip.Functions))
			for _, s := range result.RoundTrip.Structs {
				t.Logf("  struct: %s", s.Name)
			}
			for _, f := range result.RoundTrip.Functions {
				t.Logf("  func: %s", f.Name)
			}
		}
	}

	t.Log("Bootstrap simple: PASS")
}

// TestBootstrapComplex runs bootstrap on a more complex API with multiple
// param types, return types, and methods.
func TestBootstrapComplex(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not found")
	}

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
		// Log files for debugging
		for name, content := range result.Package.Files {
			if strings.HasSuffix(name, ".go") {
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

	t.Log("Bootstrap complex: PASS")
}

// TestBootstrapMultipleInterfaces tests bootstrap with multiple service
// interfaces in one package.
func TestBootstrapMultipleInterfaces(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not found")
	}

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
			if strings.HasSuffix(name, ".go") {
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
	if _, ok := pkg.Files["item_service_client.go"]; !ok {
		t.Error("missing item_service_client.go")
	}
	if _, ok := pkg.Files["search_service_client.go"]; !ok {
		t.Error("missing search_service_client.go")
	}

	t.Log("Bootstrap multiple interfaces: PASS")
}

// TestBootstrapRoundTrip verifies the round-trip: generated code can be
// re-analyzed and the structure matches what we expect.
func TestBootstrapRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not found")
	}

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

	// Original had 2 structs (EchoRequest, EchoResponse)
	// Generated should have those + EchoServiceServer + UnimplementedEchoServiceServer
	structNames := make(map[string]bool)
	for _, s := range rt.Structs {
		structNames[s.Name] = true
	}

	for _, want := range []string{
		"EchoRequest",
		"EchoResponse",
		"EchoServiceServer",
		"UnimplementedEchoServiceServer",
	} {
		if !structNames[want] {
			t.Errorf("round-trip missing struct: %s (have: %v)", want, structNames)
		}
	}

	// Verify method signatures are preserved
	for _, f := range rt.Functions {
		if f.Name == "Echo" && f.RecvType != "" {
			if !f.HasContext {
				t.Error("Echo method should have context")
			}
			if !f.HasError {
				t.Error("Echo method should have error")
			}
		}
	}

	t.Log("Bootstrap round-trip: PASS")
}

// TestBootstrapPingOnly tests the minimal case: an interface with only
// parameterless methods.
func TestBootstrapPingOnly(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not found")
	}

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
			if strings.HasSuffix(name, ".go") {
				t.Logf("=== %s ===\n%s", name, content)
			}
		}
		t.Fatalf("compile failed: %v", result.CompileError)
	}

	t.Log("Bootstrap ping-only: PASS")
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
		"types.go",
		"svc_server.go",
		"svc_client.go",
		"svc.proto",
		"go.mod",
	}

	for _, name := range expectedFiles {
		content, ok := pkg.Files[name]
		if !ok {
			t.Errorf("missing file: %s", name)
			continue
		}
		if len(content) == 0 {
			t.Errorf("empty file: %s", name)
		}
	}

	// types.go should contain original structs
	typesGo := pkg.Files["types.go"]
	if !strings.Contains(typesGo, "type Req struct") {
		t.Error("types.go should contain Req struct")
	}
	if !strings.Contains(typesGo, "type Resp struct") {
		t.Error("types.go should contain Resp struct")
	}
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
