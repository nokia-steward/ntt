# Custom C/C++ test ports via the cabi/cgo bridge

The `ntt` binary published to releases is a static Go program. It
does **not** `dlopen()` test-port shared libraries at runtime.
Shipping a TTCN-3 suite that depends on a user-supplied C/C++ test
port means building a small purpose-built `ntt-<suite>` executable
that statically links the port's `.cc` / `.hh` against the bridge.

This page is the canonical recipe. It assumes you already have a
working test port in Eclipse Titan style (`MyClient_PT.hh` /
`.cc` etc.) and a TTCN-3 suite that declares the matching port
types (`port MyClient_PT cli` in some component).

If you are starting from scratch, read the
[runtime/port/api/cabi/cgo](../runtime/port/api/cabi/cgo/doc.go)
package overview first; it walks through the C ABI in detail.

## 1. Quick build (`-tags cabicgo`)

The shortest path is to rebuild `ntt` itself with the
`cabicgo` build tag. The tag pulls
[runtime/port/api/cabi/cgo](../runtime/port/api/cabi/cgo/doc.go) in
via [`main_cabicgo.go`](../main_cabicgo.go), which transitively
registers the bridge as the runtime's port-driver provider:

```shell
go build -tags cabicgo -o bin/ntt-cabicgo .
```

The resulting `bin/ntt-cabicgo` is functionally identical to the
default `bin/ntt` for every TTCN-3 program that does not use C/C++
ports. For programs that *do* use them, every TTCN-3
`p.send(...)` / `map(self:p, system:p)` operation against an
instance whose port type matches a `ntt_port_register()`-ed name
is routed through the C side automatically.

This single command is enough when your test port is built into
the same Go module (e.g. via cgo `//go:linkname` in a sibling
package). Suites that link a standalone `.so` / `.a` need step 2.

## 2. Custom binary with statically-linked C++ ports

The standard pattern is a small `cmd/ntt-mysuite/` subdirectory in
your own project:

```
cmd/ntt-mysuite/
  main.go            # calls runtime/exec with interpreterdriver
  register.go        # cgo registration wrapper
  ports/
    MyClient_PT.cc # your existing Titan-style ports
    MyClient_PT.hh
    MyServer_PT.cc
    MyServer_PT.hh
```

### `cmd/ntt-mysuite/main.go`

For product canaries that only need `ntt exec`, depend on the importable
interpreter driver and the generic executor:

```go
package main

import (
  "context"
  "fmt"
  "os"

  nttexec "github.com/nokia/ntt/runtime/exec"
  "github.com/nokia/ntt/runtime/cfg"
  "github.com/nokia/ntt/runtime/exec/interpreterdriver"
  "github.com/nokia/ntt/runtime/report"
)

func main() {
  cfgFile, _, err := cfg.Load("cfg/standalone.cfg")
  if err != nil { panic(err) }
  files := interpreterdriver.CollectTTCN3Files([]string{"."})
  suite, err := nttexec.Run(context.Background(), nttexec.Options{
    SuiteName: "ntt",
    Driver:    interpreterdriver.New(files),
    Config:    cfgFile,
  })
  if err != nil { panic(err) }
  fmt.Fprintf(os.Stdout, "suite %q: %s (%d cases)\n",
    suite.Name, suite.Verdict(), len(suite.Cases))
  _ = report.RenderJSON // use report.Render* for CI formats
}
```

An external test-port runner is the concrete pattern: it accepts
`exec --cfg <file>`, renders the same text format as `ntt exec`, and
links the suite's C++ HTTP ports.

### `cmd/ntt-mysuite/register.go`

```go
//go:build cgo

package main

/*
#cgo LDFLAGS: -lssl -lcrypto -lpthread -lstdc++

extern int register_my_ports(void);
*/
import "C"

import _ "github.com/nokia/ntt/runtime/port/api/cabi/cgo"

func init() { C.register_my_ports() }
```

The blank import installs the cgo bridge as the runtime's port-driver
provider. The explicit C call registers the suite-local ports in this
binary.

### `cmd/ntt-mysuite/ports/ports.go`

Tell `cgo` which C/C++ sources to compile in. This is a
single-file shim that lives next to your port `.cc` / `.hh`:

```go
//go:build cgo

package ports

/*
#cgo CXXFLAGS: -std=c++17 -I${SRCDIR}
#cgo CFLAGS:   -I${SRCDIR}
#cgo LDFLAGS:  -lssl -lcrypto -lpthread

#include "MyClient_PT.hh"
#include "MyServer_PT.hh"

// One-shot registration: declared in the port .cc files, invoked
// from a Go side init() (see init_ports.go below).
extern int register_my_ports(void);
*/
import "C"

func init() { C.register_my_ports() }
```

The corresponding `register_my_ports()` lives in one of the port
`.cc` files and calls `ntt_port_register()` for each port type the
suite uses:

```cpp
// MyClient_PT.cc
#include "ntt_port.h"
#include "ntt_titan_compat.h"  // Titan-style helpers (TTCN_Buffer, Handler_Add_Fd_Read, ...)

static int on_map(void *user)  { /* open socket */ return 0; }
static int on_unmap(void *user){ /* close      */ return 0; }
static int on_send(void *user, NTT_Buffer payload) { /* write */ return 0; }

extern "C" int register_my_ports(void) {
  static NTT_TestPort http_client = {
    .name     = "MyClient_PT",
    .user     = nullptr,
    .on_map   = on_map,
    .on_unmap = on_unmap,
    .send     = on_send,
  };
  return ntt_port_register(&http_client);
}
```

### Build

```shell
CGO_CFLAGS="-I/path/to/ntt/runtime/port/api/cabi" \
CGO_CXXFLAGS="-std=c++17 -I/path/to/ntt/runtime/port/api/cabi" \
  go build -o bin/ntt-mysuite ./cmd/ntt-mysuite/
```

The resulting `bin/ntt-mysuite` is a drop-in replacement for `ntt`:

```shell
bin/ntt-mysuite exec --cfg cfg/standalone.cfg
```

`p.send(req)` on the `MyClient_PT cli` port now calls
`on_send()` in your C++ code; incoming traffic that the port
pushes back via `ntt_port_inject()` (or the soon-to-land
companion API) shows up on the matching TTCN-3 `cli.receive(...)`.

## 3. JSON-encoded payloads

By default the bridge passes payloads to the C side as raw bytes:

* `[]byte` payloads → bytes verbatim
* `string` payloads → UTF-8 bytes
* every other `runtime.Object` (typed records, lists, integers,
  booleans, …) is **JSON-encoded** by
  [`runtime/port/api/cabi/cgo/codec.go`](../runtime/port/api/cabi/cgo/codec.go)
  before reaching the C `send` hook.

So a TTCN-3 program that sends a record:

```ttcn3
// `port` is a TTCN-3 keyword, so the field is named serverPort.
type record HttpRequest { charstring path, integer serverPort };

template HttpRequest probe := { path := "/probe/liveness", serverPort := 8080 };

cli.send(probe);
```

reaches the C side as the JSON document

```json
{"path":"/probe/liveness","serverPort":8080}
```

with deterministic field ordering (lexicographic). Any mainstream
JSON library (cJSON, nlohmann/json, jansson, …) decodes that
without extra glue.

If your port wants raw octets instead of JSON, declare the
payload as `octetstring` and the bridge will hand the bytes
straight through.

## 4. Migrating an existing Titan test port

The header
[`runtime/port/api/cabi/ntt_titan_compat.h`](../runtime/port/api/cabi/ntt_titan_compat.h)
maps the Titan primitives the average test port uses
(`TTCN_Buffer`, `Handler_Add_Fd_Read`, `Handler_Add_Fd_Write`,
`Handler_Remove_Fd`, `TTCN_Logger::log_event_str`) onto the
matching ntt C ABI symbols. In practice migrating a Titan port
is a two-line change at the top of every `.cc` file:

```cpp
// Before:
#include <TTCN3.hh>

// After:
#include "ntt_port.h"
#include "ntt_titan_compat.h"
```

…and the constructor that used to register the port with the
Titan runtime now calls `ntt_port_register(&self)` exactly once
from a Go-driven `init()` (see step 2 above).

## 5. Troubleshooting

* `ntt exec` reports `undefined value` for every `setverdict(...)`
  argument that depends on a port operation → the bridge is not
  loaded. Confirm the binary was built with `-tags cabicgo` (the
  default `bin/ntt` does **not** include the bridge).
* The C `on_send` hook is never called → the TTCN-3 port type name
  does not match the string passed to `ntt_port_register()`. The
  bridge looks up the driver by *port type name*; the
  *instance name* (`cli` in `port MyClient_PT cli`) is only used
  as a fallback.
* The C++ build fails with `undefined reference to
  ntt_port_register` → the cgo bridge package is not in the binary.
  Re-check that step 2's `cgo_bridge.go` exists and is compiled
  (the `//go:build cgo` tag must be active; running `go build`
  without `-tags cabicgo` is fine for the bridge itself, but the
  custom binary must opt in to keep the runtime registration alive).
* `payloadBytes` returns `errUnsupportedPayload` → your TTCN-3
  value is of a type the JSON encoder cannot handle (e.g. a
  `TypeDesc` reference or a lazy thunk). File an issue; this
  usually points at an interpreter bug rather than a missing
  encoder rule.

## 6. Compatibility matrix

| ntt feature                          | Available in default `bin/ntt` | Available with `-tags cabicgo` |
| ------------------------------------ | ------------------------------ | ------------------------------ |
| Loopback ports (test talks to itself)| yes                            | yes                            |
| Cross-module symbol resolution       | yes                            | yes                            |
| C/C++ test ports via `ntt_port_register` | **no**                     | yes                            |
| File-descriptor event loop hooks     | no                             | yes                            |
| Titan-compat shim                    | no                             | yes (via `ntt_titan_compat.h`) |

If you only need loopback ports the default binary is the right
one. The `cabicgo` tag is needed exclusively for suites that link
real C/C++ test ports.
