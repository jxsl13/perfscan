package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5089 removes Clone chains before exact synchronous os write operations.
var PS5089 = register(&lint.Check{
	ID:       "PS5089",
	Category: "alloc",
	Slug:     "clone-fed-synchronous-os-write",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a synchronous os write receives a throwaway Clone",
		Text: `os.File.Write, WriteAt, and WriteString consume their input before
returning. os.WriteFile likewise opens, writes, and closes the file within the
call. A Clone immediately around the data is therefore allocated and copied
only to be discarded after the synchronous write:

  file.Write(bytes.Clone(data))
  file.WriteAt(slices.Clone(bytes.Clone(data)), offset)
  file.WriteString(strings.Clone(text))
  os.WriteFile(name, slices.Clone(data), mode)

This check resolves the exact *os.File method or os.WriteFile package function
and every bytes.Clone/slices.Clone/strings.Clone layer through go/types.
Aliases, explicit generic instantiations, promoted *os.File methods, and
arbitrary heterogeneous clone depth work. Interface dispatch, arbitrary
io.Writer implementations, method values/expressions, same-named user methods,
dot-imported Clone calls, and type-changing wrappers stay untouched.

Argument evaluation is part of the snapshot proof. A Clone is removed only
when every later argument contains no arbitrary call or channel receive that
could mutate the original before the write starts. Thus
file.WriteAt(bytes.Clone(data), mutateAndOffset(data)) and an os.WriteFile mode
expression that mutates data retain their pre-mutation snapshots. Conversions,
len/cap, and recursively inspected stdlib Clone calls remain eligible.

For race-free safe Go programs the rewrite preserves bytes written, partial
write counts, errors, file offsets, WriteAt positioning, file creation/mode
behavior, panics, and evaluation order. The base expression is still evaluated
once in the same argument position; only the redundant allocation/copy is
removed. Comments keep the finding advisory. The shared typed call-chain and
import-liveness editor removes all Clone layers and newly unused imports in one
fix, while terminal ownership prevents overlapping nested-Clone diagnostics.`,
		Before: `n, err := file.WriteAt(
	slices.Clone(bytes.Clone(data)),
	offset,
)`,
		After: `n, err := file.WriteAt(data, offset)`,
		MeasuredWin: `On Apple M2 Pro writing a 68,275-byte slice to os.DevNull
(median of five 100-iteration runs), file.Write(bytes.Clone(data)) measured
8,684 ns/op, 73,728 B/op, 1 alloc/op versus file.Write(data) at 892.5 ns/op,
0 B/op, 0 allocs/op: 9.73x faster and 89.7% less time while eliminating the
full-size copy.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5089",
		Doc:  "synchronous os write consumes a throwaway Clone",
		Run:  runPS5089,
	},
})

func runPS5089(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			chain, consumer, matched := ps5089Match(pass, call)
			if !matched {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: fmt.Sprintf("%s consumes its input synchronously but receives %d throwaway standard-library Clone layer(s); write the original value directly", consumer, len(chain.calls)),
			}
			if fix, ok := fixDeletedCallScaffoldingPaths(pass, file, chain.paths, "remove clones before synchronous os write", chain.spans...); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5089Match(pass *analysis.Pass, call *ast.CallExpr) (typedUnaryCallChain, string, bool) {
	if call.Ellipsis.IsValid() {
		return typedUnaryCallChain{}, "", false
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || function.Pkg() == nil || function.Pkg().Path() != "os" {
		return typedUnaryCallChain{}, "", false
	}
	index := -1
	consumer := ""
	var allowed func(string, string) bool
	if signature.Recv() == nil {
		if function.Name() != "WriteFile" || len(call.Args) != 3 {
			return typedUnaryCallChain{}, "", false
		}
		index, consumer, allowed = 1, "os.WriteFile", isTypedSliceStdlibClone
	} else {
		receiverPackage, receiverName, ok := ps5085Receiver(signature)
		if !ok || receiverPackage != "os" || receiverName != "File" {
			return typedUnaryCallChain{}, "", false
		}
		switch function.Name() {
		case "Write":
			if len(call.Args) != 1 {
				return typedUnaryCallChain{}, "", false
			}
			index, consumer, allowed = 0, "os.File.Write", isTypedSliceStdlibClone
		case "WriteAt":
			if len(call.Args) != 2 {
				return typedUnaryCallChain{}, "", false
			}
			index, consumer, allowed = 0, "os.File.WriteAt", isTypedSliceStdlibClone
		case "WriteString":
			if len(call.Args) != 1 {
				return typedUnaryCallChain{}, "", false
			}
			index, consumer, allowed = 0, "os.File.WriteString", isTypedStringStdlibClone
		default:
			return typedUnaryCallChain{}, "", false
		}
	}
	if !cloneRemovalLaterArgumentsStable(pass, call, index) {
		return typedUnaryCallChain{}, "", false
	}
	chain, ok := matchTypedUnaryPackageCallChain(pass, call.Args[index], allowed)
	return chain, consumer, ok
}
