//go:build linux

// ptrace_flamegraph connects to a running gluon server, traces a binary via
// the Ptrace RPC, and in a single pass produces:
//
//   - A flame graph SVG (via flamegraph.pl) from reconstructed per-goroutine
//     call stacks.
//   - A Graphviz DOT digraph of the observed call edges, weighted by frequency,
//     for structural analysis (dot, xdot, d3-graphviz, etc.).
//
// Usage:
//
//	ptrace_flamegraph -binary <path> [-addr <host:port>] [-output <file>] [-dot <file>] [-- binary-args...]
//
// If flamegraph.pl is not present at -flamegraph-pl it is cloned automatically
// from https://github.com/brendangregg/FlameGraph.
// Pass -output - to write folded stacks to stdout. Pass -dot - to write the
// DOT graph to stdout. Either flag set to "" disables that output.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/accretional/gluon/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type edge struct{ caller, callee string }

// traceData holds everything collected from a single stream pass.
type traceData struct {
	// stackCounts maps a semicolon-joined call stack to its sample count.
	// Used to produce the flamegraph folded-stacks input.
	stackCounts map[string]int

	// edgeCounts maps a directed (caller→callee) edge to its call count.
	// Used to produce the DOT digraph.
	edgeCounts map[edge]int
}

func main() {
	addr := flag.String("addr", "localhost:50051", "gluon server address")
	binary := flag.String("binary", "", "path to the binary to trace (required)")
	output := flag.String("output", "temp/ptrace_flamegraph.svg", `flame graph SVG output; "-" writes folded stacks to stdout, "" skips`)
	dotOut := flag.String("dot", "temp/ptrace_callgraph.dot", `DOT digraph output; "-" writes to stdout, "" skips`)
	fgpl := flag.String("flamegraph-pl", "/tmp/flamegraph/flamegraph.pl", "path to flamegraph.pl")
	flag.Parse()
	binaryArgs := flag.Args()

	if *binary == "" {
		fmt.Fprintln(os.Stderr, "error: -binary is required")
		flag.Usage()
		os.Exit(1)
	}

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fatalf("dial %s: %v", *addr, err)
	}
	defer conn.Close()

	stream, err := pb.NewPtraceClient(conn).Run(context.Background(), &pb.TraceRequest{
		Binary: *binary,
		Args:   binaryArgs,
	})
	if err != nil {
		fatalf("Ptrace.Run: %v", err)
	}

	data := collect(stream)
	if len(data.edgeCounts) == 0 {
		fatalf("no trace events received — is the binary stripped?")
	}

	if *dotOut != "" {
		writeDOTTo(*dotOut, *binary, data)
	}

	if *output != "" {
		if *output == "-" {
			writeFolded(os.Stdout, data.stackCounts)
		} else {
			ensureFlamegraph(*fgpl)
			generateSVG(*fgpl, *binary, *output, data.stackCounts)
		}
	}
}

// collect streams TraceEvents and builds traceData in a single pass.
//
// Stack reconstruction: each event carries (caller, callee, goroutine_id).
// We search the goroutine's current stack for the caller frame, trim to that
// depth, then push the callee — giving plausible call chains without needing
// return events. Simultaneously we accumulate directed edge counts.
func collect(stream pb.Ptrace_RunClient) traceData {
	stacks := map[int64][]string{}
	data := traceData{
		stackCounts: map[string]int{},
		edgeCounts:  map[edge]int{},
	}

	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			fatalf("recv: %v", err)
		}

		gid := ev.GetGoroutineId()
		caller := ev.GetCaller()
		callee := ev.GetCallee()

		// Drop events where the caller couldn't be resolved to a symbol name.
		// These are assembly stubs or runtime trampolines excluded from the
		// symbol table — they appear as raw hex addresses and add noise.
		if strings.HasPrefix(caller, "0x") {
			continue
		}

		// Always record the raw edge regardless of stack state.
		data.edgeCounts[edge{caller, callee}]++

		// Reconstruct the goroutine stack.
		stack := stacks[gid]
		found := -1
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i] == caller {
				found = i
				break
			}
		}
		if found >= 0 {
			stack = stack[:found+1]
		} else {
			stack = []string{caller}
		}
		stack = append(stack, callee)
		stacks[gid] = stack

		data.stackCounts[strings.Join(stack, ";")]++
	}

	return data
}

// writeFolded writes the folded-stacks format expected by flamegraph.pl.
func writeFolded(w io.Writer, counts map[string]int) {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "%s %d\n", k, counts[k])
	}
}

// writeDOT writes a weighted Graphviz digraph of the observed call edges.
// Edge thickness and label reflect call frequency, making hot paths visually
// prominent when rendered with dot/xdot.
func writeDOT(w io.Writer, binary string, data traceData) {
	// Find the max edge count to normalise penwidth.
	maxCount := 1
	for _, c := range data.edgeCounts {
		if c > maxCount {
			maxCount = c
		}
	}

	fmt.Fprintf(w, "digraph ptrace {\n")
	fmt.Fprintf(w, "\tlabel=%q;\n", "ptrace: "+filepath.Base(binary))
	fmt.Fprintf(w, "\tlabelloc=t;\n")
	fmt.Fprintf(w, "\trankdir=LR;\n")
	fmt.Fprintf(w, "\tnode [shape=box fontname=\"monospace\" fontsize=9];\n")
	fmt.Fprintf(w, "\tedge [fontname=\"monospace\" fontsize=8];\n\n")

	// Sort edges for deterministic output.
	edges := make([]edge, 0, len(data.edgeCounts))
	for e := range data.edgeCounts {
		edges = append(edges, e)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].caller != edges[j].caller {
			return edges[i].caller < edges[j].caller
		}
		return edges[i].callee < edges[j].callee
	})

	for _, e := range edges {
		count := data.edgeCounts[e]
		// Scale penwidth between 1 and 5 based on relative frequency.
		penwidth := 1.0 + 4.0*float64(count)/float64(maxCount)
		fmt.Fprintf(w, "\t%q -> %q [label=%d penwidth=%.2f];\n",
			e.caller, e.callee, count, penwidth)
	}

	fmt.Fprintf(w, "}\n")
}

func writeDOTTo(path, binary string, data traceData) {
	if path == "-" {
		writeDOT(os.Stdout, binary, data)
		return
	}
	f, err := os.Create(path)
	if err != nil {
		fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	writeDOT(f, binary, data)
	fmt.Fprintf(os.Stderr, "call graph written to %s\n", path)
}

// generateSVG pipes folded stacks into flamegraph.pl and writes the SVG.
func generateSVG(fgpl, binary, output string, counts map[string]int) {
	outFile, err := os.Create(output)
	if err != nil {
		fatalf("create %s: %v", output, err)
	}
	defer outFile.Close()

	cmd := exec.Command("perl", fgpl,
		"--title", "ptrace: "+binary,
		"--colors", "hot",
	)
	cmd.Stdout = outFile
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		fatalf("pipe to flamegraph.pl: %v", err)
	}
	if err := cmd.Start(); err != nil {
		fatalf("start flamegraph.pl: %v", err)
	}

	writeFolded(stdin, counts)
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		fatalf("flamegraph.pl: %v", err)
	}
	fmt.Fprintf(os.Stderr, "flame graph written to %s\n", output)
}

// ensureFlamegraph clones Brendan Gregg's FlameGraph repo if flamegraph.pl is
// not already present.
func ensureFlamegraph(fgpl string) {
	if _, err := os.Stat(fgpl); err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "flamegraph.pl not found — cloning brendangregg/FlameGraph...")
	cmd := exec.Command("git", "clone", "--depth=1",
		"https://github.com/brendangregg/FlameGraph", "/tmp/flamegraph")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatalf("clone FlameGraph: %v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
