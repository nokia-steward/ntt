# HTTPS/JSON testing and profiling: a runnable example

Drives a **real REST/JSON service over HTTPS** from TTCN-3 and reports its
latency — no Go code, no C test port, no custom adapter. Everything that
binds the transport lives in [`app.cfg`](app.cfg)'s
`[TESTPORT_PARAMETERS]` block.

Full reference: [docs/live-testing-and-profiling.md](../../docs/live-testing-and-profiling.md).

For the plain-TCP equivalent, see [`../live-testing/`](../live-testing/).

## 1. Start a SUT

Any HTTPS endpoint on `127.0.0.1:19443` serving `/healthz` and
`/api/v1/status` works. A self-contained stand-in, using a throwaway
self-signed certificate:

```shell
openssl req -x509 -newkey rsa:2048 -keyout key.pem -out cert.pem \
        -days 1 -nodes -subj "/CN=localhost"

python3 -c '
import http.server, json, ssl
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/healthz":
            body = b"{\"status\":\"ok\"}"
        elif self.path.startswith("/api/v1/status"):
            body = json.dumps({"state": "ready", "entities": 3}).encode()
        else:
            self.send_error(404); return
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers(); self.wfile.write(body)
    def log_message(self, *a): pass
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.load_cert_chain("cert.pem", "key.pem")
srv = http.server.ThreadingHTTPServer(("127.0.0.1", 19443), H)
srv.socket = ctx.wrap_socket(srv.socket, server_side=True)
srv.serve_forever()'
```

## 2. Run it

Functional run — just the verdicts:

```shell
ntt exec --cfg app.cfg app.ttcn
```

```
httpport: TLS certificate verification is DISABLED (insecure_skip_verify); the identity of the server is not checked
suite "ntt": pass (4 cases in 183.685974ms)
  pass    app.tc_health
  pass    app.tc_latency_budget
  pass    app.tc_load
  pass    app.tc_reports_status
```

The warning is deliberate and is printed once per run: the stand-in's
certificate is self-signed, so `app.cfg` disables verification. A run that
skipped verification cannot support a claim about *which* server answered.
Against a real service, point `ca_cert` at its CA and delete that line.

Same suite, same command, plus a performance profile:

```shell
ntt exec --cfg app.cfg --profile --format=profile app.ttcn
```

```
app.tc_load  [pass]  165.462641ms
  port       sent  recv    recv/s       min       p50       p90       p99
  p            50    50     302.2  1.489ms   3.053ms   4.573ms   5.048ms
```

`--profile` implies `--live`: latency is only meaningful on the real clock.
The testcases are unchanged between the two runs — that is the point.

## What to look at

| | |
| --- | --- |
| [`app.ttcn`](app.ttcn) | four testcases: liveness, a JSON body assertion, a 50-request load loop, and a per-request budget timed with `rtt.read` |
| [`app.cfg`](app.cfg) | the only place the transport appears |

Two things in `app.ttcn` are worth copying into your own suite:

**A failed request is a value, not a silence.** `TransportError` arrives as
its own inbound type with a matchable `reason`, so the testcase says
"nothing is listening" or "TLS failed" instead of timing out with no
explanation. `refused` and `reset` are retryable; `timeout` means the
service is there but wedged.

**Declare `TransportErrorReason` with its explicit values**, exactly as
written. A TTCN-3 enumeration numbers labels by declaration order unless
the values are given, and matching compares the integer as well as the
label — so reordering those lines without the numbers would silently stop
every `TransportError` template from matching, with no error.
