# perfscan micro-benchmarks

Every rule whose remedy can be expressed as an isolated micro-benchmark
ships one here: a `BenchmarkPSxxxx_Before` / `BenchmarkPSxxxx_After` pair
whose two arms are the check's documented Before/After shapes. They keep
the advice honest on current Go versions and current hardware — run

```bash
go test -bench . -benchmem ./benchmarks/
```

and compare arms with benchstat. CI executes every benchmark once
(`-benchtime=1x`) so the pairs always compile and run; timing claims are
for humans with benchstat, not for CI gates.

Rules without a pair, and why:

- **PS1001–PS1005, PS1009** (per-element dispatch family): the cost lives in
  a project's accessor indirection; the fixture tensor here would benchmark
  its own toy dispatch, not yours. The documented wins come from the
  reference corpus; measure at your site.
- **PS3034, PS3059, PS3060, PS3063, PS3065** (parallelization family):
  results are core-count- and scheduler-dependent; a micro-benchmark
  asserting a speedup would be flaky by construction. Gate real conversions
  with the digest + `-race` procedure the checks document.
- **PS4002** (vectorized sibling): requires a SIMD kernel to compare against.
- **PS5001**: deliberately un-benchmarked here — the check's own doc explains
  that the win evaporates on any memory-touching path; its evidence table
  lives in the check text.
- **PS6004**: a verification-gap advisory; there is nothing to time.
