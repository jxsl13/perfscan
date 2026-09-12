# Exact allocation diagnostic campaigns

`go run ./cmd/allocationcampaign` builds matched diagnostics, runs two serialized
phases in fresh processes, retains all raw artifacts, and independently reparses
the complete matrix. This supplements original benchmark evidence. It makes no
optimization, allocation-site, equivalence, or rejected-candidate promotion claim.

Prepare two clean pinned git source trees with a byte-identical diagnostic test
file in the selected package. The file must expose `TestAllocationDiagnostic`,
select its existing leaf benchmark via `PERFSCAN_ALLOCATION_ARM=before|after`, and
use `benchmarkevidence.Run(1024, benchmark)`. The repository's opt-in
`benchmarks/allocation_diagnostic_test.go` is a compiled example; preserve the
existing measured bodies. Fatal/Goexit from the callback and cleanup Error/Fatal
reject the measurement. Raw cleanup-only Goexit during an initial probe is
unsupported because the public testing API can hide it when later runs succeed.

```bash
go run ./cmd/allocationcampaign -old /path/to/pinned-old -candidate /path/to/pinned-candidate -out /path/to/new-evidence -pairs 8 -procs 1,12
```

Predeclare the count, even pair count, process counts, source commits, diagnostic
file and package before collection. The runner rejects dirty trees, differing
diagnostic hashes or differing effective Go build environments. Both binaries
use the same `go test -c -trimpath -mod=readonly -tags=allocationdiagnostic`
options. Builds use retained `git archive` snapshots of the exact pinned commits,
excluding ignored/untracked files; original checkout changes cannot alter those
build inputs. Retained git ls-tree blob identities verify every archived file,
rejecting omitted/substituted files (including export-ignore/export-subst effects).
Archive creation uses command-local `core.autocrlf=false` to preserve committed
blob bytes under Windows-style checkout defaults, without changing repository
configuration. Explicit attributes such as `text eol=crlf` can still transform
archive bytes; those trees are intentionally rejected by the same blob check.
Archive symlinks and unversioned/local module replacements are
rejected, workspace resolution is disabled with GOWORK=off, and dependencies
must resolve from pinned versions/checksums with `-mod=readonly`. Supply tracked
benchmark fixtures: ignored data and workspace dependencies are unsupported.
Ambient/effective GOFLAGS are rejected, preventing external overlays, modfiles
and tool wrappers from silently replacing the archived inputs.
It retains clean pre/post-build status and HEAD, source archive SHA256, effective
Go environment, raw build stdout/stderr/exit, and binary SHA256. Instrumentation
hashes cover the wrapper and every file in `benchmarkevidence`; use
`-instrumentation` to additionally list every other diagnostic implementation
dependency. This PR deliberately supports the perfscan-layout default manifest;
another owner layout needs an explicitly reviewed replacement manifest in a
follow-up. The manifest must cover the complete nonstandard diagnostic
implementation. Source archives retain all those inputs. GODEBUG, GOGC and GOMEMLIMIT
are pinned in the plan; the measurements inherit the runner's environment while
explicitly selecting GOMAXPROCS and arm. Keep the host environment unchanged
during execution and avoid other activity in those Go processes.

Each binary executes from its archived source package directory, preserving
relative paths to tracked fixtures without adding evidence files to that package.

The first phase is a contemporaneous identical-binary negative control:
A and B both execute the old binary's `before` benchmark. The second phase runs
old `before` versus candidate `after`. For each phase, odd pairs use forward
process order and A/B; even pairs reverse process order and B/A. Equal even pair
counts balance arm and process order. All invocations use fixed N=1024. Each
phase/process-count cell has exactly two arms per pair. Repeated campaigns use
separate fresh output directories; never replace original qualification records.

The output directory must not already exist. The entire plan is written before
the first sample, and its SHA256 is printed before collection. Every planned
invocation has separate raw stdout, stderr and exit files; sample failures do
not prevent later samples from being retained. A records manifest is updated
after every invocation, and its final SHA256 is printed even when verification
rejects a sample. Retain both printed pins outside the evidence directory, for
example in the campaign's preregistration/run log. Interrupted/incomplete runs
retain partial artifacts and fail completeness verification. Failed builds keep
their logs but do not start measurements. Filesystem/write failures stop safely.

Run fresh independent verification using those external pins:

```bash
go run ./cmd/allocationcampaign -verify /path/to/evidence -plan-sha256 PREMEASUREMENT_PIN -records-sha256 POSTMEASUREMENT_PIN
```

Verification checks externally pinned manifests, matched binary/source/build
provenance, retained diagnostic hashes, exact invocation count/order, raw hashes
and exits, one PASS and one complete integer allocation record per sample. JSON
duplicate keys, unknown/missing fields, null counters, Float-typed counters,
overflow/nonfinite representations and broken quotient/remainder identities are
rejected. Exact counters must fit nonnegative int64; larger results are rejected
instead of truncated, so candidate-minus-baseline subtraction is safe.

`analysis.json` retains every A/B exact record and its signed paired total-byte
and allocation-count delta. Rounded B/op and allocs/op arm medians and their
differences appear separately as exact rational strings. The independent verifier
recomputes this report from raw artifacts rather than trusting `analysis.json`.
A paired median is not a difference of arm medians. No post-hoc control-noise
subtraction or statistical equivalence assertion is performed.

Counters are process-wide, including worker/background/runtime activity.
Identical binaries can differ in exact totals; external OS processes do not
directly allocate on the measured Go heap. A surviving raw-total difference does
not identify its allocation site or causal mechanism. GC-published profile
windows, stack normalization/collisions, zero-active profile rows, and profile
serialization tails are separate concerns: this tool does not capture or
interpret profiles. Seek allocation-site/size-class evidence before attributing
differences, and preserve the project's original qualification gates.

Related owner issue: [#968](https://github.com/jxsl13/perfscan/issues/968).
