# Live testing and profiling: a runnable example

Drives a **real TCP system under test** from TTCN-3 and reports its
latency — no Go code, no C test port. Everything that binds the port lives
in [`app.cfg`](app.cfg)'s `[TESTPORT_PARAMETERS]` block.

Full reference: [docs/live-testing-and-profiling.md](../../docs/live-testing-and-profiling.md).

## 1. Start a SUT

Any line-echo endpoint on `127.0.0.1:19000` works. With `ncat`:

```shell
ncat -lk 127.0.0.1 19000 --exec /bin/cat
```

or with Python, if you don't have `ncat`:

```shell
python3 -c '
import socket, threading
srv = socket.socket(); srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
srv.bind(("127.0.0.1", 19000)); srv.listen(8)
def handle(c):
    f = c.makefile("rwb", buffering=0)
    for line in f: f.write(line)
    c.close()
while True:
    c, _ = srv.accept(); threading.Thread(target=handle, args=(c,), daemon=True).start()'
```

## 2. Run it

Functional run — just the verdicts:

```shell
ntt exec --cfg app.cfg app.ttcn
```

With a performance profile:

```shell
ntt exec --cfg app.cfg --profile --format=profile app.ttcn
```

```
profile "ntt": pass (2 cases in 2.77723ms)

app.tc_echo  [pass]  2.134187ms
  port                     sent     recv       recv/s        min        p50        p90        p99
  p                          20       20       9436.5   38.259µs   44.653µs  109.682µs  489.831µs

app.tc_latency_budget  [pass]  637.688µs
  port                     sent     recv       recv/s        min        p50        p90        p99
  p                           1        1       1582.3   344.91µs   344.91µs   344.91µs   344.91µs
```

`--format=json` gives the same metrics machine-readably, and
`--format=html` writes a self-contained report with a **Performance
profile** table — handy as a CI artefact:

```shell
ntt exec --cfg app.cfg --profile --format=html --out ./report app.ttcn
```

## What the two testcases show

| Testcase            | Demonstrates                                                                 |
| ------------------- | ---------------------------------------------------------------------------- |
| `tc_echo`           | A request/response loop over a live socket. Each iteration is one latency sample, so `--profile` reports p50/p90/p99 across 20 transactions. |
| `tc_latency_budget` | Measuring a single round trip in-script with `rtt.read` (real wall time under `--live` / `--profile`) and asserting a budget. |

## Things worth trying

- **Stop the SUT** and re-run: the run fails at `map` time with a dial
  error instead of silently reporting a pass — the engine never fabricates
  a verdict for a SUT it could not reach.
- **Point at a different SUT** by editing `address` (or `host`/`port`) in
  `app.cfg` — no recompilation.
- **Binary protocol**: add `*.p.framing := "length-prefix"` and change the
  port type to `octetstring`; frames become a 4-byte big-endian length plus
  raw bytes, so binary round-trips intact.
- **Per-component SUTs**: a `*` entry supplies defaults that a
  specific-component entry overrides (`server.p.address := "..."`), so one
  port name can reach different endpoints on different components.
