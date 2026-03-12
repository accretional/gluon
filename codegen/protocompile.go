package codegen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ProtoCompileResult holds the output of compiling a .proto file.
type ProtoCompileResult struct {
	// ProtoFile is the input .proto file path.
	ProtoFile string

	// GoFiles are the generated Go files (message types + gRPC stubs).
	GoFiles map[string]string

	// CompileOK is true if protoc succeeded.
	CompileOK bool

	// Error is set if compilation failed.
	Error error
}

// ProtoCompiler compiles .proto files into Go code using protoc.
type ProtoCompiler struct {
	// ProtocBin is the path to the protoc binary.
	ProtocBin string

	// GenGoBin is the path to protoc-gen-go.
	GenGoBin string

	// GenGoGRPCBin is the path to protoc-gen-go-grpc.
	GenGoGRPCBin string
}

// NewProtoCompiler locates protoc and its Go plugins on PATH.
func NewProtoCompiler() (*ProtoCompiler, error) {
	protocBin, err := exec.LookPath("protoc")
	if err != nil {
		return nil, fmt.Errorf("protoc not found: %w", err)
	}
	genGo, err := exec.LookPath("protoc-gen-go")
	if err != nil {
		return nil, fmt.Errorf("protoc-gen-go not found: %w", err)
	}
	genGoGRPC, err := exec.LookPath("protoc-gen-go-grpc")
	if err != nil {
		return nil, fmt.Errorf("protoc-gen-go-grpc not found: %w", err)
	}
	return &ProtoCompiler{
		ProtocBin:    protocBin,
		GenGoBin:     genGo,
		GenGoGRPCBin: genGoGRPC,
	}, nil
}

// Compile runs protoc on a .proto file and produces Go message and gRPC files.
// The goPackage parameter sets the Go package path for the generated code.
// Output files are written to outDir.
func (pc *ProtoCompiler) Compile(protoFile, goPackage, outDir string) (*ProtoCompileResult, error) {
	result := &ProtoCompileResult{
		ProtoFile: protoFile,
		GoFiles:   make(map[string]string),
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	protoDir := filepath.Dir(protoFile)
	protoBase := filepath.Base(protoFile)

	// Run protoc with both go and go-grpc plugins
	args := []string{
		"--proto_path=" + protoDir,
		"--go_out=" + outDir,
		"--go_opt=paths=source_relative",
		"--go-grpc_out=" + outDir,
		"--go-grpc_opt=paths=source_relative",
		protoBase,
	}

	cmd := exec.Command(pc.ProtocBin, args...)
	cmd.Dir = protoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		result.Error = fmt.Errorf("protoc failed: %v\n%s", err, out)
		return result, nil
	}
	result.CompileOK = true

	// Read generated files
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return result, nil
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(outDir, entry.Name()))
		if err != nil {
			continue
		}
		result.GoFiles[entry.Name()] = string(data)
	}

	return result, nil
}

// CompileProtoString compiles proto content from a string (writes to a temp
// file first). Returns the generated Go files.
func (pc *ProtoCompiler) CompileProtoString(protoContent, goPackage string) (*ProtoCompileResult, error) {
	tmpDir, err := os.MkdirTemp("", "gluon-proto-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	// Write the proto file
	protoFile := filepath.Join(tmpDir, "service.proto")
	if err := os.WriteFile(protoFile, []byte(protoContent), 0644); err != nil {
		return nil, err
	}

	outDir := filepath.Join(tmpDir, "out")
	return pc.Compile(protoFile, goPackage, outDir)
}

// CompileBundle compiles the proto from a ServiceBundle and returns the
// generated Go files.
func (pc *ProtoCompiler) CompileBundle(bundle *ServiceBundle) (*ProtoCompileResult, error) {
	return pc.CompileProtoString(bundle.Proto, "")
}

// FullBootstrapResult extends BootstrapResult with proto compilation output.
type FullBootstrapResult struct {
	*BootstrapResult

	// ProtoResults holds the proto compilation result per service.
	ProtoResults map[string]*ProtoCompileResult

	// ProtoCompileOK is true if all protos compiled successfully.
	ProtoCompileOK bool
}

// FullBootstrap runs the complete pipeline including proto compilation:
//  1. Analyze source code
//  2. Transform interfaces → gRPC form
//  3. Generate Go package (types, server, client, proto)
//  4. Compile the generated Go code
//  5. Compile the generated .proto files with protoc
//  6. Re-analyze generated code (round-trip)
//  7. Verify round-trip preserves structure
func FullBootstrap(module, src string) (*FullBootstrapResult, error) {
	// Run the standard bootstrap first
	base, err := Bootstrap(module, src)
	if err != nil {
		return nil, err
	}

	result := &FullBootstrapResult{
		BootstrapResult: base,
		ProtoResults:    make(map[string]*ProtoCompileResult),
	}

	// Try to get a proto compiler
	pc, err := NewProtoCompiler()
	if err != nil {
		// No protoc available — skip proto compilation but don't fail
		return result, nil
	}

	// Compile each service's proto
	allOK := true
	for _, bundle := range base.Package.Bundles {
		pr, err := pc.CompileBundle(bundle)
		if err != nil {
			result.ProtoResults[bundle.Name] = &ProtoCompileResult{
				Error: err,
			}
			allOK = false
			continue
		}
		result.ProtoResults[bundle.Name] = pr
		if !pr.CompileOK {
			allOK = false
		}
	}
	result.ProtoCompileOK = allOK

	return result, nil
}

// CompilePackageProtos compiles all .proto files in a GeneratedPackage.
// Returns a map of service name → ProtoCompileResult.
func CompilePackageProtos(pkg *GeneratedPackage) (map[string]*ProtoCompileResult, error) {
	pc, err := NewProtoCompiler()
	if err != nil {
		return nil, err
	}

	results := make(map[string]*ProtoCompileResult)
	for name, content := range pkg.Files {
		if !strings.HasSuffix(name, ".proto") {
			continue
		}
		svcName := strings.TrimSuffix(name, ".proto")
		pr, err := pc.CompileProtoString(content, pkg.Module+"/pb")
		if err != nil {
			results[svcName] = &ProtoCompileResult{Error: err}
			continue
		}
		results[svcName] = pr
	}
	return results, nil
}
