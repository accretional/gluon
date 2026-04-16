// Package metaparser implements the v2 Metaparser gRPC service.
//
// It provides the bottom-up parsing pipeline: ReadBytes → ReadString →
// EBNF → CST. Each RPC has a plain-Go counterpart (ClassifyBytes,
// WrapString, …) so the logic can be unit-tested without the gRPC
// machinery; the RPC methods are thin wrappers around those functions.
package metaparser

import (
	"context"
	"unicode/utf8"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	unicodepb "github.com/accretional/gluon/pb"
	pb "github.com/accretional/gluon/v2/pb"
)

// Server implements pb.MetaparserServer.
type Server struct {
	pb.UnimplementedMetaparserServer
}

// New returns a ready-to-register Metaparser server.
func New() *Server { return &Server{} }

// ReadBytes classifies a raw byte buffer into a TextDescriptor.
func (s *Server) ReadBytes(ctx context.Context, req *wrapperspb.BytesValue) (*pb.TextDescriptor, error) {
	td, err := ClassifyBytes(req.GetValue())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "ReadBytes: %v", err)
	}
	return td, nil
}

// ReadString lifts a Go string into a DocumentDescriptor.
func (s *Server) ReadString(ctx context.Context, req *wrapperspb.StringValue) (*pb.DocumentDescriptor, error) {
	return WrapString(req.GetValue()), nil
}

// ClassifyBytes picks an encoding for buf and returns a matching
// TextDescriptor:
//
//   - buf is pure ASCII (every byte < 0x80) → AsciiChunk
//   - buf is non-ASCII but valid UTF-8     → unicode_string
//   - buf is not valid UTF-8               → error
//
// The chunk encoding (AsciiChunk vs UnicodeChunk vs string) is chosen
// for compactness on the wire: ASCII-only is cheapest as an enum run,
// anything else is cheapest as a raw UTF-8 string. UnicodeChunk
// (repeated int32) is a niche representation intended for callers that
// already hold a []rune and want per-rune addressability; ReadBytes
// never emits it.
//
// An empty buf returns an AsciiChunk with zero chars, not an error.
func ClassifyBytes(buf []byte) (*pb.TextDescriptor, error) {
	if isPureASCII(buf) {
		chars := make([]unicodepb.ASCII, len(buf))
		for i, b := range buf {
			chars[i] = unicodepb.ASCII(b)
		}
		return &pb.TextDescriptor{
			Content: &pb.TextDescriptor_Ascii{
				Ascii: &pb.AsciiChunk{Chars: chars},
			},
		}, nil
	}
	if !utf8.Valid(buf) {
		return nil, errInvalidUTF8
	}
	return &pb.TextDescriptor{
		Content: &pb.TextDescriptor_UnicodeString{UnicodeString: string(buf)},
	}, nil
}

// isPureASCII reports whether every byte of buf is in the 7-bit range.
func isPureASCII(buf []byte) bool {
	for _, b := range buf {
		if b >= 0x80 {
			return false
		}
	}
	return true
}

// WrapString lifts a Go string into a single-chunk DocumentDescriptor.
//
// A Go string passed through a proto StringValue is already guaranteed
// to be valid UTF-8 (proto3 rejects invalid-UTF-8 string fields during
// unmarshal), so WrapString never errors. The enclosed TextDescriptor
// uses the same ASCII-vs-unicode_string encoding policy as
// ClassifyBytes.
//
// The returned DocumentDescriptor has no name or uri — ReadString is
// intended as a convenience entry point; callers that need a URI for
// cross-referencing via SourceLocation should build the
// DocumentDescriptor themselves.
func WrapString(s string) *pb.DocumentDescriptor {
	return &pb.DocumentDescriptor{
		Text: []*pb.TextDescriptor{classifyString(s)},
	}
}

// classifyString picks the compact encoding for a known-valid UTF-8
// string: AsciiChunk for pure-ASCII, unicode_string otherwise.
func classifyString(s string) *pb.TextDescriptor {
	if isPureASCIIString(s) {
		chars := make([]unicodepb.ASCII, len(s))
		for i := 0; i < len(s); i++ {
			chars[i] = unicodepb.ASCII(s[i])
		}
		return &pb.TextDescriptor{
			Content: &pb.TextDescriptor_Ascii{
				Ascii: &pb.AsciiChunk{Chars: chars},
			},
		}
	}
	return &pb.TextDescriptor{
		Content: &pb.TextDescriptor_UnicodeString{UnicodeString: s},
	}
}

// isPureASCIIString reports whether every byte of s is in the 7-bit
// range. Iterating bytewise (not runewise) is intentional — any
// non-ASCII rune in valid UTF-8 has a high leading byte, so the byte
// test is both correct and cheaper than rune decoding.
func isPureASCIIString(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// errInvalidUTF8 is returned by ClassifyBytes for byte sequences that
// are neither pure ASCII nor valid UTF-8.
var errInvalidUTF8 = errorString("input is not valid UTF-8")

type errorString string

func (e errorString) Error() string { return string(e) }
