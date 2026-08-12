# perfscan++ (`perfscanxx`) — Design Document

**Architecture A: a Go CLI that orchestrates clang-tidy to bring the perfscan
model — stable check IDs, graded fix levels, baseline/ratchet, curated
benchmark-verified catalog — to C++.**

Status: prototype design, 2026-08. Companion to the Go linter
[`jxsl13/perfscan`](../README.md); read that README first for the philosophy
this document deliberately mirrors.

---

## 1. Goals and the IR-vs-AST rationale

### Goals

1. **perfscan UX for C++.** One `-level` knob that gates both reporting and
   fixing; stable IDs; `-list`/`-explain` documentation; text/JSON/SARIF
   output; a line-independent baseline for incremental adoption; YAML config
   auto-discovery.
2. **A curated catalog, not a firehose.** clang-tidy ships dozens of
   `performance-*`, `modernize-*` and `bugprone-*` checks of wildly varying
   signal. perfscan++ ships a *curated* subset where each entry has been
   benchmark-verified (Before/After pair, same policy as perfscan) and
   assigned a fix level. "Every finding is a candidate, not a verdict"
   carries over unchanged.
3. **Real fixes.** `-fix` must produce compilable edits at source level,
   including through the preprocessor, because that is where C++ programmers
   live.
4. **Extensible, zero compiled C++.** New perf checks that clang-tidy lacks
   (the PS-catalog analogues) are declarative clang-query matchers run via
   `--experimental-custom-checks` (§6) — the Go side treats them identically to
   upstream checks; nothing is compiled or linked against LLVM.

### Why clang-tidy and not LLVM IR

The naive port of "perfscan but for C++" reaches for LLVM IR because that is
the famous compiler substrate. It is the wrong layer for a *linter*:

| Property needed | LLVM IR | Clang AST (clang-tidy) |
|---|---|---|
| Source structure (loops, range-for, lambdas, templates as written) | **Lowered away** — loops are branches between basic blocks, copies are memcpys, `std::string` is opaque calls | Preserved; matchers see `cxxForRangeStmt`, `cxxConstructExpr`, template instantiations *and* their spelling |
| Macros / preprocessor | Gone entirely | `SourceManager` tracks spelling vs. expansion locations; checks can refuse to fix into macros |
| Source-accurate fix-its | **Impossible** — IR has no reliable mapping back to editable source text | Native: `FixItHint` → `Replacement{FilePath, Offset, Length, ReplacementText}` |
| Semantic types (`is this type expensive to copy?`) | Erased to layout | Full `QualType`, copy-ctor triviality, `AllowedTypes` heuristics |
| Existing perf checks | none | `performance-*` module: for-range-copy, unnecessary-value-param, inefficient-vector-operation, … each with fix-its |

The correct analogy is: **perfscan is to `go/analysis` what perfscan++ is to
clang-tidy.** perfscan did not re-implement a Go parser; it wrapped
`analysis.Analyzer` in a metadata layer (ID, category, Doc, Level, AutoFix)
and built the UX around it. clang-tidy already *is* the C++
`analysis.Analyzer` runner — an AST-matcher engine with a real C++ frontend,
a check registry, per-check options, and a fix-it/replacement pipeline
(`-export-fixes`, `clang-apply-replacements`). Rebuilding any of that in Go
(or on IR) would be years of work to reach a worse result. So perfscan++ is
a **thin, opinionated orchestrator**: Go owns policy, catalog, grading,
config, baseline and output formats; clang-tidy owns parsing, matching and
edits.

What we explicitly give up by not owning the frontend: we cannot see
profile data or codegen (no "this loop didn't vectorize" — that would be
`-Rpass=loop-vectorize` territory, noted as future work in §8), and we are
coupled to clang-tidy's diagnostic granularity.

## 2. End-to-end data flow

```
                ┌──────────────────────────────────────────────────────────┐
                │                       perfscanxx (Go)                    │
                └──────────────────────────────────────────────────────────┘
 perfscanxx.yaml ─┐
 CLI flags ───────┤ 1. resolve config     2. discover           3. plan
                  └► effective settings ─► compile_commands.json ─► TU list ×
                        (checks, level,      (walk up from cwd /     curated
                         header-filter)       -p flag / config)      -checks set
                                                                      │
                ┌─────────────────────────────────────────────────────┘
                ▼
        4. invoke clang-tidy (per TU, parallel, N workers)
           clang-tidy -p <build-dir> \
             --checks='-*,performance-for-range-copy,...' \
             --header-filter=<regex> \
             --export-fixes=<tmpdir>/fixes-<n>.yaml \
             --quiet <file.cpp>
                │
                ▼
        5. parse exported YAML (schema §2.2) + stderr diagnostics
                │
                ▼
        6. map to perfscan diagnostics
           tidy check name ─catalog─► PSX id, Level, Doc
           byte FileOffset ─────────► file:line:col
           gate by -level, apply //perfscanxx:ignore suppressions,
           subtract baseline
                │
                ├──► 7a. report: text | -json | -sarif   (exit 1 on findings)
                │
                └──► 7b. -fix: filter Replacements to gated findings,
                     rewrite filtered fixes YAML into <tmpdir>/apply/,
                     clang-apply-replacements [-format] <tmpdir>/apply
```

Step details:

**(2) Compile-database discovery.** clang-tidy needs compile flags per TU.
Resolution order: `-p <dir>` flag → `compile_db` in config → walk from each
input path upward looking for `compile_commands.json` (mirrors clang-tidy's
own parent-directory search). If none is found: hard error with actionable
help (`cmake -DCMAKE_EXPORT_COMPILE_COMMANDS=ON`, `bear -- make`,
`compiledb`), except for single-file "flagless" mode (`--` passthrough,
same escape hatch clang-tidy offers).

**(3) TU planning.** Positional args are files/dirs (default `.`). Dirs are
expanded against the compile database entries (we lint the *intersection*:
files under the requested paths that have compile commands). Headers are not
TUs; they are covered via `--header-filter` on the TUs that include them —
this is the standard clang-tidy model and we document it prominently because
it surprises everyone.

**(4) Invocation.** We run `clang-tidy` directly per TU with a worker pool
(GOMAXPROCS workers by default, `-j` to override) rather than shelling to
`run-clang-tidy.py`: we need per-TU fixes files, our own progress/output,
and no Python dependency. This is exactly what `run-clang-tidy.py` does
internally (parallel per-TU invocations, each with `-export-fixes` to a
temp file, merged afterwards), so we are reimplementing a small, stable
contract, not private behavior. `--quiet` suppresses the "N warnings
generated" chatter; we parse structured data from the YAML, using stderr
only for error passthrough (`Level: Error` entries also appear in the YAML).

**(5) Exported-fixes YAML.** Verified schema (LLVM ≥ 7, stable since the
CLion-integration patch D34404 added message/location fields):

```yaml
---
MainSourceFile: '/abs/path/foo.cpp'
Diagnostics:
  - DiagnosticName: performance-for-range-copy
    DiagnosticMessage:
      Message: 'loop variable is copied but only used as const reference; ...'
      FilePath: '/abs/path/foo.cpp'
      FileOffset: 312          # byte offset into FilePath
      Replacements:            # may be [] — diagnostic without a fix
        - FilePath: '/abs/path/foo.cpp'
          Offset: 312          # byte offset
          Length: 0            # 0 = pure insertion
          ReplacementText: 'const '
    Level: Warning             # Warning | Error | (Remark)
    BuildDirectory: '/abs/path/build'
    Notes: []                  # optional attached notes
...
```

Newer clang-tidy nests `Replacements` under `DiagnosticMessage` (as shown);
LLVM < 9 emitted it as a sibling of `DiagnosticMessage`. The parser accepts
both (`internal/tidyfixes`). `FileOffset` is a **byte** offset; we convert
to line:col by reading the file and counting (cached per file). Important
caveat we design around: `--export-fixes` output has *not* had
`cleanupAroundReplacements`/formatting applied, so applied fixes can differ
cosmetically from `clang-tidy --fix` — we pass `-format` (style from
`.clang-format`) to `clang-apply-replacements` to compensate.

**(6) Mapping to perfscan diagnostics.** The catalog (§3) maps each curated
tidy check to a stable perfscan++ ID (`PSX` prefix, e.g. `PSX2001` ←
`performance-inefficient-vector-operation`), Level, category and Doc.
Diagnostics from non-curated checks (possible when the user globs) get a
synthetic passthrough entry at L3/advisory. Output shape mirrors perfscan:

```
src/kernel.cpp:41:18: loop variable is copied each iteration; take a const reference (PSX1001 L1) [performance-for-range-copy]
```

Suppression: `//perfscanxx:ignore PSX1001 reason...` on the finding's line
or the line above (also accepts the underlying tidy name). We implement this
in Go by reading the source line — we do *not* rewrite to `// NOLINT`,
though `NOLINT`/`NOLINTNEXTLINE` in existing code is honored for free
because clang-tidy filters those before export.

Baseline: identical semantics to perfscan — identity is
`{file, checkID, message}` with counts, line-independent, stored as YAML;
`-write-baseline` accepts today's findings, subsequent runs exit 1 only on
new ones.

**(7b) Fix application.** We never run `clang-tidy --fix` directly (it
would apply *all* fixes for enabled checks, bypassing level gating and
baseline). Instead: take the parsed diagnostics that survived gating, write
them back out as valid export-fixes YAML files into a fresh directory, and
run `clang-apply-replacements -format <dir>`. `clang-apply-replacements` is
the upstream tool built exactly for this (dedups identical replacements
across TUs — the same header fix exported N times collapses to one edit —
and detects genuine conflicts, discarding conflicting files with a
diagnostic). This gives us perfscan's "you fix exactly what you see"
invariant on top of clang-tidy's own merge machinery.

## 3. The fix-level model on clang-tidy semantics

perfscan's `lint.Level` carries over verbatim (L1 idiomatic / L2 structured
/ L3 aggressive: the *reader cost* of the remedy). What changes is where the
metadata lives: clang-tidy checks have no level, so **the level is assigned
in the perfscan++ catalog**, per check (occasionally refined per-diagnostic
by message pattern, e.g. `performance-unnecessary-value-param`'s
pass-by-ref fix vs. its std::move fix).

Initial curated catalog (v0 — every row requires a Before/After benchmark
pair under `perfscanxx/benchmarks/` before it ships, per project policy):

| PSX ID | clang-tidy check | Level | Fix-it | Rationale for level |
|---|---|---|---|---|
| PSX1001 | performance-for-range-copy | L1 | yes | `const auto&` — reviewer waves through |
| PSX1002 | performance-unnecessary-copy-initialization | L1 | yes | local copy → const ref |
| PSX1003 | performance-faster-string-find | L1 | yes | `find("x")` → `find('x')` |
| PSX1004 | performance-inefficient-string-concatenation | L2 | no | restructure to `+=`/append chain |
| PSX2001 | performance-inefficient-vector-operation | L1 | yes | insert `reserve(n)` — perfscan PS2101's sibling |
| PSX2002 | performance-unnecessary-value-param | **L2** | yes | fix-it edits a *function signature*: correct but cross-TU-visible; breaks other TUs if the function is redeclared — exactly the "changes the shape" criterion |
| PSX2003 | performance-move-const-arg | L1 | yes | delete a no-op `std::move` |
| PSX2004 | performance-inefficient-algorithm | L1 | yes | `std::find(set)` → `set.find` |
| PSX3001 | performance-implicit-conversion-in-loop | L2 | no | loop variable type change |
| PSX3002 | performance-avoid-endl | L1 | yes | `std::endl` → `'\n'` |
| PSX3003 | performance-noexcept-move-constructor | L2 | yes | API contract change (noexcept) |
| PSX5001 | modernize-use-emplace | L2 | yes | curated import from outside `performance-*`; changes construction semantics (explicit ctors, aggregate rules) |

L3 is reserved for the most aggressive rewrites — ABI/contract-affecting or deep
restructurings — plus passthrough of un-curated checks. As in perfscan, an L3
check without a provably-safe fix stays advisory.

perfscan concepts and their fate:

| perfscan concept | perfscan++ realization |
|---|---|
| `lint.Check` (Analyzer + metadata) | `catalog.Check{ID, TidyName, Level, Category, Doc, HasFixIt, Options}` — no analyzer func; the "analyzer" is the tidy check name |
| `Documentation{Title, Before, After, Why}` | identical struct; `-explain PSX2002` renders it plus a link to the upstream check doc |
| `AutoFix` (SuggestedFixes) | `HasFixIt` + the exported Replacements; gating logic identical |
| Check registry, stable IDs, IDs never reused | identical (PSX prefix; upstream renames tracked in catalog, ID stays) |
| Baseline (`{file, check, message}` + counts) | identical, shared wire format |
| `//perfscan:ignore` | `//perfscanxx:ignore` + upstream `NOLINT` honored |
| Domain vocabulary config | per-check `CheckOptions` passthrough (e.g. `AllowedTypes` for intrusive-refcount types) — same "engine generic, project specifics in config" rule |

## 4. YAML config schema

Auto-discovered as `perfscanxx.yaml` walking up from cwd (stops at repo
root), or `-config file.yaml`. YAML-first per project policy.

```yaml
# perfscanxx.yaml
version: 1

# Compile database directory (contains compile_commands.json).
# Relative to this file. Overridden by -p.
compile_db: build/

# Level gate: report+fix L<=N. Overridden by -level.
level: 2

# Check selection, perfscan glob syntax over PSX ids AND tidy names.
# Overridden by -checks.
checks:
  - PSX1*            # enable a family
  - PSX2001
  - "-PSX2002"       # signature-editing check off by default here
  - performance-avoid-endl   # tidy names accepted, mapped through catalog

# Passed to clang-tidy --header-filter (regex). Default: project dirs only,
# derived from compile-db paths, NOT '.*' (avoid third-party noise).
header_filter: '^(src|include)/'

# Never analyze/fix these paths even if in the compile db.
exclude:
  - 'third_party/.*'
  - '.*\.pb\.(h|cc)$'

# Per-check clang-tidy options, passed via --config on the command line.
check_options:
  performance-unnecessary-value-param:
    AllowedTypes: '[Rr]ef(erence)?$;StatusOr'
  performance-inefficient-vector-operation:
    VectorLikeClasses: '::std::vector;::absl::InlinedVector'

# Query-based custom checks (§6) are built into the catalog — no config needed;
# they run via clang-tidy --experimental-custom-checks (zero compiled C++).

baseline: perfscanxx-baseline.yaml   # optional; also -baseline flag

clang_tidy:
  binary: ""            # override; else $PERFSCANXX_CLANG_TIDY, else PATH
                        # probe incl. brew paths (§7)
  min_version: 17       # refuse older with a clear message
  extra_args: []        # e.g. ["--extra-arg=-std=c++20"]
  jobs: 0               # 0 = GOMAXPROCS
```

Precedence: CLI flag > config file > built-in default — same one-knob
philosophy as perfscan (`-fix` follows `-level`; there is no separate
"fix-level").

## 5. CLI surface

Mirrors perfscan exactly; a perfscan user must feel at home:

```
perfscanxx [flags] [path ...]                # default path: .

perfscanxx ./src                             # report findings (exit 1 if any)
perfscanxx -checks 'PSX2*' ./src             # one family
perfscanxx -checks 'all,-PSX2002' ./src      # everything except one
perfscanxx -level 1 ./src                    # only L1 findings
perfscanxx -level 1 -fix ./src               # report + apply exactly L1 fixes
perfscanxx -fix ./src                        # default -level 3: all fixable
perfscanxx -json ./src                       # machine-readable (editor quick-fix)
perfscanxx -sarif ./src                      # SARIF 2.1.0 (GitHub Code Scanning)
perfscanxx -baseline b.yaml -write-baseline ./src   # adopt: accept today
perfscanxx -baseline b.yaml ./src            # exit 1 only on NEW findings
perfscanxx -list                             # check table: ID, tidy name, L, fix, title
perfscanxx -explain PSX2002                  # full Doc: Before/After, why, caveats
perfscanxx -p build/ ./src                   # compile-db dir (clang-tidy -p)
perfscanxx -config perfscanxx.yaml ./src
perfscanxx -j 8 ./src                        # worker count
perfscanxx -version                          # own version + resolved clang-tidy version
```

Additions beyond perfscan, justified by the C++ substrate: `-p`
(compile-db), `-j` (TU-parallel), `-doctor` (environment probe: clang-tidy
found? version? compile db found? — prints remediation
steps, exit 0/1). Findings print as `file:line:col: message (PSXid Ln)
[tidy-name]` so standard problem-matchers work; `-json` emits one object
per finding including the raw Replacements so editors can offer quick-fixes
without re-running.

Exit codes: 0 clean, 1 findings (post-baseline), 2 usage/config error,
3 environment error (no clang-tidy, no compile db).

## 6. Custom checks: query-based, ZERO compiled C++

New perf checks that clang-tidy lacks (the PS-catalog analogues — e.g.
reserve-before-loop, pessimizing `return std::move(local)`, catch-by-value) are
NOT written as compiled C++. Per the **minimal-C++ constraint**, they are
declarative **clang-query matcher strings** run via clang-tidy's
`--experimental-custom-checks` ([QueryBasedCustomChecks](https://clang.llvm.org/extra/clang-tidy/QueryBasedCustomChecks.html),
LLVM ≥ 20). Each catalog entry marked `Custom` carries a `match …` matcher, a
bound node name, and a diagnostic message; the orchestrator writes a temporary
`.clang-tidy` (a `CustomChecks:` block) and passes
`--experimental-custom-checks --config-file=…`. They surface as `custom-<name>`
tidy-names, are mapped to PX IDs like any built-in check, and flow through the
*identical* report pipeline. Zero special cases beyond emitting that config.

Consequence: `--experimental-custom-checks` is **diagnose-only** — custom checks
emit NO fix-its, so they are always **advisory** (no `-fix`/`-diff` applies to them).
This is the C++ analog of perfscan's advisory checks. Current ones: **PX2101**
reserve-before-loop, **PX2102** pessimizing-move, **PX2103** catch-by-value, **PX2104**
regex-in-loop.

The only C++ perfscanxx touches is the prebuilt `clang-tidy` binary — there is **no
compiled plugin, no `--load`, no LLVM link**, and therefore none of the plugin-ABI
version-pinning a compiled module would need. (An earlier design used an out-of-tree
compiled clang-tidy plugin module; it was removed in favor of this zero-C++ query
mechanism — the Go build never links LLVM.)

## 7. Dependencies and graceful degradation

Runtime (not build-time) deps:

- **clang-tidy** ≥ `min_version` (default 17). macOS: `brew install llvm`
  (keg-only → binary at `$(brew --prefix llvm)/bin/clang-tidy`). Linux:
  distro `clang-tidy-N` packages or apt.llvm.org.
- **clang-apply-replacements** (same package) — only needed for `-fix`.
- A **compile_commands.json** for the target project.

Binary resolution order: config `clang_tidy.binary` → `$PERFSCANXX_CLANG_TIDY`
→ `clang-tidy` on PATH → probe list: `$(brew --prefix llvm)/bin/clang-tidy`,
`/opt/homebrew/opt/llvm/bin/`, `/usr/local/opt/llvm/bin/`, versioned names
`clang-tidy-{20..17}`.

**Graceful degradation is a build requirement**: this machine does not have
clang-tidy installed, so the Go module must compile and its unit tests must
pass with no LLVM present. Enforced by structure:

- `internal/tidyrunner` isolates all exec; it is constructed from an
  interface (`Runner`) so tests use a fake that replays canned
  export-fixes YAML fixtures (`internal/tidyfixes/testdata/*.yaml`,
  including both Replacements nestings and `Level: Error` entries).
- YAML parsing, catalog, gating, baseline, suppression, SARIF/JSON
  rendering, and the fixes-YAML re-emitter are pure Go, fully unit-tested
  offline.
- At runtime without clang-tidy: `-list`, `-explain`, `-doctor`, config
  validation and baseline inspection all work; analysis commands exit 3
  with the brew/apt instructions. `-fix` without `clang-apply-replacements`
  degrades to reporting plus "would apply N edits" and exits 3.

## 8. Open questions and risks

1. **Compile-db requirement is a real adoption wall.** Go needed nothing;
   C++ needs a working build with exported commands. Header-only or
   non-CMake projects suffer. Mitigations: `-doctor` guidance, `bear`
   suggestion, single-file `--` mode. Open: ship a degraded
   "syntax-only best effort" mode (`--extra-arg` std guess, no compile db)?
   Leaning no for v0 — false positives from wrong flags poison trust.
2. **clang-tidy version pinning.** Check names, fix-it quality and YAML
   details drift across LLVM releases (e.g. `performance-avoid-endl` only
   exists ≥ 17; the proposed `--export-fixes`→`--export-diagnostics`
   rename). The catalog therefore records `since`/`until` LLVM versions per
   check; unknown-to-this-version checks are skipped with a note, not an
   error. There is no plugin-ABI concern: custom checks are query strings, not a
   compiled module (§6), so they are tolerant across LLVM versions — perfscanxx
   just needs a clang-tidy new enough for `--experimental-custom-checks` (≥ 20).
3. **Fix-it conflicts.** Overlapping replacements across checks or TUs
   (same header fixed via many TUs; two checks editing one expression).
   `clang-apply-replacements` dedups identical edits and *discards* files
   with genuine conflicts. Risk: a discarded conflict silently drops a fix
   the user saw reported. Mitigation: parse clang-apply-replacements
   stderr, re-report dropped fixes as "conflicted — rerun after applying",
   and document iterative `-fix` convergence. Open: implement our own
   conflict resolution (apply non-overlapping subset per file) in Go later.
4. **Signature-changing fixes** (`performance-unnecessary-value-param`) can
   break *other* TUs — clang-tidy's own docs warn about this. We gate it at
   L2 and only apply when the run covered the whole compile db (fix-all
   scope check); partial-path `-fix` demotes it to advisory. Needs
   validation on the corpus.
5. **Export-fixes formatting gap.** Exported replacements skip
   `cleanupAroundReplacements`/format, so `-fix` output may differ from
   `clang-tidy --fix` (e.g. leftover indentation). `-format` on
   clang-apply-replacements covers most of it; corpus-diff both paths to
   quantify the residue.
6. **Byte offsets vs. encodings.** `FileOffset`/`Offset` are byte offsets;
   fine for UTF-8, but SARIF wants UTF-16-ish columns for some consumers.
   Decide column convention (bytes, documented) and stick to it.
7. **Benchmark policy for curated checks.** Go had `testing.B`; C++ needs a
   harness. Plan: google/benchmark pairs under `perfscanxx/benchmarks/`,
   built only in CI. Open: how much of the perfscan "Before/After per rule"
   policy is enforceable pre-1.0 without slowing curation to a crawl.
8. **Future: hotness.** Neither AST nor IR knows what is hot. Post-v0 ideas:
   ingest `-Rpass-missed=loop-vectorize` remarks or perf/PGO profiles to
   rank findings — this is where an IR/remark side-channel *does* earn its
   place, as ranking input, never as the source of findings.

## Appendix: verified external references

- clang-tidy CLI (`-checks`, `-p`, `--header-filter`, `--export-fixes`,
  `--config-file`, `--list-checks`, `--fix`):
  https://clang.llvm.org/extra/clang-tidy/index.html
- Export-fixes YAML shape (MainSourceFile / Diagnostics / DiagnosticMessage
  {Message, FilePath, FileOffset} / Replacements {FilePath, Offset, Length,
  ReplacementText} / Level / BuildDirectory), and clang-apply-replacements
  usage on a directory of YAML files:
  https://discourse.llvm.org/t/how-to-run-clang-apply-replacements-on-change-set-generated-by-clang-tidy-export-fixes/4757 ;
  message/location fields history: https://reviews.llvm.org/D34404
- Export-vs-fix formatting/cleanup divergence:
  https://github.com/llvm/llvm-project/issues/55569 ,
  https://github.com/llvm/llvm-project/issues/54603
- performance-* check semantics (for-range-copy, unnecessary-value-param
  incl. signature-break caveat and AllowedTypes,
  inefficient-vector-operation incl. reserve insertion):
  https://clang.llvm.org/extra/clang-tidy/checks/list.html
- Query-based custom checks (`--experimental-custom-checks`, the zero-C++
  mechanism §6 uses instead of a compiled plugin):
  https://clang.llvm.org/extra/clang-tidy/QueryBasedCustomChecks.html
- run-clang-tidy parallel model (per-TU export-fixes, merged):
  https://reviews.llvm.org/D31326
