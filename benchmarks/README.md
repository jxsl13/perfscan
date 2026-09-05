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
- **PS4008** (serial-dot matmul → ikj/axpy): the throughput win the rewrite
  targets comes from SIMD-vectorizing the axpy inner loop — which `gc` does NOT
  do. Measured across sizes (128/256/512, Apple M2 Pro, go1.26), the ikj form is
  ≈parity to the register-accumulating ijk form: modestly SLOWER at small /
  cache-resident sizes (the extra bounds-checked `c[i][j] +=` read-modify-writes
  outweigh the broken latency chain) and only ~3% faster once the matrix spills
  cache. A Before/After pair would misrepresent a size-dependent wash as a
  reliable speedup. The rewrite stays bit-identical and L3-opt-in as a structural
  pointer for codebases that DO vectorize downstream (asm/cgo/a BLAS call); the
  measured Go reality is documented in the check text.
- **PS5001**: deliberately un-benchmarked here — the check's own doc explains
  that the win evaporates on any memory-touching path; its evidence table
  lives in the check text.
- **PS6004**: a verification-gap advisory; there is nothing to time.
- **PS6005–PS6059 and PS6062–PS6079** (accelerator and campaign verification advisories): their
  remedies require real device traces, independent processes, exact workload
  identities, topology/parity evidence, or project-specific kernels. A toy Go
  micro-benchmark cannot reproduce those contracts and would manufacture a
  speedup disconnected from the finding. The check documentation records the
  owner campaign measurements; validate candidates on the target device with
  the gates each rule requires.
- **PS6060–PS6061** (exact ReLU/Abs SIMD advisories): portable Go has no ordered SIMD
  compare-and-bit-select primitive to benchmark as an After implementation,
  while `FMAX`/`math.Max`, plain `FABS`, and a sign mask do not reproduce the
  required NaN and signed-zero contracts. Their documentation records the
  owner campaigns' native arm64 measurements and raw-bit/tail validation;
  benchmark the registered native sibling on each supported architecture.
- **PS6086** (caller-participation fan-out advisory): scheduler placement and
  crossover behavior are workload-, topology-, and threshold-dependent. The
  owner campaign found fewer allocations but a statistically significant
  production-latency regression, so a toy goroutine benchmark would imply the
  exact generic speedup this check warns users not to infer.
- **PS6087** (last-use in-place fusion advisory): the allocation win depends on
  a project-owned tensor representation, backend capability, and fused kernel.
  A toy tensor and fake in-place method would measure the fixture rather than
  the configured contract. The check documentation pins the permanent GoAI
  kernel benchmark and retained end-to-end campaign evidence.
- **PS6088** (repeated fan-out barrier evidence advisory): scheduler wait share
  cannot distinguish goroutine lifecycle cost from ordinary worker idleness.
  The owner campaign's bounded-pool candidate preserved exact output but failed
  its end-to-end speedup gate, so an isolated goroutine benchmark would imply
  precisely the generic pool win this rule requires callers to prove instead.
- **PS6090** (benchmark result-liveness advisory): the typed sink is deliberately
  performance-neutral hardening around a project-owned compute API. A toy pure
  function would either be optimized away (restating the vulnerability) or
  measure fixture-specific work rather than the configured call. The check
  documentation records the order-alternated sink/control campaign; retain the
  real operation and validate that adding its race-free observation is neutral.
- **PS6091** (Top-K(k=1) scalarization advisory): the allocation and latency
  delta belongs to a configured backend's generic Top-K implementation and its
  proposed scalar argmax capability. A local toy selector would benchmark
  fixture choices without proving prefix, tie, NaN, or error equivalence. The
  check documentation records the owner campaign and requires project-level
  equivalence plus end-to-end gates.
- **PS6092** (generic constraint-call dispatch advisory): dictionary dispatch is
  compiler-, version-, target-, and instantiation-shape-dependent, while the
  possible operation-specific rewrites have different code-size and semantic
  tradeoffs. A fixed toy pair would turn a machine-code verification obligation
  into a misleading universal speedup claim; inspect the exact instantiated
  symbol and benchmark the real operation mix instead.
- **PS6099** (scalar-transcendental output-staging advisory): the After arm is
  an already available architecture-specific SIMD/batched transcendental leaf,
  not portable Go. A toy scalar second pass would benchmark an extra memory
  walk without the mechanism the rule proves is present. The check documents
  the owner SVC campaign; benchmark the exact routed leaf, precision, band size,
  alias regime, and iterative solver end to end.
