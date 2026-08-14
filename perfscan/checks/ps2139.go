package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2139 reports w.WriteString(string(r)) — a single rune converted to a
// throwaway string only to be written — when w's WriteString/WriteRune
// pair is the standard library's (strings.Builder, bytes.Buffer), where
// WriteRune(r) appends the identical UTF-8 bytes with no intermediate
// string. The rune analog of PS2135's byte-slice form.
var PS2139 = register(&lint.Check{
	ID:       "PS2139",
	Category: "alloc",
	Slug:     "write-string-of-rune",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "w.WriteString(string(r)) allocates a one-rune string; w.WriteRune(r) encodes into the buffer directly",
		Text: `string(r) for a non-constant rune materializes a fresh 1-4 byte
string on every call just so WriteString can copy those bytes into the
buffer; WriteRune(r) runs the same UTF-8 encoder straight into the
receiver's buffer — no intermediate string, no second copy. (A CONSTANT
rune is out of scope: string('a') is folded at compile time and
allocates nothing.)

The rewrite is bit-identical on the two std writers it fixes, verified
exhaustively over every int32 value: string(r) and WriteRune produce the
same UTF-8 bytes for every rune, INCLUDING the invalid ones (negative,
the surrogate range 0xD800-0xDFFF, > 0x10FFFF), which both encode as
U+FFFD; and both calls return the same (int, error) — the encoded byte
count and a nil error, since strings.Builder and bytes.Buffer never fail
— so the fix applies even when the results are used.

Two gates are load-bearing for that identity:

  - INTEGER WIDTH. The conversion operand's type must fit losslessly in
    int32: rune/int32 itself (kept verbatim), or a named rune type,
    byte, int8, int16, uint16 (wrapped as WriteRune(rune(x)), a
    value-preserving widening). A WIDER type (int, int64, uint32, ...)
    is never touched: string(x) converts the full value (out of range
    becomes U+FFFD) while rune(x) would truncate first and diverge —
    string(int64(0x100000041)) is "�" but string(rune(...)) is "A".

  - RECEIVER. Only the standard library's own WriteString/WriteRune pair
    on strings.Builder or bytes.Buffer is rewritten (embedding that
    promotes both std methods qualifies too). bufio.Writer stays
    advisory: on its flush-error path WriteString can write a partial
    rune and report a partial count where WriteRune reports (0, err) —
    observably different results and Buffered() state. A custom pair
    carries no proof its two methods agree at all, and an interface pair
    is equally unproven — advisory only. No method-set WriteRune with
    signature (rune) (int, error), nothing to advise: silent.

One position is excluded entirely: w.WriteString(string(r)) lexically
inside w's own WriteRune method — the WriteRune-delegates-to-WriteString
implementation, where the rewrite would make WriteRune call itself
(unbounded recursion that still compiles). The original delegation is
already the correct code, so the check reports nothing there.

The automatic fix renames the selected method to WriteRune and unwraps
the conversion in place — deleting "string(" and ")" for an operand
already assignable to rune, or rewriting the conversion's type to
rune(...) for the narrower widths — leaving the receiver and operand
expressions untouched: same evaluation, same order, import-neutral. A
comment inside the call suppresses the fix and keeps the advisory
report.`,
		Before: `b.WriteString(string(r))`,
		After:  `b.WriteRune(r)`,
		MeasuredWin: `BenchmarkPS2139 (1024 mixed 1-4 byte runes into a reset
bytes.Buffer, Apple M2 Pro): 6.8 µs/op -> 3.8 µs/op (~1.8x), zero
allocations either way here (the concrete receiver lets gc stack-spill
the temporary string) — the win is the dropped encode-into-string +
copy round trip. When the conversion escapes (builder behind an
interface, or captured), string(r) additionally heap-allocates on every
multi-byte rune, which WriteRune never does.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2139",
		Doc:  "WriteString(string(r)) of a single rune instead of WriteRune(r)",
		Run:  runPS2139,
	},
})

func runPS2139(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "WriteString" {
				return true
			}
			// A method value selection only: package-qualified functions
			// (io.WriteString is PS2118's) and function-typed fields do
			// not appear in Selections as MethodVal.
			selInfo, ok := pass.TypesInfo.Selections[sel]
			if !ok || selInfo.Kind() != types.MethodVal {
				return true
			}
			ws, ok := selInfo.Obj().(*types.Func)
			if !ok || !ps2139IsWriteStringSig(ws) {
				return true
			}
			conv, inner, needsWrap := ps2139RuneConv(pass, call.Args[0])
			if conv == nil {
				return true
			}
			// WriteRune must be reachable on the receiver with the std
			// (rune) (int, error) shape. A pointer-receiver WriteString
			// that compiled proves the receiver was addressable, so a
			// pointer-receiver WriteRune is reachable the same way; a
			// value-receiver WriteString proves nothing about
			// addressability, so the lookup then demands WriteRune
			// without one (same reachability logic as PS5102).
			addressable := false
			if r := ws.Type().(*types.Signature).Recv(); r != nil {
				_, addressable = r.Type().(*types.Pointer)
			}
			wrObj, _, _ := types.LookupFieldOrMethod(selInfo.Recv(), addressable, pass.Pkg, "WriteRune")
			wr, ok := wrObj.(*types.Func)
			if !ok || !ps2139IsWriteRuneSig(wr) {
				return true
			}
			// Inside the receiver's own WriteRune method the rewrite
			// would call the enclosing method itself — the classic
			// WriteRune-delegates-to-WriteString implementation. The
			// original code is the correct delegation: stay silent.
			if writeFixSelfDispatches(pass, call, sel.X, "WriteRune") {
				return true
			}
			msg := "w.WriteString(string(r)) allocates and UTF-8-encodes a throwaway string; the receiver's WriteRune(r) encodes straight into the buffer"
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: msg,
			}
			switch key, std := ps2139StdPair(ws, wr); {
			case !std:
				// A custom or interface pair: nothing proves the two
				// methods agree — advisory only.
				diag.Message = msg + "; this WriteString/WriteRune pair is not the standard library's — verify WriteRune appends the identical UTF-8 encoding before rewriting"
			case key == "bufio.Writer":
				// On the flush-error path bufio's WriteString can write a
				// partial rune and report a partial count where WriteRune
				// reports (0, err) — observably different results and
				// Buffered() state, so the rewrite is never bit-identical
				// there (pinned in TestEquiv_PS2139WriteStringRune).
				diag.Message = msg + "; bufio.Writer diverges on the flush-error path (WriteString reports a partial count where WriteRune reports 0) — rewrite by hand if the error path allows it"
			case !ps2111CommentIn(f, call.Pos(), call.End()):
				// The fix renames the selected method and unwraps the
				// conversion in place (same expressions, same evaluation
				// order): the operand is kept verbatim when it is already
				// assignable to rune, otherwise the conversion's type is
				// rewritten to rune(...) — a value-preserving widening
				// for the int32-lossless widths the match admits. A
				// comment anywhere inside the call keeps the report
				// advisory (guarded by the enclosing case).
				edits := []analysis.TextEdit{
					{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("WriteRune")},
				}
				if needsWrap {
					edits = append(edits, analysis.TextEdit{Pos: conv.Fun.Pos(), End: conv.Fun.End(), NewText: []byte("rune")})
				} else {
					edits = append(edits,
						analysis.TextEdit{Pos: conv.Pos(), End: inner.Pos()},
						analysis.TextEdit{Pos: inner.End(), End: conv.End()},
					)
				}
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "replace WriteString(string(...)) with WriteRune(...)",
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2139RuneConv matches arg (parens stripped) as a NON-CONSTANT conversion
// string(x) of a rune-sized integer: the conversion type must be (an alias
// of) the predeclared string, and x's type, unaliased, must have an
// underlying basic integer kind that fits losslessly in int32 — rune/int32
// itself, a named rune type, byte, int8, int16 or uint16. Wider kinds
// (int, int64, uint32, uint64, uintptr) are rejected: for an out-of-range
// value string(x) yields U+FFFD while rune(x) would truncate and diverge.
// A constant conversion is rejected too — string('a') is folded at compile
// time and allocates nothing, so there is no win to report (and PS5102
// already owns the constant single-byte WriteRune territory). needsWrap
// reports whether the fix must spell rune(x): true unless x is already
// assignable to WriteRune's rune parameter.
func ps2139RuneConv(pass *analysis.Pass, arg ast.Expr) (conv *ast.CallExpr, inner ast.Expr, needsWrap bool) {
	for {
		p, ok := arg.(*ast.ParenExpr)
		if !ok {
			break
		}
		arg = p.X
	}
	c, ok := arg.(*ast.CallExpr)
	if !ok || len(c.Args) != 1 || c.Ellipsis.IsValid() {
		return nil, nil, false
	}
	tv, ok := pass.TypesInfo.Types[c.Fun]
	if !ok || !tv.IsType() {
		return nil, nil, false
	}
	tb, ok := types.Unalias(tv.Type).(*types.Basic)
	if !ok || tb.Kind() != types.String {
		return nil, nil, false
	}
	// A constant conversion (constant rune operand) never allocates.
	if ctv, ok := pass.TypesInfo.Types[c]; !ok || ctv.Value != nil {
		return nil, nil, false
	}
	it := pass.TypesInfo.TypeOf(c.Args[0])
	if it == nil {
		return nil, nil, false
	}
	ib, ok := types.Unalias(it).Underlying().(*types.Basic)
	if !ok {
		return nil, nil, false
	}
	switch ib.Kind() {
	case types.Int32, types.Int8, types.Int16, types.Uint8, types.Uint16:
		// Lossless in int32: rune(x) preserves the exact value.
	default:
		return nil, nil, false
	}
	return c, c.Args[0], !types.AssignableTo(it, types.Typ[types.Int32])
}

// ps2139IsWriteStringSig reports whether ws is a WriteString whose single
// parameter has underlying type string — the shape string(r) is assignable
// to (any tighter result check is left to the std-pair gate; a custom
// pair only ever gets an advisory report).
func ps2139IsWriteStringSig(ws *types.Func) bool {
	sig, ok := ws.Type().(*types.Signature)
	if !ok || sig.Variadic() || sig.Params().Len() != 1 {
		return false
	}
	pb, ok := sig.Params().At(0).Type().Underlying().(*types.Basic)
	return ok && pb.Kind() == types.String
}

// ps2139IsWriteRuneSig reports whether wr is exactly
// WriteRune(rune) (int, error) — the std shape whose results keep the
// original call's types.
func ps2139IsWriteRuneSig(wr *types.Func) bool {
	sig, ok := wr.Type().(*types.Signature)
	if !ok || sig.Variadic() || sig.Params().Len() != 1 || sig.Results().Len() != 2 {
		return false
	}
	pb, ok := sig.Params().At(0).Type().(*types.Basic)
	if !ok || pb.Kind() != types.Int32 {
		return false
	}
	rb, ok := sig.Results().At(0).Type().(*types.Basic)
	if !ok || rb.Kind() != types.Int {
		return false
	}
	return types.Identical(sig.Results().At(1).Type(), types.Universe.Lookup("error").Type())
}

// ps2139StdPair returns the receiver key ("strings.Builder", "bytes.Buffer"
// or "bufio.Writer") when ws and wr are the standard library's own
// WriteString/WriteRune pair on the same writer. Promotion through an
// embedded std writer still resolves both methods to the std objects, so
// embedding matches too; a shadowing custom method resolves shallower and
// fails the pairing (which is also what keeps the self-dispatch delegation
// pattern out of the fix path).
func ps2139StdPair(ws, wr *types.Func) (string, bool) {
	key, ok := ps5102StdRecv(ws)
	if !ok {
		return "", false
	}
	wrKey, ok := ps5102StdRecv(wr)
	if !ok || wrKey != key {
		return "", false
	}
	return key, true
}
