package gluon

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/accretional/gluon/pb"
	callgraphPkg "golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/static"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
	"google.golang.org/grpc"
)

func TestNewGoServer(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not found")
	}
	srv, err := NewGoServer()
	if err != nil {
		t.Fatal(err)
	}
	if srv.goBinary == "" {
		t.Error("goBinary should not be empty")
	}
}

func TestVersion(t *testing.T) {
	srv := mustGoServer(t)
	ctx := context.Background()

	resp, err := srv.Version(ctx, &pb.Nothing{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resp.GetText(), "go version") {
		t.Errorf("unexpected version output: %q", resp.GetText())
	}
}

func TestCommand(t *testing.T) {
	srv := mustGoServer(t)
	ctx := context.Background()

	// Valid command
	resp, err := srv.Command(ctx, &pb.Text{Text: "version"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.GetText(), "go version") {
		t.Errorf("expected version output, got: %q", resp.GetText())
	}

	// Empty command should fail
	_, err = srv.Command(ctx, &pb.Text{})
	if err == nil {
		t.Error("expected error for empty command")
	}
}

func TestHelp(t *testing.T) {
	srv := mustGoServer(t)
	ctx := context.Background()

	resp, err := srv.Help(ctx, &pb.Text{Text: "build"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.GetText(), "build") {
		t.Errorf("help output should mention build: %q", resp.GetText())
	}

	// Empty topic
	resp, err = srv.Help(ctx, &pb.Text{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetText() == "" {
		t.Error("help with no topic should still produce output")
	}
}

func TestEnv(t *testing.T) {
	srv := mustGoServer(t)
	ctx := context.Background()

	// Query specific vars
	resp, err := srv.Env(ctx, &pb.GoEnvRequest{Vars: []string{"GOPATH"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(resp.GetText()) == "" {
		t.Error("GOPATH should not be empty")
	}

	// JSON output
	resp, err = srv.Env(ctx, &pb.GoEnvRequest{Json: true, Vars: []string{"GOPATH"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.GetText(), "GOPATH") {
		t.Error("JSON env output should contain GOPATH key")
	}
}

func TestDoc(t *testing.T) {
	srv := mustGoServer(t)
	ctx := context.Background()

	resp, err := srv.Doc(ctx, &pb.GoDocQuery{Pkg: "fmt", Symbol: "Println"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.GetText(), "Println") {
		t.Errorf("doc should mention Println: %q", resp.GetText())
	}
}

func TestDocFlags(t *testing.T) {
	srv := mustGoServer(t)
	ctx := context.Background()

	resp, err := srv.Doc(ctx, &pb.GoDocQuery{
		Pkg:   "fmt",
		Flags: []pb.GoDocQuery_GoDocQueryFlags{pb.GoDocQuery_short},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetText() == "" {
		t.Error("short doc should produce output")
	}
}

func TestBuild(t *testing.T) {
	srv := mustGoServer(t)
	ctx := context.Background()
	dir := makeTempModule(t)

	// Build should succeed in the temp module dir
	// We need to set the working directory — use Command instead
	resp, err := srv.Command(ctx, &pb.Text{Text: "build -C " + dir + " ./..."})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp
}

func TestTest(t *testing.T) {
	srv := mustGoServer(t)
	ctx := context.Background()
	dir := makeTempModule(t)

	// Run tests in temp module (no test files, should be fine)
	resp, err := srv.Command(ctx, &pb.Text{Text: "test -C " + dir + " ./..."})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp
}

func TestList(t *testing.T) {
	srv := mustGoServer(t)
	ctx := context.Background()

	// List with modules flag — list std
	resp, err := srv.List(ctx, &pb.GoListRequest{
		Packages: []string{"fmt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.GetText(), "fmt") {
		t.Errorf("list should contain fmt: %q", resp.GetText())
	}
}

func TestVet(t *testing.T) {
	srv := mustGoServer(t)
	ctx := context.Background()
	dir := makeTempModule(t)

	resp, err := srv.Command(ctx, &pb.Text{Text: "vet -C " + dir + " ./..."})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp
}

func TestTool(t *testing.T) {
	srv := mustGoServer(t)
	ctx := context.Background()

	// List tools (no name = just list)
	resp, err := srv.Tool(ctx, &pb.GoToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetText() == "" {
		t.Error("tool list should produce output")
	}
}

func TestFormat(t *testing.T) {
	srv := mustGoServer(t)
	ctx := context.Background()
	dir := makeTempModule(t)

	resp, err := srv.Command(ctx, &pb.Text{Text: "fmt -C " + dir + " ./..."})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp
}

func TestGetRequiresPackage(t *testing.T) {
	srv := mustGoServer(t)
	ctx := context.Background()

	_, err := srv.Get(ctx, &pb.GoGetRequest{})
	if err == nil {
		t.Error("expected error for empty packages")
	}
}

func TestInstallRequiresPackage(t *testing.T) {
	srv := mustGoServer(t)
	ctx := context.Background()

	_, err := srv.Install(ctx, &pb.GoInstallRequest{})
	if err == nil {
		t.Error("expected error for empty packages")
	}
}

func TestDescribeFixAnalyzerRequiresName(t *testing.T) {
	srv := mustGoServer(t)
	ctx := context.Background()

	_, err := srv.DescribeFixAnalyzer(ctx, &pb.Text{})
	if err == nil {
		t.Error("expected error for empty analyzer name")
	}
}

func TestTargetEnv(t *testing.T) {
	// nil target
	if env := targetEnv(nil); env != nil {
		t.Errorf("nil target should produce nil env, got %v", env)
	}

	// Empty target (no OS or arch set)
	if env := targetEnv(&pb.GoCompilerTarget{}); env != nil {
		t.Errorf("empty target should produce nil env, got %v", env)
	}

	// Linux amd64
	env := targetEnv(&pb.GoCompilerTarget{
		Os:   pb.GoCompilerTarget_linux,
		Arch: pb.GoCompilerTarget_amd64,
	})
	if env["GOOS"] != "linux" {
		t.Errorf("GOOS = %q, want linux", env["GOOS"])
	}
	if env["GOARCH"] != "amd64" {
		t.Errorf("GOARCH = %q, want amd64", env["GOARCH"])
	}

	// Darwin arm64
	env = targetEnv(&pb.GoCompilerTarget{
		Os:   pb.GoCompilerTarget_darwin,
		Arch: pb.GoCompilerTarget_arm64,
	})
	if env["GOOS"] != "darwin" {
		t.Errorf("GOOS = %q, want darwin", env["GOOS"])
	}
	if env["GOARCH"] != "arm64" {
		t.Errorf("GOARCH = %q, want arm64", env["GOARCH"])
	}

	// Windows 386
	env = targetEnv(&pb.GoCompilerTarget{
		Os:   pb.GoCompilerTarget_windows,
		Arch: pb.GoCompilerTarget__386,
	})
	if env["GOOS"] != "windows" {
		t.Errorf("GOOS = %q, want windows", env["GOOS"])
	}
	if env["GOARCH"] != "386" {
		t.Errorf("GOARCH = %q, want 386", env["GOARCH"])
	}

	// ARM only
	env = targetEnv(&pb.GoCompilerTarget{
		Arch: pb.GoCompilerTarget_arm,
	})
	if env["GOARCH"] != "arm" {
		t.Errorf("GOARCH = %q, want arm", env["GOARCH"])
	}
	if _, ok := env["GOOS"]; ok {
		t.Error("GOOS should not be set when OS is unspecified")
	}
}

func TestAppendTags(t *testing.T) {
	// No tags
	args := appendTags([]string{"build"}, nil)
	if len(args) != 1 {
		t.Errorf("no tags should not add args, got %v", args)
	}

	// With tags
	args = appendTags([]string{"build"}, []string{"integration", "e2e"})
	if len(args) != 3 {
		t.Errorf("expected 3 args, got %v", args)
	}
	if args[1] != "-tags" {
		t.Errorf("expected -tags, got %s", args[1])
	}
	if args[2] != "integration,e2e" {
		t.Errorf("expected joined tags, got %s", args[2])
	}
}

func TestRegisterServices(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not found")
	}
	srv := grpcServer()
	if err := RegisterServices(srv); err != nil {
		t.Fatal(err)
	}
	// Server should have services registered — we can't easily introspect,
	// but at least it didn't panic or error.
}

// mustGoServer creates a GoServer or skips the test.
func mustGoServer(t *testing.T) *GoServer {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not found")
	}
	srv, err := NewGoServer()
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

// makeTempModule creates a temp directory with a valid go.mod and a trivial .go file.
func makeTempModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/go.mod", []byte("module test/temp\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/main.go", []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// grpcServer creates a bare gRPC server for registration tests.
func grpcServer() *grpc.Server {
	return grpc.NewServer()
}

// TestDfsFolded_Diamond verifies that the visited map prevents exponential
// re-exploration of shared nodes in a diamond DAG.
//
// Graph: A→B, A→C, B→D, C→D, D→E, D→F (E and F are leaves)
// Without visited: 4 lines (D's subtree explored twice, once from B, once from C)
// With visited:    3 lines (D explored from B, then treated as leaf from C)
func TestDfsFolded_Diamond(t *testing.T) {
	_, byName := loadTestCallgraph(t, `package main
func A() { B(); C() }
func B() { D() }
func C() { D() }
func D() { E(); F() }
func E() {}
func F() {}
func main() {}
`)

	nodeA := byName["A"]
	if nodeA == nil {
		t.Fatal("function A not found in callgraph")
	}

	var lines []string
	onPath := make(map[int]bool)
	visited := make(map[int]bool)
	if err := dfsFolded(nodeA, nil, onPath, visited, false, func(line string) error {
		lines = append(lines, line)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// After the B→D subtree is explored, D is marked visited.
	// When C reaches D, D is emitted as a leaf rather than re-explored.
	// So we expect exactly 3 lines, not 4.
	if len(lines) != 3 {
		t.Errorf("want 3 lines, got %d: %v", len(lines), lines)
	}
}

// TestDfsFolded_Cycle verifies that a mutual-recursion cycle terminates.
func TestDfsFolded_Cycle(t *testing.T) {
	_, byName := loadTestCallgraph(t, `package main
func Ping() { Pong() }
func Pong() { Ping() }
func main() {}
`)

	nodePing := byName["Ping"]
	if nodePing == nil {
		t.Fatal("function Ping not found in callgraph")
	}

	var lines []string
	onPath := make(map[int]bool)
	visited := make(map[int]bool)
	if err := dfsFolded(nodePing, nil, onPath, visited, false, func(line string) error {
		lines = append(lines, line)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(lines) == 0 {
		t.Error("expected at least one emitted line for a cycle")
	}
}

// loadTestCallgraph builds an SSA static callgraph from inline Go source and
// returns a map of short function name → callgraph node.
func loadTestCallgraph(t *testing.T, src string) (*callgraphPkg.Graph, map[string]*callgraphPkg.Node) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/go.mod", []byte("module testcg\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/main.go", []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &packages.Config{Mode: packages.LoadAllSyntax, Dir: dir}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil || len(pkgs) == 0 {
		t.Fatalf("packages.Load: %v", err)
	}

	prog, ssaPkgs := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()
	cg := static.CallGraph(prog)
	_ = ssaPkgs

	byName := make(map[string]*callgraphPkg.Node)
	for fn, node := range cg.Nodes {
		if fn != nil {
			byName[fn.Name()] = node
		}
	}
	return cg, byName
}
