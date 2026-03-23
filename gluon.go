// Package gluon wraps the Go compiler toolchain as a gRPC service.
//
// It embeds the go binary and exposes it via the Go service defined in go.proto.
// The Command RPC is a general-purpose escape hatch that runs arbitrary
// `go` subcommands via the Commander from github.com/accretional/runrpc.
package gluon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/accretional/gluon/pb"
	"github.com/accretional/runrpc/commander"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// GoServer implements the gluon.Go gRPC service.
type GoServer struct {
	pb.UnimplementedGoServer
	commander commander.CommanderServer
	goBinary  string // path to go binary
}

// NewGoServer creates a new GoServer. It locates the go binary in PATH.
func NewGoServer() (*GoServer, error) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("go binary not found in PATH: %w", err)
	}
	return &GoServer{
		commander: commander.NewCommanderServer(),
		goBinary:  goBin,
	}, nil
}

// runCommand executes the go binary with the given arguments via Commander,
// collects all output, and returns it as a Text response.
func (s *GoServer) runCommand(ctx context.Context, args ...string) (*pb.Text, error) {
	return s.runCommandWithEnv(ctx, nil, args...)
}

// runCommandWithEnv executes the go binary with environment overrides.
func (s *GoServer) runCommandWithEnv(ctx context.Context, env map[string]string, args ...string) (*pb.Text, error) {
	cmd := &commander.Command{
		Command: s.goBinary,
		Args:    args,
		Env:     env,
	}

	collector := &outputCollector{}
	stream := &unaryOutputStream{
		ctx:       ctx,
		collector: collector,
	}

	err := s.commander.Shell(cmd, stream)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "command execution failed: %v", err)
	}

	result := collector.stdout.String()
	if collector.stderr.Len() > 0 {
		if result != "" {
			result += "\n"
		}
		result += collector.stderr.String()
	}

	return &pb.Text{Text: result}, nil
}

// targetEnv builds an environment map from a GoCompilerTarget.
func targetEnv(t *pb.GoCompilerTarget) map[string]string {
	if t == nil {
		return nil
	}
	env := make(map[string]string)
	if s := goosString(t.GetOs()); s != "" {
		env["GOOS"] = s
	}
	if s := goarchString(t.GetArch()); s != "" {
		env["GOARCH"] = s
	}
	if len(env) == 0 {
		return nil
	}
	return env
}

// appendTags adds -tags flag if tags are present.
func appendTags(args []string, tags []string) []string {
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, ","))
	}
	return args
}

// Command executes a go subcommand and returns the combined output.
// The input text is parsed as command-line arguments to the go tool.
// Example: "version", "build ./...", "test -v ./pkg/..."
func (s *GoServer) Command(ctx context.Context, req *pb.Text) (*pb.Text, error) {
	if req.GetText() == "" {
		return nil, status.Error(codes.InvalidArgument, "command text cannot be empty")
	}
	args := strings.Fields(req.GetText())
	return s.runCommand(ctx, args...)
}

// Doc runs `go doc` with the given query parameters.
func (s *GoServer) Doc(ctx context.Context, req *pb.GoDocQuery) (*pb.Text, error) {
	args := []string{"doc"}

	for _, flag := range req.GetFlags() {
		switch flag {
		case pb.GoDocQuery_all:
			args = append(args, "-all")
		case pb.GoDocQuery_case_sensitive:
			args = append(args, "-c")
		case pb.GoDocQuery_include_main_cmd:
			args = append(args, "-cmd")
		case pb.GoDocQuery_short:
			args = append(args, "-short")
		case pb.GoDocQuery_full_source:
			args = append(args, "-src")
		case pb.GoDocQuery_include_unexported:
			args = append(args, "-u")
		}
	}

	if req.GetPkg() != "" {
		args = append(args, req.GetPkg())
	}

	if req.GetSymbol() != "" {
		sym := req.GetSymbol()
		if len(req.GetMethodOrField()) > 0 {
			sym += "." + strings.Join(req.GetMethodOrField(), ".")
		}
		args = append(args, sym)
	}

	return s.runCommand(ctx, args...)
}

func goosString(os pb.GoCompilerTarget_GOOS) string {
	switch os {
	case pb.GoCompilerTarget_linux:
		return "linux"
	case pb.GoCompilerTarget_darwin:
		return "darwin"
	case pb.GoCompilerTarget_windows:
		return "windows"
	default:
		return ""
	}
}

func goarchString(arch pb.GoCompilerTarget_GOARCH) string {
	switch arch {
	case pb.GoCompilerTarget_amd64:
		return "amd64"
	case pb.GoCompilerTarget__386:
		return "386"
	case pb.GoCompilerTarget_arm:
		return "arm"
	case pb.GoCompilerTarget_arm64:
		return "arm64"
	default:
		return ""
	}
}

// Fix runs `go fix` with the given query parameters.
func (s *GoServer) Fix(ctx context.Context, req *pb.GoFixQuery) (*pb.Text, error) {
	args := []string{"fix"}

	if req.GetDiff() {
		args = append(args, "-diff")
	}

	for _, a := range req.GetAnalyzers() {
		args = append(args, "-fix", a)
	}

	if req.GetPkg() != "" {
		args = append(args, req.GetPkg())
	} else if req.GetPath() != "" {
		args = append(args, req.GetPath())
	}

	return s.runCommandWithEnv(ctx, targetEnv(req.GetTarget()), args...)
}

// ListFixAnalyzers runs `go fix -help` to list available fix analyzers.
func (s *GoServer) ListFixAnalyzers(ctx context.Context, _ *pb.Nothing) (*pb.Text, error) {
	return s.runCommand(ctx, "fix", "-help")
}

// DescribeFixAnalyzer runs `go fix -help` and returns info about a specific analyzer.
func (s *GoServer) DescribeFixAnalyzer(ctx context.Context, req *pb.Text) (*pb.Text, error) {
	if req.GetText() == "" {
		return nil, status.Error(codes.InvalidArgument, "analyzer name cannot be empty")
	}
	return s.runCommand(ctx, "fix", "-help")
}

// Format runs `gofmt` (or `go fmt`) with the given parameters.
func (s *GoServer) Format(ctx context.Context, req *pb.GoFormatRequest) (*pb.Text, error) {
	if req.GetGoExpression() != "" {
		gofmtBin, err := exec.LookPath("gofmt")
		if err != nil {
			return s.runCommand(ctx, "fmt")
		}

		args := []string{}
		if req.GetPrintAllErrors() {
			args = append(args, "-e")
		}
		if req.GetSimplify() {
			args = append(args, "-s")
		}
		if rr := req.GetRewriteRule(); rr != nil && rr.GetSourceExpression() != "" {
			args = append(args, "-r", rr.GetSourceExpression()+" -> "+rr.GetTargetExpression())
		}
		switch req.GetPrintOptions() {
		case pb.GoFormatRequest_Diffs:
			args = append(args, "-d")
		case pb.GoFormatRequest_Replace:
			args = append(args, "-w")
		case pb.GoFormatRequest_Filenames:
			args = append(args, "-l")
		}

		cmd := &commander.Command{
			Command: gofmtBin,
			Args:    args,
		}
		collector := &outputCollector{}
		stream := &unaryOutputStream{ctx: ctx, collector: collector}
		if err := s.commander.Shell(cmd, stream); err != nil {
			return nil, status.Errorf(codes.Internal, "gofmt failed: %v", err)
		}
		result := collector.stdout.String()
		if collector.stderr.Len() > 0 {
			if result != "" {
				result += "\n"
			}
			result += collector.stderr.String()
		}
		return &pb.Text{Text: result}, nil
	}

	args := []string{"fmt"}
	pkgs := req.GetPackages()
	if len(pkgs) == 0 {
		pkgs = []string{"./..."}
	}
	args = append(args, pkgs...)
	return s.runCommand(ctx, args...)
}

// Get runs `go get` with the given parameters.
func (s *GoServer) Get(ctx context.Context, req *pb.GoGetRequest) (*pb.Text, error) {
	args := []string{"get"}
	if req.GetUpgrade() {
		args = append(args, "-u")
	}
	if req.GetIncludeTests() {
		args = append(args, "-t")
	}
	pkgs := req.GetPackages()
	if len(pkgs) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one package is required")
	}
	args = append(args, pkgs...)
	return s.runCommand(ctx, args...)
}

// Install runs `go install` with the given packages.
func (s *GoServer) Install(ctx context.Context, req *pb.GoInstallRequest) (*pb.Text, error) {
	args := []string{"install"}
	pkgs := req.GetPackages()
	if len(pkgs) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one package is required")
	}
	args = append(args, pkgs...)
	return s.runCommand(ctx, args...)
}

// List runs `go list` with the given parameters.
func (s *GoServer) List(ctx context.Context, req *pb.GoListRequest) (*pb.Text, error) {
	args := []string{"list"}
	if req.GetModules() {
		args = append(args, "-m")
	}
	if req.GetJson() {
		args = append(args, "-json")
	}
	if req.GetUpgradeInfo() {
		args = append(args, "-u")
	}
	if req.GetRetracted() {
		args = append(args, "-retracted")
	}
	if req.GetVersions() {
		args = append(args, "-versions")
	}
	pkgs := req.GetPackages()
	if len(pkgs) == 0 {
		if req.GetModules() {
			pkgs = []string{"all"}
		} else {
			pkgs = []string{"./..."}
		}
	}
	args = append(args, pkgs...)
	return s.runCommand(ctx, args...)
}

// Env runs `go env` with the given parameters.
func (s *GoServer) Env(ctx context.Context, req *pb.GoEnvRequest) (*pb.Text, error) {
	args := []string{"env"}
	if req.GetJson() {
		args = append(args, "-json")
	}
	if req.GetChanged() {
		args = append(args, "-changed")
	}
	for _, v := range req.GetSet() {
		args = append(args, "-w", v.GetName()+"="+v.GetValue())
	}
	for _, name := range req.GetUnset() {
		args = append(args, "-u", name)
	}
	args = append(args, req.GetVars()...)
	return s.runCommand(ctx, args...)
}

// Build runs `go build` with the given parameters.
func (s *GoServer) Build(ctx context.Context, req *pb.GoBuildRequest) (*pb.Text, error) {
	args := []string{"build"}
	if req.GetOutput() != "" {
		args = append(args, "-o", req.GetOutput())
	}
	if req.GetVerbose() {
		args = append(args, "-v")
	}
	if req.GetRace() {
		args = append(args, "-race")
	}
	if req.GetLdflags() != "" {
		args = append(args, "-ldflags", req.GetLdflags())
	}
	args = appendTags(args, req.GetTags())
	if req.GetTrimpath() {
		args = append(args, "-trimpath")
	}
	if req.GetGcflags() != "" {
		args = append(args, "-gcflags", req.GetGcflags())
	}
	if req.GetAsmflags() != "" {
		args = append(args, "-asmflags", req.GetAsmflags())
	}
	if req.GetWork() {
		args = append(args, "-work")
	}
	pkgs := req.GetPackages()
	if len(pkgs) == 0 {
		pkgs = []string{"./..."}
	}
	args = append(args, pkgs...)
	return s.runCommandWithEnv(ctx, targetEnv(req.GetTarget()), args...)
}

// Run runs `go run` with the given parameters.
func (s *GoServer) Run(ctx context.Context, req *pb.GoRunRequest) (*pb.Text, error) {
	args := []string{"run"}
	if req.GetVerbose() {
		args = append(args, "-v")
	}
	if req.GetRace() {
		args = append(args, "-race")
	}
	args = appendTags(args, req.GetTags())
	if req.GetExecCmd() != "" {
		args = append(args, "-exec", req.GetExecCmd())
	}
	if req.GetWork() {
		args = append(args, "-work")
	}
	pkg := req.GetPackage()
	if pkg == "" {
		pkg = "."
	}
	args = append(args, pkg)
	args = append(args, req.GetArgs()...)
	return s.runCommandWithEnv(ctx, targetEnv(req.GetTarget()), args...)
}

// Test runs `go test` with the given parameters.
func (s *GoServer) Test(ctx context.Context, req *pb.GoTestRequest) (*pb.Text, error) {
	args := []string{"test"}
	if req.GetVerbose() {
		args = append(args, "-v")
	}
	if req.GetRace() {
		args = append(args, "-race")
	}
	if req.GetJson() {
		args = append(args, "-json")
	}
	args = appendTags(args, req.GetTags())
	if req.GetShort() {
		args = append(args, "-short")
	}
	if req.GetFailfast() {
		args = append(args, "-failfast")
	}
	if req.GetCover() {
		args = append(args, "-cover")
	}
	if req.GetCoverprofile() != "" {
		args = append(args, "-coverprofile", req.GetCoverprofile())
	}
	if req.GetRun() != "" {
		args = append(args, "-run", req.GetRun())
	}
	if req.GetBench() != "" {
		args = append(args, "-bench", req.GetBench())
	}
	if req.GetBenchmem() {
		args = append(args, "-benchmem")
	}
	if req.GetBenchtime() != "" {
		args = append(args, "-benchtime", req.GetBenchtime())
	}
	if req.GetCount() > 0 {
		args = append(args, "-count", fmt.Sprintf("%d", req.GetCount()))
	}
	if req.GetTimeout() != "" {
		args = append(args, "-timeout", req.GetTimeout())
	}
	pkgs := req.GetPackages()
	if len(pkgs) == 0 {
		pkgs = []string{"./..."}
	}
	args = append(args, pkgs...)
	return s.runCommandWithEnv(ctx, targetEnv(req.GetTarget()), args...)
}

// Generate runs `go generate` with the given parameters.
func (s *GoServer) Generate(ctx context.Context, req *pb.GoGenerateRequest) (*pb.Text, error) {
	args := []string{"generate"}
	if req.GetVerbose() {
		args = append(args, "-v")
	}
	if req.GetDryRun() {
		args = append(args, "-n")
	}
	if req.GetPrintCommands() {
		args = append(args, "-x")
	}
	if req.GetRun() != "" {
		args = append(args, "-run", req.GetRun())
	}
	pkgs := req.GetPackages()
	if len(pkgs) == 0 {
		pkgs = []string{"./..."}
	}
	args = append(args, pkgs...)
	return s.runCommand(ctx, args...)
}

// Vet runs `go vet` with the given parameters.
func (s *GoServer) Vet(ctx context.Context, req *pb.GoVetRequest) (*pb.Text, error) {
	args := []string{"vet"}
	if req.GetVerbose() {
		args = append(args, "-v")
	}
	if req.GetJson() {
		args = append(args, "-json")
	}
	args = appendTags(args, req.GetTags())
	pkgs := req.GetPackages()
	if len(pkgs) == 0 {
		pkgs = []string{"./..."}
	}
	args = append(args, pkgs...)
	return s.runCommand(ctx, args...)
}

// Tool runs `go tool` with the given parameters.
func (s *GoServer) Tool(ctx context.Context, req *pb.GoToolRequest) (*pb.Text, error) {
	args := []string{"tool"}
	if req.GetDryRun() {
		args = append(args, "-n")
	}
	if req.GetName() != "" {
		args = append(args, req.GetName())
		args = append(args, req.GetArgs()...)
	}
	return s.runCommand(ctx, args...)
}

// Help runs `go help` with an optional topic.
func (s *GoServer) Help(ctx context.Context, req *pb.Text) (*pb.Text, error) {
	args := []string{"help"}
	if topic := strings.TrimSpace(req.GetText()); topic != "" {
		args = append(args, strings.Fields(topic)...)
	}
	return s.runCommand(ctx, args...)
}

// Version runs `go version`.
func (s *GoServer) Version(ctx context.Context, _ *pb.Nothing) (*pb.Text, error) {
	return s.runCommand(ctx, "version")
}

// outputCollector accumulates stdout and stderr from Commander streaming output.
type outputCollector struct {
	stdout bytes.Buffer
	stderr bytes.Buffer
}

// unaryOutputStream adapts the Commander's server-streaming interface to collect
// all output into buffers, so we can return a single unary response.
type unaryOutputStream struct {
	grpc.ServerStreamingServer[commander.Output]
	ctx       context.Context
	collector *outputCollector
}

func (s *unaryOutputStream) Send(out *commander.Output) error {
	if out.Stdout {
		s.collector.stdout.Write(out.Data)
	} else {
		s.collector.stderr.Write(out.Data)
	}
	return nil
}

func (s *unaryOutputStream) Context() context.Context {
	return s.ctx
}

func (s *unaryOutputStream) SendMsg(m any) error          { return nil }
func (s *unaryOutputStream) RecvMsg(m any) error          { return io.EOF }
func (s *unaryOutputStream) SetHeader(metadata.MD) error  { return nil }
func (s *unaryOutputStream) SendHeader(metadata.MD) error { return nil }
func (s *unaryOutputStream) SetTrailer(metadata.MD)       {}

// RegisterServices registers all gluon services on the given gRPC server
// and enables reflection.
func RegisterServices(srv *grpc.Server) error {
	goSrv, err := NewGoServer()
	if err != nil {
		return err
	}
	modSrv, err := NewGoModServer()
	if err != nil {
		return err
	}
	pb.RegisterGoServer(srv, goSrv)
	pb.RegisterGoModServer(srv, modSrv)
	commander.RegisterCommanderServer(srv, commander.NewCommanderServer())
	registerTraceServices(srv)
	reflection.Register(srv)
	return nil
}
