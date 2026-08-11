<!-- For new checks, CONTRIBUTING.md is the authoritative checklist. -->

## What

<!-- One paragraph: the change and why. -->

## Checklist

- [ ] `make check` passes (gofmt, vet, race tests)
- [ ] `make selfscan` is clean (perfscan on perfscan, all levels)
- [ ] `make docs` regenerated if check metadata changed (CI enforces drift)

**New checks additionally:**

- [ ] Stable ID claimed (free on `main` AND in open PRs); reference ports keep their upstream number, originals use the category's `x1xx` block
- [ ] Fix level chosen honestly (L1 mechanical/bit-identical, L2 restructuring, L3 benchmark-gated advisory)
- [ ] `AutoFix` only for deterministic, bit-identical rewrites
- [ ] Positive AND negative fixtures with `// want` comments (+ `.golden` for auto-fixes)
- [ ] Measured evidence in `Doc.MeasuredWin` or the PR description
