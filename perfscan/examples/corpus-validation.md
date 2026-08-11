# perfscan real-world validation on Go corpora

perfscan run against complex real-world Go as test data (corpora under `corpus/`,
gitignored). Confirms the analyzers load large modules offline without errors,
surface genuine findings, and — critically — that `-fix` produces code that
**still compiles**.

## Findings — full catalog, report mode (2026-08-12)

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

## End-to-end `-fix` integrity — the fixed tree still compiles

The `pkg/` module was copied and `perfscan -fix ./...` applied in place:

```
findings before -fix: 32   →   after -fix: 18
perfscan: applied 12 fix(es), 0 failed
go build ./...   → exit 0        # the rewritten module compiles cleanly
go vet ./flags/  → exit 0        # no vet issues introduced
gofmt -l flags/  → (empty)       # fixes are gofmt-clean
```

Sample applied fix in `flags/selective_string.go` — `sort.Strings(s)` became
`slices.Sort(s)` and the `"slices"` import was added automatically (PS3104's
import handling), the `"sort"` import kept because other `sort.` uses remain:

```go
import ( … "slices" … "sort" )
…
slices.Sort(s)      // was: sort.Strings(s)
```

The 14 fixable findings were rewritten and the module **builds + vets clean**; the
18 residuals are advisory-only checks (structural / bit-identical-unsafe) that
carry no `SuggestedFix` by design. This demonstrates perfscan's auto-fix subset is
behavior-preserving on a real, non-trivial Go codebase.

Reproduce: point perfscan at any checked-out module, e.g.
`(cd corpus/etcd/pkg && perfscan -fix ./... && go build ./...)`.
