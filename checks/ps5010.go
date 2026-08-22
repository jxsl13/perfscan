package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5010 reports string(bytes.ToUpper([]byte(s))) — and the
// ToLower/ToTitle/Title siblings — with s a plain string: a case-mapping
// round-trip through []byte that pays two throwaway copies on top of the
// one allocation the mapping itself needs. It rewrites to
// strings.ToUpper(s), which performs the identical case mapping reading
// straight off the string and allocates at most the single result
// string. The structurally identical Trim-family round-trips are
// PS2012/PS5005/PS5006.
var PS5010 = register(&lint.Check{
	ID:       "PS5010",
	Category: "alloc",
	Slug:     "string-bytes-case-roundtrip",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "string(bytes.ToUpper([]byte(s))) pays two throwaway copies; strings.ToUpper(s) is the same case mapping with at most one allocation",
		Text: `string(bytes.ToUpper([]byte(s))) does three allocation-worthy
steps: []byte(s) heap-copies s into a fresh byte slice, bytes.ToUpper
allocates and fills a new result slice, and string(...) heap-copies
that result into yet another string. strings.ToUpper(s) performs the
identical case mapping reading directly off the string header and
allocates at most the single result string — and on its all-ASCII
no-change fast path it returns s itself, allocating NOTHING. The case
mapping is the same work in both spellings; the entire saving is data
movement: up to two extra allocations and two full copies per call
drop away. The same holds for the ToLower and ToTitle siblings, and
for the deprecated Title (whose strings twin is equally deprecated, so
the rewrite does not change the code's deprecation posture — it merely
stops paying for the round-trip until the call is retired).

The rewrite is bit-identical for every input. []byte(s) is a byte-exact
copy of s (invalid UTF-8 included), and each bytes function shares the
IDENTICAL machinery with its strings twin: the same all-ASCII fast
path, the same unicode case tables and special-casing, and rune
decoding via utf8.DecodeRune (bytes) vs DecodeRuneInString (strings),
which agree on every byte sequence — an invalid sequence yields
RuneError with width 1 in both, and BOTH spellings then re-encode that
RuneError as the U+FFFD replacement bytes, so even the invalid-input
mangling coincides byte for byte. The mapping is a pure per-rune
transformation with no normalization and no context sensitivity, so
the output bytes coincide and string(result) == strings.ToUpper(s) for
every input, including the adversarial set (empty, invalid UTF-8, the
German ß, final sigma ς, the Kelvin sign K, dotless ı and dotted İ,
the titlecase digraph ǅ, the ohm sign Ω). The empty input maps to ""
on both sides (string(nil) == ""). s is kept byte-verbatim in place
and evaluated exactly once in both forms. The runtime differential
test pins the equivalence for all four functions over exhaustive short
inputs on an adversarial alphabet, targeted Unicode edge-case seeds,
and randomized inputs biased toward invalid UTF-8.

Unlike the Trim-family round-trips there is no memory-retention nuance
at all: strings.ToUpper either returns a freshly allocated string or
(on the no-change fast path) s itself — it never pins a larger backing
array than the original spelling did.

The automatic fix applies only when type information proves the shape:
the outer conversion target is the predeclared string itself (a named
string type or a shadowed identifier does not match), the callee is
the standard library's package-level bytes.ToUpper, bytes.ToLower,
bytes.ToTitle, or bytes.Title (a shadowed bytes or a same-named method
never matches), the inner conversion target is exactly the predeclared
[]byte (a defined slice type is not matched), and the operand's static
type is a plain string — a NAMED string operand is excluded because
the strings twin would not compile for it. The operand expression is
kept byte-verbatim in place; only the punctuation around it is
replaced, so the replacement is a call wherever the original call was
legal and never needs parentheses. The fix edits imports as needed:
strings is added when missing (reusing the file's existing alias when
it imports strings under another name), and the bytes import is
dropped when the rewrites remove the file's last bytes reference. A
comment inside the replaced punctuation, a shadowed or dot/blank
strings import at the call site, or a cgo file (whose import block
must never be edited) keeps that report advisory. ToUpperSpecial/
ToLowerSpecial/ToTitleSpecial take a unicode.SpecialCase argument and
their strings twins exist too, but they are a different shape and are
deliberately not matched.`,
		Before: `out := string(bytes.ToUpper([]byte(s)))`,
		After:  `out := strings.ToUpper(s)`,
		MeasuredWin: `BenchmarkPS5010 (Apple M2 Pro, go1.26): on a 54-byte
mixed-case input containing ß (the mapping must allocate),
string(bytes.ToUpper([]byte(s))) 199.7 ns/op, 128 B/op, 2 allocs/op ->
strings.ToUpper(s) 161.9 ns/op, 64 B/op, 1 alloc/op (~1.2x, half the
allocations). On the common all-ASCII no-change input the win is
larger: 95.4 ns/op, 128 B/op, 2 allocs/op -> 56.8 ns/op, 0 B/op,
0 allocs/op (~1.7x, allocation-free — strings.ToUpper's fast path
returns s itself). The copy savings grow linearly with len(s).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5010",
		Doc:  "string(bytes.ToUpper([]byte(s))) round-trips s through two throwaway copies; strings.ToUpper(s) performs the identical case mapping with at most one allocation",
		Run:  runPS5010,
	},
})

func ps5010Msg(fn string) string {
	return "string(bytes." + fn + "([]byte(s))) copies s into a throwaway []byte and copies the case-mapped result into a new string; strings." + fn + "(s) performs the identical case mapping with at most one allocation — same bytes, up to two allocations fewer"
}

func runPS5010(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// site removes one bytes reference and may need the strings
		// import, and both decisions depend on ALL sites together (same
		// per-file site collection as PS2012/PS5005).
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
			s, fn, matched := ps5010Match(pass, conv)
			if !matched {
				return true
			}
			fix := ps5010Fix(pass, f, conv, s, fn)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{conv, fn, fix})
			// Keep descending: a nested match can only sit inside the
			// verbatim operand span, whose edits never overlap this
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
		// qualifier of bytes.To*); when those are all of the file's
		// bytes references, the rewrites orphan the import and the fix
		// must drop it (the runner never prunes imports itself).
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
			// PS2012/PS5005).
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
				Message: ps5010Msg(st.fn),
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5010Match matches conv against string(bytes.ToUpper([]byte(s))) —
// or the ToLower/ToTitle/Title siblings — with s statically a plain
// string: the outer conversion target is the predeclared string itself,
// the callee is the standard library's package-level
// bytes.ToUpper/ToLower/ToTitle/Title pinned by type information, the
// inner conversion target is exactly the predeclared []byte, and the
// operand's static type is a plain (possibly untyped-constant) string.
// It returns the operand s and the function name.
func ps5010Match(pass *analysis.Pass, conv *ast.CallExpr) (s ast.Expr, fn string, ok bool) {
	if len(conv.Args) != 1 || conv.Ellipsis.IsValid() {
		return nil, "", false
	}
	if !ps2108IsUniverseString(pass, conv.Fun) {
		return nil, "", false
	}
	call, isCall := ps2108Unparen(conv.Args[0]).(*ast.CallExpr)
	if !isCall || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return nil, "", false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel {
		return nil, "", false
	}
	fnObj, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || fnObj.Pkg() == nil || fnObj.Pkg().Path() != "bytes" {
		return nil, "", false
	}
	switch fnObj.Name() {
	case "ToUpper", "ToLower", "ToTitle", "Title":
	default:
		// ToUpperSpecial/ToLowerSpecial/ToTitleSpecial take a
		// unicode.SpecialCase argument — a different shape, deliberately
		// out of scope.
		return nil, "", false
	}
	if sig, isSig := fnObj.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, "", false
	}
	inner, isConv := ps2108Unparen(call.Args[0]).(*ast.CallExpr)
	if !isConv || len(inner.Args) != 1 || inner.Ellipsis.IsValid() {
		return nil, "", false
	}
	if !ps2108IsByteSliceConv(pass, inner.Fun) {
		return nil, "", false
	}
	arg := inner.Args[0]
	tv, found := pass.TypesInfo.Types[arg]
	if !found || tv.Type == nil {
		return nil, "", false
	}
	basic, isBasic := types.Unalias(tv.Type).(*types.Basic)
	if !isBasic || basic.Info()&types.IsString == 0 {
		// A NAMED string operand is deliberately not matched: the
		// strings twin would not compile for it, and the semantics
		// belong to the named type.
		return nil, "", false
	}
	return arg, fnObj.Name(), true
}

// ps5010Fix builds the strings.To*(s) rewrite for one site, or nil when
// a guard fails and the report must stay advisory. Only the punctuation
// around the operand is replaced; the operand stays untouched in place,
// preserving its text and single evaluation (same technique as
// PS2012/PS5005). Import edits are appended later, once per file.
func ps5010Fix(pass *analysis.Pass, f *ast.File, conv *ast.CallExpr, s ast.Expr, fn string) *analysis.SuggestedFix {
	stringsName, usable := ps2012StringsName(pass, f, conv.Pos())
	if !usable {
		return nil
	}
	// The replaced spans are the punctuation around the kept operand; a
	// comment there would be silently destroyed — advisory then.
	if ps2111CommentIn(f, conv.Pos(), s.Pos()) ||
		ps2111CommentIn(f, s.End(), conv.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + stringsName + "." + fn + "(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: conv.Pos(), End: s.Pos(), NewText: []byte(stringsName + "." + fn + "(")},
			{Pos: s.End(), End: conv.End(), NewText: []byte(")")},
		},
	}
}
