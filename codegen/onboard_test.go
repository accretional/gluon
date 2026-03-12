package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnboardSource(t *testing.T) {
	src := `package example

import "context"

type CreateRequest struct {
	Name string
	Type string
}

type CreateResponse struct {
	ID   string
	Name string
}

type Widget interface {
	Create(ctx context.Context, req *CreateRequest) (*CreateResponse, error)
	Delete(ctx context.Context, id string) error
	Ping() error
}
`
	bundles, err := OnboardSource("example", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) != 1 {
		t.Fatalf("expected 1 bundle, got %d", len(bundles))
	}

	bundle := bundles[0]
	if bundle.Name != "Widget" {
		t.Errorf("bundle name = %q", bundle.Name)
	}

	// Proto should contain service and messages
	for _, want := range []string{
		"service Widget",
		"message CreateRequest",
		"message CreateResponse",
		"rpc Create",
		"rpc Delete",
		"rpc Ping",
	} {
		if !strings.Contains(bundle.Proto, want) {
			t.Errorf("proto missing %q:\n%s", want, bundle.Proto)
		}
	}

	// Server code should contain implementation
	for _, want := range []string{
		"WidgetServer",
		"NewWidgetServer",
		"func (s *WidgetServer)",
	} {
		if !strings.Contains(bundle.ServerCode, want) {
			t.Errorf("server code missing %q:\n%s", want, bundle.ServerCode)
		}
	}

	// Client code should contain wrapper
	for _, want := range []string{
		"WidgetClient",
		"NewWidgetClient",
		"func (c *WidgetClient)",
		"Invoke",
	} {
		if !strings.Contains(bundle.ClientCode, want) {
			t.Errorf("client code missing %q:\n%s", want, bundle.ClientCode)
		}
	}

	// Register func should be present
	if !strings.Contains(bundle.RegisterFunc, "RegisterWidgetServer") {
		t.Errorf("register func missing RegisterWidgetServer:\n%s", bundle.RegisterFunc)
	}
}

func TestOnboardMultipleInterfaces(t *testing.T) {
	src := `package example

import "context"

type UserRequest struct {
	ID string
}

type UserResponse struct {
	Name string
}

type UserService interface {
	GetUser(ctx context.Context, req *UserRequest) (*UserResponse, error)
}

type HealthService interface {
	Ping() error
}
`
	bundles, err := OnboardSource("example", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) != 2 {
		t.Fatalf("expected 2 bundles, got %d", len(bundles))
	}
	if bundles[0].Name != "UserService" {
		t.Errorf("bundle 0 = %q", bundles[0].Name)
	}
	if bundles[1].Name != "HealthService" {
		t.Errorf("bundle 1 = %q", bundles[1].Name)
	}
}

func TestOnboardDir(t *testing.T) {
	bundles, err := OnboardDir("astkit", "../astkit")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Found %d interfaces in astkit", len(bundles))
}

func TestOnboardAndCompile(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go binary not found")
	}

	src := `package example

import "context"

type Item struct {
	ID   string
	Name string
}

type GetItemRequest struct {
	ID string
}

type ListItemsRequest struct {
	Prefix string
	Limit  int32
}

type ListItemsResponse struct {
	Items []Item
	Total int32
}

type ItemService interface {
	GetItem(ctx context.Context, req *GetItemRequest) (*Item, error)
	ListItems(ctx context.Context, req *ListItemsRequest) (*ListItemsResponse, error)
	DeleteItem(ctx context.Context, req *GetItemRequest) error
	Health() error
}
`
	bundles, err := OnboardSource("main", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) != 1 {
		t.Fatalf("expected 1 bundle, got %d", len(bundles))
	}

	bundle := bundles[0]

	// Build a compilable program from the bundle
	var b strings.Builder
	b.WriteString("package main\n\nimport \"context\"\n\n")
	b.WriteString("var _ = context.Background\n\n")

	b.WriteString(`type Item struct {
	ID   string
	Name string
}

type GetItemRequest struct {
	ID string
}

type ListItemsRequest struct {
	Prefix string
	Limit  int32
}

type ListItemsResponse struct {
	Items []Item
	Total int32
}

type UnimplementedItemServiceServer struct{}

`)

	for _, msg := range bundle.Messages {
		b.WriteString("type " + msg.Name + " struct {\n")
		for _, f := range msg.Fields {
			b.WriteString("\t" + f.Name + " " + f.TypeStr + "\n")
		}
		b.WriteString("}\n\n")
	}

	b.WriteString(bundle.ServerCode)
	b.WriteString("\n\nfunc main() {\n\ts := NewItemServiceServer()\n\t_ = s\n}\n")

	fullSrc := b.String()

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(fullSrc), 0644); err != nil {
		t.Fatal(err)
	}

	modInit := exec.Command(goBin, "mod", "init", "test/onboard")
	modInit.Dir = tmpDir
	if out, err := modInit.CombinedOutput(); err != nil {
		t.Fatalf("go mod init: %v\n%s", err, out)
	}

	build := exec.Command(goBin, "build", "-o", "/dev/null", ".")
	build.Dir = tmpDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("COMPILE FAILED: %v\n%s\n\nSource:\n%s", err, out, fullSrc)
	}

	t.Log("Onboard + compile succeeded!")
}

// TestWriteBundleServerCompiles verifies that the server file produced by
// WriteBundle compiles standalone (with type stubs for referenced types).
func TestWriteBundleServerCompiles(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go binary not found")
	}

	src := `package example

import "context"

type Req struct {
	Name string
}

type Resp struct {
	Value string
}

type Echo interface {
	Echo(ctx context.Context, req *Req) (*Resp, error)
	Ping() error
}
`
	bundles, err := OnboardSource("main", src)
	if err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	if err := WriteBundle(bundles[0], outDir, "main"); err != nil {
		t.Fatal(err)
	}

	// Read the generated server file
	serverData, err := os.ReadFile(filepath.Join(outDir, "echo_server.go"))
	if err != nil {
		t.Fatal(err)
	}

	serverSrc := string(serverData)
	t.Logf("Server file:\n%s", serverSrc)

	// Verify it has imports
	if !strings.Contains(serverSrc, `"context"`) {
		t.Error("server file should import context")
	}

	// Verify it has Unimplemented type
	if !strings.Contains(serverSrc, "UnimplementedEchoServer") {
		t.Error("server file should contain UnimplementedEchoServer")
	}

	// Build a compilable program: prepend type stubs, add main
	var b strings.Builder
	// The server file already has package + imports + UnimplementedEchoServer
	// We just need the Req/Resp types and a main func
	b.WriteString(serverSrc)
	b.WriteString("\ntype Req struct{ Name string }\n")
	b.WriteString("type Resp struct{ Value string }\n")
	b.WriteString("\nfunc main() {\n\ts := NewEchoServer()\n\t_ = s\n}\n")

	fullSrc := b.String()

	// Write and compile
	compileDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(compileDir, "main.go"), []byte(fullSrc), 0644); err != nil {
		t.Fatal(err)
	}

	modInit := exec.Command(goBin, "mod", "init", "test/server-compile")
	modInit.Dir = compileDir
	if out, err := modInit.CombinedOutput(); err != nil {
		t.Fatalf("go mod init: %v\n%s", err, out)
	}

	build := exec.Command(goBin, "build", "-o", "/dev/null", ".")
	build.Dir = compileDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("COMPILE FAILED: %v\n%s\n\nSource:\n%s", err, out, fullSrc)
	}

	t.Log("WriteBundle server file compiles!")
}

func TestWriteBundle(t *testing.T) {
	src := `package example

import "context"

type Req struct {
	Name string
}

type Resp struct {
	Value string
}

type Echo interface {
	Echo(ctx context.Context, req *Req) (*Resp, error)
}
`
	bundles, err := OnboardSource("example", src)
	if err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	if err := WriteBundle(bundles[0], outDir, "example"); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"echo.proto", "echo_server.go", "echo_client.go"} {
		path := filepath.Join(outDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("missing %s: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("%s is empty", name)
		}
		t.Logf("%s:\n%s", name, data)
	}

	// Verify server file has imports and Unimplemented type
	serverData, _ := os.ReadFile(filepath.Join(outDir, "echo_server.go"))
	serverSrc := string(serverData)
	if !strings.Contains(serverSrc, `"context"`) {
		t.Error("server file should import context")
	}
	if !strings.Contains(serverSrc, "UnimplementedEchoServer") {
		t.Error("server file should contain UnimplementedEchoServer")
	}

	// Verify client file has imports
	clientData, _ := os.ReadFile(filepath.Join(outDir, "echo_client.go"))
	clientSrc := string(clientData)
	if !strings.Contains(clientSrc, `"context"`) {
		t.Error("client file should import context")
	}
	if !strings.Contains(clientSrc, `"google.golang.org/grpc"`) {
		t.Error("client file should import grpc")
	}
}

func TestCompileCheck(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not found")
	}

	err := CompileCheck(`package main

func main() {}
`)
	if err != nil {
		t.Errorf("valid source should compile: %v", err)
	}

	err = CompileCheck(`package main

func main() {
	undefined_var
}
`)
	if err == nil {
		t.Error("invalid source should not compile")
	}
}

// TestOnboardEmptyInterface verifies we handle an interface with no methods.
func TestOnboardEmptyInterface(t *testing.T) {
	src := `package example

type Empty interface{}
`
	bundles, err := OnboardSource("example", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) != 1 {
		t.Fatalf("expected 1 bundle, got %d", len(bundles))
	}
	bundle := bundles[0]
	if len(bundle.NormalizedInterface.Methods) != 0 {
		t.Errorf("empty interface should have 0 methods, got %d", len(bundle.NormalizedInterface.Methods))
	}
	// Proto should still have the service block (just empty)
	if !strings.Contains(bundle.Proto, "service Empty") {
		t.Error("proto should contain service Empty")
	}
}

// TestOnboardSingleMethod verifies a single-method interface works.
func TestOnboardSingleMethod(t *testing.T) {
	src := `package example

import "context"

type Pinger interface {
	Ping(ctx context.Context) error
}
`
	bundles, err := OnboardSource("example", src)
	if err != nil {
		t.Fatal(err)
	}
	bundle := bundles[0]
	if len(bundle.NormalizedInterface.Methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(bundle.NormalizedInterface.Methods))
	}
	if !strings.Contains(bundle.Proto, "rpc Ping") {
		t.Error("proto should contain rpc Ping")
	}
}

// TestOnboardNoContextNoError verifies methods with no context and no error.
func TestOnboardNoContextNoError(t *testing.T) {
	src := `package example

type Counter interface {
	Count() int
	Reset()
}
`
	bundles, err := OnboardSource("example", src)
	if err != nil {
		t.Fatal(err)
	}
	bundle := bundles[0]
	// Both methods should be transformed to have context+error
	for _, m := range bundle.NormalizedInterface.Methods {
		if !m.HasContext {
			t.Errorf("method %s should have context after transform", m.Name)
		}
		if !m.HasError {
			t.Errorf("method %s should have error after transform", m.Name)
		}
	}
}

// TestCollectImports verifies import detection from type strings.
func TestCollectImports(t *testing.T) {
	iface := InterfaceInfo{
		Name: "Test",
		Methods: []FuncInfo{
			{
				Name: "Do",
				Params: []FieldInfo{
					{TypeStr: "context.Context"},
					{TypeStr: "*http.Request"},
				},
				Results: []FieldInfo{
					{TypeStr: "*http.Response"},
					{TypeStr: "error"},
				},
			},
		},
	}

	imports := collectImports(iface)
	found := make(map[string]bool)
	for _, imp := range imports {
		found[imp] = true
	}

	if !found["context"] {
		t.Error("should detect context import")
	}
	if !found["net/http"] {
		t.Error("should detect net/http import")
	}
}
