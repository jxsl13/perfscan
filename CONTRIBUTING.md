# Contributing to perfscan

Thanks for helping! perfscan aims to be a community-maintained registry of
**benchmark-verified** Go performance checks. The bar for a new check is
deliberately higher than for a style lint: every check must be justified by a
measured win, not by folklore.

## Development setup

```bash
git clone https://github.com/jxsl13/perfscan
cd perfscan
make hooks    # install the git hooks: pre-commit (gofmt/vet/build/test) and
              # pre-push (full CI mirror: +staticcheck, -race, docs, benchmarks, dogfood)
make test
```

## Adding a check

1. **Claim an ID.** IDs are stable and never reused. Checks ported from
   the original reference registry keep their
   original PS number; new checks take the next free slot in the `x1xx`
   block of their category (PS2101, PS2102, … for allocation, PS3101… for
   indirection, and so on). An ID must be free on `main` *and* in every open
   PR that touches the registry, not merely in your branch.
2. **Write the analyzer** in `checks/psXXXX.go` as a standard
   `analysis.Analyzer` wrapped in `lint.Check` metadata via `register`.
   Choose the **level** honestly:
   - L1 `LevelIdiomatic` — the fix is idiomatic Go, mechanical, bit-identical.
   - L2 `LevelStructured` — the fix restructures code; review expected.
   - L3 `LevelAggressive` — hyper-optimization; advisory, benchmark-gated.
3. **Set `AutoFix` only for deterministic, bit-identical rewrites** and
   attach `SuggestedFixes` to the diagnostic. Everything else is advisory:
   a transform that needs an A/B benchmark or a bit-identity proof is not a
   mechanical fix.
4. **Document the measured win.** Fill `Doc.Text` (pattern, remedy,
   preconditions), `Doc.Before`/`Doc.After`, and where you have numbers,
   `Doc.MeasuredWin`. If the pattern is domain-specific, wire its vocabulary
   through `config.Config` and set `NeedsConfig` + `Vocab`.
5. **Add fixtures**: a positive *and* a negative case under
   `checks/testdata/src/psXXXX/`, with `// want` comments, plus a `.golden`
   file when the check auto-fixes. Register the test in
   `checks/checks_test.go`.
6. **Add the micro-benchmark pair.** Every rule whose remedy can be
   expressed as an isolated micro-benchmark ships one in `benchmarks/`:
   `BenchmarkPSxxxx_Before` / `BenchmarkPSxxxx_After`, the two arms being
   the check's documented Before/After shapes. CI runs each once; humans
   compare with benchstat. If the rule genuinely cannot be
   micro-benchmarked (parallel machine-dependence, project-specific
   dispatch, verification advisories), add it to the exemption list in
   `benchmarks/README.md` with the reason.
7. `make check` must pass.

## Fixing bugs

**Every regression fix ships a regression test.** When a check or fix
misbehaves (wrong edit, changed semantics, false positive), the fix commit
must contain a fixture case (or unit test) that reproduces the original
breakage and pins the corrected behavior — named or commented after the
failure it guards (see the nilPreserved case in the PS2101 fixtures, born
from a Kubernetes scheduler test catching a nil-vs-empty rewrite). A fix
without its regression test is not done.

## Design rules

- **A finding is a candidate, not a verdict.** Static analysis sees syntax,
  not hotness. Messages should state the cost and the remedy, and where the
  payoff depends on runtime facts (loop trip counts, cache residency,
  share-of-profile), say so.
- **Prefer precision over recall.** A check that cries wolf gets disabled;
  a suppressed true positive can be re-found. Narrow the shape until the
  false-positive rate is boring, and encode known-benign shapes as explicit
  suppressions (the reference registry documents several hard-won ones).
- **Domain vocabulary lives in config, not code.** The engine must run on
  any Go module with zero configuration; anything project-specific
  (accessors, allocators, fan-out helpers) comes from `perfscan.yaml`.
- **Starved checks must be loud.** A domain check with no vocabulary stays
  silent and the runner warns — never let a silent zero read as "no
  instances".

## Release process

Releases use a plain version tag with no path prefix: `vX.Y.Z`. On a tag,
the GitHub Actions **release workflow** validates that exact shape,
cross-compiles the binary per OS/arch, and publishes the archives with
`softprops/action-gh-release`. Versioned installs use
`go install github.com/jxsl13/perfscan@vX.Y.Z`.

Policy: cut a release per feature (bump the minor); ID semantics never change.
Only the **last 3 releases** are kept — older tags/releases are pruned.

## Code of conduct

Be kind, assume good faith, argue with benchmarks.
