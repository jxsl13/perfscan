# Authoring perfscan checks

Hard-won invariants for adding a check to `checks/`. perfscan is a **performance**
linter with a **bit-identical** safety bar: an `AutoFix` may only ever produce code
that behaves identically to the original for *every* input. When in doubt, report
without a fix (advisory) — the ~two dozen advisory checks are advisory *by design*.

## 1. The bit-identical bar (the one rule that matters)

`AutoFix: true` is a promise: the rewrite is provably behavior-preserving. Before
attaching a fix, write down *why* the output is identical for all inputs, including
the nasty edges:

- **Empty / boundary inputs** — empty string, empty slice, single element, `nil`.
- **Invalid UTF-8** — `[]rune(s)` and `utf8.RuneCountInString(s)` agree (one rune
  per bad byte); `string(b) == "x"` is compiler-optimized; etc.
- **Floats** — `slices.Sort` on `[]float64` is NOT bit-identical to `sort.Float64s`
  (NaN ordering, ±0.0 ties). Exclude float element types from ordered-sort rewrites.
- **Stability** — for `[]int`/`[]string` a stable and unstable sort produce the same
  slice (equal elements are bitwise identical), so `sort.Stable(sort.IntSlice(x))` →
  `slices.Sort(x)` is safe; in general it is NOT (it flips stable→unstable), which
  is why staticcheck's S1032 withholds that rewrite. Gate on the element type.
- **Wrapping verbs** — `errors.New(fmt.Sprintf(f,...))` → `fmt.Errorf` changes
  behavior if `f` contains `%w`; guard on a literal format with no `%w`.

If the rewrite is only *parity* on the gc compiler (see §6) but a real win on other
toolchains, that is acceptable — but say so honestly in the Doc. If it can change
observable behavior on any input, it is advisory, full stop.

## 2. COMPILE THE REWRITE TARGET (analysistest does not)

`analysistest.RunWithSuggestedFixes` diffs the rewritten *text* against a `.golden`
file — it does **not** type-check or compile the result. A fix that emits a call to
a non-existent API (e.g. `slices.SortStable`, which does not exist — only
`SortStableFunc`) will pass every golden test and still break every codebase it
"fixes". Always prove the fix target compiles:

- add a **Before/After benchmark** in `benchmarks/` that actually *calls* the
  rewritten shape (a compiled package — this is your safety net), and/or
- build a throwaway scratch program using the exact replacement expression.

## 3. Check anatomy — copy a close sibling

`register(&lint.Check{ ID, Category, Slug, Level, AutoFix, Doc{Title,Text,Before,
After,MeasuredWin}, Analyzer })`. Resolve callees via **type info**
(`pass.TypesInfo.Uses[sel.Sel]` → receiver-less `*types.Func` with the expected
`Pkg().Path()`/`Name()`), never by name alone — this rejects shadowed packages and
same-named locals. Closest siblings to mirror:

- string/alloc rewrites & the "→ concat" family: `ps2122.go`, `ps2123.go`, `ps2124.go`.
- `len(...)` / conversion rewrites: `ps2121.go`, `ps2125.go`.
- sort → slices with imports + version gate: `ps3104.go`, `ps3105.go`.
- writer rewrites + import-orphan: `ps2118.go`, `ps2113.go`.

## 4. Emitting the fix

- Render sub-expressions **byte-verbatim** from source (slice `pass.Fset` offsets or
  `format.Node`); never reformat user code inside a fix.
- **Precedence:** replacing a primary expression (a call) with a binary `a + b`
  needs parens in some parents (`x * len(...)`, indexing). Reuse `ps2121NeedsParens`
  (parent-based): no parens in self-delimiting contexts (assign/return RHS, call
  arg, index bracket, composite element), parens otherwise. A single-ident
  replacement never needs parens.
- **A comment inside the scaffolding you delete withholds the fix** (report stays
  advisory) — reuse `ps2111CommentIn`. Never silently drop a comment.

## 5. Imports

- **Add** an import (e.g. `slices`, `unicode/utf8`) with the shared `ps2110`
  import-add helper — it is alias-aware and withholds the fix on dot/blank imports
  or a shadowed qualifier.
- **Remove** an import the rewrite orphans with the `pkgRefCount` guard (see
  `ps2118.go`/`ps3002.go`): collect sites per file, decide import edits **once**,
  drop the import only when the file has no other reference. Count refs correctly —
  a `sort.Sort(sort.IntSlice(x))` rewrite removes **two** `sort.` references per site.

## 6. Go-version gating & the gc peephole

- APIs like `slices.Sort` (1.21) or `strings.SplitSeq` (1.24) require a version gate:
  `pass.TypesInfo.FileVersions[file]` compared with `go/version`, falling back to
  `pass.Pkg.GoVersion()`; treat unknown/empty as the module default (allowed) and
  block only a *known* lower version. Ship a pinned `oldversion.go` negative fixture.
  Note x/tools clamps per-file `//go:build go1.<20` tags up to 1.21.
- cmd/compile already **peepholes** some shapes to zero cost (`len([]rune(s))` →
  `runtime.countrunes`, `len([]byte(s))` → `len(s)`, `sort.Ints` is a `slices.Sort`
  wrapper since go1.22). Such rewrites are gc-**parity**; ship them only with honest
  "the win is robustness / non-gc toolchains / idiom" framing (see PS2125, PS3102,
  PS3104). `sort.Sort`/`sort.Stable` are NOT peepholed — those are real wins.

## 7. Dogfood & tests

- **Dogfood ratchet:** `go run . -level 3 -baseline .perfscan-baseline.yaml ./...`
  must stay green. If your new check flags perfscan's own code, **fix that
  occurrence** (apply your own advice) — do not baseline it. If the fix legitimately
  can't apply in context (an aliasing/import guard withholds it), that report-only
  finding is expected.
- **Tests:** per-file and parallel-safe. `analysistest.RunWithSuggestedFixes` with a
  `.golden`, covering positives (each rewrite form, precedence, import add/keep/remove,
  alias) and negatives (type-guard failures, shadowed pkg, stored-then-used, spread,
  the excluded edge cases). Every benchmarkable rule ships a **Before/After benchmark**
  with real measured numbers — never fabricate them.
- **Regressions** ship a pinned reproduction fixture.

## 8. CI gates (all enforced by the pre-push hook)

`gofmt` (excl. `testdata/`), `go vet`, `go build`, `go test -race`, **staticcheck**,
generated docs up to date (`go run ./gendocs`), the plugin module builds, the
benchmarks compile+run once, and the dogfood ratchet. Green locally via the
`.githooks/pre-push` gate means green in CI.
