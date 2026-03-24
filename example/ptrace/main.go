//go:build linux

// ptrace_flamegraph connects to a running gluon server, traces a binary via
// the Ptrace RPC, and streams all observed call events to an intermediate file
// in real time. On exit (natural process exit or Ctrl+C), it reads the
// intermediate file and generates a flame graph SVG.
//
// The intermediate file is tab-separated:
//
//	caller\tcallee\tgoroutine_id\ttimestamp_ns
//
// It is human-readable and can be grepped/awk'd directly while the trace is
// still running.
//
// Usage:
//
//	ptrace_flamegraph -binary <path> [-addr <host:port>] [-events <file>] [-output <file>] [-- binary-args...]
//	ptrace_flamegraph -pid <pid>    [-addr <host:port>] [-events <file>] [-output <file>]
//
// If flamegraph.pl is not present at -flamegraph-pl it is cloned automatically
// from https://github.com/brendangregg/FlameGraph.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"time"
	"strings"
	"syscall"

	"github.com/accretional/gluon/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type edge struct{ caller, callee string }

type traceData struct {
	stackCounts map[string]int
	edgeCounts  map[edge]int
}

func main() {
	addr := flag.String("addr", "localhost:50051", "gluon server address")
	binary := flag.String("binary", "", "launch this binary as the trace target")
	pid := flag.Int("pid", 0, "attach to this already-running PID as the trace target")
	eventsFile := flag.String("events", "temp/ptrace_events.tsv", "intermediate events file written during tracing")
	output := flag.String("output", "temp/ptrace_flamegraph.svg", `flame graph SVG output; "" skips`)
	fgpl := flag.String("flamegraph-pl", "/tmp/flamegraph/flamegraph.pl", "path to flamegraph.pl")
	flag.Parse()
	binaryArgs := flag.Args()

	if *binary == "" && *pid == 0 {
		fmt.Fprintln(os.Stderr, "error: one of -binary or -pid is required")
		flag.Usage()
		os.Exit(1)
	}
	if *binary != "" && *pid != 0 {
		fmt.Fprintln(os.Stderr, "error: -binary and -pid are mutually exclusive")
		flag.Usage()
		os.Exit(1)
	}

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fatalf("dial %s: %v", *addr, err)
	}
	defer conn.Close()

	req := &pb.TraceRequest{}
	if *pid != 0 {
		req.Target = &pb.TraceRequest_Pid{Pid: int32(*pid)}
	} else {
		req.Target = &pb.TraceRequest_Launch{
			Launch: &pb.LaunchTarget{
				Binary: *binary,
				Args:   binaryArgs,
			},
		}
	}

	// Cancel the stream on SIGINT/SIGTERM so we can still generate outputs.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		fmt.Fprintln(os.Stderr, "\nstopping trace...")
		cancel()
	}()

	stream, err := pb.NewPtraceClient(conn).Run(ctx, req)
	if err != nil {
		fatalf("Ptrace.Run: %v", err)
	}

	label := *binary
	if *pid != 0 {
		label = fmt.Sprintf("pid:%d", *pid)
	}

	n, err := streamToFile(stream, *eventsFile)
	if err != nil {
		fatalf("stream: %v", err)
	}
	if n == 0 {
		fatalf("no trace events received — is the binary stripped?")
	}
	fmt.Fprintf(os.Stderr, "%d events written to %s\n", n, *eventsFile)

	if *output != "" {
		data, err := readEvents(*eventsFile)
		if err != nil {
			fatalf("read events: %v", err)
		}
		ensureFlamegraph(*fgpl)
		generateSVG(*fgpl, label, *output, data.stackCounts)
	}
}

// streamToFile reads TraceEvents from the stream and appends each event as a
// tab-separated line to path. Returns the number of events written.
// The file is flushed on every write so it is readable while tracing is live.
func streamToFile(stream pb.Ptrace_RunClient, path string) (int, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	fmt.Fprintln(w, "caller\tcallee\tgoroutine_id\ttime")

	n := 0
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Context cancellation (Ctrl+C) is a normal shutdown, not an error.
			if strings.Contains(err.Error(), "context canceled") {
				break
			}
			return n, fmt.Errorf("recv: %w", err)
		}

		caller := ev.GetCaller()
		if strings.HasPrefix(caller, "0x") {
			continue // unresolved address, skip
		}

		t := time.Unix(0, ev.GetTimestampNs()).Format("15:04:05.000")
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
			caller,
			ev.GetCallee(),
			ev.GetGoroutineId(),
			t,
		)
		n++

		// Flush periodically so the file is live-readable.
		if n%500 == 0 {
			w.Flush()
		}
	}
	w.Flush()
	return n, nil
}

// readEvents reads the intermediate TSV file and reconstructs traceData,
// applying the same per-goroutine stack reconstruction as the live collector.
func readEvents(path string) (traceData, error) {
	f, err := os.Open(path)
	if err != nil {
		return traceData{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	data := traceData{
		stackCounts: map[string]int{},
		edgeCounts:  map[edge]int{},
	}
	stacks := map[int64][]string{}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	scanner.Scan() // skip header

	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), "\t", 4)
		if len(fields) < 4 {
			continue
		}
		caller := fields[0]
		callee := fields[1]
		gid, _ := strconv.ParseInt(fields[2], 10, 64)

		data.edgeCounts[edge{caller, callee}]++

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

	return data, scanner.Err()
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

// generateSVG pipes folded stacks into flamegraph.pl and writes the SVG.
func generateSVG(fgpl, label, output string, counts map[string]int) {
	outFile, err := os.Create(output)
	if err != nil {
		fatalf("create %s: %v", output, err)
	}
	defer outFile.Close()

	cmd := exec.Command("perl", fgpl,
		"--title", "ptrace: "+filepath.Base(label),
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
