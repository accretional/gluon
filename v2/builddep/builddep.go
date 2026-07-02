// Package builddep loads cross-grammar build dependencies declared in a
// Starlark (Bazel-style) file.
//
// A grammar can depend on another grammar without physically merging their
// EBNF sources. The dependency is declared in a Starlark file that evaluates
// to a top-level GRAMMAR_DEPS list of struct() records — the same shape Bazel
// uses for provider structs. Each record names another grammar, says where its
// EBNF sources and compiled proto live, and lists the rule names it provides at
// the seam.
//
// The file is consumed by two independent pipelines that share the one
// declaration:
//
//   - A grammar's genproto (compile/emit): the externalize pass retypes every
//     field that references a dependency-provided rule to the dependency's
//     proto package, drops the local placeholder message, and adds the proto
//     import — so html.proto imports css.proto instead of inlining CSS.
//
//   - The Metaparser.ResolveDependencies RPC (parse/compose): streams the
//     dependency's EBNF DocumentDescriptors so a merged grammar can be built
//     and the CST can descend into embedded content (e.g. CSS inside <style>).
//
// Example grammar_deps.bzl:
//
//	GRAMMAR_DEPS = [
//	    struct(
//	        name           = "css",
//	        grammar_srcs   = "../proto-css/lang",
//	        proto          = "css.proto",
//	        proto_include  = "../proto-css/proto",
//	        proto_package  = "css",
//	        go_package     = "github.com/accretional/proto-css/proto/pb/css",
//	        external_rules = ["CssStyleSheet", "MediaQueryListType"],
//	    ),
//	]
package builddep

import (
	"fmt"
	"os"
	"path/filepath"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
)

// GrammarDep is one cross-grammar build dependency, as declared by a single
// struct() record in a GRAMMAR_DEPS list.
type GrammarDep struct {
	// Name is the dependency's short name (e.g. "css").
	Name string

	// GrammarSrcs is the directory holding the dependency's EBNF sources,
	// consumed at parse/compose time. Relative paths are resolved against the
	// directory of the dep file (see Load).
	GrammarSrcs string

	// Proto is the dependency's proto file name to import (e.g. "css.proto").
	// This becomes an entry in the importing file's FileDescriptorProto
	// dependency list.
	Proto string

	// ProtoInclude is the -I include directory where Proto resolves when
	// running protoc. Relative paths are resolved against the dep file dir.
	ProtoInclude string

	// ProtoPackage is the proto package the dependency's types live in (e.g.
	// "css"), used to build the external fully-qualified type names
	// (.css.CssStyleSheet).
	ProtoPackage string

	// GoPackage is the Go import path of the dependency's generated proto
	// bindings (e.g. github.com/accretional/proto-css/proto/pb/css), used for
	// the protoc M-mapping so the importing file's generated Go references the
	// dependency's package rather than regenerating it.
	GoPackage string

	// ExternalRules are the rule names the dependency provides at the seam.
	// genproto retypes any field referencing .<localpkg>.<rule> to
	// .<ProtoPackage>.<rule>, drops the local placeholder message named <rule>,
	// and imports Proto.
	ExternalRules []string
}

// Load reads and evaluates the Starlark build-dependency file at path and
// returns the GrammarDeps it declares. Relative GrammarSrcs / ProtoInclude
// paths are resolved against the directory containing path, so a dep file can
// be evaluated from any working directory.
func Load(path string) ([]GrammarDep, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	deps, err := Parse(path, src)
	if err != nil {
		return nil, err
	}
	base := filepath.Dir(path)
	for i := range deps {
		deps[i].GrammarSrcs = resolveRel(base, deps[i].GrammarSrcs)
		deps[i].ProtoInclude = resolveRel(base, deps[i].ProtoInclude)
	}
	return deps, nil
}

// Parse evaluates Starlark source (named for diagnostics) and returns the
// GrammarDeps its GRAMMAR_DEPS global declares. Paths are returned verbatim;
// callers that need them resolved should use Load.
func Parse(name string, src []byte) ([]GrammarDep, error) {
	thread := &starlark.Thread{Name: "builddep"}
	predeclared := starlark.StringDict{
		// struct() is the Bazel provider-struct constructor.
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
	}
	globals, err := starlark.ExecFileOptions(&syntax.FileOptions{}, thread, name, src, predeclared)
	if err != nil {
		return nil, fmt.Errorf("evaluate %s: %w", name, err)
	}
	raw, ok := globals["GRAMMAR_DEPS"]
	if !ok {
		return nil, fmt.Errorf("%s: no GRAMMAR_DEPS defined", name)
	}
	list, ok := raw.(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("%s: GRAMMAR_DEPS is %s, want list", name, raw.Type())
	}
	var deps []GrammarDep
	for i := 0; i < list.Len(); i++ {
		st, ok := list.Index(i).(*starlarkstruct.Struct)
		if !ok {
			return nil, fmt.Errorf("%s: GRAMMAR_DEPS[%d] is %s, want struct", name, i, list.Index(i).Type())
		}
		d, err := structToDep(st)
		if err != nil {
			return nil, fmt.Errorf("%s: GRAMMAR_DEPS[%d]: %w", name, i, err)
		}
		deps = append(deps, d)
	}
	return deps, nil
}

func structToDep(st *starlarkstruct.Struct) (GrammarDep, error) {
	var d GrammarDep
	var err error
	if d.Name, err = strField(st, "name", true); err != nil {
		return d, err
	}
	if d.GrammarSrcs, err = strField(st, "grammar_srcs", false); err != nil {
		return d, err
	}
	if d.Proto, err = strField(st, "proto", false); err != nil {
		return d, err
	}
	if d.ProtoInclude, err = strField(st, "proto_include", false); err != nil {
		return d, err
	}
	if d.ProtoPackage, err = strField(st, "proto_package", false); err != nil {
		return d, err
	}
	if d.GoPackage, err = strField(st, "go_package", false); err != nil {
		return d, err
	}
	if d.ExternalRules, err = strListField(st, "external_rules"); err != nil {
		return d, err
	}
	return d, nil
}

// strField reads a string attribute. Missing optional attributes yield "".
func strField(st *starlarkstruct.Struct, name string, required bool) (string, error) {
	v, err := st.Attr(name)
	if err != nil || v == nil {
		if required {
			return "", fmt.Errorf("missing required field %q", name)
		}
		return "", nil
	}
	s, ok := starlark.AsString(v)
	if !ok {
		return "", fmt.Errorf("field %q is %s, want string", name, v.Type())
	}
	return s, nil
}

// strListField reads a list-of-strings attribute. A missing attribute yields
// nil.
func strListField(st *starlarkstruct.Struct, name string) ([]string, error) {
	v, err := st.Attr(name)
	if err != nil || v == nil {
		return nil, nil
	}
	list, ok := v.(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("field %q is %s, want list", name, v.Type())
	}
	var out []string
	for i := 0; i < list.Len(); i++ {
		s, ok := starlark.AsString(list.Index(i))
		if !ok {
			return nil, fmt.Errorf("field %q[%d] is %s, want string", name, i, list.Index(i).Type())
		}
		out = append(out, s)
	}
	return out, nil
}

// resolveRel joins rel onto base unless rel is empty or already absolute.
func resolveRel(base, rel string) string {
	if rel == "" || filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(base, rel)
}
