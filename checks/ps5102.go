package checks

import (
	"go/ast"
	"go/constant"
	"go/printer"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS5102 reports w.WriteRune(R) where R is a compile-time constant below
// utf8.RuneSelf (0x80) — a rune that encodes to exactly one byte — on a
// receiver whose method set has WriteByte(byte) error.
var PS5102 = register(&lint.Check{
	ID:       "PS5102",
	Category: "arith",
	Slug:     "writerune-single-byte",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "WriteRune of a single-byte rune runs the UTF-8 encoder; WriteByte is direct",
		Text: `WriteRune accepts any rune and routes it through the UTF-8
encoding path; a constant rune below utf8.RuneSelf (0x80) always encodes to
exactly that one byte, so WriteByte states the intent directly and skips the
rune plumbing. On the standard writers (bytes.Buffer, strings.Builder,
bufio.Writer) the single-byte fast path exists inside WriteRune too, so the
saving is the wrapper itself: the encoder branch and the (int, error) result
construction that WriteByte's plain error result does not carry.

Only a compile-time constant argument whose value the type checker proves to
be in [0, 0x80) is matched — a variable rune, or any constant >= 0x80, would
encode to multiple bytes and is never reported. The receiver's method set
must contain WriteByte(byte) error, verified through the type checker.

The automatic fix rewrites w.WriteRune(C) to w.WriteByte(C) when three
proofs hold: the WriteRune/WriteByte pair is the standard library's own
(bytes.Buffer, strings.Builder, or bufio.Writer — for those, WriteByte is
bit-identical to WriteRune below 0x80: same byte appended, same nil/flush
error behavior); the call's results are discarded (WriteRune returns
(int, error), WriteByte only error, so a used result changes shape); and the
argument stays legal as-is — a rune or int literal is kept verbatim (an
untyped constant below 0x80 is representable in byte), anything else is
wrapped in a byte(...) conversion, which is a constant conversion the
compiler proves in range. A custom type carrying both methods gets an
advisory report only: nothing guarantees its two methods agree. The rewrite
touches no imports.`,
		Before: `buf.WriteRune(',')`,
		After:  `buf.WriteByte(',')`,
		MeasuredWin: `BenchmarkPS5102 (1024 constant ASCII runes into a reset
bytes.Buffer, Apple M2 Pro): 2.46 µs/op -> 2.10 µs/op (~1.17x, ~0.35 ns per
write), zero allocations either way. The win is modest because the std
writers already fast-path runes below 0x80 inside WriteRune — what is
saved is the wrapper itself: the encoder branch and the unused
(int, error) result.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5102",
		Doc:  "WriteRune of a constant single-byte rune instead of WriteByte",
		Run:  runPS5102,
	},
})

func runPS5102(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m := ps5102Match(pass, call)
			if m == nil {
				return true
			}
			msg := "WriteRune of the single-byte constant " + m.argText +
				" takes the rune-encoding path; WriteByte writes the byte directly"
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: msg,
			}
			switch {
			case !m.stdPair:
				// A custom WriteRune/WriteByte pair: nothing proves the two
				// methods agree even below 0x80 — advisory only.
				diag.Message = msg + "; this WriteByte/WriteRune pair is not the standard library's — verify they agree before rewriting"
			case !ps5102ResultDiscarded(call, stack):
				// WriteRune returns (int, error), WriteByte only error: a
				// used result changes the statement's shape — advisory only.
				diag.Message = msg + "; WriteRune returns (int, error) but WriteByte returns only error — adjust the result handling by hand"
			default:
				// Rewrite the selected name; the argument is kept verbatim
				// when it is a bare rune/int literal (an untyped constant
				// below 0x80 is representable in byte), otherwise wrapped in
				// a byte(...) conversion around its original source range —
				// same expression, same evaluation, provably in range.
				edits := []analysis.TextEdit{
					{Pos: m.sel.Sel.Pos(), End: m.sel.Sel.End(), NewText: []byte("WriteByte")},
				}
				if !m.literal {
					edits = append(edits,
						analysis.TextEdit{Pos: m.arg.Pos(), End: m.arg.Pos(), NewText: []byte("byte(")},
						analysis.TextEdit{Pos: m.arg.End(), End: m.arg.End(), NewText: []byte(")")},
					)
				}
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "replace WriteRune with WriteByte",
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps5102Site is a matched w.WriteRune(C) call: the selector to rewrite, the
// constant argument, whether its spelling may be kept verbatim, and whether
// the method pair is the standard library's (fix-eligible).
type ps5102Site struct {
	sel     *ast.SelectorExpr
	arg     ast.Expr
	argText string
	literal bool
	stdPair bool
}

// ps5102Match matches a method call w.WriteRune(C) with C a compile-time
// constant in [0, 0x80), on a receiver whose method set carries
// WriteByte(byte) error. Type information pins every part: the selection
// must be a real method (not a function-typed field or a package function),
// WriteRune must take exactly one rune, and WriteByte is looked up on the
// receiver's type with the addressability WriteRune itself required.
func ps5102Match(pass *analysis.Pass, call *ast.CallExpr) *ps5102Site {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "WriteRune" || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return nil
	}
	selection, ok := pass.TypesInfo.Selections[sel]
	if !ok || selection.Kind() != types.MethodVal {
		return nil
	}
	wr, ok := selection.Obj().(*types.Func)
	if !ok {
		return nil
	}
	wrSig, ok := wr.Type().(*types.Signature)
	if !ok || wrSig.Variadic() || wrSig.Params().Len() != 1 {
		return nil
	}
	if pb, ok := wrSig.Params().At(0).Type().Underlying().(*types.Basic); !ok || pb.Kind() != types.Int32 {
		return nil
	}
	// The constant argument, proven in [0, 0x80): anything else may need
	// (or already got) a multi-byte encoding and is out of scope.
	tv, ok := pass.TypesInfo.Types[call.Args[0]]
	if !ok || tv.Value == nil {
		return nil
	}
	iv := constant.ToInt(tv.Value)
	if iv.Kind() != constant.Int {
		return nil
	}
	v, exact := constant.Int64Val(iv)
	if !exact || v < 0 || v >= 0x80 {
		return nil
	}
	// WriteByte on the receiver expression's type. WriteRune on a pointer
	// receiver means the checker already had an address for w, so a
	// pointer-receiver WriteByte is reachable too; a value-receiver
	// WriteRune proves nothing about addressability, so the lookup then
	// demands WriteByte without one.
	addressable := false
	if r := wrSig.Recv(); r != nil {
		_, addressable = r.Type().(*types.Pointer)
	}
	// LookupFieldOrMethod returns a nil object when the only WriteByte has
	// a pointer receiver and the receiver is neither a pointer nor
	// addressable — exactly the case where the rewrite would not compile.
	wbObj, _, _ := types.LookupFieldOrMethod(selection.Recv(), addressable, pass.Pkg, "WriteByte")
	wb, ok := wbObj.(*types.Func)
	if !ok || !ps5102IsWriteByteSig(wb) {
		return nil
	}
	arg := call.Args[0]
	argText, ok := ps5102ExprText(arg)
	if !ok {
		return nil
	}
	lit, isLit := arg.(*ast.BasicLit)
	return &ps5102Site{
		sel:     sel,
		arg:     arg,
		argText: argText,
		literal: isLit && (lit.Kind == token.CHAR || lit.Kind == token.INT),
		stdPair: ps5102StdPair(wr, wb),
	}
}

// ps5102IsWriteByteSig reports whether wb is exactly WriteByte(byte) error.
func ps5102IsWriteByteSig(wb *types.Func) bool {
	sig, ok := wb.Type().(*types.Signature)
	if !ok || sig.Variadic() || sig.Params().Len() != 1 || sig.Results().Len() != 1 {
		return false
	}
	pb, ok := sig.Params().At(0).Type().(*types.Basic)
	if !ok || pb.Kind() != types.Uint8 {
		return false
	}
	return types.Identical(sig.Results().At(0).Type(), types.Universe.Lookup("error").Type())
}

// ps5102StdPair reports whether wr and wb are the standard library's own
// WriteRune/WriteByte pair on the same writer — the only pairs proven
// bit-identical for runes below 0x80. Promotion through an embedded std
// writer still resolves both methods to the std objects, so embedding
// matches too; a shadowing custom WriteByte would resolve shallower and
// fail the pairing.
func ps5102StdPair(wr, wb *types.Func) bool {
	name, ok := ps5102StdRecv(wr)
	if !ok {
		return false
	}
	wbName, ok := ps5102StdRecv(wb)
	return ok && wbName == name
}

// ps5102StdRecv returns the "path.Type" of f's receiver when f is a method
// of one of the three standard writers.
func ps5102StdRecv(f *types.Func) (string, bool) {
	if f.Pkg() == nil {
		return "", false
	}
	sig, ok := f.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return "", false
	}
	t := sig.Recv().Type()
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return "", false
	}
	key := f.Pkg().Path() + "." + named.Obj().Name()
	switch key {
	case "bytes.Buffer", "strings.Builder", "bufio.Writer":
		return key, true
	}
	return "", false
}

// ps5102ResultDiscarded reports whether the call's results are unused: the
// call is directly an expression statement, or the callee of a go/defer
// statement. Parentheses around the call are looked through.
func ps5102ResultDiscarded(call *ast.CallExpr, stack []ast.Node) bool {
	i := len(stack) - 1
	for ; i >= 0; i-- {
		if _, ok := stack[i].(*ast.ParenExpr); !ok {
			break
		}
	}
	if i < 0 {
		return false
	}
	switch p := stack[i].(type) {
	case *ast.ExprStmt:
		return true
	case *ast.GoStmt:
		return p.Call == call
	case *ast.DeferStmt:
		return p.Call == call
	}
	return false
}

// ps5102ExprText renders e for splicing into the diagnostic message.
func ps5102ExprText(e ast.Expr) (string, bool) {
	var b strings.Builder
	if err := printer.Fprint(&b, token.NewFileSet(), e); err != nil {
		return "", false
	}
	return b.String(), true
}
