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

	cg, err := s.loadAndBuild(req)
	if err != nil {
		return err
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

// RunFolded builds the callgraph, performs a DFS from root_folder, and
// streams folded stack lines ready for FlameGraph consumption.
// Each line is of the form "func1;func2;func3 1".
// If exclude_stdlib is set, any node whose package is part of the Go standard
// library is treated as a leaf (its subtree is not explored).
func (s *CallGraphServer) RunFolded(req *pb.CallGraphFoldedRequest, stream grpc.ServerStreamingServer[pb.Text]) error {
	ctx := stream.Context()

	root := req.GetRootFolder()
	if root == "" {
		return status.Error(codes.InvalidArgument, "root_folder is required for RunFolded")
	}

	cg, err := s.loadAndBuild(req.GetCallgraphRequest())
	if err != nil {
		return err
	}

	// Find the root node by matching function name.
	var rootNode *callgraph.Node
	for fn, node := range cg.Nodes {
		if fn != nil && fn.String() == root {
			rootNode = node
			break
		}
	}
	if rootNode == nil {
		return status.Errorf(codes.NotFound, "root_folder %q not found in callgraph", root)
	}

	// DFS from root, emitting folded stack lines.
	onPath := make(map[int]bool)
	visited := make(map[int]bool)
	return dfsFolded(rootNode, nil, onPath, visited, req.GetExcludeStdlib(), func(line string) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		return stream.Send(&pb.Text{Text: line})
	})
}

// isStdlib reports whether a callgraph node belongs to the Go standard library.
// Standard library packages have no module path (their import path contains no
// dot in the first path element), e.g. "fmt", "os", "internal/bytealg".
func isStdlib(node *callgraph.Node) bool {
	if node.Func == nil || node.Func.Package() == nil {
		return false
	}
	pkg := node.Func.Package().Pkg
	if pkg == nil {
		return false
	}
	path := pkg.Path()
	// A stdlib package path never contains a dot before the first slash.
	for _, c := range path {
		if c == '.' {
			return false
		}
		if c == '/' {
			break
		}
	}
	return true
}

// dfsFolded performs a DFS from node, building the call path. At each leaf,
// cycle, or already-visited node it emits a folded stack line via emit.
// onPath tracks nodes on the current DFS path for cycle detection.
// visited tracks nodes whose full subtrees have already been explored,
// preventing exponential re-traversal of shared nodes in a DAG.
// If excludeStdlib is true, stdlib nodes are treated as leaves.
func dfsFolded(node *callgraph.Node, path []string, onPath map[int]bool, visited map[int]bool, excludeStdlib bool, emit func(string) error) error {
	name := node.Func.String()
	path = append(path, name)

	// Cycle on current path → emit as leaf.
	if onPath[node.ID] {
		return emit(strings.Join(path, ";") + " 1")
	}
	// Already fully explored, stdlib boundary (if requested), or true leaf → emit as leaf.
	if visited[node.ID] || len(node.Out) == 0 || (excludeStdlib && isStdlib(node)) {
		return emit(strings.Join(path, ";") + " 1")
	}

	onPath[node.ID] = true
	for _, edge := range node.Out {
		if err := dfsFolded(edge.Callee, path, onPath, visited, excludeStdlib, emit); err != nil {
			return err
		}
	}
	delete(onPath, node.ID)
	visited[node.ID] = true // mark subtree done; won't be re-explored
	return nil
}

// loadAndBuild is shared by Run and RunFolded: loads packages, builds SSA,
// and returns the callgraph.
func (s *CallGraphServer) loadAndBuild(req *pb.CallGraphRequest) (*callgraph.Graph, error) {
	pkgs := req.GetPackages()
	if len(pkgs) == 0 {
		pkgs = []string{"."}
	}

	cfg := &packages.Config{
		Mode:  packages.LoadAllSyntax,
		Tests: req.GetTest(),
		Dir:   dirForPatterns(pkgs),
	}
	initial, err := packages.Load(cfg, pkgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to load packages: %v", err)
	}
	if len(initial) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "no packages found for patterns: %v", pkgs)
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
		return nil, status.Errorf(codes.InvalidArgument, "all packages failed to load for patterns: %v", pkgs)
	}

	prog, ssaPkgs := ssautil.AllPackages(clean, ssa.InstantiateGenerics)
	prog.Build()

	return buildCallGraph(req.GetAlgo(), prog, ssaPkgs)
}

// dirForPatterns returns a working directory to use when any pattern is a
// filesystem path (starts with ./ ../ or /). It resolves the base directory
// of the first such pattern so that go/packages loads it in its own module
// context. Returns "" (use process cwd) if all patterns are module paths.
func dirForPatterns(patterns []string) string {
	for _, p := range patterns {
		if strings.HasPrefix(p, "../") || filepath.IsAbs(p) {
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
	case pb.CallGraphRequest_rta:
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
	default: // vta or ALGORITHM_UNKNOWN → vta
		return vta.CallGraph(ssautil.AllFunctions(prog), cha.CallGraph(prog)), nil
	}
}
