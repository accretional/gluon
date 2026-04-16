package metaparser

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/accretional/gluon/lexkit"
	pb "github.com/accretional/gluon/pb"
)

// TestSmallGrammar runs Parse on a tiny EBNF grammar and checks that
// Build emits a reasonable FileDescriptorProto: one message per
// production, a keyword message for each terminal, a oneof for the
// top-level `stmt` alternation, and a repeated field for `{stmt}`.
func TestSmallGrammar(t *testing.T) {
	ebnf := `
stmt_list = stmt , { ";" , stmt } ;
stmt = create_table | drop_table ;
create_table = "CREATE" , "TABLE" , table_name ;
drop_table = "DROP" , "TABLE" , table_name ;
table_name = "name" ;
`
	// Minimal LexDescriptor matching standard EBNF punctuation.
	lex := &pb.LexDescriptor{
		Definition:    utf8Rune('='),
		Termination:   utf8Rune(';'),
		Alternation:   utf8Rune('|'),
		Concatenation: utf8Rune(','),
		OptionalLhs:   utf8Rune('['),
		OptionalRhs:   utf8Rune(']'),
		RepetitionLhs: utf8Rune('{'),
		RepetitionRhs: utf8Rune('}'),
		GroupingLhs:   utf8Rune('('),
		GroupingRhs:   utf8Rune(')'),
		Terminal:      utf8Rune('"'),
		Whitespace:    []*pb.UTF8{utf8Rune(' '), utf8Rune('\t'), utf8Rune('\n'), utf8Rune('\r')},
	}
	gd, err := lexkit.Parse(ebnf, lex)
	if err != nil {
		t.Fatalf("lexkit.Parse: %v", err)
	}
	ld := &pb.LanguageDescriptor{Name: "lang", Grammar: gd}

	fdp, err := Build(ld)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	byName := map[string]bool{}
	for _, m := range fdp.GetMessageType() {
		byName[m.GetName()] = true
	}
	for _, want := range []string{"StmtList", "Stmt", "CreateTable", "DropTable", "TableName"} {
		if !byName[want] {
			t.Errorf("missing production message %q; have %v", want, byName)
		}
	}
	for _, want := range []string{"CreateKw", "TableKw", "DropKw", "NameKw"} {
		if !byName[want] {
			t.Errorf("missing keyword message %q; have %v", want, byName)
		}
	}

	// Stmt should have a oneof over create_table / drop_table.
	var stmt *proto.Message
	_ = stmt
	for _, m := range fdp.GetMessageType() {
		if m.GetName() != "Stmt" {
			continue
		}
		if len(m.OneofDecl) != 1 {
			t.Errorf("Stmt: want 1 oneof, got %d", len(m.OneofDecl))
		}
		if len(m.Field) < 2 {
			t.Errorf("Stmt: want >=2 fields, got %d", len(m.Field))
		}
	}
}

func utf8Rune(r rune) *pb.UTF8 { return lexkit.Char(r) }
