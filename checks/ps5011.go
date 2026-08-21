package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5011 reports string(bytes.ReplaceAll([]byte(s), []byte(old),
// []byte(new))) — and the n-taking bytes.Replace sibling — with s, old
// and new plain strings: a round-trip through []byte that heap-copies
// every operand and the result. It rewrites to
// strings.ReplaceAll(s, old, new) (strings.Replace(s, old, new, n)),
// the identical byte-substring substitution allocating only the result.
// The prefix/suffix trim twin of this shape is PS5006; the cutset trims
// are PS5005.
var PS5011 = register(&lint.Check{
	ID:       "PS5011",
	Category: "alloc",
	Slug:     "string-bytes-replace-roundtrip",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "string(bytes.ReplaceAll([]byte(s), []byte(old), []byte(new))) heap-copies every operand and the result; strings.ReplaceAll(s, old, new) is the same substitution with only the result allocation",
		Text: `string(bytes.ReplaceAll([]byte(s), []byte(old), []byte(new)))
converts three strings into throwaway byte slices, runs the replacement
into a freshly allocated []byte, and then heap-copies that result back
into a new string. strings.ReplaceAll(s, old, new) performs the
identical substitution reading the operands straight off their string
headers and allocates only the result string. On top of the saved
conversions, the strings form has a strictly better no-match fast path:
when old does not occur in s (or n is zero), strings.Replace returns s
ITSELF — zero allocations — where bytes.Replace must return a fresh
copy of s, which the outer conversion then copies AGAIN. The same holds for the n-taking
bytes.Replace sibling, which rewrites to strings.Replace(s, old, new, n)
with n kept verbatim. Fewer copies and allocations per call, identical
asymptotics.

The rewrite is bit-identical for every input. []byte(x) round-trips
every operand byte-exactly (invalid UTF-8 and NUL bytes included), and
bytes.Replace/ReplaceAll run the same literal byte-substring
search-and-splice algorithm as strings.Replace/ReplaceAll — no rune
decoding for matching, no case folding, no Unicode normalization
anywhere. The two packages agree on every corner: the n semantics
(negative means all, zero means none), the empty-old shape (both count
RUNES via utf8 decoding — with identical treatment of invalid sequences
as RuneError width 1 — and insert new before every rune and at the end),
overlapping-candidate handling (both resume the search after a
consumed match), and the fully-replaced and no-match extremes. So the
produced byte sequence coincides for every (s, old, new, n), and the
value returned is the same string. Each operand is kept byte-verbatim
in place and evaluated exactly once, in the same order, in both forms.
The runtime differential test pins the equivalence for both functions
over exhaustive short operand triples (including invalid UTF-8, NUL and
empty old), targeted seeds, and randomized full-byte-range inputs
crossed with every n regime.

One honest nuance that is NOT observable in value semantics: on the
no-match fast path the rewritten form returns s's own header, keeping
s's backing memory reachable for as long as the result lives, where the
bytes form allocated fresh memory. Strings are immutable, so no Go
program can distinguish the values; in the rare case where the result
must not retain a huge s, follow up with strings.Clone — an explicit
memory-retention decision, not part of this rewrite.

The automatic fix applies only when type information proves the shape:
the outer conversion target is the predeclared string itself (a named
string type or a shadowed identifier does not match), the callee is the
standard library's package-level bytes.Replace or bytes.ReplaceAll (a
shadowed bytes or a same-named method never matches), and ALL THREE
byte-slice argument positions are exactly a predeclared-[]byte
conversion of an operand whose static type is a plain string — a NAMED
string operand in any position is excluded because the strings twin
would not compile for it, and a genuine []byte argument (a variable, or
a []byte{...} literal with no string source) is never rewritten because
it has no string spelling. The n argument of bytes.Replace needs no
constraint: its parameter type is int in both spellings, so it is
passed through verbatim. The operand expressions are kept byte-verbatim
in place; only the punctuation around them is replaced, so the
replacement is a call wherever the original call was legal and never
needs parentheses. The fix edits imports as needed: strings is added
when missing (reusing the file's existing alias when it imports strings
under another name), and the bytes import is dropped when the rewrites
remove the file's last bytes reference. A comment inside the replaced
punctuation, a shadowed or dot/blank strings import at the call site,
or a cgo file (whose import block must never be edited) keeps that
report advisory.`,
		Before: `out := string(bytes.ReplaceAll([]byte(s), []byte(old), []byte(new)))`,
		After:  `out := strings.ReplaceAll(s, old, new)`,
		MeasuredWin: `BenchmarkPS5011 (a 61-byte line with four "//" ->
"/" replacements, Apple M2 Pro, go1.26):
string(bytes.ReplaceAll([]byte(s), []byte(old), []byte(new)))
264 ns/op, 192 B/op, 3 allocs/op -> strings.ReplaceAll(s, old, new)
172 ns/op, 64 B/op, 1 alloc/op (~1.5x faster, a third of the
allocations and bytes: the []byte(s) operand copy, the replaced-result
[]byte and its copy back into a string all disappear — only the result
string remains). In the no-match case the gap is larger still: strings
returns s itself with zero allocations where the bytes form pays two
full copies of s.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5011",
		Doc:  "string(bytes.ReplaceAll([]byte(s), []byte(old), []byte(new))) round-trips three strings and the result through throwaway copies; strings.ReplaceAll(s, old, new) is the identical substitution allocating only the result",
		Run:  runPS5011,
	},
})

func ps5011Msg(fn string) string {
	n := ""
	if fn == "Replace" {
		n = ", n"
	}
	return "string(bytes." + fn + "([]byte(s), []byte(old), []byte(new)" + n + ")) copies all three operands into throwaway byte slices and copies the replaced result into a new string; strings." + fn + "(s, old, new" + n + ") performs the identical substitution allocating only the result — same bytes, fewer copies"
}

func runPS5011(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// site removes one bytes reference and may need the strings
		// import, and both decisions depend on ALL sites together (same
		// per-file site collection as PS5005/PS5006/PS2012).
		type site struct {
			conv *ast.CallExpr
			fn   string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			conv, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m, matched := ps5011Match(pass, conv)
			if !matched {
				return true
			}
			fix := ps5011Fix(pass, f, conv, m)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{conv, m.fn, fix})
			// Keep descending: a nested match can only sit inside the
			// verbatim operand spans, whose edits never overlap this
			// site's punctuation edits.
			return true
		})
		if len(sites) == 0 {
			continue
		}
		stringsImported := false
		for _, imp := range f.Imports {
			if imp.Path.Value == `"strings"` {
				stringsImported = true
				break
			}
		}
		needStrings := fixable > 0 && !stringsImported
		// Each fixable site holds exactly one bytes reference (the
		// qualifier of bytes.Replace/ReplaceAll); when those are all of
		// the file's bytes references, the rewrites orphan the import and
		// the fix must drop it (the runner never prunes imports itself).
		orphansBytes := fixable > 0 && pkgRefCount(pass, f, "bytes") == fixable
		importEdits, importsOK := ps2012ImportEdits(f, needStrings, orphansBytes)
		if !importsOK {
			// cgo file needing import surgery, or a bytes spec we could
			// not locate: keep every report advisory.
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			// All fixes of a run are applied together, so only the first
			// fixable site carries the import edits (same convention as
			// PS5005/PS5006/PS2012).
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.conv.Pos(),
				End:     st.conv.End(),
				Message: ps5011Msg(st.fn),
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5011Match is the matched shape of one site: the three plain-string
// operands unwrapped from their []byte conversions, the verbatim n
// argument (nil for ReplaceAll), and the function name.
type ps5011Site struct {
	s, old, new ast.Expr
	n           ast.Expr
	fn          string
}

// ps5011Match matches conv against
// string(bytes.ReplaceAll([]byte(s), []byte(old), []byte(new))) — or the
// n-taking bytes.Replace sibling — with s, old and new statically plain
// strings: the outer conversion target is the predeclared string itself,
// the callee is the standard library's package-level
// bytes.Replace/ReplaceAll pinned by type information, and ALL THREE
// byte-slice argument positions are exactly a predeclared-[]byte
// conversion of a plain-string operand. The n argument of bytes.Replace
// needs no constraint of its own: its parameter type is int in both
// spellings, so whatever compiled for bytes.Replace compiles unchanged
// for strings.Replace.
func ps5011Match(pass *analysis.Pass, conv *ast.CallExpr) (m ps5011Site, ok bool) {
	if len(conv.Args) != 1 || conv.Ellipsis.IsValid() {
		return m, false
	}
	if !ps2108IsUniverseString(pass, conv.Fun) {
		return m, false
	}
	call, isCall := ps2108Unparen(conv.Args[0]).(*ast.CallExpr)
	if !isCall || call.Ellipsis.IsValid() {
		return m, false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel {
		return m, false
	}
	fnObj, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || fnObj.Pkg() == nil || fnObj.Pkg().Path() != "bytes" {
		return m, false
	}
	switch fnObj.Name() {
	case "ReplaceAll":
		if len(call.Args) != 3 {
			return m, false
		}
	case "Replace":
		if len(call.Args) != 4 {
			return m, false
		}
		m.n = call.Args[3]
	default:
		return m, false
	}
	if sig, isSig := fnObj.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return m, false
	}
	if m.s, ok = ps5006PlainStringOperand(pass, call.Args[0]); !ok {
		return m, false
	}
	if m.old, ok = ps5006PlainStringOperand(pass, call.Args[1]); !ok {
		// A genuine []byte old/new (a variable, or a []byte{...} literal
		// with no string source) has no string spelling — never
		// rewritten, never reported.
		return m, false
	}
	if m.new, ok = ps5006PlainStringOperand(pass, call.Args[2]); !ok {
		return m, false
	}
	m.fn = fnObj.Name()
	return m, true
}

// ps5011Fix builds the strings.Replace/ReplaceAll rewrite for one site,
// or nil when a guard fails and the report must stay advisory. Only the
// punctuation around the kept operands is replaced; the operand
// expressions (and the verbatim n of Replace) stay untouched in place,
// preserving their text and single evaluation (same technique as
// PS5005/PS5006/PS2012). Import edits are appended later, once per file.
func ps5011Fix(pass *analysis.Pass, f *ast.File, conv *ast.CallExpr, m ps5011Site) *analysis.SuggestedFix {
	stringsName, usable := ps2012StringsName(pass, f, conv.Pos())
	if !usable {
		return nil
	}
	// The replaced spans are the punctuation around the kept
	// expressions; a comment there would be silently destroyed —
	// advisory then.
	last := m.new
	if m.n != nil {
		last = m.n
	}
	if ps2111CommentIn(f, conv.Pos(), m.s.Pos()) ||
		ps2111CommentIn(f, m.s.End(), m.old.Pos()) ||
		ps2111CommentIn(f, m.old.End(), m.new.Pos()) ||
		(m.n != nil && ps2111CommentIn(f, m.new.End(), m.n.Pos())) ||
		ps2111CommentIn(f, last.End(), conv.End()) {
		return nil
	}
	edits := []analysis.TextEdit{
		{Pos: conv.Pos(), End: m.s.Pos(), NewText: []byte(stringsName + "." + m.fn + "(")},
		{Pos: m.s.End(), End: m.old.Pos(), NewText: []byte(", ")},
		{Pos: m.old.End(), End: m.new.Pos(), NewText: []byte(", ")},
	}
	if m.n != nil {
		edits = append(edits,
			analysis.TextEdit{Pos: m.new.End(), End: m.n.Pos(), NewText: []byte(", ")},
			analysis.TextEdit{Pos: m.n.End(), End: conv.End(), NewText: []byte(")")},
		)
	} else {
		edits = append(edits, analysis.TextEdit{Pos: m.new.End(), End: conv.End(), NewText: []byte(")")})
	}
	return &analysis.SuggestedFix{
		Message:   "replace with " + stringsName + "." + m.fn + "(...)",
		TextEdits: edits,
	}
}
