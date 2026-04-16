package metaparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/accretional/gluon/lexkit"
	pb "github.com/accretional/gluon/pb"
)

// TestBuildThenProtoc serializes the emitted FileDescriptorProto into a
// FileDescriptorSet and feeds it back to protoc via --descriptor_set_in,
// confirming the mapping produces a valid proto file.
func TestBuildThenProtoc(t *testing.T) {
	if _, err := exec.LookPath("protoc"); err != nil {
		t.Skip("protoc not on PATH")
	}
	ebnf := `
stmt_list = stmt , { ";" , stmt } ;
stmt = create_table | drop_table ;
create_table = "CREATE" , "TABLE" , table_name ;
drop_table = "DROP" , "TABLE" , table_name ;
table_name = "name" ;
`
	lex := &pb.LexDescriptor{
		Definition:    lexkit.Char('='),
		Termination:   lexkit.Char(';'),
		Alternation:   lexkit.Char('|'),
		Concatenation: lexkit.Char(','),
		OptionalLhs:   lexkit.Char('['),
		OptionalRhs:   lexkit.Char(']'),
		RepetitionLhs: lexkit.Char('{'),
		RepetitionRhs: lexkit.Char('}'),
		GroupingLhs:   lexkit.Char('('),
		GroupingRhs:   lexkit.Char(')'),
		Terminal:      lexkit.Char('"'),
		Whitespace:    []*pb.UTF8{lexkit.Char(' '), lexkit.Char('\t'), lexkit.Char('\n'), lexkit.Char('\r')},
	}
	gd, err := lexkit.Parse(ebnf, lex)
	if err != nil {
		t.Fatal(err)
	}
	fdp, err := Build(&pb.LanguageDescriptor{Name: "lang", Grammar: gd})
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	set := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{fdp}}
	blob, err := proto.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	setPath := filepath.Join(dir, "set.pb")
	if err := os.WriteFile(setPath, blob, 0o644); err != nil {
		t.Fatal(err)
	}
	// protoc can parse a descriptor_set_in; we just want it to accept
	// the file without type-resolution errors.
	cmd := exec.Command("protoc",
		"--descriptor_set_in="+setPath,
		"--descriptor_set_out="+filepath.Join(dir, "out.pb"),
		fdp.GetName(),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("protoc rejected emitted file: %v\n%s", err, out)
	}
}
