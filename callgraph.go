package gluon

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/accretional/gluon/pb"
	"github.com/accretional/runrpc/commander"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CallGraphServer implements the gluon.CallGraph gRPC service.
type CallGraphServer struct {
	pb.UnimplementedCallGraphServer
	commander       commander.CommanderServer
	callgraphBinary string
}

// NewCallGraphServer creates a new CallGraphServer. It locates the callgraph binary in PATH.
func NewCallGraphServer() (*CallGraphServer, error) {
	bin, err := exec.LookPath("callgraph")
	if err != nil {
		return nil, fmt.Errorf("callgraph binary not found in PATH: %w", err)
	}
	return &CallGraphServer{
		commander:       commander.NewCommanderServer(),
		callgraphBinary: bin,
	}, nil
}

func (s *CallGraphServer) runCommandText(ctx context.Context, args ...string) (*pb.Text, error) {
	cmd := &commander.Command{
		Command: s.callgraphBinary,
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

// Command runs callgraph with arbitrary arguments.
func (s *CallGraphServer) Command(ctx context.Context, req *pb.Text) (*pb.Text, error) {
	if req.GetText() == "" {
		return nil, status.Error(codes.InvalidArgument, "command text cannot be empty")
	}
	return s.runCommandText(ctx, strings.Fields(req.GetText())...)
}

// Run runs callgraph in digraph mode and streams one CallGraphEdge per call edge found.
func (s *CallGraphServer) Run(req *pb.CallGraphRequest, stream grpc.ServerStreamingServer[pb.CallGraphEdge]) error {
	ctx := stream.Context()

	args := []string{"-format=digraph"}

	if algo := algoString(req.GetAlgo()); algo != "" {
		args = append(args, "-algo="+algo)
	}
	if req.GetTest() {
		args = append(args, "-test")
	}

	pkgs := req.GetPackages()
	if len(pkgs) == 0 {
		pkgs = []string{"."}
	}
	args = append(args, pkgs...)

	cmd := exec.CommandContext(ctx, s.callgraphBinary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return status.Errorf(codes.Internal, "failed to get stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		return status.Errorf(codes.Internal, "failed to start callgraph: %v", err)
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		caller, callee, ok := parseDigraphEdge(line)
		if !ok {
			continue
		}
		if err := stream.Send(&pb.CallGraphEdge{Caller: caller, Callee: callee}); err != nil {
			cmd.Process.Kill()
			return err
		}
	}

	if err := cmd.Wait(); err != nil {
		return status.Errorf(codes.Internal, "callgraph exited with error: %v", err)
	}
	return nil
}

// parseDigraphEdge parses a digraph line of the form: "caller" "callee"
func parseDigraphEdge(line string) (caller, callee string, ok bool) {
	line = strings.TrimSpace(line)
	if len(line) == 0 {
		return "", "", false
	}
	// Both tokens are Go-quoted strings: "foo" "bar"
	// Find the boundary between the two quoted tokens.
	if line[0] != '"' {
		return "", "", false
	}
	// Find end of first quoted string
	end := 1
	for end < len(line) {
		if line[end] == '\\' {
			end += 2
			continue
		}
		if line[end] == '"' {
			end++
			break
		}
		end++
	}
	caller = strings.Trim(line[:end], `"`)
	callee = strings.Trim(strings.TrimSpace(line[end:]), `"`)
	if caller == "" || callee == "" {
		return "", "", false
	}
	return caller, callee, true
}

func algoString(algo pb.CallGraphRequest_Algorithm) string {
	switch algo {
	case pb.CallGraphRequest_static:
		return "static"
	case pb.CallGraphRequest_cha:
		return "cha"
	case pb.CallGraphRequest_rta:
		return "rta"
	case pb.CallGraphRequest_vta:
		return "vta"
	default:
		return "" // let tool use its default (rta)
	}
}
