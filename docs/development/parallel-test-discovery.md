# Bounded test discovery

`internal/testparallel` discovers package test names with at most the existing
`-workers` limit of concurrent `go test -list` commands. It finishes the entire
successful discovery phase before starting any execution jobs. Package-index
results preserve the previous sorted package/job order and exact test names;
external package/name hashing and inner round-robin partitioning are unchanged.
Discovery errors or cancellation stop dispatch, cancel and drain active listing
commands, retain observed errors, and publish no partial successful inventory.

Each listing still uses the existing race flag, per-package timeout, shared
Windows crash-retry budget, diagnostic grace and conclusive-failure checks.
Execution worker/ordinary-test parallelism defaults, all platforms and SDKs,
race coverage and benchmark exclusion are unchanged. There is no new chunking,
duration-weighted assignment, or increase in macOS worker count. Discovery and
test execution do not overlap.

## Observed critical path, not a measured improvement

In [main CI run 34705208064](https://github.com/jxsl13/perfscan/actions/runs/34705208064),
[macOS stable external shard 1](https://github.com/jxsl13/perfscan/actions/runs/34705208064/job/103583914893)
took 19m54s in its test step. Its first completed job appeared about 120.38s
after the step began and reported 4.322s of test-binary execution. Thus the
discovery/build/startup prefix preceding that execution is bounded above by
roughly 116s; the old logs do not isolate discovery itself. That bound is not a
measured speedup or a prediction that concurrent discovery removes all of it.
Compilation contention can reduce or reverse any benefit.

The same runner's checks groups reported 674.201s and 1033.348s; external shard
0 reported 326.108s and 342.495s. These logs demonstrate imbalance, but do not
identify individual expensive tests or separate host effects. This change
does not attempt to solve that imbalance. Four macOS execution processes
previously caused oversubscription; its existing two-worker cap is retained.

Discovery emits a start line with package count, worker cap and race mode, then
a successful completion line with discovered/selected test counts, execution
job count and listing-phase elapsed time. A failure emits elapsed time and its
errors rather than successful counts. Elapsed discovery starts after package
enumeration, and excludes execution; it is not total job or setup time.

## Bounded paired measurement plan

Compare the serial parent and this change at otherwise identical source,
platform, SDK, worker and ordinary-test parallelism settings. Use three
order-alternated pairs with `-race -workers 2 -shard-count 2` on macOS, covering
both external indices; retain the normal Linux/Windows worker settings for
their platform checks. Preserve cache/runner configuration and record which
runs are cold or warm rather than comparing mixed conditions as equivalent.
An approved serial control can retain the same telemetry while using one
listing callback at a time, without changing execution workers or selection.

For each attempt retain discovery elapsed/counts, complete step/job duration,
all inner group results and every failure/retry. Compare canonical manifests
of `(package, external index, inner index, test name)` from the exact requested
build in both versions: their selections and exhaustive exact-once coverage
must match, including race-tagged tests, examples and fuzz seeds. Do not use
ordinary builds as substitutes for race discovery. Report medians and paired
differences without discarding failed/slow attempts or promising a gain from
the old prefix bound. Do not tune worker counts or shard assignment in the
same campaign; revert or investigate a consistent regression before promotion.

Channel-based virtual-time regressions exercise concurrency caps, out-of-order
completion, deterministic inventories, cancellation/error draining and the
successful barrier without launching actual compilers or tests from the mock
listing callbacks. Existing subprocess discovery, exact-once selection,
timeout and retry tests remain intact.

## Local listing-only pilot, 2026-09-12

A temporary test overlay called the actual production listing functions on
Darwin/arm64, Go 1.27.0, GOMAXPROCS 12, with two concurrent listing workers.
Other project compiler/test processes were paused. The outer probe was not
race-instrumented; every measured child package listing explicitly used
`-race`, the unchanged listing/retry function, and a two-minute bound. Empty
GOFLAGS meant the outer command-line overlay was not inherited by child Go
commands and did not add the probe to their test inventory.

All six legs passed for the same 23 repository packages and 1940 test names.
Their full external-index/inner-index/name manifests were byte-identical,
with SHA256 `62ee678e7d030fce8bb98b6ac847a98236b5d543f3f4779ee3bb9bc0eae149e0`.
Both external indices were reconstructed with exact-once coverage; execution
jobs were not run by this probe.

| Pair | Run order | Serial listing | Two-worker listing |
| --- | --- | ---: | ---: |
| 1 | serial, parallel | 77.401 s | 17.038 s |
| 2 | parallel, serial | 34.078 s | 16.910 s |
| 3 | serial, parallel | 33.637 s | 16.960 s |

Existing shared caches were retained, with no eviction or claim of identical
cold-cache state. The initial serial leg may include warming and is retained,
not discarded. Across all three pairs, median listing time was 34.078 s serial
and 16.960 s concurrent, about 50.2% lower for this phase on this host.
This is not a measured whole-CI or Linux/Windows improvement. Setup, package
enumeration and actual test execution are outside the timed interval.

Retained local evidence includes source pins, environment metadata, elapsed
times, parsed test-name inventories and complete partition manifests. It does
not include successful child stdout/stderr transcripts. Normal full hooks and
the unchanged cross-platform CI matrix are separate validation gates.
