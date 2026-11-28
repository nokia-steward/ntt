# Clean-room declaration for `runtime/cpp` and `backend/cpp`

This package is a clean-room reimplementation of a TTCN-3 C++ runtime.

- No Eclipse Titan source code (titan.core, titan.Libraries.*) was
  consulted during the design or implementation of any file in
  `runtime/cpp/` or `backend/cpp/`.
- The class names that match Titan's public surface (`TTCN_Verdict`,
  `Port`, `Component`) are required by the TTCN-3 v4.11.1 specification
  or are de-facto names that Nokia 4G/5G test ports already link
  against; nothing else of Titan's implementation has been copied or
  paraphrased.
- All algorithmic choices (the slice-backed Port queue, the snapshot-
  based `alt` scheduler, the verdict precedence numbering) trace back
  to the Go runtime in `runtime/*` of this repository, which itself
  was developed against the TTCN-3 standard.
- The `runtime/cpp` headers are published under BSD-3-Clause to match
  the rest of `ntt`. Test ports that link against this runtime do not
  inherit any EPL obligations from Titan.

Reviewers who wish to confirm independence can re-run `git log` on the
files in this directory: every commit is authored by an `ntt`
contributor and contains no Titan-derived patches.
