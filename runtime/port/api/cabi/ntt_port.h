/*
 * ntt_port.h - C ABI for TTCN-3 test ports under ntt.
 *
 * This header is the C entry point existing Titan-style test ports
 * compile against to plug into the ntt runtime. It mirrors the subset
 * of Titan's TTCN_Port surface we need for the M5 milestone:
 *
 *   - Map / Unmap / Start / Stop lifecycle hooks
 *   - Send (message ports) and Call (procedure ports)
 *   - Inject() callback the port uses to push incoming traffic back
 *     into the runtime
 *
 * Implementations subclass NTT_TestPort (struct of function pointers
 * plus a void* user state) and register an instance with
 * ntt_port_register(); the Go side instantiates a port.Driver around
 * the implementation and routes calls through it.
 *
 * The header is intentionally cgo-friendly: zero macros that would
 * confuse the Go cgo translator, no platform-specific includes, and
 * every struct field has an unambiguous fixed-width type.
 */
#ifndef NTT_PORT_H
#define NTT_PORT_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct NTT_Buffer {
    const uint8_t *data;
    size_t         len;
} NTT_Buffer;

typedef enum NTT_Verdict {
    NTT_VERDICT_NONE   = 0,
    NTT_VERDICT_PASS   = 1,
    NTT_VERDICT_INCONC = 2,
    NTT_VERDICT_FAIL   = 3,
    NTT_VERDICT_ERROR  = 4
} NTT_Verdict;

/*
 * Callback table the runtime fills in before invoking a port's hooks.
 * `inject` is the path back from C into Go: a port's I/O thread calls
 * it to push an incoming payload into the runtime's port in-queue.
 */
typedef struct NTT_Runtime {
    void  *handle;                             /* opaque, owned by Go */
    int   (*inject)(void *handle, NTT_Buffer payload);
} NTT_Runtime;

/*
 * The user-implementable test port. Function pointers map 1:1 to the
 * methods on the Go-side api.TestPort interface.
 */
typedef struct NTT_TestPort {
    const char *name;
    void       *user;          /* opaque port-specific state */

    int (*on_map)   (void *user);
    int (*on_unmap) (void *user);
    int (*on_start) (void *user);
    int (*on_stop)  (void *user);

    int (*send)     (void *user, NTT_Buffer payload);
    int (*call)     (void *user, NTT_Buffer payload, NTT_Buffer *reply);

    NTT_Runtime runtime;
} NTT_TestPort;

/*
 * Register a test port instance with the runtime. The runtime keeps
 * a pointer to *port for its entire lifetime, so callers must allocate
 * NTT_TestPort with static or heap storage (NOT stack).
 *
 * Returns 0 on success, -1 on registration failure (duplicate name).
 */
int ntt_port_register(NTT_TestPort *port);

/*
 * Unregister a previously registered test port. Safe to call multiple
 * times; calls after the first are no-ops.
 *
 * The pointer is logically read-only - the runtime never writes
 * through it - but the prototype omits `const` because cgo's
 * generated header emits `char *` for the matching Go //export. The
 * usual C++ implicit conversion still applies, so callers passing a
 * string literal compile cleanly without a cast.
 */
void ntt_port_unregister(char *name);

/*
 * fd event loop - the analogue of Titan's Handler_Add_Fd_Read /
 * Handle_Fd_Event_Readable. A test port that owns a listening socket
 * or any other file descriptor it can't poll inline registers it
 * here; the runtime calls back into the C side whenever the fd
 * becomes readable / writable.
 *
 * cb is called from the runtime's I/O goroutine, so implementations
 * are expected to do the read/accept and return promptly. The
 * runtime does NOT remove the fd after the callback fires - the
 * port stays armed for further events until ntt_event_loop_remove_fd
 * is called.
 */
typedef void (*NTT_FdReadyCallback)(int fd, void *user);

/*
 * Register fd for read-readiness notifications. cb runs when the
 * runtime sees the fd as readable (POLLIN). Returns 0 on success,
 * -1 on failure (bad fd, duplicate registration, runtime not yet
 * initialised).
 */
int ntt_event_loop_add_fd_read(int fd, NTT_FdReadyCallback cb, void *user);

/*
 * Register fd for write-readiness notifications. Same semantics as
 * add_fd_read but watches POLLOUT.
 */
int ntt_event_loop_add_fd_write(int fd, NTT_FdReadyCallback cb, void *user);

/*
 * Stop watching fd for any direction. Safe to call multiple times.
 */
void ntt_event_loop_remove_fd(int fd);

#ifdef __cplusplus
}
#endif

#endif /* NTT_PORT_H */
