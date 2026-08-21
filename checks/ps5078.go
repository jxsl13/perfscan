package checks

import (
	"fmt"
	"go/ast"
	"go/types"
	"unicode"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5078 collapses strings/bytes Trim* calls whose constant cutsets contain
// only Unicode whitespace when they are composed with TrimSpace.
var PS5078 = register(&lint.Check{
	ID:       "PS5078",
	Category: "arith",
	Slug:     "trimspace-absorbs-whitespace-trim",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "TrimSpace is composed with a redundant constant whitespace-only Trim operation",
		Text: `strings.TrimSpace and bytes.TrimSpace already remove every leading
and trailing rune classified by unicode.IsSpace. A neighboring Trim, TrimLeft,
or TrimRight whose compile-time cutset contains only those runes is therefore
absorbed in either order:

  strings.TrimSpace(strings.Trim(s, " \t")) -> strings.TrimSpace(s)
  strings.Trim(strings.TrimSpace(s), " \t") -> strings.TrimSpace(s)

The rule handles strings and bytes Trim/TrimLeft/TrimRight, empty cutsets,
Unicode whitespace, and arbitrarily deep whitespace-only Trim chains on either
side of TrimSpace. It removes the maximal adjacent chain in one fix. Dynamic
cutsets and any cutset containing a non-space rune—including invalid UTF-8's
RuneError—are excluded.

The rewrite is BIT-IDENTICAL. In the first order, trimming a subset of the
whitespace boundary before TrimSpace cannot change the final boundary. In the
second, TrimSpace has already removed every rune the narrower operation could
remove. For bytes, sequential subslicing and direct TrimSpace finish at the
same backing-array offsets, preserving nil state, start pointer, length, and
capacity; no bytes are copied or mutated. The original value expression is
still evaluated once.

Every call is resolved through go/types and must use the same concrete stdlib
package binding. Aliases work; shadowed helpers, methods, dot imports,
cross-package compositions, dynamic/non-space cutsets, and ellipsis calls do
not match. The shared deletion-liveness guard withholds a fix that would orphan
an import or function-local constant, and comments in deleted scaffolding are
also preserved by keeping the finding advisory.

There is no locale or ASCII assumption: cutsets are validated rune-by-rune
with the same unicode.IsSpace predicate documented by TrimSpace.`,
		Before: `clean := strings.TrimSpace(strings.Trim(payload, " \t\r\n"))`,
		After:  `clean := strings.TrimSpace(payload)`,
		MeasuredWin: `BenchmarkPS5078 trims an already-stable short string with
TrimSpace outside a full Unicode-whitespace constant Trim (Apple M2 Pro,
go1.26; five runs): composed calls median 15.58 ns/op, 0 B/op, 0 allocs/op ->
TrimSpace alone median 2.360 ns/op, 0 B/op, 0 allocs/op (~6.60x, -84.85%). The
rewrite removes cutset setup and two redundant boundary membership scans.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5078",
		Doc:  "TrimSpace composed with a whitespace-only constant Trim operation",
		Run:  runPS5078,
	},
})

var ps5078TrimNames = map[string]bool{"Trim": true, "TrimLeft": true, "TrimRight": true}

type ps5078Match struct {
	outer   *ast.CallExpr
	calls   []*ast.CallExpr
	spans   []tokenSpan
	pkgPath string
	layers  int
	order   string
}

func runPS5078(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		covered := make(map[*ast.CallExpr]bool)
		ast.Inspect(file, func(node ast.Node) bool {
			outer, ok := node.(*ast.CallExpr)
			if !ok || covered[outer] {
				return true
			}
			matched, ok := ps5078MatchChain(pass, outer)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     matched.outer.Pos(),
				End:     matched.outer.End(),
				Message: fmt.Sprintf("%s.TrimSpace %s %d adjacent constant whitespace-only Trim layer(s); TrimSpace already removes their entire boundary set", matched.pkgPath, matched.order, matched.layers),
			}
			if ps5078Fixable(pass, file, matched) {
				edits := make([]analysis.TextEdit, len(matched.spans))
				for i, span := range matched.spans {
					edits[i] = analysis.TextEdit{Pos: span.start, End: span.end}
				}
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "remove whitespace-only Trim scaffolding",
					TextEdits: edits,
				}}
			}
			pass.Report(diagnostic)
			for _, call := range matched.calls {
				covered[call] = true
			}
			return true
		})
	}
	return nil, nil
}

func ps5078MatchChain(pass *analysis.Pass, outer *ast.CallExpr) (ps5078Match, bool) {
	fn, sig, ok := typedCallee(pass, outer.Fun)
	if !ok || sig.Recv() != nil || fn.Pkg() == nil || (fn.Pkg().Path() != "strings" && fn.Pkg().Path() != "bytes") {
		return ps5078Match{}, false
	}
	binding, ok := typedPackageBinding(pass, outer.Fun)
	if !ok {
		return ps5078Match{}, false
	}
	pkgPath := fn.Pkg().Path()

	if fn.Name() == "TrimSpace" && len(outer.Args) == 1 && !outer.Ellipsis.IsValid() {
		first, ok := ps2110Unparen(outer.Args[0]).(*ast.CallExpr)
		if !ok {
			return ps5078Match{}, false
		}
		calls := []*ast.CallExpr{outer}
		current := first
		for ps5078WhitespaceTrim(pass, current, pkgPath, binding) {
			calls = append(calls, current)
			next, ok := ps2110Unparen(current.Args[0]).(*ast.CallExpr)
			if !ok || !ps5078WhitespaceTrim(pass, next, pkgPath, binding) {
				value := current.Args[0]
				return ps5078Match{
					outer: outer, calls: calls, pkgPath: pkgPath,
					layers: len(calls) - 1, order: "subsumes",
					spans: []tokenSpan{
						{start: first.Pos(), end: value.Pos()},
						{start: value.End(), end: first.End()},
					},
				}, true
			}
			current = next
		}
		return ps5078Match{}, false
	}

	if !ps5078WhitespaceTrim(pass, outer, pkgPath, binding) {
		return ps5078Match{}, false
	}
	calls := []*ast.CallExpr{outer}
	current := outer
	for {
		inner, ok := ps2110Unparen(current.Args[0]).(*ast.CallExpr)
		if !ok {
			return ps5078Match{}, false
		}
		if ps5078TrimSpace(pass, inner, pkgPath, binding) {
			calls = append(calls, inner)
			return ps5078Match{
				outer: outer, calls: calls, pkgPath: pkgPath,
				layers: len(calls) - 1, order: "precedes",
				spans: []tokenSpan{
					{start: outer.Pos(), end: inner.Pos()},
					{start: inner.End(), end: outer.End()},
				},
			}, true
		}
		if !ps5078WhitespaceTrim(pass, inner, pkgPath, binding) {
			return ps5078Match{}, false
		}
		calls = append(calls, inner)
		current = inner
	}
}

func ps5078WhitespaceTrim(pass *analysis.Pass, call *ast.CallExpr, pkgPath string, binding *types.PkgName) bool {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() || !ps5078WhitespaceCutset(pass, call.Args[1]) {
		return false
	}
	fn, sig, ok := typedCallee(pass, call.Fun)
	if !ok || sig.Recv() != nil || fn.Pkg() == nil || fn.Pkg().Path() != pkgPath || !ps5078TrimNames[fn.Name()] {
		return false
	}
	innerBinding, ok := typedPackageBinding(pass, call.Fun)
	return ok && innerBinding == binding
}

func ps5078TrimSpace(pass *analysis.Pass, call *ast.CallExpr, pkgPath string, binding *types.PkgName) bool {
	if len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return false
	}
	fn, sig, ok := typedCallee(pass, call.Fun)
	if !ok || sig.Recv() != nil || fn.Pkg() == nil || fn.Pkg().Path() != pkgPath || fn.Name() != "TrimSpace" {
		return false
	}
	innerBinding, ok := typedPackageBinding(pass, call.Fun)
	return ok && innerBinding == binding
}

func ps5078WhitespaceCutset(pass *analysis.Pass, expr ast.Expr) bool {
	cutset, ok := ps5077Cutset(pass, expr)
	if !ok {
		return false
	}
	for _, r := range cutset {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func ps5078Fixable(pass *analysis.Pass, file *ast.File, matched ps5078Match) bool {
	for _, span := range matched.spans {
		if ps2111CommentIn(file, span.start, span.end) {
			return false
		}
	}
	return deletionsKeepRequiredUses(pass, file, matched.spans...)
}
