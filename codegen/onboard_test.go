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

	t.Logf("Proto:\n%s", bundle.Proto)
	t.Logf("Server:\n%s", bundle.ServerCode)
	t.Logf("Client:\n%s", bundle.ClientCode)
	t.Logf("Register:\n%s", bundle.RegisterFunc)
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
	// Onboard the astkit package itself
	bundles, err := OnboardDir("astkit", "../astkit")
	if err != nil {
		t.Fatal(err)
	}

	// astkit doesn't define interfaces, so we expect 0 bundles
	// but it shouldn't error
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

	// Type definitions from the source
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

	// Generated message structs
	for _, msg := range bundle.Messages {
		b.WriteString("type " + msg.Name + " struct {\n")
		for _, f := range msg.Fields {
			b.WriteString("\t" + f.Name + " " + f.TypeStr + "\n")
		}
		b.WriteString("}\n\n")
	}

	// Server code
	b.WriteString(bundle.ServerCode)
	b.WriteString("\n\nfunc main() {\n\ts := NewItemServiceServer()\n\t_ = s\n}\n")

	fullSrc := b.String()
	t.Logf("Full source:\n%s", fullSrc)

	// Write and compile
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

	// Verify files were written
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
}

func TestCompileCheck(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not found")
	}

	// Valid source should pass
	err := CompileCheck(`package main

func main() {}
`)
	if err != nil {
		t.Errorf("valid source should compile: %v", err)
	}

	// Invalid source should fail
	err = CompileCheck(`package main

func main() {
	undefined_var
}
`)
	if err == nil {
		t.Error("invalid source should not compile")
	}
}
