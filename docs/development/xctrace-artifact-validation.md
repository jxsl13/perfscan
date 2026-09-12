# Artifact-first xctrace capture

Owner issue [#866](https://github.com/jxsl13/perfscan/issues/866) reports a
recorder time limit that returned status 54 after saving a usable Metal trace.
`traceevidence.Capture` and `go run ./cmd/tracecapture` retain such a capture
only after observing its artifacts and successfully exporting required data.
This is executable collection/validation, not a manifest of asserted booleans.

The collector creates a new directory and saves its plan before launching any
command. It invokes the configured `xcrun` with fixed `xctrace` record/export
arguments, without a shell. It observes the tool version and retains each
command's arguments, stdout, stderr, and exit/failure status, along with the
trace bundle, target output, XML exports and final result. All rejected or
partial captures remain in that directory. Nothing is retried, deleted,
published, or silently replaced.

The schema configuration must itself be a regular file no larger than 1 MiB;
directories, symlinks, devices and pipes are not accepted as configurations.

## Required evidence

Both recorder status 0 and status 54 require:

- A nonempty directory trace bundle containing only directories and regular
  files; symlinks, special files and over-limit bundles are rejected.
- The configured exact start and successful-completion lines in target output,
  each once, in order. Substring matches do not count.
- An actually successful `xctrace export --toc`, complete well-formed XML,
  exactly one fresh run numbered 1, and each required schema uniquely present.
- An actually successful export of each required table, joined to its observed
  TOC position and schema identity, required column mnemonics, nonempty rows,
  complete column counts, and populated required values. Shared value references
  must resolve to definitions of the same XML value type; empty values,
  unresolved references, duplicate identities and malformed XML are rejected.
- Identical trace member names/content before and after export, unchanged
  target output, and no command timeout, cancellation or stream-write failure.

Status 54 additionally requires ordered, unique time-limit, recording-completed
and saved-output lines. Conflicting or duplicate saved paths are rejected.
The installed xctrace 16.0 (17F113) prints the basename `capture.trace` despite
an absolute output argument. That exact basename is accepted only with its
observed native time-limit/completed spellings, the collector's fixed absolute
output argument, and the independently validated fresh bundle. The text alone
is never used to locate a trace or establish its identity.
No other nonzero status is accepted. Status 54 on its own never establishes a
timeout or usable result. Unsupported marker spellings fail closed.

Native exports can report a resolved positional node path instead of echoing
the selector supplied to `--xpath`. The collector derives that position from
the TOC and checks the returned header against it. The schema/reference layout
and positional form were cross-checked against the author's original
[Xcode 15.3 GPU export example](https://zenn.dev/r_ngtm/articles/export-instruments-gputrace-xcode15).
The CLI arguments were checked against installed `xcrun xctrace help record`
and `help export`. Synthetic tests use newly constructed data, not a copy of
that trace or a claim of new hardware measurements.

## Use

First inspect the selected Instruments version's TOC/schema and create an
explicit JSON configuration listing **all** tables and column mnemonics required
by the capture policy. Names are version-specific; instrument display names
are not table identifiers. For example, one required value table might be:

```json
[
  {"name":"gpu-counter-value","columns":["timestamp","counter-id","value"]}
]
```

This illustrative single table is **not** the complete Metal policy in #866:
that policy also needs its performance-limiter, GPU-interval and shader-profiler
tables, with their observed names and required columns. Do not omit required
tables to make a failing capture pass.

```sh
go run ./cmd/tracecapture \
  -out /approved/captures/new-sample \
  -schemas /approved/captures/required-schemas.json \
  -inputs /approved/captures/declared-inputs.json \
  -dir /private/tmp/approved-workload-directory \
  -instrument 'Metal GPU Counters' \
  -instrument 'Metal Application' -instrument GPU \
  -started WORKLOAD_STARTED -completed WORKLOAD_SUCCEEDED \
  -- /approved/bin/metal-workload
```

The workload must emit the completion marker **only after its required work
and correctness checks succeed**. A marker attests that defined phase; xctrace
does not provide the target exit status through the recorder exit code. A
workload that emits success and subsequently fails is not certified as a
successful process by this API. Applications needing that guarantee must
independently observe it. Counter identity, value semantics, active intervals,
contamination, workload correctness and statistical qualification remain
separate checks; readable XML is not a measured performance win.

Command streams/exports are bounded while being retained; exceeding a limit
cancels that command and retains the captured prefix. Trace/target limits are
checked **after recording**, not a live disk quota. They do not constrain
Instruments' internal memory use. Use short prequalified captures and suitable
external disk/process limits; a larger volume policy is not inferred here.
Cancellation terminates the direct command and bounds pipe waiting. It is not
a guarantee that Instruments has terminated every launched descendant; callers
must separately supervise the workload/process group when that is required.
The complete XML parser has explicit byte, depth and element-count limits.
Trace members are hashed as streams without loading entire files into memory.

Input preflight (#867) now requires an explicit audited-complete inventory and
post-input-open marker before any tool is launched. See the separate
[input preflight policy](xctrace-input-preflight.md). It fails early only: it
does not copy/link protected inputs, change permissions, request Full Disk
Access, or work around TCC. A parent read/hash is not proof of profiler-child
access. Permission failures retain diagnostics and fail closed.
Raw artifacts can contain sensitive workload details; keep them local and
outside a repository intended for publication.

## Validation scope

Parallel tests exercise actual subprocess execution using an explicitly
synthetic xcrun stand-in, including success/54 acceptance and all rejection
paths. They do not launch Instruments, require macOS privacy changes, or measure
GPU performance.

A separate local native Time Profiler smoke with xctrace 16.0 (17F113) validated
both recorder status 0 (1,580 exported time-sample rows) and status 54 (1,677
rows). The controlled workload performed CPU-only work, emitted its successful
phase marker, and, for the timeout case, remained alive without further work.
This qualified native command/XML/marker integration, **not** Metal counter
semantics, complete target-process success, profiling accuracy or a speedup.
This #866 smoke predates the required #867 input policy and does not qualify
profiler-child privacy access under that new policy.

All four native development attempts were retained. The first status-54 attempt
exposed the native marker/basename spelling and was rejected by the earlier
matcher. A later capture exceeded its explicitly selected 30-second command
deadline and remained rejected. The successful native-54 integration used the
CLI's documented two-minute default, with a new output directory; the rejected
captures were not reclassified or replaced. These were integration tests, not
a sampled performance campaign. Raw traces remain local, outside this repository.
No supplied result JSON is treated as authenticated evidence and no old capture
is silently relabelled. Each real Metal capture still needs all of its
predeclared required tables and independent semantic/contamination qualification.
