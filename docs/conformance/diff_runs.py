#!/usr/bin/env python3
"""Diff two `ntt conformance --json` reports file by file.

Usage:
    python3 docs/conformance/diff_runs.py BEFORE.json AFTER.json

Reports the headline deltas and, more importantly, every file whose match
state or provenance changed. A slice is only safe to commit when the
`match: true -> false` list is empty; a pure provenance shift (e.g.
`static -> executed`) with the same match state means the file is now
genuinely executed rather than statically decided, which is the metric
`remaining-work.md` asks us to grow.
"""

import json
import sys


def load(path):
    with open(path) as fh:
        d = json.load(fh)
    return d, {r["path"]: r for r in d["results"]}


def main():
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    before, bmap = load(sys.argv[1])
    after, amap = load(sys.argv[2])

    print("headline")
    for k in ("total", "matched", "skipped", "pass_rate", "real_exec_rate"):
        b, a = before.get(k), after.get(k)
        if isinstance(b, float):
            print(f"  {k:<15} {b:.4f} -> {a:.4f}  ({a - b:+.4f})")
        else:
            print(f"  {k:<15} {b} -> {a}  ({a - b:+d})")

    print("\nprovenance")
    keys = sorted(set(before["provenance"]) | set(after["provenance"]))
    for k in keys:
        b = before["provenance"].get(k, 0)
        a = after["provenance"].get(k, 0)
        if b != a:
            print(f"  {k:<15} {b} -> {a}  ({a - b:+d})")

    regressed, fixed, moved = [], [], []
    for path, ar in amap.items():
        br = bmap.get(path)
        if br is None:
            continue
        if br["match"] and not ar["match"]:
            regressed.append((path, br, ar))
        elif not br["match"] and ar["match"]:
            fixed.append((path, br, ar))
        elif br.get("provenance") != ar.get("provenance"):
            moved.append((path, br, ar))

    def show(title, rows, limit=None):
        print(f"\n{title}: {len(rows)}")
        for path, br, ar in rows[:limit]:
            short = path.split("/ATS/", 1)[-1]
            print(f"  {short}")
            print(
                f"      expected={ar['expected']} "
                f"actual: {br['actual']} -> {ar['actual']} "
                f"provenance: {br.get('provenance')} -> {ar.get('provenance')}"
            )

    show("REGRESSED (match true -> false)", regressed)
    show("FIXED (match false -> true)", fixed)
    show("provenance moved (match unchanged)", moved, limit=40)

    return 1 if regressed else 0


if __name__ == "__main__":
    sys.exit(main())
