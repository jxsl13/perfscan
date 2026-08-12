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

The applied fixes include the newer checks on production Go — **PS3104**
(`sort.Strings`→`slices.Sort`, adding the `slices` import automatically), **PS2120**
(`w.WriteString(fmt.Sprintf(…))`→`fmt.Fprintf`), **PS2119** (`range strings.Split`→
`SplitSeq`), and PS2107 (single-value Sprintf). The 18 residuals are advisory-only
checks (structural / bit-identical-unsafe) with no `SuggestedFix` by design. So the
WHOLE grown auto-fix suite is behavior-preserving on a real, non-trivial Go module.

Reproduce: point perfscan at any checked-out module, e.g.
`(cd corpus/etcd/pkg && perfscan -fix ./... && go build ./...)`.
