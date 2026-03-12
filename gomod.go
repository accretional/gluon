package gluon

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/accretional/gluon/pb"
	"github.com/accretional/runrpc/commander"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GoModServer implements the gluon.GoMod gRPC service.
type GoModServer struct {
	pb.UnimplementedGoModServer
	commander commander.CommanderServer
	goBinary  string
}

// NewGoModServer creates a new GoModServer.
func NewGoModServer() (*GoModServer, error) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("go binary not found in PATH: %w", err)
	}
	return &GoModServer{
		commander: commander.NewCommanderServer(),
		goBinary:  goBin,
	}, nil
}

func (s *GoModServer) runCommand(ctx context.Context, args ...string) (*pb.Text, error) {
	cmd := &commander.Command{
		Command: s.goBinary,
		Args:    args,
	}
	collector := &outputCollector{}
	stream := &unaryOutputStream{ctx: ctx, collector: collector}
	if err := s.commander.Shell(cmd, stream); err != nil {
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

// Download runs `go mod download`.
func (s *GoModServer) Download(ctx context.Context, req *pb.GoModDownloadRequest) (*pb.Text, error) {
	args := []string{"mod", "download"}
	if req.GetJson() {
		args = append(args, "-json")
	}
	if req.GetPrintCommands() {
		args = append(args, "-x")
	}
	args = append(args, req.GetModules()...)
	return s.runCommand(ctx, args...)
}

// Edit runs `go mod edit`.
func (s *GoModServer) Edit(ctx context.Context, req *pb.GoModEditRequest) (*pb.Text, error) {
	args := []string{"mod", "edit"}

	if req.GetModule() != "" {
		args = append(args, "-module", req.GetModule())
	}
	if req.GetGoVersion() != "" {
		args = append(args, "-go", req.GetGoVersion())
	}
	for _, r := range req.GetRequire() {
		args = append(args, "-require", r)
	}
	for _, r := range req.GetDropRequire() {
		args = append(args, "-droprequire", r)
	}
	for _, r := range req.GetReplace() {
		args = append(args, "-replace", r)
	}
	for _, r := range req.GetDropReplace() {
		args = append(args, "-dropreplace", r)
	}
	for _, e := range req.GetExclude() {
		args = append(args, "-exclude", e)
	}
	for _, e := range req.GetDropExclude() {
		args = append(args, "-dropexclude", e)
	}
	for _, r := range req.GetRetract() {
		args = append(args, "-retract", r)
	}
	for _, r := range req.GetDropRetract() {
		args = append(args, "-dropretract", r)
	}
	if req.GetJson() {
		args = append(args, "-json")
	}
	if req.GetPrint() {
		args = append(args, "-print")
	}
	if req.GetFmt() {
		args = append(args, "-fmt")
	}

	return s.runCommand(ctx, args...)
}

// Graph runs `go mod graph`.
func (s *GoModServer) Graph(ctx context.Context, req *pb.GoModGraphRequest) (*pb.Text, error) {
	args := []string{"mod", "graph"}
	if req.GetGoVersion() != "" {
		args = append(args, "-go", req.GetGoVersion())
	}
	return s.runCommand(ctx, args...)
}

// Init runs `go mod init`.
func (s *GoModServer) Init(ctx context.Context, req *pb.GoModInitRequest) (*pb.Text, error) {
	args := []string{"mod", "init"}
	if req.GetModulePath() != "" {
		args = append(args, req.GetModulePath())
	}
	return s.runCommand(ctx, args...)
}

// Tidy runs `go mod tidy`.
func (s *GoModServer) Tidy(ctx context.Context, req *pb.GoModTidyRequest) (*pb.Text, error) {
	args := []string{"mod", "tidy"}
	if req.GetVerbose() {
		args = append(args, "-v")
	}
	if req.GetGoVersion() != "" {
		args = append(args, "-go", req.GetGoVersion())
	}
	if req.GetDiff() {
		args = append(args, "-diff")
	}
	return s.runCommand(ctx, args...)
}

// Vendor runs `go mod vendor`.
func (s *GoModServer) Vendor(ctx context.Context, req *pb.GoModVendorRequest) (*pb.Text, error) {
	args := []string{"mod", "vendor"}
	if req.GetVerbose() {
		args = append(args, "-v")
	}
	if req.GetOutputDir() != "" {
		args = append(args, "-o", req.GetOutputDir())
	}
	return s.runCommand(ctx, args...)
}

// Verify runs `go mod verify`.
func (s *GoModServer) Verify(ctx context.Context, _ *pb.GoModVerifyRequest) (*pb.Text, error) {
	return s.runCommand(ctx, "mod", "verify")
}

// Why runs `go mod why`.
func (s *GoModServer) Why(ctx context.Context, req *pb.GoModWhyRequest) (*pb.Text, error) {
	args := []string{"mod", "why"}
	if req.GetModules() {
		args = append(args, "-m")
	}
	if req.GetVendor() {
		args = append(args, "-vendor")
	}
	pkgs := req.GetPackages()
	if len(pkgs) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one package or module is required")
	}
	args = append(args, pkgs...)
	return s.runCommand(ctx, args...)
}
