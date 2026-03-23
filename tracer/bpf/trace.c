//go:build ignore

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "GPL";

// event is sent to userspace for every observed function call.
struct event {
    __u64 caller_addr;  // return address sitting at top of stack (inside caller)
    __u64 callee_addr;  // function entry address passed via attach cookie
    __u64 goroutine_id; // runtime.g.goid, 0 if unavailable
    __u64 timestamp_ns; // monotonic clock nanoseconds (bpf_ktime_get_ns)
};

// Ring buffer shared with userspace. 1 MiB is enough for high-frequency bursts;
// the Go consumer should drain it quickly.
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 20);
} events SEC(".maps");

// config holds the byte offset of goid within runtime.g.
// Populated from Go userspace after loading the eBPF objects.
// Key 0 = goid_offset.
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u64);
} config SEC(".maps");

SEC("uprobe")
int trace_entry(struct pt_regs *ctx)
{
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e)
        return 0;

    // Callee: the address we attached the uprobe to, passed via cookie.
    // Requires kernel >= 5.15 (bpf_get_attach_cookie for uprobes).
    e->callee_addr = bpf_get_attach_cookie(ctx);

    // Caller: the return address at the top of the user stack.
    // On x86-64, CALL pushes the return address to [RSP] before jumping.
    __u64 caller_addr = 0;
    bpf_probe_read_user(&caller_addr, sizeof(caller_addr),
                        (void *)(unsigned long)PT_REGS_SP(ctx));
    e->caller_addr = caller_addr;

    // Goroutine ID: Go 1.17+ stores *runtime.g in R14 on amd64.
    // Read goid at the offset retrieved from the config map.
    __u32 key = 0;
    __u64 *goid_offset = bpf_map_lookup_elem(&config, &key);
    if (goid_offset && *goid_offset != 0) {
        __u64 g_ptr = ctx->r14;
        if (g_ptr != 0) {
            __u64 goid = 0;
            bpf_probe_read_user(&goid, sizeof(goid),
                                (void *)(unsigned long)(g_ptr + *goid_offset));
            e->goroutine_id = goid;
        }
    }

    e->timestamp_ns = bpf_ktime_get_ns();

    bpf_ringbuf_submit(e, 0);
    return 0;
}
