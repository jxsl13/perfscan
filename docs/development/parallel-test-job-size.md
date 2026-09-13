# Bounded inner test jobs

The [main CI run on 746406b](https://github.com/jxsl13/perfscan/actions/runs/34761114704)
failed in macOS oldstable external shard 1: both checks jobs exhausted the
existing 20-minute timeout (1200.721s and 1200.376s). The other 13 matrix jobs
passed. Active stacks included analyzer fixture package loading and the
owner-shaped end-to-end campaign, with many ordinary tests queued behind
`t.Parallel`; the evidence did not identify an assertion failure or a single
hung test.

`-max-tests-per-job` defaults to 150; CI now explicitly selects 100 (see the
follow-up below). Local hooks retain the default.
After stable external assignment, each package uses
`max(workers, ceil(selected / max-tests-per-job))` balanced round-robin groups,
capped by the number of selected names. Empty selections produce no jobs.
This bounds how many discovered names share one process timeout without
increasing concurrent Go test processes. macOS still uses two workers, Linux
and Windows still use four, and every job retains race detection and the
20-minute timeout. Benchmarks remain in their dedicated gate.

Race-enabled discovery of the initial implementation on the local macOS checkout (`go test -race -p 2 -list .
./...`) found 23 packages and 2004 ordinary names, including the added job-size
regression. The checks package contains 1589 names: external shards 0 and 1
select 765 and 824 respectively. With two workers, their former two inner jobs
contained at most 383 and 412 names. The cap produces six jobs per external
shard, containing at most 128 and 138 names. Across all packages, external
shards 0 and 1 select 962 and 1042 names, and use 38 and 35 jobs rather than 34
and 31. External selection and the complete test census are preserved.
The final revision additionally keeps the original seven-name composition
fixture and adds a separate large composition test; the census above predates
that additional test.

Focused ordinary and race tests cover stable balanced partitioning, the cap
boundaries, empty/small selections, and exact-once coverage of 1001 names
across both external shards. Discovery still respects the race build. These
checks establish partition correctness, not a measured CI speedup or proof
that every macOS job fits its time budget; the complete CI matrix must validate
the runtime result. A name-count cap cannot guarantee wall time for a single
heavy test or its subtests.

## Windows cumulative workload and timeout diagnostics

[PR 1019's first CI run](https://github.com/jxsl13/perfscan/actions/runs/34776488035)
passed twelve gates, but both Windows external-shard-1 jobs ended inner checks
group 4/6 at exactly the outer 20-minute budget plus five-second diagnostic
grace. Windows reported the killed `go.exe` as exit status 1. Its ordinary
package-output buffering hid progress; neighboring groups spent about six to
nine seconds in command startup/build before their inner test timer began.

The same 144-name group passed locally with Go 1.26, race detection, one CPU
and `-parallel=1` in 612.179s. The end-to-end PS6131 campaign took 375.34s and
the twenty CUDA source-class cases added about 119s. All cases and assertions
remain. The previous main Windows stable run had already taken 1045.470s in
group 4/6 before these additional source-class tests.

At the observed 1678-name checks census, external shard 1 contains 863 names.
The CI cap of 100 creates nine groups of 95/96 names instead of six groups of
143/144. The campaign remains in the first four-worker wave (group 4), while
the CUDA matrix moves to group 7. This separates the observed heavy workloads
without changing workers, CPU shares, external selection, race detection or
the timeout. The runtime improvement is an inference pending the full CI
matrix, not a Windows per-test measurement or a wall-time guarantee.

The runner now records each job's start, test count and elapsed time. Raw
`go test -v` output reaches its existing capture before package completion,
preserving progress if the outer command is killed. Actual context expiry is
joined with the original process error, keeping both errors discoverable and
the captured output intact. Deadline/cancellation failures are never retried;
the existing narrowly recognized Windows runtime-crash rules are unchanged.
Deterministic clock tests exercise the original 20-minute-plus-five-second
deadline, cancellation, ordinary failures and retained process errors. Added
100-name-cap regressions preserve exact-once coverage across both external
shards for the two- and four-worker settings; all earlier 150-cap cases remain.
