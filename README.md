# perfscan

`perfscan` is a staticcheck-style **performance linter for Go**: a registry of
stable, benchmark-verified checks (`PS1001`…) that find performance
anti-patterns, with **graded auto-fixing** — from idiomatic rewrites a
reviewer waves through to hyper-optimizations that must be benchmark-gated.

It grew out of the `internal/perfscan` tool of
[goai](https://github.com/jxsl13/goai), which found dozens of measured wins
(2x–6x on hot kernels) in a numerical codebase. This repository generalizes
that engine into a standalone, community-maintained utility with a
staticcheck-like architecture: every check is a standard
`golang.org/x/tools/go/analysis.Analyzer`, wrapped in metadata for stable
IDs, categories, documentation and fix-level gating.

## Install

```bash
go install github.com/jxsl13/perfscan/cmd/perfscan@latest
```

Or download a release binary from the
[releases page](https://github.com/jxsl13/perfscan/releases).

## Usage

```bash
perfscan ./...                     # report all findings
perfscan -checks PS2* ./...        # only allocation checks
perfscan -checks all,-PS3003 ./... # everything except one check
perfscan -level 1 ./...            # only findings with idiomatic (L1) fixes
perfscan -fix ./...                # apply L1 auto-fixes in place
perfscan -fix -fix-level 2 ./...   # also apply L2 (structured) fixes
perfscan -json ./...               # machine-readable output (editor quick-fixes)
perfscan -list                     # the check table
perfscan -explain PS2005           # a check's full documentation
```

Findings print in the standard `file:line:col: message (PSid Ln)` shape, so
any editor problem-matcher picks them up.

## Fix levels: graded optimization

Performance fixes differ wildly in what they cost the *reader*. perfscan
makes that cost a first-class property: every check carries a **level**, and
`-fix` never applies a rewrite above the requested `-fix-level`.

| Level | Name | Character | Auto-fix policy |
|------:|------|-----------|-----------------|
| **L1** | idiomatic | The fix is idiomatic Go a reviewer waves through: hoist a `regexp.MustCompile` out of the loop, pre-size a slice or builder. Deterministic, mechanical, bit-identical. | Applied by plain `perfscan -fix`. |
| **L2** | structured | The fix restructures code: loop interchange, slab allocation, `sync.Pool` scratch, map→slice densification. Correct, but it changes the shape of the code. | Applied only with `-fix -fix-level 2`; review + benchmark expected. |
| **L3** | aggressive | Hyper-optimization: unroll-and-jam, register tiling, band parallelization, branchless clamps — what the goai reference does on its hot kernels. Buys the last factor at a real maintainability price. | Advisory. Never auto-applied; requires an A/B benchmark and (where claimed) a bit-identity proof. |

The philosophy is inherited from the reference tool's pattern catalog:
**every finding is a candidate, not a verdict**. Static analysis sees syntax,
not hotness — confirm with a pre/post benchmark and skip cold paths where the
fix isn't worth the code.

## Check IDs

Every check has a stable PS-prefixed 4-digit ID, grouped by the thousands
digit:

| Range | Category |
|-------|----------|
| PS1xxx | per-element access in hot loops |
| PS2xxx | allocation |
| PS3xxx | indirection / reflection |
| PS4xxx | vectorization |
| PS5xxx | arithmetic |
| PS6xxx | verification gaps |
| PS7xxx | offload / device transfer |

IDs are **never reused**: a retired check leaves a hole. IDs inherited from
the goai reference registry keep their original numbers; perfscan-original
checks use the `x1xx` block of each category (e.g. `PS2101`). Run
`perfscan -list` for the live table.

## Suppressing findings

```go
//perfscan:ignore PS2005 pattern varies per tenant, compile is cold
for _, t := range tenants {
	re := regexp.MustCompile(t.Pattern)
	...
}
```

The directive suppresses the named checks (comma/space separated) on its own
line or the line below; a bare `//perfscan:ignore` suppresses everything on
that line. Suppressions name IDs, which is why IDs are stable.

## Project vocabulary (domain checks)

Most checks are pure language/stdlib shapes and run on any Go module with no
configuration. **Domain checks** key on a project's own vocabulary — its
element accessors, allocators, fast-path helpers, vectorized kernels — which
lives in a JSON config, not in the engine:

```jsonc
// perfscan.json (auto-discovered up to the module root, or -config file.json)
{
  "elementAccessors":  ["AtF64", "SetF64"],
  "fastPathHelpers":   ["flatF64", "flatF32"],
  "elementCountMethods": ["Numel"],
  "allocatorFuncs":    ["New", "Zeros", "Cast"],
  "perElementVisitors": ["readGen", "fillGen"],
  "vectorizedSiblingFuncs": ["vexpF32", "vsiluF32"],
  "fanOutHelpers":     ["parallel.For"]
}
```

With no vocabulary a domain check stays silent — **and says so**: the runner
names each starved check in a stderr warning, because a silent zero from a
starved check reads as "no instances".

## Editor integration

The text output matches a standard problem-matcher; for VS Code:

```jsonc
// .vscode/tasks.json
{
  "label": "perfscan",
  "type": "shell",
  "command": "perfscan ./...",
  "problemMatcher": {
    "owner": "perfscan",
    "fileLocation": ["relative", "${workspaceFolder}"],
    "pattern": { "regexp": "^(.*):(\\d+):(\\d+): (.*) \\((PS\\d{4})[^)]*\\)$",
                 "file": 1, "line": 2, "column": 3, "message": 4, "code": 5 }
  }
}
```

`perfscan -json` emits every finding with its fix's text edits (line/col
ranges, byte offsets, replacement text) for quick-fix integrations.

Because every check is a plain `analysis.Analyzer`, you can also embed them
in your own multichecker or `go vet -vettool` binary.

## Status & roadmap

perfscan is young. The engine, CLI, fix-level gating, vocabulary config and
a first slice of checks are in place; the goai reference registry (~80
checks, all benchmark-verified) is being ported check by check. See
[ROADMAP.md](ROADMAP.md) for the porting status and
[CONTRIBUTING.md](CONTRIBUTING.md) if you want to help — new checks need a
measured win, a positive + negative fixture, and a stable ID.

## License

[MPL-2.0](LICENSE.md) — same license as the goai reference implementation.
