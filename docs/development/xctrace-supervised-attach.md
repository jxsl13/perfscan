# Direct-target supervised attach

For [#867](https://github.com/jxsl13/perfscan/issues/867), Capture no longer
lets `xctrace --launch` create an unowned target. It first completes the existing
audited input preflight, then uses its own embedded native supervisor to spawn
the canonical real executable suspended, attach Time Profiler to exactly that
owned PID, and resume only after a uniquely named Darwin tracing-start
notification. No wrapper substitutes for the measured executable. This is
explicitly **supervised attach**, not timing/instrumentation equivalence to
`--launch`; parentage, security identity and instrument behavior can differ.

## Migration and supported scope

All API requests need `ProcessScope: "direct-executable"`, positive bounded
`ReadinessTimeout` and `CleanupTimeout`; the CLI requires
`-process-scope direct-executable` and defaults those timeouts to 20s and 5s.
Only Darwin arm64/amd64 and a single `Time Profiler` template are initially
supported. Other instruments/platforms/scope choices, missing input policy,
application bundles and unavailable native compiler/SDK fail before target spawn.
There is no launch fallback or arbitrary user-configured supervisor path.
Builds and releases remain pure Go/CGO_ENABLED=0 on all six existing targets.

This acknowledgement is an audited limitation, **not proof** that a workload
never forks, detaches, delegates to XPC, or asks a service to launch another
process. The guarantee covers the supervisor's direct executable and recorder
children only. Workloads requiring descendant/service cleanup are unsupported;
neither a process group nor a PID/name scan establishes that ownership. No
existing user process is discovered, attached or signaled. This bounded feature
does not by itself close the literal all-targets scope of #867.

## Local compiler and binding

The supervisor source is embedded in the Go binary and written exclusively
under a newly created private evidence directory. The fixed system
`/usr/bin/xcrun` and `/usr/bin/xcode-select` observe compiler/developer-directory/
macOS SDK selection; no supervisor executable comes from evidence or config.
Compiler version, canonical paths, source/protocol identity, selected SDK
spawn/notification header digests, compiler digest, bounded compile command and
streams, native Mach-O CPU/type and produced executable digest are retained.
Source/binary/tool/header checks detect changes across compilation. Compiler
hashing is bounded at 256MiB, helper at 16MiB, each selected header at 1MiB and
uses existing context-aware streaming file identity checks. Missing or unknown
capabilities fail closed. This trusts the user's installed SDK/compiler, not a
signature claim of tool provenance or an atomic sandbox against a hostile
same-UID host modifying executable files after checks.

## Lifecycle and acceptance

One native thread exclusively owns wait/reap and signal operations. A PID is
cleared immediately on reap; an unexpected wait ownership error also prevents
further signaling. Readiness failure never resumes the suspended target.
SIGINT/SIGTERM cancellation, private parent-control-pipe EOF, command deadlines and stream
errors enter the same independent cleanup path: TERM, bounded grace, KILL if
necessary, and bounded reap. Cancellation does not first kill the supervisor.
If the supervisor fails its cleanup deadline, the parent may terminate only
its directly owned helper, and records target cleanup **unresolved**, never
accepted. Other signals that terminate the supervisor, including SIGKILL,
kernel-uninterruptible waits, hardware or machine failure cannot guarantee cleanup.

Four live bounded raw streams retain target stdout/stderr and recorder
stdout/stderr. Open writer pipes after both children exit are rejected rather
than silently accepting detached writers. Trace size remains a post-recording
bound, not a live disk quota. Private protocol stdout/stderr, lifecycle result,
actual source-derived recorder command/notification/PIDs, and all partial
trace/export/input artifacts remain private and are never removed or retried.

Acceptance additionally requires readiness-before-resume, target exit0 and
both-child reap without forced cleanup, exact attached PID/canonical executable
in the complete bounded TOC, and all existing marker/schema/reference/hash/
recorder 0-or-qualified-54 assertions. The workload alone emits its actual
start/input-open/completion markers. Emitting completion before later process
failure or termination does not qualify. None of this establishes correctness,
GPU counter semantics, uncontaminated samples or a measured speedup.

## Validation boundary

A separately authorized synthetic native feasibility experiment on macOS 26.5.1,
xctrace 16.0 (17F113), observed suspended-target readiness, all three stdout markers,
an attached Time Profiler trace with target exit0, and one cancellation which
terminated/reaped both owned children and retained a partial trace. It predates
this production implementation and does not qualify Metal/Allocations or earliest
startup sampling. Portable parallel tests exercise strict protocol, lifecycle
rejection, EOF cleanup, stream bounds, helper failure and exact TOC identity,
without native profiling.
An additional Darwin SDK C unit harness replaces every spawn/wait/signal/notify
call with deterministic fakes, proving that pending readiness with a reaped
recorder never resumes the fake target, a reaped target is never reported resumed,
and inherited ignored SIGCHLD/no-zombie policy is reset before spawning. Its
positive control proves the resume path is actually exercised. No fake PID can
reach an OS process-control call.

A separately reviewed production-native validation then ran exactly one success
and one cancellation, with a new directly owned CPU-only synthetic executable,
an audited empty external-input inventory, a minimal child environment, and a
40s owning-coordinator bound per run. Success produced CLI exit 0, readiness
before resume, target/recorder exit 0 and confirmed reap, exact attached identity,
all three actual markers, and 1,984 exported time-sample rows with required
`time`, `thread`, `thread-state` and `sample-type` values. Cancellation sent
SIGTERM only to the coordinator's owned unreaped CLI after the input-open marker;
CLI exit 1 rejected the capture, the target terminated with SIGTERM and recorder
exit 1, both were reaped, and partial trace/streams remained retained. No retry,
SIGKILL fallback, protected input, staging link or permission change was used.
This qualifies only these Time Profiler lifecycle/command/XML paths; it is not
startup sampling accuracy, launch equivalence, GPU policy or a speedup. The
production helper source SHA256 was
`77b05e0a8146788f765a06f4e1a5bed3caf1dd6744b38928a401272b4b91593b`
under observed SDK 26.5 and xctrace 16.0 (17F113). Raw evidence remains private
outside the repository.

Relevant primary contracts: installed `xcrun xctrace help record` documents
`--attach` and `--notify-tracing-started`; [Go CommandContext](https://go.dev/src/os/exec/exec.go)
controls only its direct process; [Apple posix_spawn](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/posix_spawn.2.html)
and [wait](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/wait.2.html)
define direct-child ownership. Darwin's selected SDK `sys/spawn.h` supplies
POSIX_SPAWN_START_SUSPENDED. No undocumented xctrace launch containment is assumed.
