package metaparser_test

import (
	"context"

	pb "github.com/accretional/gluon/v2/pb"
)

// ebnfStream drives the client-streaming EBNF RPC with one or more documents
// and returns the merged grammar. It mirrors the former unary EBNF call so the
// e2e tests can send a single document; multi-document callers exercise the
// merge/override path.
func ebnfStream(ctx context.Context, client pb.MetaparserClient, docs ...*pb.DocumentDescriptor) (*pb.GrammarDescriptor, error) {
	stream, err := client.EBNF(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range docs {
		if err := stream.Send(d); err != nil {
			return nil, err
		}
	}
	return stream.CloseAndRecv()
}
