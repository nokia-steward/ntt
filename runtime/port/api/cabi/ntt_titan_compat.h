/*
 * ntt_titan_compat.h - thin Titan -> ntt compatibility shim.
 *
 * Existing Eclipse Titan test ports use a small subset of the
 * `TTCN3.hh` surface area: TTCN_Buffer, Handler_Add_Fd_Read,
 * Handler_Add_Fd_Write, Handler_Remove_Fd, TTCN_Logger::log_event_str
 * (for diagnostic messages). This header maps those names onto the
 * ntt C ABI so a port like MyServer_PT.cc can be rebuilt against
 * ntt by changing only its top-level include:
 *
 *     #include <TTCN3.hh>            ->   #include <ntt_port.h>
 *                                          #include <ntt_titan_compat.h>
 *
 * The compat layer is intentionally minimal - anything beyond fd
 * watching and a fixed-size byte buffer needs the port to be ported
 * to the native ntt API. This shim's only job is to remove the busy
 * work of search-and-replace for the boilerplate parts.
 */
#ifndef NTT_TITAN_COMPAT_H
#define NTT_TITAN_COMPAT_H

#include <stddef.h>
#include <stdint.h>
#include <string.h>

#include "ntt_port.h"

#ifdef __cplusplus
extern "C" {
#endif

/*
 * TTCN_Buffer-shaped append-only byte buffer. The runtime never
 * touches it - it exists purely so existing Titan ports compile.
 * Implementations should not assume any of Titan's heavy-weight
 * methods (e.g. encode/decode helpers) - only the byte accessors
 * and length / clear / put_xxx are provided.
 */
typedef struct TTCN_Buffer {
    uint8_t *data;
    size_t   len;
    size_t   cap;
} TTCN_Buffer;

static inline void TTCN_Buffer_init(TTCN_Buffer *b) {
    b->data = NULL; b->len = 0; b->cap = 0;
}
static inline void TTCN_Buffer_free(TTCN_Buffer *b) {
    free(b->data); b->data = NULL; b->len = 0; b->cap = 0;
}
static inline void TTCN_Buffer_clear(TTCN_Buffer *b) { b->len = 0; }
static inline void TTCN_Buffer_put(TTCN_Buffer *b, const void *src, size_t n) {
    if (b->len + n > b->cap) {
        size_t ncap = (b->cap == 0) ? 256 : b->cap;
        while (ncap < b->len + n) { ncap *= 2; }
        b->data = (uint8_t *)realloc(b->data, ncap);
        b->cap  = ncap;
    }
    memcpy(b->data + b->len, src, n);
    b->len += n;
}

/*
 * Titan-named aliases for the ntt event-loop primitives. Existing
 * ports use Handler_Add_Fd_Read / Handler_Remove_Fd unchanged.
 */
#define Handler_Add_Fd_Read(fd, cb, user)  ntt_event_loop_add_fd_read((fd), (cb), (user))
#define Handler_Add_Fd_Write(fd, cb, user) ntt_event_loop_add_fd_write((fd), (cb), (user))
#define Handler_Remove_Fd(fd)              ntt_event_loop_remove_fd((fd))

/*
 * Titan's TTCN_Logger::log_event_str collapses to a tagged stderr
 * write in the ntt port; the runtime captures stderr per testcase
 * so the message still surfaces in the final report.
 */
#define TTCN_log_event_str(msg) fprintf(stderr, "[ntt-port] %s\n", (msg))

#ifdef __cplusplus
} // extern "C"
#endif

#endif /* NTT_TITAN_COMPAT_H */
