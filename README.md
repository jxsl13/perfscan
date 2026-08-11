# perfscan monorepo

Two independent, benchmark-verified performance linters that share a philosophy —
**every finding is a benchmark-backed pattern with a graded, opt-in fix**, one
`-level` knob gating both reporting and fixing — across two languages.

| Module | Path | What it is |
|--------|------|------------|
| **perfscan** | [`perfscan/`](perfscan/) | A Go performance linter (`go/analysis`-based). ~60 checks with bit-identical auto-fixes graded L1/L2/L3, YAML config, baseline, SARIF, and a golangci-lint plugin. |
| **perfscan++** (`perfscanxx`) | [`perfscanxx/`](perfscanxx/) | The C++ analog: a Go CLI that **orchestrates clang-tidy**. It maps clang-tidy's `performance-*` checks into the same graded PX-id model and adds query-based custom checks — **zero of our own C++** (the only C++ is the prebuilt `clang-tidy` binary). |

## Install

Both tools are plain Go binaries. Install the one(s) you want with `go install`:

```bash
go install github.com/jxsl13/perfscan/perfscan@latest      # the Go linter
go install github.com/jxsl13/perfscan/perfscanxx@latest    # the C++ analyzer
```

Or download a prebuilt binary from the
[releases page](https://github.com/jxsl13/perfscan/releases). Each tool releases
independently on its own tag prefix — `perfscan/vX.Y.Z` and `perfscanxx/vX.Y.Z`
(Go requires the subdir prefix for versioned installs) — producing a release
with that tool's binaries, named `<tool>_<version>_<os>_<arch>.tar.gz`
(`.zip` on Windows), for linux/darwin/windows × amd64/arm64.

perfscanxx additionally needs `clang-tidy` at runtime (LLVM ≥ 20 —
`brew install llvm`); see [perfscanxx/README.md](perfscanxx/README.md).

## Clean separation

The two tools are **peer Go modules with disjoint source trees** — neither
imports the other:

```
perfscan/            module github.com/jxsl13/perfscan/perfscan     (Go linter)
perfscanxx/          module github.com/jxsl13/perfscan/perfscanxx   (C++ analyzer)
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
