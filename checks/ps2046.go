package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"go/version"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2046 reports fmt.Appendf(buf, "%x", bs) — a lone lowercase %x over a
// byte slice — where hex.AppendEncode(buf, bs) (go1.22+) appends the
// identical lowercase hex digits without fmt's formatter machinery. The
// Appendf-destination twin of PS2107's %x []byte -> hex.EncodeToString arm
// and the hex sibling of PS2141 (Appendf lone %s -> append).
//
// ADVISORY BY DESIGN (AutoFix:false): the rewrite is byte-identical for
// every input where bs does not overlap buf's spare capacity, but a
// safe-Go divergence input exists — see the Doc and the pinned runtime
// differential in equiv_PS2046_test.go — so no mechanical fix is offered.
var PS2046 = register(&lint.Check{
	ID:       "PS2046",
	Category: "alloc",
	Slug:     "appendf-single-x-bytes",
	Level:    lint.LevelIdiomatic,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "fmt.Appendf(buf, \"%x\", bs) runs fmt's formatter to hex-encode bytes hex.AppendEncode writes directly",
		Text: `fmt.Appendf parses its format string, boxes the argument into an
interface (a heap allocation) and drives fmt's formatter state machine
through a pooled pp buffer — even when the format is a lone %x
hex-dumping one byte slice onto the buffer. fmt's bare lowercase %x over
a []byte emits exactly two lowercase hex digits per byte with no
separators — the same bytes hex.Encode produces, which is why PS2107
already maps fmt.Sprintf("%x", bs) to hex.EncodeToString. Since go1.22
the append-destination form exists too: hex.AppendEncode(buf, bs) runs a
tight encode loop that writes the digits straight into buf's existing
capacity — no interface boxing, no format parse, no pp round-trip.

The match is deliberately narrow. The callee is pinned by type
information to the package-level fmt.Appendf (a shadowed fmt or a method
named Appendf never matches), the call must not spread its arguments,
the format must be a string literal that is EXACTLY the bare lowercase
"%x" (case-sensitive: %X — uppercase digits — %#x, widths and flags all
disqualify), and the value must be a byte slice whose element type is
the predeclared byte itself (a named element type would make the slice
unassignable to hex's []byte parameter). %x over an integer is PS5015's
subject and %x over a string or float is different formatting entirely —
neither is reported here. The report only fires when the file's
effective language version is at least go1.22 — below that
hex.AppendEncode does not exist and the suggestion would not compile.

Two caveats keep a human in the loop. First, a NAMED []byte operand (or
destination) may implement fmt.Formatter, which %x would honor and
hex.AppendEncode would not — check for a Format method before rewriting
a named type. Second — and this is why the check is ADVISORY BY DESIGN,
with no automatic fix — the rewrite is NOT bit-identical when bs
overlaps buf's SPARE CAPACITY (the region between len(buf) and
cap(buf)): fmt.Appendf formats bs into a separate pooled buffer before
appending, so it always reads bs's original bytes, while
hex.AppendEncode encodes forward directly into buf[len(buf):] and can
overwrite source bytes before reading them. Concretely, with
buf := make([]byte, 4, 16) holding "abcd" and bs := buf[4:6] holding
{0xAB, 0xCD}, fmt.Appendf(buf, "%x", bs) yields "abcdabcd" but
hex.AppendEncode(buf, bs) yields "abcdab62" — the second source byte was
clobbered by the first digit pair (pinned in equiv_PS2046_test.go). That
shape is pure safe Go (a reslice past len within cap), and no local type
check can rule it out, so the rewrite is left to a human who can see
that bs and buf are disjoint — which in ordinary code they are.`,
		Before: `buf = fmt.Appendf(buf, "%x", bs)`,
		After:  `buf = hex.AppendEncode(buf, bs) // go1.22+; verify bs does not alias buf's spare capacity`,
		MeasuredWin: `BenchmarkPS2046 (a 32-byte slice hex-encoded onto a
preallocated buffer, Apple M2 Pro, go1.26): fmt.Appendf(buf, "%x", bs)
~77.9 ns/op, 24 B/op, 1 alloc/op vs hex.AppendEncode(buf, bs)
~25.1 ns/op, 0 B/op, 0 allocs/op (~3.1x, and the interface-boxing
allocation disappears; both write into the buffer's existing
capacity — the entire gap is fmt's format parse, boxing and formatter
state machine vs hex's tight encode loop).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2046",
		Doc:  "fmt.Appendf with a lone %x over a []byte; hex.AppendEncode (go1.22) appends the identical hex digits directly",
		Run:  runPS2046,
	},
})

func runPS2046(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if !ps2046HexAppendAvailable(pass, f) {
			// hex.AppendEncode exists only from go1.22 on; suggesting it
			// below that would not compile (same policy as PS2119's
			// SplitSeq gate).
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 || call.Ellipsis.IsValid() {
				return true
			}
			// Type info pins the callee to the package-level fmt.Appendf; a
			// shadowed fmt or a method named Appendf never matches.
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "fmt", map[string]bool{"Appendf": true}); !ok {
				return true
			}
			// The format must be the string LITERAL "%x" exactly — the bare
			// lowercase verb. %X, %#x, widths and any literal text never
			// reach the hex-identical shape.
			lit, ok := call.Args[1].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if format, err := strconv.Unquote(lit.Value); err != nil || format != "%x" {
				return true
			}
			// Only a byte slice whose element is the predeclared byte itself
			// is hex-encodable: fmt hex-dumps any byte-kind slice, but a
			// named element type would make the slice unassignable to hex's
			// []byte parameter. Integers are PS5015's subject; strings and
			// floats are different formatting entirely.
			if !ps2046HexableByteSlice(pass.TypesInfo.TypeOf(call.Args[2])) {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "fmt.Appendf with a lone %x over a []byte boxes the argument and walks fmt's formatter state machine; hex.AppendEncode appends the identical lowercase hex digits directly into buf (go1.22; verify bs does not alias buf's spare capacity)",
			})
			return true
		})
	}
	return nil, nil
}

// ps2046HexableByteSlice reports whether %x over a value of type t is a hex
// dump hex.AppendEncode can reproduce: t's underlying type is a slice whose
// element is EXACTLY the predeclared byte (not a named byte type, which
// would make the slice unassignable to hex's []byte parameter). Covers named
// and unnamed []byte — the Doc's Formatter caveat narrows the named case to
// human judgment.
func ps2046HexableByteSlice(t types.Type) bool {
	if t == nil {
		return false
	}
	sl, ok := t.Underlying().(*types.Slice)
	if !ok {
		return false
	}
	b, ok := sl.Elem().(*types.Basic)
	return ok && b.Kind() == types.Byte
}

// ps2046HexAppendAvailable reports whether f's effective language version
// has hex.AppendEncode (go1.22). The per-file version (a //go:build go1.N
// line moves it, up or down) wins over the package's. An unknown or
// unparseable version does not block the check: perfscan itself requires a
// far newer toolchain, so an empty version means "module default", not
// "ancient" — the same policy as PS2119's SplitSeq gate.
func ps2046HexAppendAvailable(pass *analysis.Pass, f *ast.File) bool {
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
	return version.Compare(lang, "go1.22") >= 0
}
