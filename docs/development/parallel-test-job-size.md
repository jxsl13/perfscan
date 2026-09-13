# Bounded inner test jobs

The [main CI run on 746406b](https://github.com/jxsl13/perfscan/actions/runs/34761114704)
failed in macOS oldstable external shard 1: both checks jobs exhausted the
existing 20-minute timeout (1200.721s and 1200.376s). The other 13 matrix jobs
passed. Active stacks included analyzer fixture package loading and the
owner-shaped end-to-end campaign, with many ordinary tests queued behind
`t.Parallel`; the evidence did not identify an assertion failure or a single
hung test.

`-max-tests-per-job` now defaults to 150, and CI specifies that value explicitly.
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
