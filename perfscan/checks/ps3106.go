package checks

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// ps3106Threshold is the size, in bytes, above which a by-value receiver
// or parameter is reported. 128 bytes (two cache lines) matches go-critic's
// hugeParam default and PS3103's rangeValCopy gate: below it the per-call
// copy is a handful of MOVs the CPU hides behind the call overhead, and
// reporting it would be noise.
const ps3106Threshold = 128

// PS3106 reports functions and methods that take a large struct or array
// BY VALUE: every call copies the whole thing onto the callee's frame.
// ADVISORY ONLY — switching to a pointer changes semantics (nil-ability,
// mutation visibility, the method set), so no mechanical fix is safe.
var PS3106 = register(&lint.Check{
	ID:       "PS3106",
	Category: "indirect",
	Slug:     "large-value-param",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "large value receivers and parameters copy on every call",
		Text: `A function or method that takes a large struct or array by
value copies the entire thing onto the callee's frame at every call site:
memmove traffic that scales with sizeof(T) per call, plus larger frames
that push the caller's hot data out of cache. Receivers and parameters
larger than 128 bytes (two cache lines, go-critic's hugeParam default)
are reported. Pointer, slice, map, channel, func, interface, and string
types are never reported — they are small fixed-size headers regardless
of what they point at. Variadic parameters are a slice at the call
boundary and are likewise cheap. Types containing a free type parameter
are skipped: Sizeof is meaningless for generics, where the size differs
per instantiation. Blank (_) receivers and parameters are skipped too:
the compiler is free to elide a copy nobody reads.

The manual remedy is to take a pointer:

	func process(r *Report) { ... }   // 8-byte pointer instead of a copy
	func (r *Report) Sum() int { ... }

NO AUTOMATIC FIX IS ATTACHED, deliberately. Changing T to *T is
observable, not cosmetic: the callee's writes now mutate the caller's
value instead of a private copy; nil becomes a possible argument where
it could not exist before; a value receiver method belongs to the method
set of both T and *T, while a pointer receiver method belongs only to
*T — the rewrite can silently remove the method from interfaces T used
to satisfy; and every call site's argument expression may need &.
Whether the copy is load-bearing (deliberate isolation from the caller)
or accidental is a design question only a human can answer — exactly the
kind of judgment an advisory is for.`,
		Before: `type Report struct{ buf [4096]byte }

func (r Report) Sum() int { // copies 4096 bytes on every call
	...
}

func process(r Report) { // copies 4096 bytes on every call
	...
}`,
		After: `type Report struct{ buf [4096]byte }

func (r *Report) Sum() int { // 8-byte pointer; callee writes now ALIAS the caller's value
	...
}

func process(r *Report) { // callers must pass &report; nil becomes possible
	...
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3106",
		Doc:  "large value receivers/parameters copy the whole value on every call",
		Run:  runPS3106,
	},
})

func runPS3106(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if fn.Recv != nil {
				for _, field := range fn.Recv.List {
					ps3106CheckField(pass, field, "receiver")
				}
			}
			if fn.Type.Params != nil {
				for _, field := range fn.Type.Params.List {
					ps3106CheckField(pass, field, "parameter")
				}
			}
		}
	}
	return nil, nil
}

// ps3106CheckField reports field when its type is a large by-value copy.
// kind is "receiver" or "parameter", used verbatim in the message.
func ps3106CheckField(pass *analysis.Pass, field *ast.Field, kind string) {
	// A variadic ...T parameter arrives as a []T slice header: cheap.
	if _, variadic := field.Type.(*ast.Ellipsis); variadic {
		return
	}
	t := pass.TypesInfo.TypeOf(field.Type)
	if t == nil || !ps3106IsBulkValue(t) || ps3103HasTypeParam(t, nil) {
		return
	}
	size := pass.TypesSizes.Sizeof(t)
	if size <= ps3106Threshold {
		return
	}
	tstr := types.TypeString(t, types.RelativeTo(pass.Pkg))
	if len(field.Names) == 0 {
		// Unnamed parameter (or embedded-style receiver): the copy at the
		// call boundary happens regardless of whether the callee names it.
		pass.Report(analysis.Diagnostic{
			Pos: field.Type.Pos(),
			End: field.Type.End(),
			Message: fmt.Sprintf(
				"%s of type %s is %d bytes; passing it by value copies %d bytes on every call — take a *%s for large values (advisory)",
				kind, tstr, size, size, tstr),
		})
		return
	}
	// One field can carry several names (func f(a, b Big)); format the
	// shared tail once and splice each name in, keeping Sprintf out of
	// the loop.
	tail := fmt.Sprintf(
		" of type %s is %d bytes; passing it by value copies %d bytes on every call — take a *%s for large values (advisory)",
		tstr, size, size, tstr)
	for _, name := range field.Names {
		if name.Name == "_" {
			continue // the compiler may elide a copy nobody reads
		}
		pass.Report(analysis.Diagnostic{
			Pos:     name.Pos(),
			End:     name.End(),
			Message: kind + " " + name.Name + tail,
		})
	}
}

// ps3106IsBulkValue reports whether t is a type whose by-value passing
// cost scales with its declared size: structs and arrays. Everything
// else — pointers, slices, maps, channels, funcs, interfaces, strings,
// and basic types — is a small fixed-size header or scalar; the Sizeof
// gate would exclude them anyway, but excluding them structurally keeps
// the check honest on exotic architectures.
func ps3106IsBulkValue(t types.Type) bool {
	switch t.Underlying().(type) {
	case *types.Struct, *types.Array:
		return true
	}
	return false
}
