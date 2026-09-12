# Issue856: reusable native snapshot output

PS6137 is an opt-in, multi-function advisory, with no automatic rewrite.
It covers the allocating GoAI Metal `Recorder.Profile` before and after
[PR1184](https://github.com/jxsl13/goai/pull/1184), including its actual
single-event fast path, result struct, materialization helper and label cache.
It does not substitute PS2140's parameter-sized direct slice rule or PS6110's
extraction-local string deduplication: this owner already deduplicates labels
within each extraction but freshly owns them again on later extractions.

## Source and contract boundary

The exact parent is `9501df31a4b9db0852875322afecd5b352c64f0d`; the merge is
`e2304d986be11a3f27b3b6e72b58dbdcb730d4b0`. The frozen fixtures contain complete
`Recorder`/result/cache declarations, `Free`, `Profile`, `fillRecorderProfileEvents`
and `recorderProfileLabels.own`. The merge additionally contains the complete
`ProfileInto`, `recorderProfileLabelView`, `remember`, `ownExisting` and Into fill
helper. `Profile` remains unchanged. Complete native header and snapshot/token
functions are pinned separately; the merge includes the complete by-value view.

Source proves the same receiver handle, distinct local pointer/count/scalar
out-arguments, integer status validation and signed negative-count/nonempty
nil-pointer validation. The count==1 branch uses `C.GoString` and a one-element
composite slice. The other branch makes result storage from the same count,
forms a native `unsafe.Slice`, validates the second token acquisition, and
passes the same result/native/token roles to an exact typed fill helper.
That auxiliary token acquisition can fail after the fresh allocation; this is
authentic allocating-API behavior, not caller-owned destination mutation.

The fill has a fresh zero-state extraction-local cache, one canonical event
index, exact numeric field mappings and EventSpan update. The cache's token and
exact-content lookup guards, bounded NUL scan, `strings.Clone` provenance and
all cache writes/returns are checked from source. Borrowed native strings or
keys, pointer arithmetic, callbacks, rebinding, aliases, escapes, extra
dispatches and unsupported helper/control-flow shapes remain silent.

Reviewed project contracts supply native array extent/storage validity,
termination, immutable synchronous lifetime through extraction, the matching
token array extent and token/content identity semantics, native release,
concurrency/completion policy and representative repeated extraction. None of
these native/provider facts is inferred from an identifier, C declaration or
boolean acknowledgement. The source-owned copying summary does not prove every
runtime shape, valid calibration, caller workload or a profitable reuse ratio.

The eight zero-based acquisition roles in the owner are:

| Role | Argument | Owned result field |
| --- | --- | --- |
| Receiver handle | 0 | — |
| Event pointer out | 1 | Events |
| Event count out | 2 | Events length |
| Omitted MPS out | 3 | OmittedMPS |
| Overflow out | 4 | OmittedOverflow |
| Unsupported out | 5 | OmittedUnsupported |
| Frequency out | 6 | TimestampFrequency |
| Command duration out | 7 | CommandDuration |

The native header actually declares a **96-byte** label array, C `int`
count/omission fields and `unsigned long long` timing fields. Genuine cgo replay
uses the frozen declarations, not a structural lookalike at invented widths.
It proves compiler name/argument/ABI binding, including pointer-to-pointer base
aliases and observed compiler pointer checks. The C function implementations
are explicitly non-recording stubs; opaque `ResidentQGroup` is a type scaffold.
No real GoAI GPU recording, ownership-after-Free test or benchmark is executed
by this replay. The pinned Objective-C implementations are human-review
evidence, not an executable native semantic proof supplied by PS6137.

## Existing API and remedy

An exact supported Into sibling changes the recommendation rather than claiming
the API is missing. For the actual owner it is `ProfileInto(*RecorderProfile)
error`, on the identical receiver. Direct-slice APIs with this supported
acquisition/materialization grammar may instead use `*[]T` plus `error`, or
`[]T` destination plus returned `[]T,error`. An unrelated same-name method,
different destination, wrong receiver/result or variadic/generic signature is
not accepted. Signature recognition proves existence only, not ownership,
capacity reuse, parity, destination atomicity or measured allocation behavior.

When an additive Into API is warranted, preserve the allocating convenience
API and move every fallible acquisition/validation before caller storage is
mutated. Grow insufficient capacity, then reuse sufficient capacity. Reuse only
existing Go-owned strings after exact native-content equality; never retain a
native `unsafe.String` view. Validate all scalar metadata and EventSpan, not
merely labels and event count. Preserve omitted/invalid samples, NUL byte
semantics, error ordering, concurrency and synchronization.

A compact by-value native view is separate platform/ABI/compiler benchmark
advice. Its C source uses by-value return, but an ABI can still implement a
large struct result through a hidden return pointer. Removing explicit Go
out-argument locals does not prove removal of all native pointer arguments,
an allocation reduction or an isolated ABI speedup.

## Acceptance and evidence

Parallel ordinary tests replay both complete owner versions through genuine
cgo, direct-slice variants and supported/wrong Into signatures. Adversarial
tests cover status/count/pointer/scalar/field/index/provider mismatches,
geometry writes/address escapes, wrong token extent, opaque calls/aliases,
borrowed strings/cache keys, wrong token/content guards, scan bounds/native
offsets, global/cache escapes, missing review policies, config deep cloning,
duplicate contracts and extra generated-cgo effects. Compiler-disabled builds
explicitly skip cgo replay; config and vocabulary tests still run.

These are downstream native acceptance requirements, **not detector-proven
runtime results**: zero/one/many events; growth from insufficient capacity then
steady-state zero allocation; changed/reordered labels and more than sixteen
distinct-label cache overflow; exact result/scalar/EventSpan parity; destination
unchanged on every error; labels remaining owned after native destruction; and
nonregression of the unchanged allocating convenience API. PR1184's tests
cover repeated result/string backing identity, parity, after-Free labels and
selected error atomicity. Its warmed benchmarks report zero allocations;
neither the detector nor those selected tests exhaustively prove all scenarios.

The owner reports three independent, order-alternated count-seven 340-event
campaigns on Apple M2 Pro: Profile medians2.713/2.735/2.786us versus
Into1.697/1.691/1.827us (1.60x/1.62x/1.52x),14400B/6allocs to0B/0allocs.
Ten-label cases report1.40x–1.44x and one-event cases1.29x–1.33x, also warmed
zero-allocation Into. These historical owner results combine destination reuse,
owned-label reuse and native view changes; they are not perfscan measurements,
an isolated native-view gain, or a universal speedup. Reproduce representative
native workloads with pinned SDK/compiler/binary and alternating controls
before adoption; reduced allocations alone do not imply lower latency.

Frozen fixture SHA256 values (including provenance comments):

- Parent Go: `9f564cdd882c28dc11d497e7fe1c58a5f608903fbae8f335155092c294eaa4fb`.
- Merge Go: `91a20c9c43d8ac15164721b9831cfc8cd73456b5fe83173d7854e9f30f9fb68d`.
- Parent header: `73f4f3279d32e77fafc722c3230eecdb762d487ccfa48e546de708fc4391c6a0`.
- Merge header: `b7fe8e71c4b9c59bc66a9721bce4da181178c1ec1703dd1cdf8dfae653a1d992`.
- Parent bridge: `0014b85b5a44ba42f5afe426e1401754961f3ebb020a76ec5d266c953a688343`.
- Merge bridge: `6d8d61b53ab3e663a859ddd55b205cd6bec63d595da6784fb4c02f7efef4ab6d`.
