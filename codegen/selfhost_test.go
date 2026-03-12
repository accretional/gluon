package codegen

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// =============================================================================
// Self-Host Validation Tests
// =============================================================================
//
// WHY THIS MATTERS
//
// Gluon's codegen pipeline transforms Go interfaces into fully compiled gRPC
// services. The pipeline is:
//
//   Go source → AST analysis → proto generation → protoc → server stubs
//   → main.go → go.mod → go mod tidy → go build → round-trip verification
//
// Self-hosting means running this pipeline on interfaces that mirror gluon's
// OWN services (Go and GoMod). If the codegen system can bootstrap a gRPC
// service with the same shape and complexity as itself, we have high
// confidence that it works for any real-world service.
//
// This is the "eating your own dog food" milestone: gluon's codegen proves
// itself by generating services equivalent to the ones it's built to serve.
//
// WHAT WE'RE TESTING
//
// 1. The full pipeline handles 17+ RPC methods across two services
// 2. Complex request types with many fields (bool, string, int32, repeated)
// 3. Multiple services sharing a proto namespace
// 4. Services with both parameterized and parameterless methods
// 5. The generated code actually compiles with real grpc/protobuf deps
// 6. Round-trip: codegen can re-analyze its own output
//
// The interfaces below are simplified versions of gluon's Go and GoMod
// services. They use plain Go types (since codegen works from Go source,
// not proto-generated types), but preserve the method count, parameter
// shapes, and structural complexity of the real services.
// =============================================================================

// TestSelfHostGluon runs the full bootstrap pipeline on interfaces that
// mirror gluon's own Go and GoMod services. This is the definitive
// validation that the codegen system can handle real-world service complexity.
func TestSelfHostGluon(t *testing.T) {
	skipIfNoBuildTools(t)

	// This source mirrors gluon's own service architecture:
	// - GoCompiler wraps the 17 RPCs from the Go service (go.proto)
	// - GoModManager wraps the 8 RPCs from the GoMod service
	// - Request types mirror the real proto messages with their field shapes
	src := `package gluon

import "context"

// --- Request/Response types mirroring go.proto messages ---

type TextMessage struct {
	Text string
}

type Empty struct{}

type CompilerTarget struct {
	OS   string
	Arch string
}

type DocQuery struct {
	Pkg           string
	Symbol        string
	MethodOrField string
	All           bool
	Short         bool
	Source        bool
}

type FixQuery struct {
	Pkg       string
	Path      string
	Diff      bool
	Analyzers string
}

type FormatRequest struct {
	Packages       string
	GoExpression   string
	PrintAllErrors bool
	Simplify       bool
}

type GetRequest struct {
	Packages     string
	Upgrade      bool
	IncludeTests bool
}

type InstallRequest struct {
	Packages string
}

type ListRequest struct {
	Modules     bool
	UpgradeInfo bool
	Retracted   bool
	Versions    bool
	Json        bool
	Packages    string
}

type EnvRequest struct {
	Json    bool
	Changed bool
	Vars    string
}

type BuildRequest struct {
	Packages string
	Output   string
	Verbose  bool
	Race     bool
	Ldflags  string
	Trimpath bool
	Gcflags  string
	Asmflags string
	Work     bool
}

type RunRequest struct {
	Package string
	Args    string
	Verbose bool
	Race    bool
	ExecCmd string
	Work    bool
}

type TestRequest struct {
	Packages     string
	Verbose      bool
	Race         bool
	Run          string
	Bench        string
	Count        int32
	Timeout      string
	Short        bool
	Cover        bool
	Coverprofile string
	Json         bool
	Failfast     bool
	Benchmem     bool
	Benchtime    string
}

type GenerateRequest struct {
	Packages      string
	Verbose       bool
	DryRun        bool
	PrintCommands bool
	Run           string
}

type VetRequest struct {
	Packages string
	Verbose  bool
	Json     bool
}

type ToolRequest struct {
	Name   string
	Args   string
	DryRun bool
}

// --- GoMod request types ---

type ModDownloadRequest struct {
	Modules       string
	Json          bool
	PrintCommands bool
}

type ModEditRequest struct {
	Module    string
	GoVersion string
	Json      bool
	Print     bool
	Fmt       bool
}

type ModGraphRequest struct {
	GoVersion string
}

type ModInitRequest struct {
	ModulePath string
}

type ModTidyRequest struct {
	Verbose   bool
	GoVersion string
	Diff      bool
}

type ModVendorRequest struct {
	Verbose   bool
	OutputDir string
}

type ModVerifyRequest struct{}

type ModWhyRequest struct {
	Modules  bool
	Vendor   bool
	Packages string
}

// GoCompiler mirrors the Go service from go.proto — 17 RPCs wrapping
// the Go compiler toolchain.
type GoCompiler interface {
	Command(ctx context.Context, req *TextMessage) (*TextMessage, error)
	Doc(ctx context.Context, req *DocQuery) (*TextMessage, error)
	Fix(ctx context.Context, req *FixQuery) (*TextMessage, error)
	ListFixAnalyzers(ctx context.Context) (*TextMessage, error)
	DescribeFixAnalyzer(ctx context.Context, req *TextMessage) (*TextMessage, error)
	Format(ctx context.Context, req *FormatRequest) (*TextMessage, error)
	Get(ctx context.Context, req *GetRequest) (*TextMessage, error)
	Install(ctx context.Context, req *InstallRequest) (*TextMessage, error)
	List(ctx context.Context, req *ListRequest) (*TextMessage, error)
	Env(ctx context.Context, req *EnvRequest) (*TextMessage, error)
	Build(ctx context.Context, req *BuildRequest) (*TextMessage, error)
	Run(ctx context.Context, req *RunRequest) (*TextMessage, error)
	Test(ctx context.Context, req *TestRequest) (*TextMessage, error)
	Generate(ctx context.Context, req *GenerateRequest) (*TextMessage, error)
	Vet(ctx context.Context, req *VetRequest) (*TextMessage, error)
	Tool(ctx context.Context, req *ToolRequest) (*TextMessage, error)
	Help(ctx context.Context, req *TextMessage) (*TextMessage, error)
	Version(ctx context.Context) (*TextMessage, error)
}

// GoModManager mirrors the GoMod service from go.proto — 8 RPCs wrapping
// Go module management.
type GoModManager interface {
	Download(ctx context.Context, req *ModDownloadRequest) (*TextMessage, error)
	Edit(ctx context.Context, req *ModEditRequest) (*TextMessage, error)
	Graph(ctx context.Context, req *ModGraphRequest) (*TextMessage, error)
	Init(ctx context.Context, req *ModInitRequest) (*TextMessage, error)
	Tidy(ctx context.Context, req *ModTidyRequest) (*TextMessage, error)
	Vendor(ctx context.Context, req *ModVendorRequest) (*TextMessage, error)
	Verify(ctx context.Context) (*TextMessage, error)
	Why(ctx context.Context, req *ModWhyRequest) (*TextMessage, error)
}
`
	result, err := Bootstrap("selfhost/gluon", src)
	if err != nil {
		t.Fatal(err)
	}

	// --- Verify compilation ---
	if !result.CompileOK {
		for name, content := range result.Package.Files {
			if strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".proto") {
				t.Logf("=== %s ===\n%s", name, content)
			}
		}
		t.Fatalf("self-host compilation failed: %v", result.CompileError)
	}
	t.Log("PASS: self-hosted gluon services compiled successfully")

	// --- Verify package structure ---
	pkg := result.Package
	expectedFiles := []string{
		"pb/gluon.proto",
		"go_compiler_server.go",
		"go_mod_manager_server.go",
		"main.go",
		"go.mod",
	}
	for _, f := range expectedFiles {
		if _, ok := pkg.Files[f]; !ok {
			t.Errorf("missing file: %s (have: %v)", f, fileKeys(pkg.Files))
		}
	}

	// --- Verify both services are registered in main.go ---
	mainGo := pkg.Files["main.go"]
	for _, want := range []string{
		"pb.RegisterGoCompilerServer",
		"pb.RegisterGoModManagerServer",
		"NewGoCompilerServer()",
		"NewGoModManagerServer()",
		"grpc.NewServer()",
	} {
		if !strings.Contains(mainGo, want) {
			t.Errorf("main.go should contain %q", want)
		}
	}

	// --- Verify GoCompiler server has all 17+ methods ---
	goCompilerGo := pkg.Files["go_compiler_server.go"]
	goCompilerMethods := []string{
		"Command", "Doc", "Fix", "ListFixAnalyzers",
		"DescribeFixAnalyzer", "Format", "Get", "Install",
		"List", "Env", "Build", "Run", "Test",
		"Generate", "Vet", "Tool", "Help", "Version",
	}
	for _, m := range goCompilerMethods {
		if !strings.Contains(goCompilerGo, "func (s *GoCompilerServer) "+m+"(") {
			t.Errorf("GoCompilerServer missing method: %s", m)
		}
	}

	// --- Verify GoModManager server has all 8 methods ---
	goModGo := pkg.Files["go_mod_manager_server.go"]
	goModMethods := []string{
		"Download", "Edit", "Graph", "Init",
		"Tidy", "Vendor", "Verify", "Why",
	}
	for _, m := range goModMethods {
		if !strings.Contains(goModGo, "func (s *GoModManagerServer) "+m+"(") {
			t.Errorf("GoModManagerServer missing method: %s", m)
		}
	}

	// --- Verify both servers embed pb.Unimplemented ---
	if !strings.Contains(goCompilerGo, "pb.UnimplementedGoCompilerServer") {
		t.Error("GoCompilerServer should embed pb.UnimplementedGoCompilerServer")
	}
	if !strings.Contains(goModGo, "pb.UnimplementedGoModManagerServer") {
		t.Error("GoModManagerServer should embed pb.UnimplementedGoModManagerServer")
	}

	// --- Verify proto has both services and key message types ---
	proto := pkg.Files["pb/gluon.proto"]
	for _, want := range []string{
		"service GoCompiler",
		"service GoModManager",
		"message BuildRequest",
		"message TestRequest",
		"message ModTidyRequest",
		"message TextMessage",
	} {
		if !strings.Contains(proto, want) {
			t.Errorf("proto should contain %q", want)
		}
	}

	// --- Verify round-trip ---
	if !result.RoundTripOK {
		t.Error("round-trip verification failed")
	}
	if result.RoundTrip != nil {
		t.Logf("round-trip found %d structs, %d functions",
			len(result.RoundTrip.Structs), len(result.RoundTrip.Functions))

		// Verify round-trip found both server types
		rtStructs := make(map[string]bool)
		for _, s := range result.RoundTrip.Structs {
			rtStructs[s.Name] = true
		}
		for _, want := range []string{"GoCompilerServer", "GoModManagerServer"} {
			if !rtStructs[want] {
				t.Errorf("round-trip missing struct: %s", want)
			}
		}

		// Verify round-trip found all method names
		rtFuncs := make(map[string]bool)
		for _, f := range result.RoundTrip.Functions {
			rtFuncs[f.Name] = true
		}
		allMethods := append(goCompilerMethods, goModMethods...)
		for _, m := range allMethods {
			if !rtFuncs[m] {
				t.Errorf("round-trip missing method: %s", m)
			}
		}
	}

	t.Logf("PASS: self-host validation complete — %d files, %d services, %d total methods",
		len(pkg.Files), 2, len(goCompilerMethods)+len(goModMethods))
}

// TestSelfOnboardGluon runs the onboard pipeline on gluon's own source files.
// It parses the Go and GoMod server implementations from the parent directory,
// analyzing the actual server struct types and their methods.
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
