# TTCN-3 conformance tests

A vendored copy of the ETSI TTCN-3 conformance tests (see LICENSE), used as
the corpus for `ntt conformance` and the CI regression gate.

## Provenance

| | |
| --- | --- |
| Upstream | <https://forge.etsi.org/rep/mts/ttcn3-conformance-tests> |
| Revision | `fdc17d2c03c4074ceee77c1be4614dcce04421a5` (2024-03-28), branch `main` |
| Imported | 2026-10-29; before that `99ac02bc` (2022-09-07), imported 2022-09-10 |
| Vendored | `ATS/` and `LICENSE`; upstream's `docs/`, `standards/`, `build-aux/` and `Makefile` are not |
| Local delta | two `@purpose` comment lines (below) |
| `.ttcn` files | 5,164 — the same count as upstream at that revision |

Note the upstream group is **`mts`** (ETSI TC MTS). A `ttcn3` group does not
exist; that path returns a sign-in redirect, which looks like a permissions
problem but is GitLab's response to an unknown path.

Upstream describes itself as the *source* repository for these tests and
states that it "does not provide active ETSI MTS deliverables" — the
published suites live on ttcn-3.org. This copy therefore tracks the
drafting source, which is the right thing for a regression corpus (it is
stable and citable) but is not the same as a published deliverable.

### Local delta

Two `@purpose` comments were corrected from "a external function" to "an
external function":

- `ATS/.../1602_toplevel/NegSem_1602_toplevel_050.ttcn`
- `ATS/.../2002_the_alt_statement/NegSem_2002_TheAltStatement_088.ttcn`

Both are documentation comments; no test input, expected outcome or
semantics is affected. The error is still present upstream, so a refresh
would reintroduce it. A patch ready to send upstream is kept outside this
repository.

Everything else is byte-identical to the revision above. Keep it that way:
the corpus is the measurement instrument, and every local edit is a delta
someone reproducing a pass rate has to be told about.

## Re-deriving the revision after a refresh

The 2022 import recorded no upstream revision, so its revision was
recovered from content rather than metadata; the 2026 one was taken from
upstream `main` directly, and checked the same way. The same method
re-establishes it after any future refresh, and is worth knowing because
it does not depend on anyone having written the provenance down:

```shell
git clone https://forge.etsi.org/rep/mts/ttcn3-conformance-tests up

# our tree as (blob-sha, path) — straight out of git's index
git ls-files -s testdata/ttcn3-conformance-tests/ATS \
  | awk '{sub("testdata/ttcn3-conformance-tests/","",$4); print $2" "$4}' \
  | sort > ours.manifest

# the upstream commit whose ATS tree differs least is the candidate
cd up && for c in $(git rev-list HEAD); do
  git ls-tree -r "$c" ATS | awk '{print $3" "$4}' | sort > up.manifest
  echo "$(comm -3 up.manifest ../ours.manifest | wc -l) $c"
done | sort -n | head
```

Git blob hashes make this exact and cheap: identical content yields
identical hashes, so no file contents need comparing. Two caveats. Adjacent
upstream commits often share an identical `ATS/` tree, so content alone
narrows the answer to a range rather than a point — the revision above is
singled out because it was upstream HEAD on the import date, which is
provenance rather than content. And a local delta shows up as differing
files, so expect the best match to be "n files differ" rather than zero;
check that those files are the ones you expect.
