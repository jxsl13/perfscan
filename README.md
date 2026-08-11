# perfscan monorepo

Two independent, benchmark-verified performance linters that share a philosophy —
**every finding is a benchmark-backed pattern with a graded, opt-in fix**, one
`-level` knob gating both reporting and fixing — across two languages.

| Module | Path | What it is |
|--------|------|------------|
| **perfscan** | [`perfscan/`](perfscan/) | A Go performance linter (`go/analysis`-based). ~60 checks with bit-identical auto-fixes graded L1/L2/L3, YAML config, baseline, SARIF, and a golangci-lint plugin. |
| **perfscan++** (`perfscanxx`) | [`perfscanxx/`](perfscanxx/) | The C++ analog: a Go CLI that **orchestrates clang-tidy**. It maps clang-tidy's `performance-*` checks into the same graded PX-id model and adds query-based custom checks — **zero of our own C++** (the only C++ is the prebuilt `clang-tidy` binary). |

## Clean separation

The two tools are **peer Go modules with disjoint source trees** — neither
imports the other:

```
perfscan/            module github.com/jxsl13/perfscan     (Go linter)
perfscanxx/          module github.com/jxsl13/perfscanxx   (C++ analyzer)
go.work              dev workspace tying both (not fetched by consumers)
corpus/              shared real-world test codebases (gitignored)
LICENSE.md .github/  repo-level, shared
```

`perfscan` mirrors `go/analysis`; `perfscanxx` mirrors `clang-tidy`. Same UX,
same grading model, independent code.

## Develop

```sh
# the workspace makes both modules resolve locally
go work sync

# per module
cd perfscan   && go build ./... && go test ./...
cd perfscanxx && go build ./... && go test ./...   # clang-tidy needed only at runtime
```

perfscan build/test needs only Go. perfscanxx builds and unit-tests without
clang-tidy; running it against C++ needs `clang-tidy` (LLVM ≥ 20 for query-based
custom checks — `brew install llvm`). See each module's README for details.
