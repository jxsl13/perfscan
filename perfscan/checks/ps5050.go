package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5050 reports slices.Index(b, c) and slices.Contains(b, c) where b is a
// byte slice and c a byte: the generic element-by-element scan (an inlined
// loop comparing one byte at a time) where bytes.IndexByte runs the
// architecture's SIMD byte search. slices.Index -> bytes.IndexByte and
// slices.Contains -> bytes.IndexByte(...) >= 0 answer the identical question
// over the identical bytes, an order of magnitude faster on any non-trivial
// slice. The byte-slice specialization of the generic slices scan; the
// membership/precedence handling mirrors PS5014 (bytes.Contains one-byte
// needle) and PS5013.
var PS5050 = register(&lint.Check{
	ID:       "PS5050",
	Category: "arith",
	Slug:     "slices-index-byte-to-bytes-indexbyte",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "slices.Index/Contains over a byte slice runs the generic element scan; bytes.IndexByte runs the SIMD byte search for the identical result",
		Text: `slices.Index(b, c) and slices.Contains(b, c), when b is a []byte and c a
byte, resolve to the generic slices scan: a straight element-by-element loop
comparing one byte per iteration. bytes.IndexByte(b, c) dispatches to the
architecture-optimized byte search (bytealg / SIMD), scanning many bytes per
instruction. slices.Index(b, c) -> bytes.IndexByte(b, c) returns the identical
index (-1 when absent), and slices.Contains(b, c) -> bytes.IndexByte(b, c) >= 0
returns the identical membership bool.

The rewrite is BYTE-IDENTICAL: both scans compare raw bytes with == and return
the first matching index (or -1) — no rune decoding, no case folding, no UTF-8
validation — so index and membership agree for every haystack (empty or nil,
match at either end, repeated matches, NUL bytes, invalid UTF-8), pinned by the
equivalence suite over all 256 byte values crossed with adversarial haystacks.
Both arguments are evaluated once in the original order.

The match is deliberately narrow — it is the whole safety story:
  - the callee is the package-level slices.Index or slices.Contains, pinned by
    type information (a shadowed slices, an aliased import kept verbatim, or a
    same-named method never matches), called with two arguments and no spread;
  - the slice argument's element type is EXACTLY byte (uint8). A []byte or a
    named slice whose underlying type is []byte both qualify (a named byte
    slice is assignable to bytes.IndexByte's []byte parameter, and Index /
    Contains return the unnamed int / bool regardless), while a defined byte
    element type (type Nibble byte; []Nibble) is excluded — []Nibble is not
    assignable to []byte. The byte value carries over verbatim (in
    slices.Index(b, c) it already has byte type, exactly bytes.IndexByte's
    second parameter);
  - bytes must be importable at the site (no import "C" file, no shadow) and,
    because the rewrite drops these slices references, the fix is withheld
    file-wide unless slices retains another use afterward — so it never orphans
    the slices import (that residual case is reported advisory).
For slices.Contains the introduced >= 0 needs the same precedence care as
PS5014: a leading ! is folded into < 0, and an outer comparison / ! operand is
parenthesized; a go/defer position (which requires a call expression) is
advisory. A comment inside the renamed selector withholds the fix.`,
		Before: `i := slices.Index(buf, '\n')
if slices.Contains(line, '=') {`,
		After: `i := bytes.IndexByte(buf, '\n')
if bytes.IndexByte(line, '=') >= 0 {`,
		MeasuredWin: `1 KB haystack, match absent (Apple M2 Pro, go1.26): slices.Index(b, 'z') ` +
			`~311 ns/op -> bytes.IndexByte(b, 'z') ~13 ns/op (~24x); both 0 B/op, 0 allocs/op. ` +
			`The generic element loop scans one byte per iteration; IndexByte scans a machine ` +
			`word at a time — the gap widens with the slice length.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5050",
		Doc:  "slices.Index/Contains over a byte slice instead of bytes.IndexByte",
		Run:  runPS5050,
	},
})

func runPS5050(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		type site struct {
			diag analysis.Diagnostic
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		importAdded := false
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "slices" {
				return true
			}
			isContains := false
			switch fn.Name() {
			case "Index":
			case "Contains":
				isContains = true
			default:
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			// The slice's element type must be exactly byte, so the slice is
			// assignable to bytes.IndexByte's []byte parameter and the scans
			// are byte-wise identical.
			st, ok := pass.TypesInfo.TypeOf(call.Args[0]).Underlying().(*types.Slice)
			if !ok || !types.Identical(st.Elem(), types.Typ[types.Uint8]) {
				return true
			}

			// Precedence bookkeeping for the introduced comparison (Contains
			// only), mirroring PS5014: a direct leading ! is absorbed
			// (!Contains -> IndexByte < 0), and the result is parenthesized
			// when its parent is a comparison operand or another ! (>= and ==
			// share a precedence level; ! binds tighter than >=). A go/defer
			// position requires a call expression, so no comparison fits there.
			op := ">= 0"
			var flip *ast.UnaryExpr
			goDefer := false
			needParens := false
			if isContains {
				parentIdx := len(stack) - 1
				if parentIdx >= 0 {
					if u, isUnary := stack[parentIdx].(*ast.UnaryExpr); isUnary && u.Op == token.NOT {
						flip = u
						op = "< 0"
						parentIdx--
					}
				}
				if parentIdx >= 0 {
					switch p := stack[parentIdx].(type) {
					case *ast.GoStmt:
						goDefer = flip == nil && p.Call == call
					case *ast.DeferStmt:
						goDefer = flip == nil && p.Call == call
					case *ast.BinaryExpr:
						needParens = p.Op != token.LAND && p.Op != token.LOR
					case *ast.UnaryExpr:
						needParens = true
					}
				}
			}

			name := fn.Name()
			after := "bytes.IndexByte(...)"
			if isContains {
				after += " " + op
				if needParens {
					after = "(" + after + ")"
				}
			}
			msg := "slices." + name + " over a byte slice runs the generic element scan; " +
				after + " runs the SIMD bytes.IndexByte search for the identical result"

			diagPos, diagEnd := call.Pos(), call.End()
			if flip != nil {
				diagPos = flip.Pos()
			}
			s := site{diag: analysis.Diagnostic{Pos: diagPos, End: diagEnd, Message: msg}}

			// Advisory-only positions get a message-only report.
			switch {
			case goDefer:
				s.diag.Message = msg + "; a go/defer statement requires a call expression, so the comparison cannot be spliced here — rewrite by hand"
			case ps2111CommentIn(f, sel.Pos(), sel.End()) || (flip != nil && ps2111CommentIn(f, flip.OpPos, call.Pos())):
				s.diag.Message = msg + "; a comment inside the rewritten syntax withholds the automatic fix — rewrite by hand"
			default:
				useName, needImport, usable := ps2107PkgUsable(pass, f, call.Pos(), "bytes", "bytes")
				if usable && !(needImport && ps2107ImportsC(f)) {
					open, closing := "", ""
					if needParens {
						open, closing = "(", ")"
					}
					var edits []analysis.TextEdit
					if flip != nil {
						edits = append(edits, analysis.TextEdit{Pos: flip.OpPos, End: call.Pos(), NewText: []byte(open)})
					} else if open != "" {
						edits = append(edits, analysis.TextEdit{Pos: call.Pos(), End: call.Pos(), NewText: []byte(open)})
					}
					// Rename slices.Index/Contains -> <bytes>.IndexByte, keeping
					// both arguments verbatim.
					edits = append(edits, analysis.TextEdit{Pos: sel.Pos(), End: sel.End(), NewText: []byte(useName + ".IndexByte")})
					if isContains {
						edits = append(edits, analysis.TextEdit{Pos: call.End(), End: call.End(), NewText: []byte(" " + op + closing)})
					}
					if needImport && !importAdded {
						edits = append(edits, ps2107ImportEdit(f, "bytes"))
						importAdded = true
					}
					s.fix = &analysis.SuggestedFix{
						Message:   "replace slices." + name + " with " + useName + ".IndexByte",
						TextEdits: edits,
					}
					fixable++
				}
			}
			sites = append(sites, s)
			return true
		})

		// Withhold every fix in the file unless slices retains a use after the
		// rewrite drops these references — never orphan the slices import.
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "slices") > fixable
		for _, s := range sites {
			d := s.diag
			if emitFixes && s.fix != nil {
				d.SuggestedFixes = []analysis.SuggestedFix{*s.fix}
			}
			pass.Report(d)
		}
	}
	return nil, nil
}
