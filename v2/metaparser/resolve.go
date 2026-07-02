package metaparser

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/accretional/gluon/v2/builddep"
	pb "github.com/accretional/gluon/v2/pb"
)

// ResolveDependencies reads the request's Starlark build-dependency file and
// streams one DocumentDescriptor per declared grammar dependency, each carrying
// that dependency grammar's concatenated EBNF sources. The stream is meant to
// be fed (after the local grammar's own documents) into EBNF to compose a
// merged grammar. This RPC is non-hermetic: it reads the build file and the
// grammar sources it references from the local filesystem.
func (s *Server) ResolveDependencies(req *pb.ResolveRequest, stream pb.Metaparser_ResolveDependenciesServer) error {
	docs, err := ResolveDependencyDocs(req.GetBuildFile())
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "ResolveDependencies: %v", err)
	}
	for _, d := range docs {
		if err := stream.Send(d); err != nil {
			return err
		}
	}
	return nil
}

// ResolveDependencyDocs is the pure-Go entry point behind ResolveDependencies.
// It loads the build-dependency file and, for each declared dependency, reads
// and concatenates that dependency grammar's *.ebnf sources into a single
// DocumentDescriptor named after the dependency (uri = file://<grammar dir>).
func ResolveDependencyDocs(buildFile string) ([]*pb.DocumentDescriptor, error) {
	deps, err := builddep.Load(buildFile)
	if err != nil {
		return nil, err
	}
	var docs []*pb.DocumentDescriptor
	for _, d := range deps {
		if d.GrammarSrcs == "" {
			continue
		}
		src, err := concatEBNFDir(d.GrammarSrcs)
		if err != nil {
			return nil, fmt.Errorf("dependency %q: %w", d.Name, err)
		}
		doc := WrapString(src)
		doc.Name = d.Name
		doc.Uri = "file://" + d.GrammarSrcs
		docs = append(docs, doc)
	}
	return docs, nil
}

// concatEBNFDir reads every *.ebnf file in dir (lexically sorted) and
// concatenates them, newline-separated. Ordering within one dependency grammar
// does not matter: rule union is order-independent apart from the cross-document
// override policy in ParseEBNFStream.
func concatEBNFDir(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".ebnf" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "", fmt.Errorf("no .ebnf files in %s", dir)
	}
	var b strings.Builder
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String(), nil
}
