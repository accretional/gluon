# Google Repos — Notes

Notes on the repositories in `google_repos.textproto`. Only includes information
the author is confident about; absence of a repo from this list does not mean
anything — it just means no notes were written.

## google/xls
XLS is Google's hardware synthesis toolchain. It includes a high-level
intermediate representation (IR) for hardware, a compiler from that IR to
Verilog, and tooling for formal verification. Uses protobufs extensively for
IR representation and configuration.

## google/gvisor
gVisor is a container sandbox runtime that implements a substantial portion of
the Linux system call interface in Go. It can run as an OCI runtime. Uses
protobufs for configuration, state serialization, and internal control plane
communication.

## google/perfetto
Perfetto is a production-grade tracing ecosystem for Android, Linux, and
Chrome. Its trace format is protobuf-based (`perfetto.protos.Trace`), and it
defines a large number of proto messages for trace events, metrics, and
configuration. One of the heaviest protobuf users in the list.

## google/or-tools
Google OR-Tools is a combinatorial optimization library supporting constraint
programming, linear programming, routing, and related solvers. Uses protobufs
for model serialization (e.g. `CpModelProto`).

## google/sentencepiece
SentencePiece is an unsupervised text tokenizer and detokenizer for neural
network-based text processing. Its model format is protobuf-based
(`ModelProto`).

## google/gnostic
gnostic is a compiler for OpenAPI specifications. It parses OpenAPI (Swagger)
documents and represents them as protocol buffer messages, making it directly
relevant to protobuf tooling.

## google/protobuf.dart
The official Dart protobuf runtime library and protoc plugin. Defines and
consumes `.proto` files as part of its core function.

## google/cel-spec
The Common Expression Language (CEL) specification. CEL is used in various
Google APIs for policy and filtering expressions. The spec defines its syntax
and semantics via protobufs.

## google/certificate-transparency-go
Go implementation of Certificate Transparency (RFC 6962). Uses protobufs for
log entry serialization and API communication.

## google/dawn
Dawn is Google's implementation of the WebGPU and WebGPU Shading Language
(WGSL) standards. It serves as the backend for Chromium's WebGPU.

## google/binexport
BinExport is a binary export tool for disassemblers (IDA Pro, Ghidra, Binary
Ninja) that exports disassembly into a protobuf format for use with BinDiff
and other analysis tools.

## google/bindiff
BinDiff is a binary diffing tool for comparing disassemblies. Consumes
BinExport's protobuf format.

## google/osv-scalibr
SCALIBR (Software Composition Analysis Library) is a library for extracting
software inventory and detecting vulnerabilities. Part of the OSV ecosystem.

## google/clusterfuzz
ClusterFuzz is a scalable fuzzing infrastructure used by Google for finding
bugs in Chrome and other projects. Uses OSS-Fuzz. Protobufs for configuration
and results.

## google/nsjail
nsjail is a lightweight process isolation tool (sandbox) for Linux, using
namespaces, cgroups, and seccomp-bpf. Its configuration format is protobuf
text format.

## google/fscrypt
fscrypt is a tool for managing Linux filesystem encryption (fscrypt policies).
Go-based.

## google/j2objc
J2ObjC is a translator from Java source to Objective-C, used for sharing
Java code with iOS applications.

## google/libprotobuf-mutator
A library for structured fuzzing of protobuf messages. Directly relevant to
protobuf tooling — it mutates proto messages for fuzz testing.

## google/struct2tensor
Converts structured data (e.g. protocol buffer messages) to TensorFlow
tensors. The name directly reflects proto-to-tensor conversion.

## google/s2a-proto
Proto definitions for S2A (Session and Secure Agent), Google's mTLS
authentication system.

## google/tensorstore
TensorStore is a library for reading and writing large multi-dimensional
arrays. C++ and Python, used in ML workflows.

## google/tsl
TSL (TensorFlow Support Library) contains shared infrastructure extracted
from TensorFlow, including protobuf definitions.

## openxla/xla
XLA (Accelerated Linear Algebra) is a compiler for ML workloads, originally
part of TensorFlow, now a standalone project under OpenXLA. Heavy protobuf
usage for HLO IR representation.

## google/deps.dev
deps.dev provides dependency information for open-source packages. The
repository contains API definitions and tooling.

## google/silifuzz
SiliFuzz is a system for finding CPU bugs by running randomly generated
programs and checking for divergence. Uses protobufs for test case
representation.

## transparency-dev/tessera
Tessera is a Go library for building transparency log-backed map/tile servers.
Related to the certificate transparency ecosystem.

## google/capslock
Capslock is a capability analysis tool for Go packages. It determines what
privileged operations a Go package can perform.

## google/oss-rebuild
Tools for verifying the reproducibility of open-source package builds.

## google/yggdrasil-decision-forests
A library for training, serving, and interpreting decision forest models
(random forests, gradient-boosted trees). Uses protobufs for model
serialization.

## google/turbine
A Java header-only compiler, used to speed up Java compilation in build
systems like Bazel.

## google/ebpf-transport-monitoring
eBPF-based network transport monitoring tool.

## google/webrisk
Client library for the Google Web Risk API (successor to Safe Browsing).
