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

// TestSelfOnboardGluon runs the onboard pipeline on gluon's own source files.
// It parses the Go and GoMod server interfaces from go.proto's generated code,
// analyzes the actual server implementations, and generates new service bundles.
func TestSelfOnboardGluon(t *testing.T) {
	// Parse the main gluon package
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "..", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	pkg, ok := pkgs["gluon"]
	if !ok {
		t.Fatal("gluon package not found")
	}

	// Merge all files
	merged := &PackageInfo{}
	for _, f := range pkg.Files {
		info := AnalyzeFile(f, fset)
		if merged.Name == "" {
			merged.Name = info.Name
		}
		merged.Structs = append(merged.Structs, info.Structs...)
		merged.Interfaces = append(merged.Interfaces, info.Interfaces...)
		merged.Functions = append(merged.Functions, info.Functions...)
	}

	t.Logf("Gluon package: %d structs, %d interfaces, %d functions",
		len(merged.Structs), len(merged.Interfaces), len(merged.Functions))

	// Log the struct types found
	for _, s := range merged.Structs {
		t.Logf("  struct %s (%d fields, %d methods)", s.Name, len(s.Fields), len(s.Methods))
	}

	// Log functions
	for _, f := range merged.Functions {
		recv := ""
		if f.RecvType != "" {
			recv = "(" + f.RecvType + ") "
		}
		t.Logf("  func %s%s (ctx:%v err:%v)", recv, f.Name, f.HasContext, f.HasError)
	}

	// Verify we found the key types
	found := make(map[string]bool)
	for _, s := range merged.Structs {
		found[s.Name] = true
	}
	for _, want := range []string{"GoServer", "GoModServer"} {
		if !found[want] {
			t.Errorf("missing struct: %s", want)
		}
	}

	// Check GoServer methods
	for _, s := range merged.Structs {
		if s.Name == "GoServer" {
			t.Logf("GoServer has %d methods", len(s.Methods))
			if len(s.Methods) == 0 {
				t.Error("GoServer should have methods")
			}
		}
	}
}

// TestOnboardAstkit runs the full onboard pipeline on astkit, generates
// code as if it were a service, and verifies it compiles.
func TestOnboardAstkit(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go binary not found")
	}

	// Parse astkit
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "../astkit", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	pkg := pkgs["astkit"]
	merged := &PackageInfo{Name: "astkit"}
	for _, f := range pkg.Files {
		info := AnalyzeFile(f, fset)
		merged.Structs = append(merged.Structs, info.Structs...)
		merged.Functions = append(merged.Functions, info.Functions...)
	}

	t.Logf("astkit: %d structs, %d functions", len(merged.Structs), len(merged.Functions))

	// Pick the File struct — it has interesting methods
	var fileStruct *StructInfo
	for i := range merged.Structs {
		if merged.Structs[i].Name == "File" {
			fileStruct = &merged.Structs[i]
			break
		}
	}
	if fileStruct == nil {
		t.Fatal("File struct not found")
	}

	t.Logf("File struct has %d methods", len(fileStruct.Methods))

	// Build an interface from File's simple methods (only those returning
	// primitive types — methods returning *ast.FuncDecl etc. aren't
	// serializable without go/ast import).
	iface := InterfaceInfo{Name: "FileService"}
	for _, m := range fileStruct.Methods {
		switch m.Name {
		case "AddImport", "AddNamedImport":
			iface.Methods = append(iface.Methods, m)
		}
	}

	if len(iface.Methods) == 0 {
		t.Skip("no suitable methods")
	}

	// Transform and generate
	xform := TransformInterface(iface, nil)
	serverCode, err := GenerateServiceImpl("main", xform.Interface)
	if err != nil {
		t.Fatal(err)
	}

	// Build compilable program
	var b strings.Builder
	b.WriteString("package main\n\nimport \"context\"\n\nvar _ = context.Background\n\n")

	// Message types
	for _, msg := range xform.Messages {
		b.WriteString("type " + msg.Name + " struct {\n")
		for _, f := range msg.Fields {
			b.WriteString("\t" + f.Name + " " + f.TypeStr + "\n")
		}
		b.WriteString("}\n\n")
	}

	b.WriteString("type UnimplementedFileServiceServer struct{}\n\n")
	b.WriteString(serverCode)
	b.WriteString("\n\nfunc main() {\n\ts := NewFileServiceServer()\n\t_ = s\n}\n")

	fullSrc := b.String()
	t.Logf("Generated source:\n%s", fullSrc)

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(fullSrc), 0644); err != nil {
		t.Fatal(err)
	}

	modInit := exec.Command(goBin, "mod", "init", "test/astkit-onboard")
	modInit.Dir = tmpDir
	if out, err := modInit.CombinedOutput(); err != nil {
		t.Fatalf("go mod init: %v\n%s", err, out)
	}

	build := exec.Command(goBin, "build", "-o", "/dev/null", ".")
	build.Dir = tmpDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("COMPILE FAILED: %v\n%s\n\nSource:\n%s", err, out, fullSrc)
	}

	t.Log("Astkit File onboard + compile succeeded!")
}

// TestOnboardPipelineEndToEnd tests the complete pipeline: analyze → transform
// → generate proto + server + client → write to disk → verify files exist.
func TestOnboardPipelineEndToEnd(t *testing.T) {
	src := `package myapi

import "context"

type User struct {
	ID    string
	Name  string
	Email string
}

type CreateUserRequest struct {
	Name  string
	Email string
}

type ListUsersRequest struct {
	Limit  int32
	Offset int32
}

type ListUsersResponse struct {
	Users []User
	Total int32
}

type UserAPI interface {
	CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error)
	GetUser(ctx context.Context, id string) (*User, error)
	ListUsers(ctx context.Context, req *ListUsersRequest) (*ListUsersResponse, error)
	DeleteUser(ctx context.Context, id string) error
}
`

	bundles, err := OnboardSource("myapi", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) != 1 {
		t.Fatalf("expected 1 bundle, got %d", len(bundles))
	}

	bundle := bundles[0]

	// Write to temp dir
	outDir := t.TempDir()
	if err := WriteBundle(bundle, outDir, "myapi"); err != nil {
		t.Fatal(err)
	}

	// Check all files exist and have content
	files := []string{"user_a_p_i.proto", "user_a_p_i_server.go", "user_a_p_i_client.go"}
	for _, name := range files {
		path := filepath.Join(outDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("missing %s: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("%s is empty", name)
		}
	}

	// Verify proto has all RPCs
	for _, want := range []string{
		"rpc CreateUser",
		"rpc GetUser",
		"rpc ListUsers",
		"rpc DeleteUser",
		"message CreateUserRequest",
		"message ListUsersRequest",
		"message ListUsersResponse",
	} {
		if !strings.Contains(bundle.Proto, want) {
			t.Errorf("proto missing %q", want)
		}
	}

	// Verify server has all methods
	for _, want := range []string{
		"func (s *UserAPIServer) CreateUser",
		"func (s *UserAPIServer) GetUser",
		"func (s *UserAPIServer) ListUsers",
		"func (s *UserAPIServer) DeleteUser",
	} {
		if !strings.Contains(bundle.ServerCode, want) {
			t.Errorf("server missing %q", want)
		}
	}

	// Verify client has all methods
	for _, want := range []string{
		"func (c *UserAPIClient) CreateUser",
		"func (c *UserAPIClient) GetUser",
		"func (c *UserAPIClient) ListUsers",
		"func (c *UserAPIClient) DeleteUser",
	} {
		if !strings.Contains(bundle.ClientCode, want) {
			t.Errorf("client missing %q", want)
		}
	}

	t.Logf("Proto:\n%s", bundle.Proto)
}
