// AST_GENERALIZATION_DESIGN.proto
//
// Design document: Language-Agnostic AST Serialization and Transpilation
// via Protocol Buffers
//
// This file describes the architectural vision for gluon's approach to
// cross-language transpilation using protobuf as the universal interchange
// format.
//
// ============================================================================
// VISION
// ============================================================================
//
// The goal is to formally define, encode, serialize, and interoperate the
// entire end-to-end process of transpilation into protobuf and back. Once
// a language's grammar is encoded as a GrammarDescriptor and its compiler
// is accessible via a Compiler descriptor, gluon can:
//
//   1. Parse source code into a language-agnostic ASTDescriptor
//   2. Transform ASTs between languages (ast2ast)
//   3. Generate source code from ASTs in the target language
//   4. Compile and validate the output using the target's Compiler
//
// Protobuf provides the common structure, tooling, and encoding across all
// languages. This enables bootstrapping transpilations to many languages
// starting from Go and Protocol Buffers themselves.
//
// ============================================================================
// CURRENT STATE
// ============================================================================
//
// Proto definitions:
//   - lex.proto       — LexDescriptor: configurable EBNF lexical operators
//   - grammar.proto   — GrammarDescriptor + ProductionDescriptor (TODO: fields)
//   - language.proto   — LanguageDescriptor, VersionDescriptor, Compiler
//   - ast.proto        — ASTDescriptor + ASTNodeDescriptor (TODO: fields)
//   - go.proto         — Go compiler and toolchain wrapped as gRPC services
//
// Working implementations:
//   - lexkit/          — Configurable EBNF lexer/parser that produces
//                        GrammarDescriptors from EBNF text definitions
//   - lexkit/go_ebnf.txt    — Go's complete EBNF grammar (166 productions)
//   - lexkit/proto_ebnf.txt — Proto3's complete EBNF grammar (56 productions)
//   - lexkit/ebnf.txt       — EBNF's self-describing grammar (13 productions)
//   - astkit/          — Go AST manipulation library (227 tests)
//   - codegen/         — Full pipeline: Go source → proto → gRPC server
//
// Validated against:
//   - golang.org/x/exp/ebnf — Go grammar parses and verifies (SourceFile root)
//   - EBNF self-description — Parses itself, all meta-productions found
//   - codegen round-trip    — Generated code re-analyzes to same structure
//
// ============================================================================
// ARCHITECTURE: THE EBNF → AST BOOTSTRAP
// ============================================================================
//
// The key insight is that EBNF is itself a language with a grammar, and that
// grammar can describe both itself and other languages. This creates a
// bootstrapping chain:
//
//   EBNF (the notation)
//     └─ describes → Go's grammar (go_ebnf.txt, 166 productions)
//     └─ describes → Proto's grammar (proto_ebnf.txt, 56 productions)
//     └─ describes → EBNF's own grammar (ebnf.txt, 13 productions)
//
// Each grammar, when paired with a LexDescriptor, tells lexkit how to
// tokenize EBNF text written in that notation variant:
//
//   LexDescriptor (Go variant)
//     definition:  '=' (61)
//     termination: '.' (46)
//     concatenation: 0 (implicit/juxtaposition)
//     alternation: '|' (124)
//     optional:    '[' ']'
//     repetition:  '{' '}'
//     grouping:    '(' ')'
//     terminal:    '"' (also '`' for Go)
//     comments:    '/' '/' (// and /* */)
//
//   LexDescriptor (Proto variant)
//     Same as Go except: termination = ';' (59)
//
//   LexDescriptor (Standard EBNF / ISO 14977)
//     Same except: concatenation = ',' (44), termination = ';' (59),
//     comments = '(' ')' for (* ... *)
//
// ============================================================================
// PIPELINE: FROM GRAMMAR TO TRANSPILATION
// ============================================================================
//
// Phase 1 (DONE): EBNF → GrammarDescriptor
//   lexkit reads EBNF text + LexDescriptor → GrammarDescriptor
//   Productions are parsed and counted; expression bodies captured as raw text.
//
// Phase 2 (TODO): GrammarDescriptor → ASTNodeDescriptor schema
//   Each ProductionDescriptor defines an AST node type. The production's
//   expression tree (alternation, repetition, etc.) defines the node's
//   possible children. This is mechanical:
//     - Terminal → leaf node with string value
//     - NonTerminal → child node reference
//     - Alternation → oneof in the node
//     - Repetition → repeated field
//     - Optional → optional field
//
// Phase 3 (TODO): Source code → ASTDescriptor
//   Given a GrammarDescriptor, generate a parser that reads source code
//   and produces an ASTDescriptor. For Go, this can delegate to go/parser
//   and map the go/ast types to ASTNodeDescriptors. For proto, this can
//   delegate to protocompile.
//
// Phase 4 (TODO): ASTDescriptor → ASTDescriptor (cross-language)
//   ast2ast transformations that map nodes from one language's grammar to
//   another's. The existing codegen.TransformInterface is an early example
//   of this (Go interface → gRPC-normalized interface).
//
// Phase 5 (TODO): ASTDescriptor → Source code
//   Given an ASTDescriptor in the target language's grammar, emit source
//   code. The existing codegen.GenerateProto is an early example (Go
//   types → proto text). codegen.Bootstrap generates Go source from
//   analyzed structures.
//
// ============================================================================
// COMBINING WITH go.proto
// ============================================================================
//
// go.proto wraps the Go compiler as a gRPC service, giving us:
//   - Go.Build, Go.Run, Go.Test — compile and execute Go code
//   - Go.Vet, Go.Format — validate and format Go source
//   - GoMod.Init, GoMod.Tidy — manage Go modules
//
// Combined with the grammar/AST pipeline, this enables:
//   1. Analyze Go source → ASTDescriptor (pkg2ast, via go/parser)
//   2. Transform AST to proto schema (struct2proto, function2service)
//   3. Generate proto source from AST
//   4. Compile proto → Go code (protoc via ProtoCompiler)
//   5. Generate server/client Go source (service2server)
//   6. Build and test via Go.Build/Go.Test (go.proto services)
//   7. Validate round-trip (re-analyze generated code)
//
// ============================================================================
// BOOTSTRAPPING TO OTHER LANGUAGES
// ============================================================================
//
// Once the EBNF → Grammar → AST → Source pipeline is working for Go and
// Proto, adding a new language requires:
//
//   1. Write the language's EBNF grammar (e.g. python_ebnf.txt)
//   2. Define its LexDescriptor (which EBNF variant it uses)
//   3. Define its Compiler (uri to the compiler binary)
//   4. Implement ast2ast mappings from/to existing languages
//   5. Implement source emitter for the new language
//
// Steps 1-3 are largely mechanical. Steps 4-5 are where the real work is,
// but the grammar-driven approach means AST node types are derived
// automatically from the EBNF productions rather than hand-coded.
//
// ============================================================================
// NOTES ON EBNF VARIANTS
// ============================================================================
//
// Standard EBNF (ISO 14977) also defines:
//   - Special sequences: ? ... ? (for informal descriptions)
//   - Exceptions: - (set difference, e.g. "letter - vowel")
//
// These are NOT yet implemented in LexDescriptor. Go's EBNF uses /* */
// comments for the role that special sequences serve (describing character
// classes informally). Proto's EBNF similarly uses comments.
//
// The "…" (ellipsis) operator in Go's EBNF (e.g. "0" … "9") represents
// character ranges and is not part of standard EBNF. It is handled by
// x/exp/ebnf as a Range expression.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.11
// 	protoc        v7.34.1
// source: AST_GENERALIZATION_DESIGN.proto

package pb

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// Status of implementation.
type PipelineStage_Status int32

const (
	PipelineStage_UNSPECIFIED PipelineStage_Status = 0
	PipelineStage_DONE        PipelineStage_Status = 1
	PipelineStage_PARTIAL     PipelineStage_Status = 2
	PipelineStage_TODO        PipelineStage_Status = 3
)

// Enum value maps for PipelineStage_Status.
var (
	PipelineStage_Status_name = map[int32]string{
		0: "UNSPECIFIED",
		1: "DONE",
		2: "PARTIAL",
		3: "TODO",
	}
	PipelineStage_Status_value = map[string]int32{
		"UNSPECIFIED": 0,
		"DONE":        1,
		"PARTIAL":     2,
		"TODO":        3,
	}
)

func (x PipelineStage_Status) Enum() *PipelineStage_Status {
	p := new(PipelineStage_Status)
	*p = x
	return p
}

func (x PipelineStage_Status) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (PipelineStage_Status) Descriptor() protoreflect.EnumDescriptor {
	return file_AST_GENERALIZATION_DESIGN_proto_enumTypes[0].Descriptor()
}

func (PipelineStage_Status) Type() protoreflect.EnumType {
	return &file_AST_GENERALIZATION_DESIGN_proto_enumTypes[0]
}

func (x PipelineStage_Status) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use PipelineStage_Status.Descriptor instead.
func (PipelineStage_Status) EnumDescriptor() ([]byte, []int) {
	return file_AST_GENERALIZATION_DESIGN_proto_rawDescGZIP(), []int{1, 0}
}

// TranspilationPipeline describes the full end-to-end conversion from
// one language to another via protobuf-encoded ASTs.
type TranspilationPipeline struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Source language definition.
	SourceLanguage string `protobuf:"bytes,1,opt,name=source_language,json=sourceLanguage,proto3" json:"source_language,omitempty"`
	// Target language definition.
	TargetLanguage string `protobuf:"bytes,2,opt,name=target_language,json=targetLanguage,proto3" json:"target_language,omitempty"`
	// Stages in the pipeline, in order.
	Stages        []*PipelineStage `protobuf:"bytes,3,rep,name=stages,proto3" json:"stages,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TranspilationPipeline) Reset() {
	*x = TranspilationPipeline{}
	mi := &file_AST_GENERALIZATION_DESIGN_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TranspilationPipeline) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TranspilationPipeline) ProtoMessage() {}

func (x *TranspilationPipeline) ProtoReflect() protoreflect.Message {
	mi := &file_AST_GENERALIZATION_DESIGN_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TranspilationPipeline.ProtoReflect.Descriptor instead.
func (*TranspilationPipeline) Descriptor() ([]byte, []int) {
	return file_AST_GENERALIZATION_DESIGN_proto_rawDescGZIP(), []int{0}
}

func (x *TranspilationPipeline) GetSourceLanguage() string {
	if x != nil {
		return x.SourceLanguage
	}
	return ""
}

func (x *TranspilationPipeline) GetTargetLanguage() string {
	if x != nil {
		return x.TargetLanguage
	}
	return ""
}

func (x *TranspilationPipeline) GetStages() []*PipelineStage {
	if x != nil {
		return x.Stages
	}
	return nil
}

// PipelineStage represents one step in a transpilation pipeline.
type PipelineStage struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Stage name (e.g. "pkg2ast", "ast2ast", "struct2proto").
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Description of what this stage does.
	Description   string               `protobuf:"bytes,2,opt,name=description,proto3" json:"description,omitempty"`
	Status        PipelineStage_Status `protobuf:"varint,3,opt,name=status,proto3,enum=gluon.PipelineStage_Status" json:"status,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PipelineStage) Reset() {
	*x = PipelineStage{}
	mi := &file_AST_GENERALIZATION_DESIGN_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PipelineStage) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PipelineStage) ProtoMessage() {}

func (x *PipelineStage) ProtoReflect() protoreflect.Message {
	mi := &file_AST_GENERALIZATION_DESIGN_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PipelineStage.ProtoReflect.Descriptor instead.
func (*PipelineStage) Descriptor() ([]byte, []int) {
	return file_AST_GENERALIZATION_DESIGN_proto_rawDescGZIP(), []int{1}
}

func (x *PipelineStage) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *PipelineStage) GetDescription() string {
	if x != nil {
		return x.Description
	}
	return ""
}

func (x *PipelineStage) GetStatus() PipelineStage_Status {
	if x != nil {
		return x.Status
	}
	return PipelineStage_UNSPECIFIED
}

var File_AST_GENERALIZATION_DESIGN_proto protoreflect.FileDescriptor

const file_AST_GENERALIZATION_DESIGN_proto_rawDesc = "" +
	"\n" +
	"\x1fAST_GENERALIZATION_DESIGN.proto\x12\x05gluon\"\x97\x01\n" +
	"\x15TranspilationPipeline\x12'\n" +
	"\x0fsource_language\x18\x01 \x01(\tR\x0esourceLanguage\x12'\n" +
	"\x0ftarget_language\x18\x02 \x01(\tR\x0etargetLanguage\x12,\n" +
	"\x06stages\x18\x03 \x03(\v2\x14.gluon.PipelineStageR\x06stages\"\xb6\x01\n" +
	"\rPipelineStage\x12\x12\n" +
	"\x04name\x18\x01 \x01(\tR\x04name\x12 \n" +
	"\vdescription\x18\x02 \x01(\tR\vdescription\x123\n" +
	"\x06status\x18\x03 \x01(\x0e2\x1b.gluon.PipelineStage.StatusR\x06status\":\n" +
	"\x06Status\x12\x0f\n" +
	"\vUNSPECIFIED\x10\x00\x12\b\n" +
	"\x04DONE\x10\x01\x12\v\n" +
	"\aPARTIAL\x10\x02\x12\b\n" +
	"\x04TODO\x10\x03B!Z\x1fgithub.com/accretional/gluon/pbb\x06proto3"

var (
	file_AST_GENERALIZATION_DESIGN_proto_rawDescOnce sync.Once
	file_AST_GENERALIZATION_DESIGN_proto_rawDescData []byte
)

func file_AST_GENERALIZATION_DESIGN_proto_rawDescGZIP() []byte {
	file_AST_GENERALIZATION_DESIGN_proto_rawDescOnce.Do(func() {
		file_AST_GENERALIZATION_DESIGN_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_AST_GENERALIZATION_DESIGN_proto_rawDesc), len(file_AST_GENERALIZATION_DESIGN_proto_rawDesc)))
	})
	return file_AST_GENERALIZATION_DESIGN_proto_rawDescData
}

var file_AST_GENERALIZATION_DESIGN_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_AST_GENERALIZATION_DESIGN_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_AST_GENERALIZATION_DESIGN_proto_goTypes = []any{
	(PipelineStage_Status)(0),     // 0: gluon.PipelineStage.Status
	(*TranspilationPipeline)(nil), // 1: gluon.TranspilationPipeline
	(*PipelineStage)(nil),         // 2: gluon.PipelineStage
}
var file_AST_GENERALIZATION_DESIGN_proto_depIdxs = []int32{
	2, // 0: gluon.TranspilationPipeline.stages:type_name -> gluon.PipelineStage
	0, // 1: gluon.PipelineStage.status:type_name -> gluon.PipelineStage.Status
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_AST_GENERALIZATION_DESIGN_proto_init() }
func file_AST_GENERALIZATION_DESIGN_proto_init() {
	if File_AST_GENERALIZATION_DESIGN_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_AST_GENERALIZATION_DESIGN_proto_rawDesc), len(file_AST_GENERALIZATION_DESIGN_proto_rawDesc)),
			NumEnums:      1,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_AST_GENERALIZATION_DESIGN_proto_goTypes,
		DependencyIndexes: file_AST_GENERALIZATION_DESIGN_proto_depIdxs,
		EnumInfos:         file_AST_GENERALIZATION_DESIGN_proto_enumTypes,
		MessageInfos:      file_AST_GENERALIZATION_DESIGN_proto_msgTypes,
	}.Build()
	File_AST_GENERALIZATION_DESIGN_proto = out.File
	file_AST_GENERALIZATION_DESIGN_proto_goTypes = nil
	file_AST_GENERALIZATION_DESIGN_proto_depIdxs = nil
}
