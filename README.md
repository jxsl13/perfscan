# perfscan

`perfscan` is a staticcheck-style **performance linter for Go**: a registry of
stable, benchmark-verified checks (`PS1001`…) that find performance
anti-patterns, with **graded auto-fixing** — from idiomatic rewrites a
reviewer waves through to hyper-optimizations that must be benchmark-gated.

It generalizes an internal performance scanner that found dozens of
measured wins (2x–6x on hot kernels) in a large numerical-computing
codebase into a standalone, community-maintained utility with a
staticcheck-like architecture: every check is a standard
`golang.org/x/tools/go/analysis.Analyzer`, wrapped in metadata for stable
IDs, categories, documentation and fix-level gating. The engine itself is
fully generic — anything project-specific lives in a JSON vocabulary
config, so any codebase (including the one it came from) is supported
given the right configuration.

## Install

```bash
go install github.com/jxsl13/perfscan@latest
```

Or download a prebuilt binary from the
[releases page](https://github.com/jxsl13/perfscan/releases) — assets are named
`perfscan_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows), published on
plain `vX.Y.Z` tags.

## Usage

```bash
perfscan ./...                     # report all findings
perfscan -checks PS2* ./...        # only allocation checks
perfscan -checks all,-PS3003 ./... # everything except one check
perfscan -level 1 ./...            # only findings with idiomatic (L1) fixes
perfscan -level 1 -fix ./...       # report + apply only L1 (idiomatic) fixes
perfscan -level 2 -fix ./...       # report + apply L1 and L2 fixes
perfscan -fix ./...                # default -level 3: apply every available fix
perfscan -diff ./...               # dry run: unified diff of what -fix would do,
                                   #   changes nothing, exit 1 if fixes are pending
perfscan -json ./...               # machine-readable output (editor quick-fixes)
perfscan -sarif ./...              # SARIF 2.1.0 (GitHub Code Scanning)
perfscan -list                     # the check table
perfscan -explain PS2005           # a check's full documentation
```

Findings print in the standard `file:line:col: message (PSid Ln)` shape, so
any editor problem-matcher picks them up.

## Adopting perfscan on an existing codebase (baseline / ratchet)

Record the current findings once, then fail CI only on regressions while
the backlog is burned down incrementally:

```bash
perfscan -baseline perfscan-baseline.yaml -write-baseline ./...  # accept today's findings
perfscan -baseline perfscan-baseline.yaml ./...                  # exit 1 only on NEW findings
```

Baseline identity is line-independent (`{file, check, message}` with
counts), so unrelated edits that shift line numbers do not resurrect
accepted findings. Re-run `-write-baseline` after fixing a batch to ratchet
the accepted set down.

## Fix levels: graded optimization

Performance fixes differ wildly in what they cost the *reader*. perfscan
makes that cost a first-class property: every check carries a **level**, and
one `-level` knob gates **both** what is reported and what `-fix` applies: you fix exactly what you see.

| Level | Name | Character | Auto-fix policy |
|------:|------|-----------|-----------------|
| **L1** | idiomatic | The fix is idiomatic Go a reviewer waves through: hoist a `regexp.MustCompile` out of the loop, pre-size a slice or builder. Deterministic, mechanical, bit-identical. | Fixed whenever reported (`-fix`). |
| **L2** | structured | The fix restructures code: loop interchange, slab allocation, `sync.Pool` scratch, map→slice densification. Correct, but it changes the shape of the code. | Fixed when reported (`-level` ≥ 2 with `-fix`); review + benchmark expected. |
| **L3** | aggressive | Hyper-optimization: unroll-and-jam, register tiling, band parallelization, branchless clamps — what the reference corpus does on its hot kernels. Buys the last factor at a real maintainability price. | Fixed when reported (default `-level 3` with `-fix`), and only where the shape makes the rewrite provably behavior-preserving; everything else stays advisory pending an A/B benchmark. |

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
the original internal registry keep their numbers; perfscan-original
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

```yaml
# perfscan.yaml (auto-discovered up to the module root, or -config file.yaml;
# JSON files still parse — YAML is a superset)
elementAccessors: [AtF64, SetF64]
fastPathHelpers: [flatF64, flatF32]
elementCountMethods: [Numel]
allocatorFuncs: [New, Zeros, Cast]
perElementVisitors: [readGen, fillGen]
vectorizedSiblingFuncs: [vexpF32, vsiluF32]
fanOutHelpers: [parallel.For]
dtypeMethods: [Dtype]
```

Domain checks are **opt-in**: without their vocabulary they are skipped
silently (the CONFIG column of `perfscan -list` shows what each needs).
Naming one explicitly (`-checks PS1001`) without its vocabulary prints a
warning saying exactly which fields are missing — an explicitly requested
check that cannot fire is worth one loud line, a wildcard run is not.

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

## golangci-lint integration

perfscan ships a [module plugin](plugin/) for golangci-lint's custom build
system:

```yaml
# .custom-gcl.yml
version: v2.1.0
plugins:
  - module: 'github.com/jxsl13/perfscan/plugin'
    version: latest
```

```yaml
# .golangci.yml
linters:
  enable: [perfscan]
  settings:
    custom:
      perfscan:
        type: module
        settings:
          maxLevel: 2          # only L1/L2 findings
          vocabulary:          # optional domain vocabulary (perfscan.yaml shape)
            fanOutHelpers: [parallelFor]
```

Then `golangci-lint custom` builds the binary.

## Status & roadmap

perfscan is young. The engine, CLI, fix-level gating, vocabulary config and
a growing check set are in place; the original reference registry (~80
checks, all benchmark-verified) is being ported check by check. See
[ROADMAP.md](ROADMAP.md) for the porting status and
[CONTRIBUTING.md](CONTRIBUTING.md) if you want to help — new checks need a
measured win, a positive + negative fixture, and a stable ID.

## License

[MPL-2.0](LICENSE.md)
