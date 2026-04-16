package metaparser

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	astkitpkg "github.com/accretional/gluon/v2/astkit"
	"github.com/accretional/gluon/v2/astkit/server"
	pb "github.com/accretional/gluon/v2/pb"
	expr "github.com/accretional/proto-expr"
	"github.com/accretional/proto-expr/protosh"
)

// Transform is the RPC entry point. It parses the textproto script,
// pre-populates the "ast" register with the request's ASTDescriptor
// serialized as binary Data, wires the astkit Transformer as a handler
// under the "astkit://<Method>" URI scheme, runs the script, and
// returns the final Data.
//
// Errors from script parsing or handler invocation are translated to
// gRPC status codes (InvalidArgument for malformed input, Internal for
// handler failures).
func (s *Server) Transform(ctx context.Context, req *pb.TransformRequest) (*pb.TransformResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "nil request")
	}
	if req.GetAst() == nil {
		return nil, status.Error(codes.InvalidArgument, "ast required")
	}
	if req.GetScriptTextproto() == "" {
		return nil, status.Error(codes.InvalidArgument, "script_textproto required")
	}

	out, err := Transform(ctx, req.GetAst(), req.GetScriptTextproto())
	if err != nil {
		if errors.Is(err, errInvalidScript) {
			return nil, status.Errorf(codes.InvalidArgument, "Transform: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "Transform: %v", err)
	}
	return out, nil
}

// Transform is the pure-Go entry point behind the RPC. Split out so
// unit tests can drive it directly.
func Transform(ctx context.Context, ast *pb.ASTDescriptor, scriptTextproto string) (*pb.TransformResponse, error) {
	if ast == nil {
		return nil, fmt.Errorf("%w: ast required", errInvalidScript)
	}
	if scriptTextproto == "" {
		return nil, fmt.Errorf("%w: script_textproto required", errInvalidScript)
	}

	var script expr.ScriptDescriptor
	if err := prototext.Unmarshal([]byte(scriptTextproto), &script); err != nil {
		return nil, fmt.Errorf("%w: parse script textproto: %v", errInvalidScript, err)
	}

	if ast.GetRoot() == nil {
		return nil, fmt.Errorf("%w: ast.root required", errInvalidScript)
	}
	// Carry the bare ASTNode root through the pipeline: Transformer
	// handlers return ASTNodes, so storing the initial register as an
	// ASTNode keeps the wire format consistent across dispatches.
	astBytes, err := proto.Marshal(ast.GetRoot())
	if err != nil {
		return nil, fmt.Errorf("marshal ast: %w", err)
	}

	rt := protosh.New()
	registerAstkitHandlers(rt)

	// Pre-populate the "ast" register so scripts can reference it
	// via {Data{text: "ast"}} on their first Dispatch.
	astReg := &expr.Data{
		Type:     "gluon.v2.ASTNode",
		Encoding: &expr.Data_Binary{Binary: astBytes},
	}
	script.Statements = append([]*expr.StatementDescriptor{
		{Kind: &expr.StatementDescriptor_ConstVar{
			ConstVar: &expr.VariableDescriptor{Name: "ast", Data: astReg},
		}},
	}, script.Statements...)

	final, err := rt.Run(ctx, &script)
	if err != nil {
		return nil, fmt.Errorf("run script: %w", err)
	}

	out := &pb.TransformResponse{DataType: final.GetType()}
	switch enc := final.GetEncoding().(type) {
	case *expr.Data_Binary:
		out.DataBinary = enc.Binary
	case *expr.Data_Text:
		out.DataText = enc.Text
	case nil:
		// empty Data — leave payload fields empty.
	default:
		return nil, fmt.Errorf("unknown Data encoding %T", enc)
	}
	return out, nil
}

var errInvalidScript = errors.New("invalid script")

// registerAstkitHandlers wires the Transformer service (v2/astkit) as
// in-process handlers under URIs of the form "astkit://<Method>".
// The request Data payload is expected to be a marshaled ASTDescriptor
// (type "gluon.v2.ASTDescriptor"); for Find/FindAll/Count/Filter the
// kind and value predicates are parsed out of Data.type using a
// comma-delimited "kind=X,value=Y" convention — compact enough to fit
// in the Data.type field without adding extra request plumbing. For
// ReplaceKind/ReplaceValue, Data.type encodes "from=X,to=Y".
//
// This is deliberately minimal: the first real Transform use case
// (CST → AST cleanup) only needs Filter and Replace* on the tree,
// and the type-field encoding keeps the script textprotos tiny.
// If the Transform pipeline grows, we can swap in Any-wrapped typed
// request messages without breaking the "astkit://<Method>" scheme.
func registerAstkitHandlers(rt *protosh.Runtime) {
	t := server.New()
	register := func(method string, fn func(ctx context.Context, root *pb.ASTNode, params map[string]string) (*pb.ASTNode, *expr.Data, error)) {
		uri := "astkit://" + method
		rt.Register(uri, func(ctx context.Context, req *expr.Data) (*expr.Data, error) {
			root, err := unmarshalAST(req)
			if err != nil {
				return nil, fmt.Errorf("astkit://%s: %w", method, err)
			}
			params := parseParams(req.GetType())
			outRoot, rawOut, err := fn(ctx, root, params)
			if err != nil {
				return nil, err
			}
			if rawOut != nil {
				return rawOut, nil
			}
			return marshalAST(outRoot)
		})
	}

	register("Find", func(ctx context.Context, root *pb.ASTNode, p map[string]string) (*pb.ASTNode, *expr.Data, error) {
		resp, err := t.Find(ctx, &astkitpkg.FindRequest{Root: root, Kind: p["kind"], Value: p["value"]})
		if err != nil {
			return nil, nil, err
		}
		return resp.Node, nil, nil
	})
	register("Filter", func(ctx context.Context, root *pb.ASTNode, p map[string]string) (*pb.ASTNode, *expr.Data, error) {
		resp, err := t.Filter(ctx, &astkitpkg.FilterRequest{Root: root, Kind: p["kind"], Value: p["value"]})
		if err != nil {
			return nil, nil, err
		}
		return resp.Root, nil, nil
	})
	register("ReplaceKind", func(ctx context.Context, root *pb.ASTNode, p map[string]string) (*pb.ASTNode, *expr.Data, error) {
		resp, err := t.ReplaceKind(ctx, &astkitpkg.ReplaceRequest{Root: root, From: p["from"], To: p["to"]})
		if err != nil {
			return nil, nil, err
		}
		return resp.Root, nil, nil
	})
	register("ReplaceValue", func(ctx context.Context, root *pb.ASTNode, p map[string]string) (*pb.ASTNode, *expr.Data, error) {
		resp, err := t.ReplaceValue(ctx, &astkitpkg.ReplaceRequest{Root: root, From: p["from"], To: p["to"]})
		if err != nil {
			return nil, nil, err
		}
		return resp.Root, nil, nil
	})
}

func unmarshalAST(d *expr.Data) (*pb.ASTNode, error) {
	bin := d.GetBinary()
	if len(bin) == 0 {
		return nil, errors.New("empty Data.binary")
	}
	var node pb.ASTNode
	if err := proto.Unmarshal(bin, &node); err != nil {
		return nil, fmt.Errorf("unmarshal ast: %w", err)
	}
	return &node, nil
}

func marshalAST(root *pb.ASTNode) (*expr.Data, error) {
	bs, err := proto.Marshal(root)
	if err != nil {
		return nil, err
	}
	return &expr.Data{
		Type:     "gluon.v2.ASTNode",
		Encoding: &expr.Data_Binary{Binary: bs},
	}, nil
}

// parseParams splits "k=v,k2=v2" into a map. Unrecognised segments
// are ignored; this is a convenience syntax for small scripts, not
// a general-purpose parser.
func parseParams(s string) map[string]string {
	out := map[string]string{}
	i := 0
	for i < len(s) {
		// find next separator
		j := i
		for j < len(s) && s[j] != ',' {
			j++
		}
		seg := s[i:j]
		eq := -1
		for k := 0; k < len(seg); k++ {
			if seg[k] == '=' {
				eq = k
				break
			}
		}
		if eq > 0 && eq < len(seg)-1 {
			out[seg[:eq]] = seg[eq+1:]
		}
		i = j + 1
	}
	return out
}
