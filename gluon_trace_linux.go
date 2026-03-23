//go:build linux

package gluon

import (
	"github.com/accretional/gluon/pb"
	"github.com/accretional/gluon/tracer"
	"google.golang.org/grpc"
)

// registerTraceServices registers the ptrace and eBPF trace services.
// These are Linux-only implementations.
func registerTraceServices(srv *grpc.Server) {
	pb.RegisterPtraceServer(srv, tracer.NewPtraceServer())
	pb.RegisterBPFTraceServer(srv, tracer.NewBPFTraceServer())
}
