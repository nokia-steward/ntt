# Live testing and performance profiling

`ntt exec` runs the same strict TTCN-3 engine in two modes:

- **Functional (default)** — a deterministic discrete-event scheduler over
  a *virtual* clock. Timers fire instantly, concurrent components
  interleave deterministically, and verdicts are reproducible and free of
  real-clock races. This is the mode the conformance suite runs in.
- **Live (`--live`)** — the *same* engine on the *real* clock with real
  concurrency. Timers pace real I/O, so a testcase can drive a real system
  under test (SUT) over the network and measure how it actually behaves.

The clock is the only thing that changes: the semantics are identical.
Live mode is what turns a functional test into a load / latency probe,
and it is the foundation for **performance profiling** (`--profile`).

This page shows how to drive a live SUT over TCP with no user code, how to
measure latency from inside a testcase, and how to produce a performance
report. For a ready-to-run version of everything below, see
[`examples/live-testing/`](../examples/live-testing/).

> **Background.** The idea that a single functional TTCN-3 test can double
> as a performance probe comes from:
>
> Manuel Peuster, Christian Dröge, Clemens Boos, Holger Karl. *Joint testing
> and profiling of microservice-based network services using TTCN-3.* ICT
> Express, Volume 5, Issue 2, 2019, pp. 150–153.
> [doi:10.1016/j.icte.2019.02.001](https://doi.org/10.1016/j.icte.2019.02.001).
>
> This is that approach, realized on `ntt`'s strict engine.

## Contents

- [Two clocks: functional vs live](#two-clocks-functional-vs-live)
- [Measuring elapsed time in a testcase (`t.read`)](#measuring-elapsed-time-in-a-testcase-tread)
- [The built-in TCP test port](#the-built-in-tcp-test-port)
- [Configuring a TCP port from a `.cfg` file](#configuring-a-tcp-port-from-a-cfg-file)
- [Performance profiling (`--profile`)](#performance-profiling---profile)
- [A complete example](#a-complete-example)
- [Embedding the port from Go](#embedding-the-port-from-go)
- [Limitations and notes](#limitations-and-notes)

## Two clocks: functional vs live

```shell
# Functional: virtual clock, reproducible. A 30s timer fires instantly.
ntt exec MySuite.tc_smoke

# Live: real clock, real concurrency. A 30s timer waits 30 real seconds.
ntt exec --live MySuite.tc_smoke
```

Under `--live` a run is *functional but not reproducible-by-construction*:
verdicts depend on how the real SUT and the real network behave. Bound a
live run with `--timeout` (a 60-second safety timeout applies when unset):

```shell
ntt exec --live --timeout 30s MySuite.tc_load
```

`--live` is implied automatically when the engine needs the real clock —
when you `--profile`, or when a `.cfg` file wires an external transport
(see below). You rarely have to pass it by hand.

## Measuring elapsed time in a testcase (`t.read`)

On the real clock, `t.read` reports the actual wall-clock time elapsed
since the timer started, so a testcase can time an operation directly:

```ttcn3
timer rtt := 60.0;   // a default duration is required (ETSI 12); it is
rtt.start;           // just an upper bound here — we only ever `read` it
p.send(request);
p.receive(response);
if (rtt.read > 0.100) {
    setverdict(fail, "SUT slower than 100ms");
}
```

A timer used purely as a stopwatch still needs a default duration, since
`.start` without an argument requires one; `read` clamps to it, so pick a
bound comfortably above any latency you expect to measure.

Under the virtual clock (`ntt exec` without `--live`) `t.read` follows
ETSI §23.4 virtual-time semantics instead — a freshly started timer reads
`0.0` — so this pattern is meaningful only in live mode.

## The built-in TCP test port

`ntt` ships a built-in TCP test port. Mapping a port to it opens a real TCP
connection to the SUT:

- `map(self:p, system:sp)` dials the configured address.
- `p.send(v)` writes `v` as one frame.
- each inbound frame the SUT sends back is delivered to `p.receive`.
- `unmap` / test teardown closes the connection.

It supports two framings (the `framing` parameter, below):

- **newline** (default) — one text record per line. Payloads are
  `charstring`, written as UTF-8 + `\n` and delivered with the trailing
  newline stripped. Ideal for line / JSON / text protocols.
- **length-prefix** — a 4-byte big-endian length followed by that many raw
  bytes. Payloads are `octetstring`, so arbitrary binary (newlines and NULs
  included) round-trips intact.

The TTCN-3 side is a plain message port carrying the matching type:

```ttcn3
type port P message { inout charstring }   // newline framing
type port B message { inout octetstring }  // length-prefix framing
type component C { port P p; port B b }
```

A failed connection surfaces as an error at `map` time — the engine never
reports a pass for a SUT it could not reach.

## Configuring a TCP port from a `.cfg` file

You do **not** need to write any Go code to use the TCP port. Declare it in
the standard `[TESTPORT_PARAMETERS]` section of a module configuration
file, keyed by port instance name (`*` matches any component):

```ini
[TESTPORT_PARAMETERS]
*.p.transport := "tcp"
*.p.host      := "127.0.0.1"
*.p.port      := "9000"

[EXECUTE]
app.tc
```

Recognised parameters:

| Parameter      | Meaning                                                         |
| -------------- | -------------------------------------------------------------- |
| `transport`    | `"tcp"` selects the built-in TCP port                          |
| `host`, `port` | combined into `host:port`                                       |
| `address`      | `"host:port"` (overrides `host` + `port`)                      |
| `dial_timeout` | a Go duration (e.g. `"5s"`), default `10s`                     |
| `framing`      | `"newline"` (default, charstring) or `"length-prefix"` (octetstring) |

For a binary protocol, use length-prefix framing and an `octetstring` port:

```ini
[TESTPORT_PARAMETERS]
*.b.transport := "tcp"
*.b.address   := "127.0.0.1:9000"
*.b.framing   := "length-prefix"
```

## The built-in HTTP test port

Most services under test speak HTTP rather than a raw socket, so `ntt` also
ships an HTTP port. It models one request/response exchange as a record
pair:

```ttcn3
type record HttpRequest  { charstring method, charstring path, charstring body }
type record HttpResponse { integer status, charstring body }
type port ApiPort message { out HttpRequest; in HttpResponse }
type component C { port ApiPort p }
```

`map(self:p, system:sp)` binds the base URL, `p.send(req)` issues the
request, and the response arrives at `p.receive`:

```ttcn3
p.send(HttpRequest:{ method := "GET", path := "/api/v1/health", body := "" });
alt {
    [] p.receive(HttpResponse:{ status := 200, body := ? }) { setverdict(pass); }
    [] g.timeout { setverdict(fail, "no response"); }
}
```

Wire it from a `.cfg` the same way as TCP:

```ini
[TESTPORT_PARAMETERS]
*.p.transport := "http"
*.p.host      := "127.0.0.1"
*.p.port      := "8080"
# or: *.p.base_url := "http://127.0.0.1:8080"
# optional: *.p.scheme := "http"   *.p.timeout := "5s"
```

Request fields — only `path` is required; `method` defaults to `GET`, and
`contentType` defaults to `application/json` when a body is present.
Bodies stay `charstring`, so a JSON API composes with whatever types your
suite already generates from its schema; this port does not need to know
them.

**A failed request is visible, not silent — and classified.** A 4xx/5xx is
an ordinary response you match on: the SUT answered, it just said no. A
request that never completed is a different kind of event — the SUT was not
there — so it arrives as a *second inbound type* whose reason you can branch
on:

```ttcn3
type enumerated TransportErrorReason {
    refused(0), unreachable(1), timeout(2), dns(3), tls(4), other(5), reset(6)
}
type record TransportError { TransportErrorReason reason, charstring detail }
type port ApiPort message { out HttpRequest; in HttpResponse, TransportError }

alt {
    [] p.receive(HttpResponse:{ status := 200, body := ? })         { /* healthy */ }
    [] p.receive(HttpResponse:{ status := ?,   body := ? })         { /* answered; assert on it */ }
    [] p.receive(TransportError:{ reason := refused, detail := ? }) { /* nothing listening — retry */ }
    [] p.receive(TransportError:{ reason := reset,   detail := ? }) { /* dropped mid-request — retry */ }
    [] p.receive(TransportError:{ reason := timeout, detail := ? }) { /* listening but wedged */ }
    [] g.timeout                                                    { /* harness problem */ }
}
```

| `reason` | Meaning |
| --- | --- |
| `refused` | Connection actively refused — nothing listening |
| `unreachable` | No route to host, or the network is down |
| `timeout` | No answer within the deadline |
| `dns` | The name did not resolve |
| `tls` | Handshake or certificate verification failed |
| `reset` | Accepted, then dropped mid-request — a service being torn down |
| `other` | Anything unclassified |

`detail` carries the underlying message for logging. **Match on `reason`, not
on `detail`** — the message text is diagnostic, not API.

Declare the enumeration with the explicit values above. A TTCN-3
enumeration's integers come from declaration order unless written down, and
matching compares the integer as well as the label, so pinning them stops a
later reordering of those lines from silently breaking every
`TransportError` template.

Requests run on their own goroutine, so the engine never blocks on I/O.

### TLS and mutual TLS

An `https://` base URL enables TLS. By default the server is verified
against the system roots; for a service with its own CA, point at the
bundle:

```ini
[TESTPORT_PARAMETERS]
*.p.transport   := "http"
*.p.base_url    := "https://10.0.0.7:8443"
*.p.ca_cert     := "/etc/certs/ca.pem"
*.p.server_name := "my-service"          # SNI / cert name, e.g. when dialling by IP

# mutual TLS — both are required together
*.p.client_cert := "/etc/certs/tls.crt"
*.p.client_key  := "/etc/certs/tls.key"
```

Certificate files are read at **map** time, so a wrong path fails the map
operation and names the file, instead of surfacing later as a puzzling
transport error.

For a self-signed ("snake oil") endpoint in a test environment, verification
can be turned off:

```ini
*.p.insecure_skip_verify := "true"
```

This is supported deliberately, but it prints a warning on stderr each run,
because a suite that skipped verification cannot support a claim about *which*
server it talked to. Prefer `ca_cert` — pointing at the self-signed
certificate itself works and keeps verification on.

Not modelled: arbitrary request/response headers beyond content type, and
binary bodies.

### Per-component addresses

The `<component>` field selects which components a setting applies to. A `*`
(any-component) entry supplies defaults that a specific entry inherits and
overrides, so one port name can reach different SUTs on different
components:

```ini
[TESTPORT_PARAMETERS]
*.p.transport    := "tcp"           # default transport for every p
*.p.address      := "10.0.0.1:8080" # the default SUT
server.p.address := "10.0.0.2:8080" # component "server" talks elsewhere
mtc.p.address    := "10.0.0.3:8080" # the MTC talks elsewhere again
```

At map time the port picks the rule whose component best matches the
mapping component, in order: its **name** (from `C.create("name")`), then
**`mtc`** for the main test component, then its **component type** name,
then **`*`**. So `server` above may be a component name or a component
type; `mtc.p` overrides `*.p` for the MTC.

Run it with `--cfg`; the presence of an external transport switches the
engine to the real clock automatically:

```shell
ntt exec --cfg app.cfg app.ttcn
```

## Performance profiling (`--profile`)

`--profile` captures, per port, over a live run:

- **sends** and **receives** — message counts,
- **latency** — the send→receive round-trip time of each request/response,
  aggregated as min / p50 / p90 / p99,
- **throughput** — receives per second over the testcase duration.

`--profile` implies `--live` (latency is only meaningful on the real
clock). Report it as a table with `--format=profile`:

```shell
ntt exec --cfg app.cfg --profile --format=profile app.ttcn
```

```
profile "ntt": pass (1 cases in 1.081745ms)

app.tc  [pass]  1.078115ms
  port                     sent     recv       recv/s        min        p50        p90        p99
  p                           6        6       5644.9   39.417µs   53.826µs  353.346µs  353.346µs
```

or as machine-readable JSON with `--format=json` (each case gains a
`ports` array with a `latency` object in milliseconds):

```shell
ntt exec --cfg app.cfg --profile --format=json app.ttcn
```

```json
{
  "cases": [
    {
      "name": "tc", "module": "app", "verdict": "pass",
      "ports": [
        {
          "port": "p", "sends": 6, "receives": 6,
          "throughput_per_second": 5644.9,
          "latency": { "count": 6, "min_ms": 0.039, "p50_ms": 0.053,
                       "p90_ms": 0.353, "p99_ms": 0.353, "mean_ms": 0.12, "max_ms": 0.353 }
        }
      ]
    }
  ]
}
```

The latency model pairs each send with the next receive on the same port,
which is exactly a synchronous request/response transaction. Write the
testcase as a request/response loop and each iteration contributes one
latency sample:

```ttcn3
var integer i := 0;
while (i < 1000) {
    p.send(request);
    alt {
        [] p.receive(expected) { }
        [] g.timeout { setverdict(fail, "SUT stopped responding"); }
    }
    i := i + 1;
}
setverdict(pass);
```

## A complete example

Also available ready to run, with a copy-pasteable stand-in SUT, in
[`examples/live-testing/`](../examples/live-testing/).

`app.ttcn` — a request/response loop against a line-based SUT:

```ttcn3
module app {
  type port P message { inout charstring }
  type component C { port P p }
  testcase tc() runs on C system C {
    timer g := 5.0;
    map(self:p, system:p);
    var integer i := 0;
    while (i < 100) {
      g.start;
      p.send("ping");
      alt {
        [] p.receive("ping") { }
        [] g.timeout { setverdict(fail, "no reply"); }
      }
      i := i + 1;
    }
    setverdict(pass);
    unmap(self:p, system:p);
  }
}
```

`app.cfg` — point the port at your SUT:

```ini
[TESTPORT_PARAMETERS]
*.p.transport := "tcp"
*.p.host      := "127.0.0.1"
*.p.port      := "9000"

[EXECUTE]
app.tc
```

Run and profile it:

```shell
ntt exec --cfg app.cfg --profile --format=profile app.ttcn
```

Any line-based echo endpoint works as a stand-in SUT while you develop the
test — e.g. `ncat -lk 127.0.0.1 9000 --exec /bin/cat`.

## Embedding the port from Go

The `.cfg` route covers the built-in TCP port. To register a TCP port
programmatically (for example to build a custom `ntt-<suite>` binary), call
[`tcpport.Register`](../runtime/port/tcpport/tcpport.go) at startup:

```go
import "github.com/nokia/ntt/runtime/port/tcpport"

func init() {
    tcpport.Register("MyPort_PT", "127.0.0.1:9000")
    // or, for a binary protocol:
    tcpport.Register("MyBin_PT", "127.0.0.1:9001",
        tcpport.WithFraming(tcpport.FramingLengthPrefix),
        tcpport.WithDialTimeout(5*time.Second))
}
```

For a fully custom transport (a different wire protocol, a hardware
handle, a message bus), implement the pure-Go
[`api.TestPort`](../runtime/port/api/api.go) interface and register it with
[`goport.Register`](../runtime/port/goport/goport.go) — `OnMap` opens the
transport, `Send` writes, and your I/O goroutine calls `goport.Inject` to
push inbound traffic into the running testcase. The built-in TCP port is
itself a small `goport` implementation; read its source as a template. For
existing Titan-style C/C++ ports, see
[Custom C/C++ test ports](cabi-ports.md).

## Limitations and notes

- **Framing** is newline-delimited `charstring` by default, or
  length-prefixed `octetstring` (`framing := "length-prefix"`) for binary.
  A custom wire protocol (a different delimiter, TLS, a message bus) is a
  Go [`api.TestPort`](../runtime/port/api/api.go) — see below.
- **Component matching granularity.** A rule's component selector matches a
  component's name, type, `mtc`, or `*` (see above). Two *instances* of the
  same component type that need different addresses can't yet be told apart
  by the config unless they were created with distinct names.
- **Reproducibility.** Live verdicts depend on the real SUT and network;
  they are not reproducible-by-construction the way virtual-clock functional
  runs are. Keep functional conformance runs on the default clock.
- **Alt semantics hold on the real clock.** An `alt` still evaluates its
  branches against a snapshot taken at the start of each round (ETSI
  §20.2), so a message that arrives mid-evaluation cannot let a later
  catch-all clause jump ahead of an earlier, more specific one — even when
  the message arrives asynchronously from a live socket.
- **JSON bodies are `charstring`, by design.** The port does not decode
  them: your suite owns its schema, and a port that knew about it would be
  a port you had to modify per service. To assert on individual fields,
  decode in the suite with `decvalue_unichar(body, v, "JSON")` — which today
  handles scalars and enumerated values; decoding a JSON *object* into a
  record is scoped but not built (see
  [remaining-work.md §1z](conformance/remaining-work.md)). Until then,
  assertions are pattern matches against the body.

  Worth knowing regardless of decoding: **assert on a value you controlled
  in the stimulus**, not just on the value you expected back. Matching
  `"linkStatus":"down"` shows *something* is down; matching an identifier
  you set when provoking the change shows the thing you perturbed is the
  thing that changed — which is usually what the requirement actually says.
  Field access makes that assertion tidier, not stronger.
```
