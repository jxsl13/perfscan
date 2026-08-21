package checks

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5085 removes standard-library Clone chains passed immediately to concrete
// stdlib writers whose implementation copies the argument before returning.
var PS5085 = register(&lint.Check{
	ID:       "PS5085",
	Category: "alloc",
	Slug:     "clone-fed-stdlib-buffer-write",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a standard-library buffer copies a throwaway Clone result",
		Text: `bytes.Buffer and strings.Builder copy every Write or WriteString
argument into receiver-owned storage before returning. Cloning that argument
first creates an independent allocation that the receiver immediately copies
again and discards:

  buffer.Write(bytes.Clone(data))
  buffer.Write(slices.Clone(bytes.Clone(data)))
  builder.Write(slices.Clone(data))
  buffer.WriteString(strings.Clone(text))
  builder.WriteString(strings.Clone(strings.Clone(text)))

All become the same call on the original data or text.

This check resolves both the selected method and every Clone layer through
go/types. The receiver method must be exactly bytes.Buffer.Write,
bytes.Buffer.WriteString, strings.Builder.Write, or
strings.Builder.WriteString; a same-named user method, method value, interface
dispatch, reader/constructor, or retaining API stays untouched. Write accepts
only byte-preserving bytes.Clone/slices.Clone chains, while WriteString accepts
only strings.Clone chains. Aliases, promoted stdlib methods, explicit generic
instantiation, in-place buffer sources, and arbitrarily deep heterogeneous
byte Clone chains are supported.

The rewrite is bit-identical for race-free safe Go programs. Both methods
return the argument length (and Write returns a nil error), evaluate receiver
then argument once, and copy the same bytes before returning. bytes.Buffer
uses copy/append semantics, so even buffer.Write(buffer.Bytes()) overlap has
the same result without the temporary snapshot; append/copy explicitly support
overlapping source and destination. strings.Builder cannot expose mutable
backing bytes in safe Go, and its String view is immutable; WriteString's
append remains overlap-safe. Receiver growth, length, capacity, errors, and
panics are unchanged apart from eliminating an otherwise unnecessary
allocation failure.

The result is not generalized to arbitrary io.Writer implementations even
though the interface contract forbids retaining p: concrete stdlib ownership
keeps the automatic rewrite narrow and mechanically auditable. Comments keep
the finding advisory. The shared replacement/import-liveness engine removes
newly unused Clone imports, and terminal ownership prevents an overlapping
nested-clone fix so one -fix pass reaches the direct Write call.`,
		Before: `var buffer bytes.Buffer
buffer.Grow(len(data))
n, err := buffer.Write(slices.Clone(bytes.Clone(data)))
buffer.WriteString(strings.Clone(text))`,
		After: `var buffer bytes.Buffer
buffer.Grow(len(data))
n, err := buffer.Write(data)
buffer.WriteString(text)`,
		MeasuredWin: `On Apple M2 Pro with a pre-grown bytes.Buffer and a
68,817-byte argument (median of five 100-iteration runs), Write(bytes.Clone(p))
measured 9,844 ns/op, 73,728 B/op, 1 alloc/op versus Write(p) at 1,171 ns/op,
0 B/op, 0 allocs/op: 8.41x faster and 88.1% less time. WriteString over
strings.Clone measured 9,003 ns/op versus 1,049 ns/op, the same allocation
removal and an 8.58x speedup. Both direct forms retain only the receiver's
required copy.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5085",
		Doc:  "bytes.Buffer/strings.Builder Write consumes throwaway stdlib Clone chains",
		Run:  runPS5085,
	},
})

func runPS5085(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			chain, receiver, method, matched := ps5085Match(pass, call)
			if !matched {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: fmt.Sprintf("%s.%s copies the argument before returning but receives %d throwaway standard-library Clone layer(s); write the original value directly", receiver, method, len(chain.calls)),
			}
			if fix, ok := fixDeletedCallScaffoldingPaths(pass, file, chain.paths, "remove clones before the copying standard-library writer", chain.spans...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5085Match(pass *analysis.Pass, call *ast.CallExpr) (typedUnaryCallChain, string, string, bool) {
	if len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return typedUnaryCallChain{}, "", "", false
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || signature.Recv() == nil || function.Pkg() == nil {
		return typedUnaryCallChain{}, "", "", false
	}
	receiverPackage, receiverName, ok := ps5085Receiver(signature)
	if !ok || function.Pkg().Path() != receiverPackage {
		return typedUnaryCallChain{}, "", "", false
	}
	method := function.Name()
	var allowed func(string, string) bool
	switch {
	case method == "Write" && (receiverPackage == "bytes" && receiverName == "Buffer" || receiverPackage == "strings" && receiverName == "Builder"):
		allowed = isTypedSliceStdlibClone
	case method == "WriteString" && (receiverPackage == "bytes" && receiverName == "Buffer" || receiverPackage == "strings" && receiverName == "Builder"):
		allowed = isTypedStringStdlibClone
	default:
		return typedUnaryCallChain{}, "", "", false
	}
	chain, ok := matchTypedUnaryPackageCallChain(pass, call.Args[0], allowed)
	return chain, receiverPackage + "." + receiverName, method, ok
}

func ps5085Receiver(signature *types.Signature) (string, string, bool) {
	receiver := types.Unalias(signature.Recv().Type())
	if pointer, ok := receiver.(*types.Pointer); ok {
		receiver = types.Unalias(pointer.Elem())
	}
	named, ok := receiver.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return "", "", false
	}
	return named.Obj().Pkg().Path(), named.Obj().Name(), true
}
