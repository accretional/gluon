package codegen

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func skipIfNoProtoc(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"protoc", "protoc-gen-go", "protoc-gen-go-grpc"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not found", bin)
		}
	}
}

func TestNewProtoCompiler(t *testing.T) {
	skipIfNoProtoc(t)
	pc, err := NewProtoCompiler()
	if err != nil {
		t.Fatal(err)
	}
	if pc.ProtocBin == "" {
		t.Error("ProtocBin should not be empty")
	}
	if pc.GenGoBin == "" {
		t.Error("GenGoBin should not be empty")
	}
	if pc.GenGoGRPCBin == "" {
		t.Error("GenGoGRPCBin should not be empty")
	}
}

func TestCompileProtoString(t *testing.T) {
	skipIfNoProtoc(t)
	pc, err := NewProtoCompiler()
	if err != nil {
		t.Fatal(err)
	}

	proto := `syntax = "proto3";

package echo;

option go_package = "test/echo/pb";

message EchoRequest {
  string message = 1;
}

message EchoResponse {
  string message = 1;
  string echo = 2;
}

service EchoService {
  rpc Echo(EchoRequest) returns (EchoResponse);
}
`
	result, err := pc.CompileProtoString(proto, "test/echo/pb")
	if err != nil {
		t.Fatal(err)
	}
	if !result.CompileOK {
		t.Fatalf("proto compile failed: %v", result.Error)
	}

	// Should have generated .pb.go and _grpc.pb.go
	if len(result.GoFiles) < 2 {
		t.Errorf("expected at least 2 generated files, got %d: %v", len(result.GoFiles), fileNames(result.GoFiles))
	}

	foundPB := false
	foundGRPC := false
	for name, content := range result.GoFiles {
		if strings.HasSuffix(name, ".pb.go") && !strings.HasSuffix(name, "_grpc.pb.go") {
			foundPB = true
			if !strings.Contains(content, "EchoRequest") {
				t.Error("pb.go should contain EchoRequest")
			}
			if !strings.Contains(content, "EchoResponse") {
				t.Error("pb.go should contain EchoResponse")
			}
		}
		if strings.HasSuffix(name, "_grpc.pb.go") {
			foundGRPC = true
			if !strings.Contains(content, "EchoService") {
				t.Error("grpc.pb.go should contain EchoService")
			}
		}
		t.Logf("generated: %s (%d bytes)", name, len(content))
	}
	if !foundPB {
		t.Error("missing .pb.go file")
	}
	if !foundGRPC {
		t.Error("missing _grpc.pb.go file")
	}
}

func TestCompileProtoStringInvalidProto(t *testing.T) {
	skipIfNoProtoc(t)
	pc, err := NewProtoCompiler()
	if err != nil {
		t.Fatal(err)
	}

	result, err := pc.CompileProtoString("this is not valid proto", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.CompileOK {
		t.Error("invalid proto should not compile")
	}
	if result.Error == nil {
		t.Error("expected error for invalid proto")
	}
}

func TestCompileBundle(t *testing.T) {
	skipIfNoProtoc(t)
	pc, err := NewProtoCompiler()
	if err != nil {
		t.Fatal(err)
	}

	// Generate a bundle from source
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
}
`
	info, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}
	bundles, err := onboardPackageInfo("kvstore", info)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) == 0 {
		t.Fatal("no bundles generated")
	}

	t.Logf("Proto:\n%s", bundles[0].Proto)

	result, err := pc.CompileBundle(bundles[0])
	if err != nil {
		t.Fatal(err)
	}
	if !result.CompileOK {
		t.Fatalf("proto compile failed: %v", result.Error)
	}

	t.Logf("Generated %d Go files from proto", len(result.GoFiles))
	for name := range result.GoFiles {
		t.Logf("  %s", name)
	}
}

func TestCompilePackageProtos(t *testing.T) {
	skipIfNoProtoc(t)
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
}
`
	result, err := Bootstrap("test/multi", src)
	if err != nil {
		t.Fatal(err)
	}

	protos, err := CompilePackageProtos(result.Package)
	if err != nil {
		t.Fatal(err)
	}

	for name, pr := range protos {
		if !pr.CompileOK {
			t.Errorf("proto %s failed: %v", name, pr.Error)
		} else {
			t.Logf("proto %s: compiled OK (%d Go files)", name, len(pr.GoFiles))
		}
	}

	if len(protos) != 2 {
		t.Errorf("expected 2 proto results (item_service, search_service), got %d", len(protos))
	}
}

func TestFullBootstrap(t *testing.T) {
	skipIfNoProtoc(t)
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
	result, err := FullBootstrap("test/echo", src)
	if err != nil {
		t.Fatal(err)
	}

	if !result.CompileOK {
		t.Fatalf("Go compile failed: %v", result.CompileError)
	}

	if !result.ProtoCompileOK {
		for name, pr := range result.ProtoResults {
			if !pr.CompileOK {
				t.Errorf("proto %s failed: %v", name, pr.Error)
			}
		}
		t.Fatal("proto compilation failed")
	}

	// Verify proto generated expected files
	for name, pr := range result.ProtoResults {
		t.Logf("service %s: %d Go files from proto", name, len(pr.GoFiles))
		for fname := range pr.GoFiles {
			t.Logf("  %s", fname)
		}
	}

	if !result.RoundTripOK {
		t.Error("round-trip verification failed")
	}

	t.Log("Full bootstrap (Go + proto): PASS")
}

func TestFullBootstrapComplex(t *testing.T) {
	skipIfNoProtoc(t)
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

type ListUsersResponse struct {
	Users []User
	Total int32
}

type UserService interface {
	CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error)
	GetUser(ctx context.Context, id string) (*User, error)
	DeleteUser(ctx context.Context, id string) error
	ListUsers(ctx context.Context, limit int32, offset int32) (*ListUsersResponse, error)
	Ping() error
}
`
	result, err := FullBootstrap("test/userapi", src)
	if err != nil {
		t.Fatal(err)
	}

	if !result.CompileOK {
		t.Fatalf("Go compile failed: %v", result.CompileError)
	}

	if !result.ProtoCompileOK {
		for name, pr := range result.ProtoResults {
			if !pr.CompileOK {
				t.Errorf("proto %s failed: %v", name, pr.Error)
			}
		}
		t.Fatal("proto compilation failed")
	}

	// Check that proto generated gRPC stubs
	for _, pr := range result.ProtoResults {
		foundGRPC := false
		for fname := range pr.GoFiles {
			if strings.HasSuffix(fname, "_grpc.pb.go") {
				foundGRPC = true
			}
		}
		if !foundGRPC {
			t.Error("expected gRPC stub file")
		}
	}

	t.Log("Full bootstrap complex: PASS")
}

func TestCompileProtoFile(t *testing.T) {
	skipIfNoProtoc(t)
	pc, err := NewProtoCompiler()
	if err != nil {
		t.Fatal(err)
	}

	// Write a proto file to a temp dir, then compile it
	dir := t.TempDir()
	protoContent := `syntax = "proto3";

package health;

option go_package = "test/health/pb";

message Nothing {}

service HealthChecker {
  rpc Ping(Nothing) returns (Nothing);
  rpc Ready(Nothing) returns (Nothing);
}
`
	protoFile := dir + "/health.proto"
	if err := writeFile(protoFile, protoContent); err != nil {
		t.Fatal(err)
	}

	outDir := dir + "/out"
	result, err := pc.Compile(protoFile, "test/health/pb", outDir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CompileOK {
		t.Fatalf("proto compile failed: %v", result.Error)
	}
	if result.ProtoFile != protoFile {
		t.Errorf("ProtoFile = %q, want %q", result.ProtoFile, protoFile)
	}
	if len(result.GoFiles) < 2 {
		t.Errorf("expected at least 2 Go files, got %d", len(result.GoFiles))
	}
}

func TestFullBootstrapNoProtoc(t *testing.T) {
	// This test verifies FullBootstrap degrades gracefully.
	// We can't easily hide protoc, but we can at least verify the function
	// runs and returns a valid result.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not found")
	}

	src := `package simple

import "context"

type Req struct { Name string }
type Resp struct { Value string }

type Svc interface {
	Do(ctx context.Context, req *Req) (*Resp, error)
}
`
	result, err := FullBootstrap("test/simple", src)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CompileOK {
		t.Fatalf("Go compile failed: %v", result.CompileError)
	}
	// ProtoCompileOK depends on whether protoc is available — either way
	// the result should be valid
	t.Logf("ProtoCompileOK: %v", result.ProtoCompileOK)
}

func fileNames(m map[string]string) []string {
	var names []string
	for k := range m {
		names = append(names, k)
	}
	return names
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
