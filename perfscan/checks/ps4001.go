package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/config"
	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS4001 reports per-element binary decodes in loops. The presence of a
// configured bulk-copy helper in the same function silences the finding:
// the per-element loop is then the genuine-decode fallback (usually the
// big-endian arm), not a candidate.
var PS4001 = register(&lint.Check{
	ID:       "PS4001",
	Category: "vector",
	Slug:     "per-element-binary-decode",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "a per-element binary decode (encoding/binary) in a loop",
		Text: `Decoding scalars one at a time through encoding/binary in a loop
pays a call and a bounds check per element. On little-endian hosts a
same-layout bulk copy (unsafe.Slice or a memmove-style helper) moves the
whole buffer at once; keep the per-element loop as the big-endian/strided
fallback.

The presence of a configured bulk-copy helper (bulkCopyHelpers) in the
same function silences this check: the per-element loop is then the
intended fallback path, not a candidate. KEEP THAT LIST COMPLETE — a
helper missing from the config makes this check report the very fallback
the bulk path exists to guard.`,
		Before: `for i := range out {
	out[i] = math.Float64frombits(binary.LittleEndian.Uint64(buf[i*8:]))
}`,
		After: `if hostIsLE {
	rawCopyLE(out, buf) // one bulk copy
} else {
	for i := range out { // genuine-decode fallback
		out[i] = math.Float64frombits(binary.LittleEndian.Uint64(buf[i*8:]))
	}
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS4001",
		Doc:  "per-element binary decode in a loop",
		Run:  runPS4001,
	},
})

var binaryDecodeMethods = map[string]bool{
	"Uint16": true, "Uint32": true, "Uint64": true,
	"PutUint16": true, "PutUint32": true, "PutUint64": true,
}

// isBinaryEndianCall matches binary.LittleEndian.X / binary.BigEndian.X.
func isBinaryEndianCall(call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !binaryDecodeMethods[sel.Sel.Name] {
		return "", false
	}
	inner, ok := sel.X.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	id, ok := inner.X.(*ast.Ident)
	if !ok || id.Name != "binary" || id.Obj != nil {
		return "", false
	}
	if inner.Sel.Name != "LittleEndian" && inner.Sel.Name != "BigEndian" {
		return "", false
	}
	return inner.Sel.Name + "." + sel.Sel.Name, true
}

func runPS4001(pass *analysis.Pass) (any, error) {
	bulk := config.Current().BulkCopyHelpers
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			// A configured bulk-copy helper in this function marks the
			// per-element loop as the intended fallback.
			if len(bulk) > 0 && callsFanOut(fn.Body, bulk) {
				continue
			}
			reportedLoop := map[ast.Node]bool{}
			astutil.WithStack(fn.Body, func(n ast.Node, stack []ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name, ok := isBinaryEndianCall(call)
				if !ok {
					return true
				}
				loop, inLoop := astutil.InLoop(stack)
				if !inLoop || reportedLoop[loop] {
					return true
				}
				reportedLoop[loop] = true
				pass.Report(analysis.Diagnostic{
					Pos:     call.Pos(),
					End:     call.End(),
					Message: fmt.Sprintf("binary.%s decodes one scalar per iteration; on a little-endian host a same-layout bulk copy moves the whole buffer once — keep this loop as the big-endian/strided fallback", name),
				})
				return true
			})
		}
	}
	return nil, nil
}
