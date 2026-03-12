// Command gluon-server starts a gRPC server exposing the Go compiler service.
// It also provides the codegen subcommand for generating gRPC services from
// Go interfaces.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/accretional/gluon"
	"github.com/accretional/gluon/codegen"
	"github.com/accretional/gluon/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// Check for subcommands before flag parsing
	if len(os.Args) > 1 && os.Args[1] == "codegen" {
		runCodegen(os.Args[2:])
		return
	}

	addr := flag.String("addr", ":50051", "server listen address (server mode) or dial address (client mode)")
	client := flag.Bool("client", false, "run in client mode")
	flag.Parse()

	if *client {
		runClient(*addr, flag.Args())
		return
	}
	runServer(*addr)
}

// runCodegen generates a complete gRPC service from a Go source file.
func runCodegen(args []string) {
	fs := flag.NewFlagSet("codegen", flag.ExitOnError)
	outDir := fs.String("o", "", "output directory (default: derived from module)")
	module := fs.String("module", "", "Go module path (required)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gluon codegen -module <path> [-o <dir>] <source.go>")
		fmt.Fprintln(os.Stderr, "\nGenerates a complete, compilable gRPC service from Go interfaces.")
		fmt.Fprintln(os.Stderr)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	if *module == "" || fs.NArg() == 0 {
		fs.Usage()
		os.Exit(1)
	}

	sourceFile := fs.Arg(0)
	src, err := os.ReadFile(sourceFile)
	if err != nil {
		log.Fatalf("read %s: %v", sourceFile, err)
	}

	// Default output dir: last element of module path
	if *outDir == "" {
		parts := strings.Split(*module, "/")
		*outDir = parts[len(parts)-1]
	}

	log.Printf("generating gRPC service from %s", sourceFile)

	result, err := codegen.FullBootstrap(*module, string(src))
	if err != nil {
		log.Fatalf("codegen failed: %v", err)
	}

	if !result.CompileOK {
		log.Fatalf("generated code did not compile: %v", result.CompileError)
	}

	// Write all files to output directory
	for name, content := range result.Package.Files {
		path := filepath.Join(*outDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			log.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			log.Fatalf("write %s: %v", path, err)
		}
		fmt.Printf("  %s\n", name)
	}

	fmt.Printf("\ngenerated %d files in %s/\n", len(result.Package.Files), *outDir)
}

func runServer(addr string) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", addr, err)
	}

	srv := grpc.NewServer()
	if err := gluon.RegisterServices(srv); err != nil {
		log.Fatalf("failed to register services: %v", err)
	}

	log.Printf("gluon server listening on %s", addr)
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func dial(addr string) *grpc.ClientConn {
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to %s: %v", addr, err)
	}
	return conn
}

func runClient(addr string, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: gluon-server -client [-addr host:port] <subcommand> [args...]")
		fmt.Fprintln(os.Stderr, "subcommands: command, doc, env, env-json, fmt, get, install,")
		fmt.Fprintln(os.Stderr, "             list, list-json, list-modules, fix, list-analyzers,")
		fmt.Fprintln(os.Stderr, "             describe-analyzer, build, run, test, generate, vet,")
		fmt.Fprintln(os.Stderr, "             tool, help, version,")
		fmt.Fprintln(os.Stderr, "             mod-download, mod-download-json, mod-edit-json,")
		fmt.Fprintln(os.Stderr, "             mod-edit-print, mod-edit-fmt, mod-graph, mod-init,")
		fmt.Fprintln(os.Stderr, "             mod-tidy, mod-tidy-verbose, mod-tidy-diff,")
		fmt.Fprintln(os.Stderr, "             mod-vendor, mod-verify, mod-why, mod-why-m")
		os.Exit(1)
	}

	conn := dial(addr)
	defer conn.Close()

	goClient := pb.NewGoClient(conn)
	modClient := pb.NewGoModClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	subcmd := args[0]
	rest := args[1:]

	var resp *pb.Text
	var err error

	switch subcmd {
	case "command":
		resp, err = goClient.Command(ctx, &pb.Text{Text: strings.Join(rest, " ")})

	case "doc":
		req := &pb.GoDocQuery{}
		if len(rest) > 0 {
			req.Pkg = rest[0]
		}
		if len(rest) > 1 {
			req.Symbol = rest[1]
		}
		if len(rest) > 2 {
			req.MethodOrField = rest[2:]
		}
		resp, err = goClient.Doc(ctx, req)

	case "env":
		req := &pb.GoEnvRequest{Vars: rest}
		resp, err = goClient.Env(ctx, req)

	case "env-json":
		req := &pb.GoEnvRequest{Json: true, Vars: rest}
		resp, err = goClient.Env(ctx, req)

	case "fmt":
		req := &pb.GoFormatRequest{Packages: rest}
		resp, err = goClient.Format(ctx, req)

	case "get":
		if len(rest) == 0 {
			log.Fatal("get requires at least one package")
		}
		req := &pb.GoGetRequest{Packages: rest}
		resp, err = goClient.Get(ctx, req)

	case "install":
		if len(rest) == 0 {
			log.Fatal("install requires at least one package")
		}
		req := &pb.GoInstallRequest{Packages: rest}
		resp, err = goClient.Install(ctx, req)

	case "list":
		req := &pb.GoListRequest{Packages: rest}
		resp, err = goClient.List(ctx, req)

	case "list-json":
		req := &pb.GoListRequest{Json: true, Packages: rest}
		resp, err = goClient.List(ctx, req)

	case "list-modules":
		req := &pb.GoListRequest{Modules: true, Packages: rest}
		resp, err = goClient.List(ctx, req)

	case "fix":
		req := &pb.GoFixQuery{}
		if len(rest) > 0 {
			req.Pkg = rest[0]
		}
		resp, err = goClient.Fix(ctx, req)

	case "list-analyzers":
		resp, err = goClient.ListFixAnalyzers(ctx, &pb.Nothing{})

	case "describe-analyzer":
		if len(rest) == 0 {
			log.Fatal("describe-analyzer requires an analyzer name")
		}
		resp, err = goClient.DescribeFixAnalyzer(ctx, &pb.Text{Text: rest[0]})

	case "build":
		req := &pb.GoBuildRequest{Packages: rest}
		resp, err = goClient.Build(ctx, req)

	case "build-verbose":
		req := &pb.GoBuildRequest{Verbose: true, Packages: rest}
		resp, err = goClient.Build(ctx, req)

	case "run":
		req := &pb.GoRunRequest{}
		if len(rest) > 0 {
			req.Package = rest[0]
		}
		if len(rest) > 1 {
			req.Args = rest[1:]
		}
		resp, err = goClient.Run(ctx, req)

	case "test":
		req := &pb.GoTestRequest{Packages: rest}
		resp, err = goClient.Test(ctx, req)

	case "test-verbose":
		req := &pb.GoTestRequest{Verbose: true, Packages: rest}
		resp, err = goClient.Test(ctx, req)

	case "test-json":
		req := &pb.GoTestRequest{Json: true, Packages: rest}
		resp, err = goClient.Test(ctx, req)

	case "generate":
		req := &pb.GoGenerateRequest{Packages: rest}
		resp, err = goClient.Generate(ctx, req)

	case "generate-dry":
		req := &pb.GoGenerateRequest{DryRun: true, Packages: rest}
		resp, err = goClient.Generate(ctx, req)

	case "vet":
		req := &pb.GoVetRequest{Packages: rest}
		resp, err = goClient.Vet(ctx, req)

	case "vet-json":
		req := &pb.GoVetRequest{Json: true, Packages: rest}
		resp, err = goClient.Vet(ctx, req)

	case "tool":
		req := &pb.GoToolRequest{}
		if len(rest) > 0 {
			req.Name = rest[0]
		}
		if len(rest) > 1 {
			req.Args = rest[1:]
		}
		resp, err = goClient.Tool(ctx, req)

	case "help":
		resp, err = goClient.Help(ctx, &pb.Text{Text: strings.Join(rest, " ")})

	case "version":
		resp, err = goClient.Version(ctx, &pb.Nothing{})

	// GoMod service
	case "mod-download":
		req := &pb.GoModDownloadRequest{Modules: rest}
		resp, err = modClient.Download(ctx, req)

	case "mod-download-json":
		req := &pb.GoModDownloadRequest{Json: true, Modules: rest}
		resp, err = modClient.Download(ctx, req)

	case "mod-edit-json":
		resp, err = modClient.Edit(ctx, &pb.GoModEditRequest{Json: true})

	case "mod-edit-print":
		resp, err = modClient.Edit(ctx, &pb.GoModEditRequest{Print: true})

	case "mod-edit-fmt":
		resp, err = modClient.Edit(ctx, &pb.GoModEditRequest{Fmt: true})

	case "mod-graph":
		resp, err = modClient.Graph(ctx, &pb.GoModGraphRequest{})

	case "mod-init":
		req := &pb.GoModInitRequest{}
		if len(rest) > 0 {
			req.ModulePath = rest[0]
		}
		resp, err = modClient.Init(ctx, req)

	case "mod-tidy":
		resp, err = modClient.Tidy(ctx, &pb.GoModTidyRequest{})

	case "mod-tidy-verbose":
		resp, err = modClient.Tidy(ctx, &pb.GoModTidyRequest{Verbose: true})

	case "mod-tidy-diff":
		resp, err = modClient.Tidy(ctx, &pb.GoModTidyRequest{Diff: true})

	case "mod-vendor":
		resp, err = modClient.Vendor(ctx, &pb.GoModVendorRequest{})

	case "mod-vendor-verbose":
		resp, err = modClient.Vendor(ctx, &pb.GoModVendorRequest{Verbose: true})

	case "mod-verify":
		resp, err = modClient.Verify(ctx, &pb.GoModVerifyRequest{})

	case "mod-why":
		if len(rest) == 0 {
			log.Fatal("mod-why requires at least one package or module")
		}
		resp, err = modClient.Why(ctx, &pb.GoModWhyRequest{Packages: rest})

	case "mod-why-m":
		if len(rest) == 0 {
			log.Fatal("mod-why-m requires at least one module")
		}
		resp, err = modClient.Why(ctx, &pb.GoModWhyRequest{Modules: true, Packages: rest})

	default:
		// Treat unknown subcommand as a raw go command
		resp, err = goClient.Command(ctx, &pb.Text{Text: subcmd + " " + strings.Join(rest, " ")})
	}

	if err != nil {
		log.Fatalf("%s failed: %v", subcmd, err)
	}

	fmt.Print(resp.GetText())
}
