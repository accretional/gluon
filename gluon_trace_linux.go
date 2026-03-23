//go:build linux

package gluon

import (
	"github.com/accretional/gluon/pb"
	"github.com/accretional/gluon/tracer"
	"google.golang.org/grpc"
)

// registerTraceServices registers the ptrace service.
// This is a Linux-only implementation.
func registerTraceServices(srv *grpc.Server) {
	pb.RegisterPtraceServer(srv, tracer.NewPtraceServer())
}
