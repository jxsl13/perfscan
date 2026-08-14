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
perfscanxx -j 8 -p build ./...        # analyse with 8 parallel clang-tidy workers (default: one per CPU; -j 1 = sequential)
perfscanxx -timeout 5m -p build ./... # abort if the whole run exceeds 5m (CI safety valve; 0 = no limit)
perfscanxx -p build ./src/game/...    # just a subtree
perfscanxx -checks PX1* -p build ./... # only copy checks
perfscanxx -level 1 -fix -p build ./... # apply only L1 (idiomatic) fix-its
perfscanxx -fix -fix-sequential -p build ./... # apply each check's fix-its in its own pass (collision-free)
perfscanxx -diff -p build ./...       # preview what -fix would change, as a unified diff (apply nothing)
perfscanxx -baseline pxx.yaml -p build ./...  # ratchet: seed, then fail only on NEW findings
perfscanxx -json -p build ./...       # machine-readable findings; a single-edit fix-it ships as a structured `edit` (also -sarif for GitHub Code Scanning one-click fixes)
perfscanxx -v -p build ./...          # also list the TUs that did not fully parse
perfscanxx -p build src/a.cpp         # a single translation unit
perfscanxx -list                      # the PX check table (with an auto-fix coverage summary)
perfscanxx -list -fixable             # only the auto-fixable checks
perfscanxx -list -json                # the catalog as machine-readable JSON
perfscanxx -explain PX1001            # a check's documentation
perfscanxx -doctor -p build           # diagnose setup (clang-tidy, LLVM version, compile db, sysroot); exit 0 if ready
```

`-diff` is a non-destructive dry run: it snapshots the affected files, runs
clang-tidy's real `--fix`, renders the unified diff, and restores the originals — so
the preview equals `-fix` byte-for-byte and exits 1 if anything would change (a CI
gate / review preview). `-baseline` records the accepted findings of an existing
codebase so later runs report only regressions.

`-fix-sequential` (with `-fix`) applies each fixable check in its own clang-tidy
pass — one invocation per check — instead of letting a single pass combine every
check's fix-its at once. On dense C++ two independent fix-its can target the same
span and clang-tidy silently drops or garbles the overlap (e.g. adding `noexcept`
and a member-initializer to the same constructor); isolating each check means every
fix-it that *can* apply does, at the cost of extra passes. Reach for it when a plain
`-fix` leaves fixes on the table on heavily-annotated code.

`-j` parallelizes the **analysis** pass across worker processes (default: one per
CPU), each handling a disjoint slice of the translation units — so a large project
scans in a fraction of the wall-clock time. The per-worker results are merged and
sorted, so the output is byte-identical to a sequential (`-j 1`) run regardless of
worker count. An in-place `-fix` always runs as a single pass — parallel workers
rewriting a shared header could race — so `-j` is ignored there; use `-j` for the
reporting / `-json` / `-sarif` / `-diff` / `-baseline` runs that dominate CI.

### Exit codes

For scripting and CI, the process exit code is:

| Code | Meaning |
|------|---------|
| `0`  | clean — no findings; or `-fix` completed (fixes applied, or nothing to apply); or `-baseline` suppressed everything |
| `1`  | findings reported (default mode), `-diff` found pending changes, or new findings appeared past the baseline |
| `2`  | usage or configuration error — a bad flag, an unknown `-explain` id, no `compile_commands.json`, or no translation units matched |

Branch on `1` (found perf issues / would-change) versus `2` (the run itself
failed); a bare `-fix` returns `0` even when advisory-only findings remain, since
its job is to apply fixes, not to gate.

A path arg is a Go-style pattern or directory (`./...`, `./src/game/...`)
expanded against the compilation database to the translation units under it; no
args means `./...`. The `compile_commands.json` is found via `-p` or by walking
up from the current directory.

### CMake projects (opt-in auto-setup)

perfscanxx needs a `compile_commands.json`. For a CMake project you can let it
create one — and, if needed, generate build-time headers — instead of running
CMake by hand:

```bash
perfscanxx -cmake ./...        # configure the detected CMake project -> compile_commands.json
perfscanxx -cmake-build ./...  # also run `cmake --build` (incremental) to generate headers
```

`-cmake`/`-cmake-build` are opt-in because they execute the project's build
system — only use them on trusted code. Discovery also checks common build
subdirs (`build/`, `out/`, `cmake-build-*`), so an existing build is reused. If
translation units fail on headers that are generated at build time, perfscanxx
points you at `-cmake-build`.

See [examples/](examples/) for an end-to-end sample and a recipe for running
against a real C++ codebase (DDNet), plus [examples/validation.md](examples/validation.md)
for validation results on fmt, spdlog, leveldb and abseil.
