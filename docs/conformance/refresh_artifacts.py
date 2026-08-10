#!/usr/bin/env python3
"""Refresh conformance tracking artifacts from a full-suite report.

Usage:
    bin/ntt conformance --json --timeout 4s --jobs 4 \
        testdata/ttcn3-conformance-tests > /tmp/ntt-full.json
    python3 docs/conformance/refresh_artifacts.py /tmp/ntt-full.json \
        --note "JSON errorbehavior decode handling"

Updates, relative to the repository root:
  - testdata/conformance-baseline.json   (matched/skipped/pass_rate, note chain)
  - docs/conformance/current-misses.json (full miss inventory)
  - docs/conformance/current-misses.md   (human-readable summary)
  - docs/conformance/history.json        (one entry per day, replaced on re-run)

The baseline records the LAST MEASUREMENT, not a high-water mark. It can
move down: removing engine behaviour that existed only to make fixtures
pass lowers it on purpose, and the note chain says so. The ratchet that
actually prevents an accidental regression is `--regress` on the
conformance command, enforced per commit by CI - that is where a drop
has to be justified, not here.
"""

import argparse
import datetime
import json
import os
import sys

REPO = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
BASELINE = os.path.join(REPO, "testdata", "conformance-baseline.json")
MISSES_JSON = os.path.join(REPO, "docs", "conformance", "current-misses.json")
MISSES_MD = os.path.join(REPO, "docs", "conformance", "current-misses.md")
HISTORY = os.path.join(REPO, "docs", "conformance", "history.json")

ATS_PREFIX = "testdata/ttcn3-conformance-tests/ATS/"
MSG_LIMIT = 200
DETAIL_MIN_MISSES = 10
DETAIL_MAX_FILES = 20


def chapter_of(relpath):
    parts = relpath.split("/")
    return "/".join(parts[:2]) if len(parts) >= 2 else relpath


def load_report(path):
    with open(path) as f:
        report = json.load(f)
    for key in ("total", "matched", "skipped", "pass_rate", "results"):
        if key not in report:
            sys.exit(f"report {path} missing field {key!r} - "
                     "generate it with `ntt conformance --json`")
    return report


def build_misses(report, source):
    misses = []
    for r in report["results"]:
        if r.get("match"):
            continue
        rel = r["path"]
        if rel.startswith(ATS_PREFIX):
            rel = rel[len(ATS_PREFIX):]
        msg = r.get("reason", "")
        if len(msg) > MSG_LIMIT:
            msg = msg[:MSG_LIMIT] + "..."
        misses.append({
            "chapter": chapter_of(rel),
            "expected": r.get("expected", ""),
            "actual": r.get("actual", ""),
            "outcome": f'{r.get("expected", "")}->{r.get("actual", "")}',
            "relpath": rel,
            "message": msg,
        })
    misses.sort(key=lambda m: m["relpath"])

    outcomes = {}
    chapters = {}
    for m in misses:
        outcomes[m["outcome"]] = outcomes.get(m["outcome"], 0) + 1
        ch = chapters.setdefault(m["chapter"], {"chapter": m["chapter"],
                                                "misses": 0, "outcomes": {}})
        ch["misses"] += 1
        ch["outcomes"][m["outcome"]] = ch["outcomes"].get(m["outcome"], 0) + 1
    chapter_list = sorted(chapters.values(),
                          key=lambda c: (-c["misses"], c["chapter"]))

    summary = (f'ran {report["total"]} files: {report["matched"]} matched, '
               f'{report["skipped"]} skipped '
               f'({report["pass_rate"]:.2f}% match rate)')
    return {
        "summary": summary,
        "source": source,
        "miss_count": len(misses),
        "outcomes": dict(sorted(outcomes.items(), key=lambda kv: (-kv[1], kv[0]))),
        "chapters": chapter_list,
        "misses": misses,
    }


def render_md(inv):
    lines = [
        "# Current Conformance Misses",
        "",
        f"Generated from `{inv['source']}` using the current workspace runner.",
        "",
        f"- Summary: `{inv['summary']}`",
        f"- Misses: `{inv['miss_count']}`",
        "",
        "## Misses By Outcome",
        "",
    ]
    for outcome, count in inv["outcomes"].items():
        lines.append(f"- `{outcome}`: `{count}`")
    lines += ["", "## Misses By Chapter", ""]
    for ch in inv["chapters"]:
        per = ", ".join(f"`{k}`={v}" for k, v in sorted(ch["outcomes"].items()))
        lines.append(f"- `{ch['chapter']}`: `{ch['misses']}` ({per})")
    lines += ["", "## Top Chapter Details", ""]
    for ch in inv["chapters"]:
        if ch["misses"] < DETAIL_MIN_MISSES:
            continue
        lines.append(f"### `{ch['chapter']}`")
        lines.append("")
        in_ch = [m for m in inv["misses"] if m["chapter"] == ch["chapter"]]
        for m in in_ch[:DETAIL_MAX_FILES]:
            entry = f"- `{m['relpath']}`: `{m['expected']} -> {m['actual']}`"
            if m["message"]:
                entry += f" - {m['message']}"
            lines.append(entry)
        if len(in_ch) > DETAIL_MAX_FILES:
            lines.append(f"- ... `{len(in_ch) - DETAIL_MAX_FILES}` more in "
                         "`current-misses.json`")
        lines.append("")
    return "\n".join(lines).rstrip() + "\n"


def update_baseline(report, note):
    with open(BASELINE) as f:
        baseline = json.load(f)
    old_rate = baseline["pass_rate"]
    old_matched = baseline["matched"]
    new_rate = round(report["pass_rate"], 2)
    if report["matched"] == old_matched:
        print(f"baseline unchanged ({old_matched} matched); note not added")
        return
    delta = report["matched"] - old_matched
    entry = (f"{old_rate:.2f}% -> {new_rate:.2f}% ({delta:+d} matched, {note}). "
             f"PRIOR: {baseline['note']}")
    baseline["matched"] = report["matched"]
    baseline["skipped"] = report["skipped"]
    baseline["total"] = report["total"]
    baseline["pass_rate"] = new_rate
    baseline["note"] = entry
    with open(BASELINE, "w") as f:
        json.dump(baseline, f, indent=2)
        f.write("\n")
    print(f"baseline: {old_matched} -> {report['matched']} matched "
          f"({old_rate:.2f}% -> {new_rate:.2f}%)")


def update_history(report):
    with open(HISTORY) as f:
        history = json.load(f)
    today = datetime.date.today().isoformat()
    entry = {
        "date": today,
        "matched": report["matched"],
        "total": report["total"],
        "pass_rate": report["pass_rate"],
    }
    history = [h for h in history if h.get("date") != today]
    history.append(entry)
    with open(HISTORY, "w") as f:
        json.dump(history, f, indent=1)
        f.write("\n")


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("report", help="full-suite JSON report (ntt conformance --json)")
    ap.add_argument("--note", required=True,
                    help="short description of the slice for the baseline note chain")
    args = ap.parse_args()

    report = load_report(args.report)
    inv = build_misses(report, args.report)
    with open(MISSES_JSON, "w") as f:
        json.dump(inv, f, indent=1)
        f.write("\n")
    with open(MISSES_MD, "w") as f:
        f.write(render_md(inv))
    print(f"miss inventory: {inv['miss_count']} misses")
    update_baseline(report, args.note)
    update_history(report)


if __name__ == "__main__":
    main()
