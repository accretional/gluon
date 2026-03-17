package gluon

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/accretional/gluon/pb"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/callgraph/static"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CallGraphServer implements the gluon.CallGraph gRPC service.
type CallGraphServer struct {
	pb.UnimplementedCallGraphServer
}

// NewCallGraphServer creates a new CallGraphServer.
func NewCallGraphServer() *CallGraphServer {
	return &CallGraphServer{}
}

// Run builds a full callgraph for the requested packages and all transitive
// dependencies, then streams one CallGraphEdge per call edge found.
func (s *CallGraphServer) Run(req *pb.CallGraphRequest, stream grpc.ServerStreamingServer[pb.CallGraphEdge]) error {
	ctx := stream.Context()

	pkgs := req.GetPackages()
	if len(pkgs) == 0 {
		pkgs = []string{"."}
	}

	cfg := &packages.Config{
		Context: ctx,
		Mode:    packages.LoadAllSyntax,
		Tests:   req.GetTest(),
		Dir:     dirForPatterns(pkgs),
	}
	initial, err := packages.Load(cfg, pkgs...)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to load packages: %v", err)
	}
	if len(initial) == 0 {
		return status.Errorf(codes.InvalidArgument, "no packages found for patterns: %v", pkgs)
	}
	var clean []*packages.Package
	for _, pkg := range initial {
		if len(pkg.Errors) > 0 {
			var errs []string
			for _, e := range pkg.Errors {
				errs = append(errs, e.Error())
			}
			log.Printf("warning: skipping package %q due to load errors: %s", pkg.PkgPath, fmt.Sprintf("%v", errs))
			continue
		}
		clean = append(clean, pkg)

	}
	if len(clean) == 0 {
		return status.Errorf(codes.InvalidArgument, "all packages failed to load for patterns: %v", pkgs)
	}
	initial = clean

	prog, ssaPkgs := ssautil.AllPackages(initial, ssa.InstantiateGenerics)
	prog.Build()

	cg, err := buildCallGraph(req.GetAlgo(), prog, ssaPkgs)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to build callgraph: %v", err)
	}

	return callgraph.GraphVisitEdges(cg, func(e *callgraph.Edge) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		return stream.Send(&pb.CallGraphEdge{
			Caller: e.Caller.Func.String(),
			Callee: e.Callee.Func.String(),
		})
	})
}

// dirForPatterns returns a working directory to use when any pattern is a
// filesystem path (starts with ./ ../ or /). It resolves the base directory
// of the first such pattern so that go/packages loads it in its own module
// context. Returns "" (use process cwd) if all patterns are module paths.
func dirForPatterns(patterns []string) string {
	for _, p := range patterns {
		if strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") || filepath.IsAbs(p) {
			// Strip trailing /... to get the root directory
			base := strings.TrimSuffix(p, "/...")
			base = strings.TrimSuffix(base, "/.")
			abs, err := filepath.Abs(base)
			if err == nil {
				return abs
			}
		}
	}
	return ""
}

func buildCallGraph(algo pb.CallGraphRequest_Algorithm, prog *ssa.Program, roots []*ssa.Package) (*callgraph.Graph, error) {
	switch algo {
	case pb.CallGraphRequest_static:
		return static.CallGraph(prog), nil
	case pb.CallGraphRequest_cha:
		return cha.CallGraph(prog), nil
	case pb.CallGraphRequest_vta:
		return vta.CallGraph(ssautil.AllFunctions(prog), cha.CallGraph(prog)), nil
	default: // rta or ALGORITHM_UNKNOWN → rta
		var entries []*ssa.Function
		for _, pkg := range roots {
			if pkg == nil {
				continue
			}
			for _, mem := range pkg.Members {
				if fn, ok := mem.(*ssa.Function); ok {
					entries = append(entries, fn)
				}
			}
		}
		if len(entries) == 0 {
			return &callgraph.Graph{}, nil
		}
		res := rta.Analyze(entries, true)
		if res == nil || res.CallGraph == nil {
			return &callgraph.Graph{}, nil
		}
		return res.CallGraph, nil
	}
}
