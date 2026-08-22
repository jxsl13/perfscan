package checks

import (
	"go/ast"
	"go/types"
	"go/version"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2119 reports a range loop directly over strings/bytes
// Split/SplitAfter/Fields — the call allocates the full result slice just
// to be iterated once; the Go 1.24 SplitSeq/SplitAfterSeq/FieldsSeq
// counterparts yield the same pieces lazily with no slice allocation.
var PS2119 = register(&lint.Check{
	ID:       "PS2119",
	Category: "alloc",
	Slug:     "range-split-to-splitseq",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "ranging over strings.Split allocates the whole result slice; SplitSeq yields the same pieces lazily",
		Text: `strings.Split, strings.SplitAfter and strings.Fields (and their
bytes twins) allocate a slice holding ALL pieces up front. When that
slice's only consumer is a direct range loop, the allocation buys
nothing: the Go 1.24 iterator forms SplitSeq, SplitAfterSeq and
FieldsSeq take the SAME arguments and yield the SAME pieces in the SAME
order lazily, with no backing slice. Early break or return visits the
same prefix; empty-input parity holds (Split("", sep) and SplitSeq("",
sep) both yield one ""; Fields("") and FieldsSeq("") both yield none).

The callee is resolved with type information — only the package-level
strings/bytes function matches, never a shadowed package or a same-named
local. The result slice must feed the range clause DIRECTLY: a slice
stored in a variable first might be indexed, reused or returned, so it
is out of scope. The range KEY must be absent or the blank identifier —
an iterator range is single-value, so a used index blocks the rewrite
entirely (and the check stays silent: without the key guard the two
forms are not interchangeable at all). SplitN/SplitAfterN are
N-limited and have no Seq counterpart; they are never matched.

The fix renames only the selector (Split→SplitSeq, SplitAfter→
SplitAfterSeq, Fields→FieldsSeq) and drops a now-redundant blank key
("_, v :=" becomes "v :="); the arguments, the assignment token and the
loop body are kept verbatim. The Seq functions live in the very package
already imported, so no import editing is needed and the import can
never be orphaned.

The fix is offered for the strings functions ONLY; the bytes twins are
reported advisory (no fix). Strings are immutable, so the eager and
lazy forms cannot be told apart. Byte slices can: bytes.Split pins
every fragment boundary BEFORE the body runs, while bytes.SplitSeq
re-runs Index lazily between yields — a body that writes into the
ranged slice's backing array changes the number and content of later
fragments under the Seq form only. And bytes.Split's FINAL fragment is
the raw tail (its capacity reaches the source's spare capacity) while
SplitSeq cap-clamps every fragment — cap(v) diverges and an append to
the last piece writes into the source buffer under the eager form but
allocates under the lazy one. No body analysis can rule those out
soundly (any call may mutate the slice through an alias), so the bytes
report stays advisory and the human confirms the body is inert.

The report only fires when the file's effective language version is at
least go1.24 — below that the suggested API does not exist and the
advice would be moot.`,
		Before: `for _, part := range strings.Split(s, ",") {
	process(part)
}`,
		After: `for part := range strings.SplitSeq(s, ",") {
	process(part)
}`,
		MeasuredWin: `BenchmarkPS2119 (a ~1.3KB line of 64 comma-separated
~19-byte fields, split and scanned once per op, Apple M2 Pro): Split
765 ns/op, 1152 B/op, 1 alloc/op vs SplitSeq 743 ns/op, 0 B/op,
0 allocs/op — the entire 64-header []string backing array vanishes and
the scan is slightly faster. Fields: 1464 ns/op, 1152 B/op, 1 alloc/op
vs FieldsSeq 1469 ns/op, 0 B/op, 0 allocs/op — same speed,
allocation-free. The win scales with piece count and matters most on
hot parse paths where the per-call slice churns the GC.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2119",
		Doc:  "range over strings/bytes Split/SplitAfter/Fields allocates the result slice; the go1.24 Seq forms don't",
		Run:  runPS2119,
	},
})

// ps2119Seq maps the eager splitter to its lazy go1.24 counterpart and
// pins the exact argument count the eager form must carry (its Seq twin
// has the identical signature). SplitN/SplitAfterN are deliberately
// absent: they are N-limited and have no Seq counterpart.
var ps2119Seq = map[string]struct {
	seq  string
	args int
}{
	"Split":      {"SplitSeq", 2},
	"SplitAfter": {"SplitAfterSeq", 2},
	"Fields":     {"FieldsSeq", 1},
}

func runPS2119(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps2119SeqAvailable(pass, f) {
			// SplitSeq/SplitAfterSeq/FieldsSeq exist only from go1.24 on;
			// below that the advice is moot, so stay silent entirely.
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			rng, ok := n.(*ast.RangeStmt)
			if !ok {
				return true
			}
			// The KEY (index) must be unused: absent or blank. The Seq form
			// is single-value — a used index has no counterpart, so the two
			// forms are not interchangeable and the check stays silent.
			if !ps2119KeyIsBlank(rng.Key) {
				return true
			}
			// The range operand must be DIRECTLY the splitter call: a slice
			// stored in a variable first might be indexed, reused or
			// returned — out of scope.
			call, ok := rng.X.(*ast.CallExpr)
			if !ok || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// Type info pins the callee to the package-level strings/bytes
			// function: a shadowed strings/bytes resolves sel.Sel to some
			// other object, and a method carries a receiver.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Pkg() == nil {
				return true
			}
			pkgPath := fn.Pkg().Path()
			if pkgPath != "strings" && pkgPath != "bytes" {
				return true
			}
			m, ok := ps2119Seq[fn.Name()]
			if !ok || len(call.Args) != m.args {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			funText := exprTextRendered(sel)
			diag := analysis.Diagnostic{
				Pos:     rng.X.Pos(),
				End:     rng.X.End(),
				Message: "ranging directly over " + funText + " allocates the whole result slice per loop entry; " + pkgPath + "." + m.seq + " yields the same pieces in the same order with no slice allocation (go1.24)",
			}
			if pkgPath == "bytes" {
				// ADVISORY ONLY for bytes: the rewrite is NOT bit-identical
				// against a mutable source. bytes.Split pins all fragment
				// boundaries before the body runs; bytes.SplitSeq re-runs
				// Index lazily, so a body write into the ranged slice's
				// backing array changes later fragments under the Seq form
				// only. And genSplit's final fragment is the raw tail
				// (unclamped cap) while SplitSeq clamps every fragment:
				// cap(v) diverges and append on the last piece writes into
				// the source's spare capacity vs allocating. Any call in the
				// body may mutate the slice through an alias, so no sound
				// body guard exists — report, never fix. Pinned by
				// TestEquiv_SplitSeq_BytesAliasingDivergence and the
				// bytesMutatedInBody/bytesCapObserved fixtures.
				diag.Message += "; fix withheld: not bit-identical if the loop body mutates the byte slice or appends to/measures the final piece"
				pass.Report(diag)
				return true
			}
			// Two edits: rename only the selector (args stay verbatim) and
			// drop a now-redundant blank key. A comment inside the deleted
			// "_," span would be destroyed — the fix is withheld then and
			// the report stays advisory.
			edits := []analysis.TextEdit{
				{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte(m.seq)},
			}
			fixable := true
			if rng.Key != nil && rng.Value != nil {
				if ps2111CommentIn(f, rng.Key.Pos(), rng.Value.Pos()) {
					fixable = false
				} else {
					edits = append(edits, analysis.TextEdit{
						Pos: rng.Key.Pos(), End: rng.Value.Pos(),
					})
				}
			}
			if fixable {
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "range over " + pkgPath + "." + m.seq + " instead",
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2119KeyIsBlank reports whether the range key is absent or the blank
// identifier — the only forms under which the single-value Seq range is
// interchangeable with the slice range.
func ps2119KeyIsBlank(key ast.Expr) bool {
	if key == nil {
		return true
	}
	id, ok := key.(*ast.Ident)
	return ok && id.Name == "_"
}

// ps2119SeqAvailable reports whether f's effective language version has
// the SplitSeq/SplitAfterSeq/FieldsSeq iterators (go1.24). The per-file
// version (a //go:build go1.N line moves it, up or down) wins over the
// package's. An unknown or unparseable version does not block the check:
// perfscan itself requires a far newer toolchain, so an empty version
// means "module default", not "ancient" — the same policy as PS3102's
// clear-builtin gate.
func ps2119SeqAvailable(pass *analysis.Pass, f *ast.File) bool {
	v := ""
	if pass.TypesInfo.FileVersions != nil {
		v = pass.TypesInfo.FileVersions[f]
	}
	if v == "" && pass.Pkg != nil {
		v = pass.Pkg.GoVersion()
	}
	if v == "" {
		return true
	}
	lang := version.Lang(v)
	if lang == "" {
		return true
	}
	return version.Compare(lang, "go1.24") >= 0
}
