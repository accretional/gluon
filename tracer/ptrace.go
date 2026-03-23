//go:build linux

package tracer

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"

	"github.com/accretional/gluon/pb"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PtraceServer implements pb.PtraceServer using Linux ptrace with software
// breakpoints. It launches the target binary, inserts INT3 (0xCC) at every
// non-runtime function entry, and streams a TraceEvent for each call observed.
type PtraceServer struct {
	pb.UnimplementedPtraceServer
}

// NewPtraceServer creates a new PtraceServer.
func NewPtraceServer() *PtraceServer { return &PtraceServer{} }

// Run traces a process and streams TraceEvent messages until the process exits
// or the client cancels. The target is either a new process to launch or an
// existing PID to attach to.
func (s *PtraceServer) Run(req *pb.TraceRequest, stream grpc.ServerStreamingServer[pb.TraceEvent]) error {
	ctx := stream.Context()

	var binaryPath string
	var attach bool

	switch t := req.Target.(type) {
	case *pb.TraceRequest_Launch:
		if t.Launch.GetBinary() == "" {
			return status.Error(codes.InvalidArgument, "launch.binary is required")
		}
		binaryPath = t.Launch.GetBinary()
	case *pb.TraceRequest_Pid:
		if t.Pid <= 0 {
			return status.Error(codes.InvalidArgument, "pid must be > 0")
		}
		// Resolve the binary path from /proc so we can load symbols.
		path, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", t.Pid))
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "resolve binary for pid %d: %v", t.Pid, err)
		}
		binaryPath = path
		attach = true
	default:
		return status.Error(codes.InvalidArgument, "one of launch or pid is required")
	}

	syms, err := LoadSymbols(binaryPath)
	if err != nil {
		return status.Errorf(codes.Internal, "load symbols: %v", err)
	}
	if len(syms) == 0 {
		return status.Error(codes.InvalidArgument, "no traceable symbols found in binary (strip binary?)")
	}

	goidOff, err := GoroutineIDOffset(binaryPath)
	if err != nil {
		log.Printf("ptrace: goroutine ID disabled: %v", err)
		goidOff = 0
	}

	nameByAddr := addrToName(syms)

	pidCh := make(chan int, 1)
	type result struct{ err error }
	done := make(chan result, 1)

	// All ptrace calls must come from a single OS thread.
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		var pid int
		if attach {
			pid = int(req.GetPid())
			if err := attachToProcess(pid); err != nil {
				done <- result{status.Errorf(codes.Internal, "attach to pid %d: %v", pid, err)}
				return
			}
		} else {
			var err error
			pid, err = startTracedProcess(binaryPath, req.GetLaunch().GetArgs())
			if err != nil {
				done <- result{status.Errorf(codes.Internal, "start process: %v", err)}
				return
			}
		}
		pidCh <- pid
		done <- result{runPtraceLoop(pid, syms, nameByAddr, goidOff, stream)}
	}()

	select {
	case r := <-done:
		return r.err
	case <-ctx.Done():
		pid := <-pidCh
		if pid != 0 {
			if attach {
				// Detach cleanly — don't kill a process we didn't start.
				unix.PtraceDetach(pid)
			} else {
				unix.Kill(pid, unix.SIGKILL)
			}
		}
		<-done
		return ctx.Err()
	}
}

// attachToProcess attaches to an already-running process via PTRACE_ATTACH,
// which sends SIGSTOP to the target and waits for it to stop.
func attachToProcess(pid int) error {
	if err := unix.PtraceAttach(pid); err != nil {
		return fmt.Errorf("PTRACE_ATTACH: %w", err)
	}
	var ws unix.WaitStatus
	if _, err := unix.Wait4(pid, &ws, 0, nil); err != nil {
		return fmt.Errorf("wait after attach: %w", err)
	}
	if !ws.Stopped() {
		return fmt.Errorf("expected stopped after attach, got status %v", ws)
	}
	return nil
}

// startTracedProcess forks and execs the binary with PTRACE_TRACEME set in the
// child. It waits for the initial SIGTRAP that the kernel delivers after exec.
func startTracedProcess(binary string, args []string) (int, error) {
	cmd := exec.Command(binary, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Ptrace: true}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("exec: %w", err)
	}
	pid := cmd.Process.Pid

	var ws unix.WaitStatus
	if _, err := unix.Wait4(pid, &ws, 0, nil); err != nil {
		return 0, fmt.Errorf("initial wait: %w", err)
	}
	if !ws.Stopped() {
		return 0, fmt.Errorf("expected stopped, got status %v", ws)
	}
	return pid, nil
}

// runPtraceLoop sets ptrace options, inserts breakpoints, and runs the event
// loop until all threads in the tracee exit.
func runPtraceLoop(
	rootPID int,
	syms []Symbol,
	nameByAddr map[uint64]string,
	goidOff uint64,
	stream grpc.ServerStreamingServer[pb.TraceEvent],
) error {
	opts := unix.PTRACE_O_TRACECLONE |
		unix.PTRACE_O_TRACEFORK |
		unix.PTRACE_O_TRACEVFORK |
		unix.PTRACE_O_EXITKILL |
		unix.PTRACE_O_TRACESYSGOOD

	if err := unix.PtraceSetOptions(rootPID, opts); err != nil {
		return status.Errorf(codes.Internal, "PtraceSetOptions: %v", err)
	}

	// Save original bytes and insert INT3 at each function entry.
	origByte := make(map[uint64]byte, len(syms))
	for _, s := range syms {
		b, err := peekByte(rootPID, s.Addr)
		if err != nil {
			continue
		}
		if err := pokeByte(rootPID, s.Addr, 0xCC); err != nil {
			continue
		}
		origByte[s.Addr] = b
	}

	// threads tracks all known tracee thread PIDs.
	threads := map[int]bool{rootPID: true}

	// pendingReinsert maps a thread PID to the breakpoint address that needs
	// to be re-inserted after the single-step that follows a breakpoint hit.
	pendingReinsert := make(map[int]uint64)

	// Resume the initial stop.
	if err := unix.PtraceCont(rootPID, 0); err != nil {
		return status.Errorf(codes.Internal, "initial PtraceCont: %v", err)
	}

	for len(threads) > 0 {
		var ws unix.WaitStatus
		wpid, err := unix.Wait4(-1, &ws, 0, nil)
		if err != nil {
			break
		}

		// Thread exited — remove from tracking.
		if ws.Exited() || ws.Signaled() {
			delete(threads, wpid)
			continue
		}

		if !ws.Stopped() {
			unix.PtraceCont(wpid, 0) //nolint:errcheck
			continue
		}

		sig := ws.StopSignal()

		// Handle ptrace clone/fork/vfork events: register the new thread.
		if sig == unix.SIGTRAP {
			cause := ws.TrapCause()
			switch cause {
			case unix.PTRACE_EVENT_CLONE,
				unix.PTRACE_EVENT_FORK,
				unix.PTRACE_EVENT_VFORK:
				newPID, _ := unix.PtraceGetEventMsg(wpid)
				threads[int(newPID)] = true
				unix.PtraceSetOptions(int(newPID), opts) //nolint:errcheck
				unix.PtraceCont(wpid, 0)                 //nolint:errcheck
				continue
			}
		}

		// After a single-step, re-insert the breakpoint and resume.
		if addr, ok := pendingReinsert[wpid]; ok {
			delete(pendingReinsert, wpid)
			pokeByte(wpid, addr, 0xCC) //nolint:errcheck
			unix.PtraceCont(wpid, 0)   //nolint:errcheck
			continue
		}

		// A SIGTRAP that is not from a ptrace event is a breakpoint hit.
		if sig == unix.SIGTRAP {
			var regs unix.PtraceRegsAmd64
			if err := unix.PtraceGetRegsAmd64(wpid, &regs); err != nil {
				unix.PtraceCont(wpid, 0) //nolint:errcheck
				continue
			}

			// INT3 advances RIP by 1; the function entry is at RIP-1.
			hitAddr := regs.Rip - 1

			callee, known := nameByAddr[hitAddr]
			if !known {
				// Not our breakpoint — pass through.
				unix.PtraceCont(wpid, int(sig)) //nolint:errcheck
				continue
			}

			// Caller: the return address sitting at the top of the stack.
			retAddr := peekUint64(wpid, regs.Rsp)
			caller := findContainingFunction(syms, retAddr)
			if caller == "" {
				caller = fmt.Sprintf("0x%x", retAddr)
			}

			// Goroutine ID: R14 holds *runtime.g in Go 1.17+.
			var goroutineID int64
			if goidOff > 0 && regs.R14 != 0 {
				goroutineID = int64(peekUint64(wpid, regs.R14+goidOff))
			}

			event := &pb.TraceEvent{
				Caller:      caller,
				Callee:      callee,
				GoroutineId: goroutineID,
				TimestampNs: time.Now().UnixNano(),
			}
			if err := stream.Send(event); err != nil {
				return err
			}

			// Restore original byte, rewind RIP, single-step to re-execute it,
			// then re-insert the breakpoint after the step.
			if orig, ok := origByte[hitAddr]; ok {
				pokeByte(wpid, hitAddr, orig) //nolint:errcheck
				regs.Rip = hitAddr
				unix.PtraceSetRegsAmd64(wpid, &regs) //nolint:errcheck
				pendingReinsert[wpid] = hitAddr
				unix.PtraceSingleStep(wpid) //nolint:errcheck
				continue
			}
		}

		// Pass non-SIGTRAP signals back to the tracee.
		deliverSig := 0
		if sig != unix.SIGTRAP {
			deliverSig = int(sig)
		}
		unix.PtraceCont(wpid, deliverSig) //nolint:errcheck
	}

	return nil
}

// peekByte reads a single byte from the tracee's text segment.
func peekByte(pid int, addr uint64) (byte, error) {
	buf := make([]byte, 1)
	_, err := unix.PtracePeekText(pid, uintptr(addr), buf)
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}

// pokeByte writes a single byte into the tracee's text segment using a
// read-modify-write on the underlying machine word.
func pokeByte(pid int, addr uint64, b byte) error {
	// Align down to word boundary.
	aligned := addr &^ 7
	word := make([]byte, 8)
	if _, err := unix.PtracePeekText(pid, uintptr(aligned), word); err != nil {
		return err
	}
	word[addr-aligned] = b
	_, err := unix.PtracePokeText(pid, uintptr(aligned), word)
	return err
}

// peekUint64 reads an 8-byte little-endian value from the tracee's address space.
func peekUint64(pid int, addr uint64) uint64 {
	buf := make([]byte, 8)
	if _, err := unix.PtracePeekData(pid, uintptr(addr), buf); err != nil {
		return 0
	}
	return binary.LittleEndian.Uint64(buf)
}
