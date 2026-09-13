# Parallel test CPU budget

Each isolated `go test` process sees the same runner CPU quota. Previously,
`testparallel -workers W` also gave **each** process the outer runtime's full
default `-parallel` count and inherited that full GOMAXPROCS into nested Go builds.
These independent defaults multiplied runnable analyzer/compiler work by W.

The default per-worker budget is now `max(1, floor(GOMAXPROCS / W))`. Discovery
and execution commands replace GOMAXPROCS with that budget; their nested Go
commands inherit it. Default inner test parallelism uses the same share. Explicit
positive `-parallel` still overrides ordinary-test concurrency independently;
it does not raise the child CPU share. Explicit zero remains invalid. Unrelated
environment values are preserved: case-insensitive duplicate GOMAXPROCS keys are
removed on Windows only; Unix lowercase/mixed-case keys remain distinct.

Process workers, exhaustive selection, name/job partitioning, all test bodies,
race settings and twenty-minute shard deadlines are unchanged. Ordinary tests
remain concurrent across workers (and within workers when the share exceeds one).
Startup logs report host CPU/runtime counts, worker budget and inner parallelism.
Workers above available CPUs still need at least one slot each: this is a bounded
default, not a universal no-contention guarantee.

Windows stable main CI34768201012 (head12cf7f8) failed the existing output-limit
control with zero stdout after0.57s. Its unchanged200ms deadline includes file
creation and race-instrumented Go child startup. `exec.Cmd.Run` already waits for
stream-copy goroutines, and the capped writer commits its prefix before cancellation.
A child canceled before emitting bytes correctly retains empty streams, not a
manufactured prefix. This budget change addresses observed multiplicative scheduling
pressure; it does not extend/ignore cancellation, retry assertion failures or
guarantee OS startup within200ms. Actual Windows matrix CI is required to verify
the repair; no measured CI speedup or native output guarantee is claimed.
