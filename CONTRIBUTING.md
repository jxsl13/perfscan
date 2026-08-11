# Contributing to perfscan

Thanks for helping! perfscan aims to be a community-maintained registry of
**benchmark-verified** Go performance checks. The bar for a new check is
deliberately higher than for a style lint: every check must be justified by a
measured win, not by folklore.

## Development setup

```bash
git clone https://github.com/jxsl13/perfscan
cd perfscan
make hooks    # install the git pre-commit hook (gofmt, vet, build, test)
make test
```

## Adding a check

1. **Claim an ID.** IDs are stable and never reused. Checks ported from the
   [goai reference registry](https://github.com/jxsl13/goai) keep their
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
6. `make check` must pass.

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
  (accessors, allocators, fan-out helpers) comes from `perfscan.json`.
- **Starved checks must be loud.** A domain check with no vocabulary stays
  silent and the runner warns — never let a silent zero read as "no
  instances".

## Release process

Releases are tagged `vX.Y.Z` on `main`; CI runs tests on every push and
GoReleaser builds and publishes binaries on tags. Maintainers follow
semantic versioning: new checks are minor versions, message/behavior changes
of existing checks are documented in the changelog, ID semantics never
change.

## Code of conduct

Be kind, assume good faith, argue with benchmarks.
