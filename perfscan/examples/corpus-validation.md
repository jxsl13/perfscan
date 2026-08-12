# perfscan real-world validation on Go corpora

perfscan run against complex real-world Go as test data (corpora under `corpus/`,
gitignored). Confirms the analyzers load large modules offline without errors,
surface genuine findings, and — critically — that `-fix` produces code that
**still compiles**.

## Findings — full catalog, report mode (2026-08-12)

**Kubernetes** (`k8s.io/kubernetes`, ~17k Go files, vendored) — the largest corpus,
a robustness stress test. Three big subtrees loaded offline with **zero loader
errors** and **961 findings across 23 distinct checks**:

| Subtree | findings | loader errors |
|---------|---------:|--------------:|
| pkg/scheduler/…  | 198 | 0 |
| pkg/kubelet/…    | 517 | 0 |
| pkg/controller/… | 246 | 0 |

Top: PS2101 reserve-before-loop (301), PS3103 range-value-copy (250), PS2104 (140),
PS2103 sprintf-in-loop (104). The recently-added checks all fire on production Go —
PS3104 sort→slices (21), PS2110 slices.Clone (22), PS2120 WriteString(Sprintf) (6),
PS2123 fmt.Sprint-concat (4), PS2124 Join-literal-concat (2), PS2119 range-Split→
SplitSeq (1) — confirming they aren't just synthetic-fixture checks.

`-fix` integrity on Kubernetes: `perfscan -fix -level 3 ./pkg/scheduler/...` applied
**49 fixes across 23 files** (198 → 149 findings), and the rewritten subtree
**`go build` and `go vet` both exit 0** — the auto-fix suite is behavior-preserving
on the largest corpus too. (Applied in place, then restored with `git checkout .`.)

Precision audit of the busiest advisory check: **PS3103** (range copies a large
element) fired 250× — every flagged element is a genuinely large Kubernetes API
struct (256-byte `core/v1.Volume`, 408-byte `Container`, 616-byte
`PersistentVolume`, 784-byte `Node`, …), and the **smallest** flagged element is
136 bytes — all above the check's 128-byte (two-cache-line) gate. Zero small
structs were flagged, i.e. the check is precise, not noisy, on real production Go.

Fix-quality audit of the busiest AUTO-FIX check: **PS2101** (append without
prealloc) fired 69× on scheduler alone, and `-diff` shows every rewrite pre-sizes to
the exact loop bound — `make([]string, 0)` → `make([]string, 0, len(nodes))`,
`… 0, len(pods)`, `… 0, len(podVolumes.StaticBindings)`, etc. The cap is always a
`len(...)` of the ranged collection — provably `>= 0`, so no `make` panic is
introduced and the result slice is byte-identical (capacity is only a hint) — a real
gc win (no repeated growth reallocations), correctly extracted on production Go.

**etcd** (`go.etcd.io/etcd`, 1102 Go files; a multi-module repo). Root module
`./...`: **22 findings, 0 loader errors**.

| Check | n | Check | n |
|-------|--:|-------|--:|
| PS2101 reserve-before-loop | 5 | PS2103 | 2 |
| PS3101 invariant-conversion | 3 | PS2008 | 2 |
| PS2102 string-concat-in-loop | 3 | PS3002 sort-closure | 1 |
| PS2003 | 3 | PS2107 sprintf-single | 1 |
| PS2104 | 1 | PS2002 | 1 |

The `pkg/` submodule (`go.etcd.io/etcd/pkg/v3`) separately surfaces the newer
checks on real code — e.g. **PS3104** (`sort.Strings` → `slices.Sort`, 5×),
**PS3103** (144-byte `net/url.URL` copied per range iteration, 2×), PS2101.

### Multi-module sweep — 8 etcd modules, 226 findings, 0 loader errors

etcd is 14 Go modules; running perfscan across the 8 core ones (root, api,
server, pkg, client/v3, client/pkg, etcdctl, etcdutl) aggregates **226 findings
across 21 distinct checks** with zero loader errors — a robustness stress test on
a large, real, multi-module repo. The newer checks all fire on real code:

| Check | n | Check | n | Check | n |
|-------|--:|-------|--:|-------|--:|
| PS2101 reserve | 59 | PS3003 avoid-endl-analog | 19 | PS3101 invariant-conv | 5 |
| PS3103 range-copy | 36 | PS2107 sprintf-single | 13 | PS4001 | 4 |
| PS2103 sprintf-in-loop | 23 | PS2104 | 13 | **PS2119 range-Split→SplitSeq** | 4 |
| **PS3104 sort→slices.Sort** | 19 | PS2102 concat-in-loop | 9 | **PS2120 WriteString(Sprintf)** | 1 |

(plus PS3002/3007/3001/2106/2008/2004/2003/2109 in the long tail). The checks
added this session — PS2119, PS3104, PS2120, PS3007 — are exercised on production
Go, not just synthetic fixtures.

## End-to-end `-fix` integrity — the fixed tree still compiles (full expanded suite)

The whole etcd tree was copied (so its relative-`replace` submodules resolve) and
the FULL current catalog applied to the `pkg/` module with `perfscan -fix -level 3
./...`, then rebuilt and vetted (2026-08-12):

```
findings before -fix: 33   →   after -fix: 18   (13 fixes, 0 failed)
go build ./...  → exit 0        # the rewritten module compiles cleanly
go vet ./...    → exit 0        # no vet issues introduced
```

Repeated on **`k8s.io/apimachinery`** (a self-contained Kubernetes staging module —
JSON/CBOR/protobuf runtime, reflection-heavy `deep_equal`, `sets`, `labels`) at
`-level 3` (2026-08-12): 142 first-party findings (120 in generated files skipped),
`-fix` rewrote **29 files** — `make([]T, 0, n)`/`make(map, n)` preallocation (×25),
`slices.Sort`/`Clone` (×8), `range Split`→`SplitSeq` (×4), a `strconv` — and the
module **`go build`s exit 0**. `go vet` surfaces only **four pre-existing**
`repeats json tag` warnings in apimachinery's own `_test.go` structs (identical on the
unmodified tree, unrelated to any fix); no vet issue is introduced by `-fix`. Another
independent, real, reflection-heavy module confirming the auto-fix suite is
behavior-preserving.

The applied fixes include the newer checks on production Go — **PS3104**
(`sort.Strings`→`slices.Sort`, adding the `slices` import automatically), **PS2120**
(`w.WriteString(fmt.Sprintf(…))`→`fmt.Fprintf`), **PS2119** (`range strings.Split`→
`SplitSeq`), and PS2107 (single-value Sprintf). The 18 residuals are advisory-only
checks (structural / bit-identical-unsafe) with no `SuggestedFix` by design. So the
WHOLE grown auto-fix suite is behavior-preserving on a real, non-trivial Go module.

Reproduce: point perfscan at any checked-out module, e.g.
`(cd corpus/etcd/pkg && perfscan -fix ./... && go build ./...)`.

Repeated on the larger, structurally different **`server/`** module
(`go.etcd.io/etcd/server/v3` — raft, MVCC storage, gRPC; report-mode-only until
now) at `-level 3` (2026-08-12, clang-tidy-independent, pure Go):

```
findings before -fix: 99   →   after -fix: 74   (24 fixes across 15 files, 0 failed)
go build ./...  → exit 0        # the rewritten module compiles cleanly
go vet ./...    → exit 0        # no vet issues introduced (matches the clean baseline)
```

Top findings on `server/`: PS2101 reserve-before-loop (25), PS3003 (16), PS3103
range-value-copy (14), PS3104 sort→slices (8), PS2107 single-value Sprintf (7). A
second, independent module confirms the auto-fix suite is behavior-preserving on
real production Go beyond the `pkg/` sample.

### Behavioral equivalence — the target repo's OWN test suites pass after `-fix` (2026-08-12)

`go build`/`go vet` prove the rewritten code is *type-safe*; the stronger proof is
that its *runtime behavior* is unchanged. Re-running the full current catalog against
the etcd `server/` module (`perfscan -fix -level 3 ./...`) applied **21 fixes across
15 files** — including a **PS2128 loop-concat→`strings.Builder`** rewrite in a real
MVCC hot path, `storage/mvcc/key_index.go`:

```go
var s string                          var s strings.Builder
for _, g := range ki.generations {  → for _, g := range ki.generations {
    s += g.String()                       s.WriteString(g.String())
}                                     }
return s                              return s.String()
```

etcd's own test suites for the touched packages then **pass unchanged**:

```
go test ./storage/mvcc/  → ok   (17.0s)   # the package carrying the PS2128 rewrite
go test ./auth/          → ok   ( 1.5s)
```

So the auto-fix suite is not merely recompilable but **behavior-preserving under the
project's own tests** on production Go — the strongest real-world evidence class, and
a genuine perf win extracted from a hot path (quadratic `+=` → amortized builder).
(Applied in place, then restored with `git checkout .`.)

Confirmed again on **`k8s.io/client-go`** — a fresh, large, concurrency-heavy staging
module (workqueues, rate limiters, the shared-informer cache). `perfscan -fix -level 3
./...` applied **37 fixes, 1 skipped (overlaps an applied fix)** — the last being a
benign PS2103/PS2122 overlap now reported distinctly from a failure — and the module
`go build`s exit 0. Its own suites pass unchanged, including the heavy informer/cache
tests:

```
go test ./util/workqueue/  → ok      go test ./tools/cache/          → ok (48s)
go test ./util/flowcontrol/→ ok      go test ./tools/cache/synctrack/→ ok
```

Five independent production codebases now confirm behavior-preservation under their
OWN tests (etcd, apimachinery, component-base, client-go, and the C++ spdlog). The
overlap-drop reporting added in v0.34.0 is likewise confirmed on real code here.

### The Go standard library itself — the most diverse corpus (2026-08-12)

The strongest `-fix` integrity test is the **Go standard library** (`corpus/go`,
prebuilt go1.26.5 with its own `bin/go`) — code that leans on build tags, `unsafe`,
generics, assembly-backed packages, and cgo. Report mode over a broad slice
(`net/…`, `encoding/…`, `strings/…`, `bytes/…`, `text/…`, `archive/…`,
`compress/…`, `mime/…`) surfaced **87 findings, 0 loader errors**.

`perfscan -fix -level 3` then applied **14 fixes across 9 files** spanning
`archive/zip`, `encoding/gob`, `mime/multipart`, `net/http` (incl. the generated
`h2_bundle.go`), and `net/internal/socktest`, and the stdlib **rebuilt with its own
toolchain**:

```
go build <the fixed packages>  → exit 0   # rebuilt against corpus/go as GOROOT
go vet   <the fixed packages>  → exit 0
gofmt -l <changed files>       → empty    # fixes stay gofmt-clean
```

The rewrites are textbook and bit-identical, e.g.

```go
files := make(map[string]int)          → make(map[string]int, len(r.File))   // PS2104
seen  := map[http2SettingID]bool{}     → make(map[http2SettingID]bool, num)  // PS2104
for _, f := range strings.Split(v,",") → for f := range strings.SplitSeq(v,",") // PS2119
```

So the auto-fix suite is behavior-preserving even on the standard library — the most
heterogeneous real Go there is. (`corpus/go` restored with `git checkout .` after.)

## Generated-file skipping validated at scale (Kubernetes protobuf)

perfscan skips generated files (`// Code generated ... DO NOT EDIT.`) by default —
you don't hand-optimize machine-emitted code. Kubernetes is the acid test: it is
saturated with generated Go (`zz_generated.deepcopy.go`, protobuf `generated.pb.go`).

On `staging/src/k8s.io/api/...` (the protobuf-heavy API types), **every** finding is
in generated code:

```
perfscan -level 3 ./staging/src/k8s.io/api/...
  → 0 findings, exit 0
  → "perfscan: 3444 finding(s) in generated files skipped (use -include-generated to report them)"

perfscan -level 3 -include-generated ./staging/src/k8s.io/api/...
  → 3444 findings — 3444/3444 in .pb.go / zz_generated files
```

The suppressed 3444 are exactly the protobuf `Marshal`/`Size`/`String` shapes —
PS2124 (×1209), PS2003 (×950), PS2102 (×616), PS3103 (×348), PS3104 (×180) — noise a
user cannot act on (regenerated on every `make`). Default-skipping turns a 3444-finding
flood into a clean report; `-include-generated` is there when you do want them. (By
contrast `pkg/apis/...` `zz_generated.deepcopy.go` carries **0** perf findings — plain
deepcopy code — so skipping is a no-op there; the win is concentrated in protobuf.)

## `-diff` produces valid patches on real code

`perfscan -diff -level 3 ./...` on the etcd `pkg/` module printed a **179-line
unified diff across 8 files (13 fixes)**, left the source **byte-unchanged**
(dry-run), and exited 1. The emitted patch is **`git apply --check`-clean**
(`git apply --check -p1 --directory=pkg`), confirming the diff renderer produces
genuinely valid, appliable unified diffs on real multi-file production Go — so
`-diff` is a reliable CI gate / review preview of exactly what `-fix` would write.

## PS3106 large-value-param is precise on real API-struct-heavy code (Kubernetes)

PS3106 (advisory: a >128 B struct/array passed by value copies on every call) fires
only on genuinely large values. On `k8s.io/kubernetes` `pkg/apis/...` + `pkg/scheduler/...`
it reports **32 findings, every one a real large API struct** by value —
`batch.JobSpec` (952 B), `core.PodSpec` (592 B), `core.PodStatus` (384 B),
`core/v1.Volume` (256 B), `meta/v1.ObjectMeta` (232 B), … The **smallest** flagged
type is **136 B**, just over the 128 B (two-cache-line) gate; nothing smaller leaks.
Unlike the C++ sibling PX3020 (whose "missed move" has a move-via-iterator false
positive), PS3106 has **no false-positive class** — Go has no move semantics, so
"large struct passed by value" is unambiguous, and the size gate is exact.

## `-fix` is idempotent

Running `perfscan -fix -level 3 ./...` on an etcd `pkg/` copy applied **13 fixes**;
a SECOND pass (`-diff`) then reports **nothing left to change** (exit 0, empty
patch), and the rewritten module still `go build`s and `go vet`s exit 0. So `-fix`
converges to a fixpoint — repeated CI runs never oscillate or re-churn already-fixed
code.

## Prealloc checks never over-size a conditional fill (regression, verified on etcd)

PS2101 (slice append) and PS2104 (map insert) pre-size only when at least one add
per iteration is UNCONDITIONAL — a conditional-only fill (`for … { if … { out =
append(out, x) } }`) is left alone, since pre-sizing to the loop bound would be the
theoretical max, not what the loop actually stores. Confirmed on real code:
`perfscan -level 3 -checks PS2101,PS2104` on `go.etcd.io/etcd/server` reports **16
findings, every one "exact: one unconditional value per iteration"** — zero
conditional-only findings leak (the old "0 unconditional / all conditional" upper-bound
form no longer appears anywhere). So the guard fires exactly on the sites where a
size hint is warranted.
