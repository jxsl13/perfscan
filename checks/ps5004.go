package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/printer"
	"go/token"
	"go/types"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS5004 reports w.WriteString(S) where S is a compile-time constant string
// of byte-length exactly 1, on a receiver whose method set has
// WriteByte(byte) error.
var PS5004 = register(&lint.Check{
	ID:       "PS5004",
	Category: "arith",
	Slug:     "writestring-single-byte-literal",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "WriteString of a one-byte string runs the string-append machinery; WriteByte appends the byte directly",
		Text: `WriteString(",") routes a single byte through the generic
string-append path — availability check, grow bookkeeping, and a copy loop —
while WriteByte(',') is the specialized single-byte append: one bounds
check, one store. No allocation happens either way, so the saving is pure
instruction overhead per call; it matters in the delimiter-writing loops
where one-byte WriteString calls cluster.

Only a compile-time constant string argument whose byte-length is exactly 1
is matched — anything longer (including any multi-byte UTF-8 literal such
as "é", which is two bytes), the empty string, and every non-constant
argument are out of scope. The receiver's method set must carry
WriteByte(byte) error, verified through the type checker with the same
addressability WriteString itself required.

The automatic fix rewrites w.WriteString("c") to w.WriteByte('c') when
three proofs hold. First, the WriteString/WriteByte pair is the standard
library's own bytes.Buffer, strings.Builder, or bufio.Writer — pinned via
the type checker's method objects, not by name, so a user type shadowing
either method is excluded. For the first two, both methods always append
the full input and return a nil error; for bufio.Writer a one-byte
WriteString and WriteByte are state-identical by construction: the
io.StringWriter delegation needs an empty buffer AND len(s) > Available(),
which a one-byte string can never satisfy (an empty buffer has
Available() == len(buf) >= 1), so both forms flush-when-full and store one
byte, with identical error-sticky behavior. Second, the call's results are
discarded (an expression statement, or the call of a go or defer
statement): WriteString returns (int, error) but WriteByte returns only
error, so a used result would change the statement's shape. Third, the
argument is a string literal in the source — a named constant or constant
expression is reported advisory-only, because splicing its raw byte value
in would discard the symbolic name. The replacement byte literal is derived
from the type checker's constant value (the string's single byte carried
over verbatim, standard Go escaping applied — "\n" becomes '\n', "'"
becomes '\'', "\xff" becomes '\xff'), so raw literals, escape sequences,
and non-ASCII byte values all round-trip exactly. The rewrite touches no
imports.`,
		Before: `buf.WriteString(",")`,
		After:  `buf.WriteByte(',')`,
		MeasuredWin: `BenchmarkPS5004 (1024 one-byte strings into a reset
bytes.Buffer, Apple M2 Pro): 3.35 µs/op -> 2.07 µs/op (~1.6x, ~1.25 ns per
write), zero allocations either way. The saving is the string-append
bookkeeping and copy loop that the direct single-byte store skips.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5004",
		Doc:  "WriteString of a single-byte string constant instead of WriteByte",
		Run:  runPS5004,
	},
})

func runPS5004(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			m := ps5004Match(pass, call)
			if m == nil {
				return true
			}
			msg := "WriteString of the single-byte string " + m.argText +
				" runs the string-append machinery; WriteByte(" + m.byteLit +
				") appends the byte directly"
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: msg,
			}
			switch {
			case !m.stdPair:
				// A custom WriteString/WriteByte pair: nothing proves the
				// two methods append identically — advisory only.
				diag.Message = msg + "; this WriteString/WriteByte pair is not the standard library's — verify WriteByte appends identically before rewriting"
			case !ps5004ResultDiscarded(call, stack):
				// WriteString returns (int, error), WriteByte only error: a
				// used result changes the statement's shape — advisory only.
				diag.Message = msg + "; WriteString returns (int, error) but WriteByte returns only error — adjust the result handling by hand"
			case m.lit == nil:
				// A named constant or constant expression: splicing the raw
				// byte in would discard the symbolic name — advisory only.
				diag.Message = msg + "; the argument is a constant expression, not a string literal — introduce the matching byte constant and rewrite by hand"
			default:
				// Rewrite the selected name and replace the string literal
				// token with the equivalent byte literal — same single byte,
				// no side effects, import-neutral.
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: `replace WriteString("c") with WriteByte('c')`,
					TextEdits: []analysis.TextEdit{
						{Pos: m.sel.Sel.Pos(), End: m.sel.Sel.End(), NewText: []byte("WriteByte")},
						{Pos: m.lit.Pos(), End: m.lit.End(), NewText: []byte(m.byteLit)},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps5004Site is a matched w.WriteString(S) call: the selector to rewrite,
// the string literal token to replace (nil when the one-byte constant is
// not a direct literal — advisory only), the argument's source text for the
// message, the rendered replacement byte literal, and whether the method
// pair is the standard library's (fix-eligible).
type ps5004Site struct {
	sel     *ast.SelectorExpr
	lit     *ast.BasicLit
	argText string
	byteLit string
	stdPair bool
}

// ps5004Match matches a method call w.WriteString(S) with S a compile-time
// constant string of byte-length exactly 1, on a receiver whose method set
// carries WriteByte(byte) error. Type information pins every part: the
// selection must be a real method (not a function-typed field or a package
// function like io.WriteString), WriteString must have io.StringWriter's
// exact shape, and WriteByte is looked up on the receiver's type with the
// addressability WriteString itself required.
func ps5004Match(pass *analysis.Pass, call *ast.CallExpr) *ps5004Site {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "WriteString" || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return nil
	}
	selection, ok := pass.TypesInfo.Selections[sel]
	if !ok || selection.Kind() != types.MethodVal {
		return nil
	}
	ws, ok := selection.Obj().(*types.Func)
	if !ok || !ps5004IsWriteStringSig(ws) {
		return nil
	}
	// The constant argument, proven to be a string of byte-length exactly 1:
	// anything else writes a different byte count (or is not knowable at
	// compile time) and is out of scope. The byte value is taken from the
	// type checker's constant, so escapes and raw literals are already
	// decoded.
	tv, ok := pass.TypesInfo.Types[call.Args[0]]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return nil
	}
	s := constant.StringVal(tv.Value)
	if len(s) != 1 {
		return nil
	}
	// WriteByte on the receiver expression's type. WriteString on a pointer
	// receiver means the checker already had an address for w, so a
	// pointer-receiver WriteByte is reachable too; a value-receiver
	// WriteString proves nothing about addressability, so the lookup then
	// demands WriteByte without one.
	addressable := false
	if r := ps5004Recv(ws); r != nil {
		_, addressable = r.Type().(*types.Pointer)
	}
	// LookupFieldOrMethod returns a nil object when the only WriteByte has
	// a pointer receiver and the receiver is neither a pointer nor
	// addressable — exactly the case where the rewrite would not compile.
	wbObj, _, _ := types.LookupFieldOrMethod(selection.Recv(), addressable, pass.Pkg, "WriteByte")
	wb, ok := wbObj.(*types.Func)
	if !ok || !ps5004IsWriteByteSig(wb) {
		return nil
	}
	arg := call.Args[0]
	for {
		p, ok := arg.(*ast.ParenExpr)
		if !ok {
			break
		}
		arg = p.X
	}
	argText, ok := ps5004ExprText(arg)
	if !ok {
		return nil
	}
	lit, _ := arg.(*ast.BasicLit)
	if lit != nil && lit.Kind != token.STRING {
		lit = nil
	}
	return &ps5004Site{
		sel:     sel,
		lit:     lit,
		argText: argText,
		byteLit: ps5004ByteLit(s[0]),
		stdPair: ps5004StdPair(ws, wb),
	}
}

// ps5004ByteLit renders b as a Go character literal whose value is exactly
// b: printable ASCII stays verbatim (quote and backslash escaped by
// strconv), control bytes take strconv's standard escapes ('\n', '\x00'), and the
// high half 0x80..0xFF — never a valid single-byte rune spelling — is
// rendered as a hex escape ('\xff'). Every result is an untyped rune
// constant in [0, 0xFF], representable in byte, so the rewritten call
// compiles and appends the identical byte.
func ps5004ByteLit(b byte) string {
	if b >= utf8.RuneSelf {
		return fmt.Sprintf(`'\x%02x'`, b)
	}
	return strconv.QuoteRune(rune(b))
}

// ps5004IsWriteStringSig reports whether ws is exactly
// WriteString(string) (int, error).
func ps5004IsWriteStringSig(ws *types.Func) bool {
	sig, ok := ws.Type().(*types.Signature)
	if !ok || sig.Variadic() || sig.Params().Len() != 1 || sig.Results().Len() != 2 {
		return false
	}
	pb, ok := types.Unalias(sig.Params().At(0).Type()).(*types.Basic)
	if !ok || pb.Kind() != types.String {
		return false
	}
	rb, ok := sig.Results().At(0).Type().(*types.Basic)
	if !ok || rb.Kind() != types.Int {
		return false
	}
	return types.Identical(sig.Results().At(1).Type(), types.Universe.Lookup("error").Type())
}

// ps5004IsWriteByteSig reports whether wb is exactly WriteByte(byte) error.
func ps5004IsWriteByteSig(wb *types.Func) bool {
	sig, ok := wb.Type().(*types.Signature)
	if !ok || sig.Variadic() || sig.Params().Len() != 1 || sig.Results().Len() != 1 {
		return false
	}
	pb, ok := types.Unalias(sig.Params().At(0).Type()).(*types.Basic)
	if !ok || pb.Kind() != types.Uint8 {
		return false
	}
	return types.Identical(sig.Results().At(0).Type(), types.Universe.Lookup("error").Type())
}

// ps5004StdPair reports whether ws and wb are the standard library's own
// WriteString/WriteByte pair on the same fix-eligible writer — bytes.Buffer,
// strings.Builder, or bufio.Writer, the three whose one-byte WriteString and
// WriteByte are proven to append identically (for bufio.Writer the
// StringWriter delegation is unreachable at length 1, so both forms
// flush-when-full and store the byte with identical error behavior).
// Promotion through an embedded std writer still resolves both methods to
// the std objects, so embedding matches too; a shadowing custom method would
// resolve shallower and fail the pairing.
func ps5004StdPair(ws, wb *types.Func) bool {
	name, ok := ps5004StdRecv(ws)
	if !ok {
		return false
	}
	wbName, ok := ps5004StdRecv(wb)
	return ok && wbName == name
}

// ps5004StdRecv returns the "path.Type" of f's receiver when f is a method
// of one of the three fix-eligible standard writers.
func ps5004StdRecv(f *types.Func) (string, bool) {
	if f.Pkg() == nil {
		return "", false
	}
	r := ps5004Recv(f)
	if r == nil {
		return "", false
	}
	t := r.Type()
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

// ps5004Recv returns f's receiver variable, or nil for a non-method.
func ps5004Recv(f *types.Func) *types.Var {
	sig, ok := f.Type().(*types.Signature)
	if !ok {
		return nil
	}
	return sig.Recv()
}

// ps5004ResultDiscarded reports whether the call's results are unused: the
// call is directly an expression statement, or the callee of a go/defer
// statement. Parentheses around the call are looked through.
func ps5004ResultDiscarded(call *ast.CallExpr, stack []ast.Node) bool {
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

// ps5004ExprText renders e for splicing into the diagnostic message.
func ps5004ExprText(e ast.Expr) (string, bool) {
	var b strings.Builder
	if err := printer.Fprint(&b, token.NewFileSet(), e); err != nil {
		return "", false
	}
	return b.String(), true
}
