# Source-bound dispatch crossover evidence

PS6131 is an opt-in freshness advisory, not an optimization or threshold
autofix. A crossover measured for a leaf kernel also depends on its worker
pool/scheduler and toolchain. Changing any selected implementation or build
input requires fresh evidence; copying another project's threshold is not
evidence. Owner #914 describes GoAI's AbsF32 change. None of its timing gains or
numeric thresholds are claimed by perfscan.

The source matcher has direct-slice regression controls; the qualified recorder
uses the owner's actual F32 Tensor/context operation: below-boundary serial leaf followed by a switch-case
break, otherwise a worker callback slicing the same input/output. Unsupported
routes fail closed. The allocated operation's complete body is copied into
test-only forced-route functions; only the independently identified predicate
becomes `true` or `false`. Full allocation, context, dtype, result and error paths
remain. Regeneration never mutates the scanner's shared production AST.

These copies are **diagnostic instrumentation**, not exact unmodified production
timing. Policy arms use preallocated buffers. Forced allocated-operation arms
include output construction. Direct original-kernel observations are distinct,
and the original full production benchmark runs separately. Its printed `B/op`
and `allocs/op` quotients do not establish raw allocation totals.

## Repository preparation

Add a committed test factory taking one `int` extent and returning the exact
operation argument tuple. For the GoAI owner this is
`(*backend.Context, []*tensor.Tensor, backend.Attrs)`, not an artificial `[]T`
operation. Select the CPU context and one F32 tensor whose shape is `{n}`. The
factory must use a perfscan version exposing `benchmarkevidence.Measure`.
Warmups check the observed output extent/dtype and compare forced-route output
bits and observed input bits before timing. This is a bounded observed-input
check, not a proof for every dtype/shape or whole-program correctness.

Example source selectors (adjust the repository's actual package path):

```yaml
dispatchCrossoverContracts:
  - dispatchFunction: github.com/jxsl13/goai/backend/cpu.absKernelCPU
    thresholdConstant: github.com/jxsl13/goai/backend/cpu.absF32ParallelThreshold
    serialPolicy: github.com/jxsl13/goai/backend/cpu.absF32
    parallelPolicy: github.com/jxsl13/goai/backend/cpu.parallel
    workerRunner: github.com/jxsl13/goai/backend/cpu.parallel
    leafKernels: [github.com/jxsl13/goai/backend/cpu.absF32BlocksNeon]
    operationInputFactory: github.com/jxsl13/goai/backend/cpu.perfscanAbsInputs
    diagnosticFunction: github.com/jxsl13/goai/backend/cpu.TestPerfscanAbsDispatch
    productionBenchmark: BenchmarkAbsF32CPU/n{n}
    campaign: /absolute/path/to/new-campaign
    planSHA256: ''
    recordsSHA256: ''
    evidenceGoBinary: /retained/sdk/bin/go
```

The generated copy names are reserved private test-only identifiers. They do
not replace existing production operation declarations. Include the actual
factory/benchmark and dependency in the pinned commit before recording.
`serialOperation`/`parallelOperation` are optional source selectors only for
the bounded direct-slice wrapper grammar, not invented Tensor operation names.

```sh
go run ./cmd/crossover -repo /path/to/source -commit FULL_COMMIT \
  -config /path/to/source/.perfscan.yaml -package ./backend/cpu \
  -out /path/to/new-campaign -sizes BELOW,AT_OR_ABOVE -procs 2 -pairs 2 -fixed-n 16 \
  -go /retained/sdk/bin/go
```

Choose shapes actually supported by the original benchmark's subbenchmark
matrix. The boundary is resolved from source, never supplied numerically as an
independent attestation. Serial/parallel, policy/allocated/direct/original,
candidate/control, shape and processor cells are all predeclared. Every sample
starts a fresh process; even pairs reverse order. Identical-binary controls run
the same selection in both arms. Failed captures remain retained, and later
planned captures still run; failure rejects qualification rather than dropping
negative evidence.

The original benchmark adapter deliberately accepts the owner's canonical body
apart from typed package aliases, its name and literal size matrix. Other
benchmark structures and dtypes require an additional reviewed adapter; they
do not silently qualify. The literal shape list, `n` label, shape-preserving F32
constructor, CPU context and typed operation registration association are
checked from selected source. This is a bounded declaration association, not a
proof of the global backend registry's runtime state or every possible input.
Retain the OLD SDK executable and its complete SDK; verification independently
selects that executable with `-go` (and analyzer `evidenceGoBinary`). Both typed
loading and compilation force its observed GOROOT and `GOTOOLCHAIN=local`.
Typed loading re-executes the trusted current perfscan executable in an exact
private argument mode, selecting the SDK on that child's PATH (not changing
the caller's process environment). Custom package drivers are disabled.
Requests and responses are size-bounded and strictly decoded. The child
inherits the parent's deadline, capped at ten minutes, for SDK/type commands.
Cancellation kills the direct loader child; no process-group termination
guarantee is claimed for descendant Go processes after an earlier cancellation.
An unavailable or incompatible old SDK rejects replay; it does not fall back to
today's compiler. Freshness separately observes today's compiler and materials.
Only GOMAXPROCS and four runtime controls (GODEBUG, GOGC, GOMEMLIMIT,
GOTRACEBACK) are pinned. Arbitrary factory/benchmark environment, external
services and input files are not fully authenticated by this campaign.

Independently retain the printed pre-measurement plan SHA256 before the sample
phase and completed records SHA256 afterward. Put both pins into the contract.
Verification requires them and independently selected source repository:

```sh
go run ./cmd/crossover -repo /path/to/source -verify /path/to/campaign \
  -go /retained/sdk/bin/go -plan-sha256 PLAN_PIN -records-sha256 RECORDS_PIN
```

The validator re-observes the exact Git commit tree/archive, verifies every
committed blob, regenerates the harness from typed source and reproduces the
controlled binary/compiler/material. It checks the complete expected matrix,
stream hashes, process exits and raw counter identities. Unsigned hashes or
config declarations alone never qualify facts. Do not independently move the
campaign directory: original physical material identities remain part of exact
build replay. Portable freshness identities additionally retain module path /
version, import / relative file, native / embed / test selection and compiler /
target / options identities across controlled `-trimpath` snapshot relocation.

## Fixed-work and integrity boundaries

The default is fixed work (`-fixed-n >= 2`). Native records retain actual `N`,
elapsed nanoseconds, `MemBytes`, `MemAllocs`, quotient and remainder. Nested
aggregate `N=1`, failed/skipped/exited callbacks and invalid raw identities do
not qualify. No custom rounded benchmark metric substitutes for raw counters.

Adaptive runs are explicit (`-fixed-n 0 -duration 500ms`) and retain actual raw
N. Unequal-N totals are not matched paired deltas: reports use exact rational
per-operation differences and retain a GC-cadence caution. Rerun allocation-
heavy operations at fixed work before making a crossover decision; adaptive
allocation/GC cadence can differ materially between arms. Reported matched
allocation totals, normalized native timing and original printed benchmark
quotients are separate evidence, not allocation-site attribution or a noise
subtraction/gate relaxation.

The controlled collector requires CGO0, no workspaces/replacements/overlays,
fixed compiler flags and only controlled `GOFLAGS=-tags=...`. Campaign binaries
must target the host. Source executes from the retained package directory;
relative committed fixture-file semantics are preserved. Runtime controls
`GODEBUG`, `GOGC`, `GOMEMLIMIT`, `GOTRACEBACK` and GOMAXPROCS are pinned; this is
not isolation from every operating-system/service input. Allocation counters
are process-wide, not causal attribution to the dispatch or allocation sites.

The public `testing.Benchmark` API cannot expose unmarked `runtime.Goexit` only
in the initial probe cleanup if a later measurement succeeds. The helper's
existing documented boundary remains; do not treat that case as proved valid.
There are no profiles or discarded failure filters. A fresh verified campaign
clears freshness only: retain controls, negative evidence and independent
correctness review before changing production policy.

The end-to-end regression uses the pinned owner's actual operation, leaf and
original benchmark bodies against explicitly portable Tensor/context/native
stand-ins. It is tooling evidence, not a GoAI hardware performance experiment.
