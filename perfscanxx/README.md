# perfscanxx (perfscan++)

A perfscan-style **C++** performance linter — a Go CLI that orchestrates
[`clang-tidy`](https://clang.llvm.org/extra/clang-tidy/). It maps clang-tidy's
`performance-*` checks into perfscan's graded model (stable `PX` ids, one
`-level` knob gating both reporting and fixing, text/JSON/SARIF output) and adds
query-based custom checks — **zero of our own C++** (the only C++ is the prebuilt
`clang-tidy` binary). See [DESIGN.md](DESIGN.md).

## Install

```bash
go install github.com/jxsl13/perfscan/perfscanxx@latest
```

Or download a prebuilt binary from the
[releases page](https://github.com/jxsl13/perfscan/releases) — assets are named
`perfscanxx_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows), published on
`perfscanxx/vX.Y.Z` tags.

### Runtime dependency: clang-tidy

perfscanxx builds and unit-tests without clang-tidy, but analyzing C++ needs it
(LLVM ≥ 20 for the query-based custom checks):

```bash
brew install llvm            # macOS (keg-only)
# then point perfscanxx at it (brew llvm is not on PATH):
export PERFSCANXX_CLANG_TIDY="$(brew --prefix llvm)/bin/clang-tidy"
# on Linux: apt install clang-tidy
```

## Usage

```bash
perfscanxx -p build ./...             # analyse the whole project (like `perfscan ./...`)
perfscanxx -p build ./src/game/...    # just a subtree
perfscanxx -checks PX1* -p build ./... # only copy checks
perfscanxx -level 1 -fix -p build ./... # apply only L1 (idiomatic) fix-its
perfscanxx -json -p build ./...       # machine-readable output
perfscanxx -p build src/a.cpp         # a single translation unit
perfscanxx -list                      # the PX check table
perfscanxx -explain PX1001            # a check's documentation
```

A path arg is a Go-style pattern or directory (`./...`, `./src/game/...`)
expanded against the compilation database to the translation units under it; no
args means `./...`. The `compile_commands.json` is found via `-p` or by walking
up from the current directory.

See [examples/](examples/) for an end-to-end sample and a recipe for running
against a real C++ codebase (DDNet), plus [examples/validation.md](examples/validation.md)
for validation results on fmt, spdlog, leveldb and abseil.
