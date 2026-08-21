package checks

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5094 removes bytes.Buffer values constructed only to extract their initial
// bytes/string, composing terminal Clone removal where the extraction already
// supplies the required independent copy.
var PS5094 = register(&lint.Check{
	ID:       "PS5094",
	Category: "alloc",
	Slug:     "ephemeral-buffer-extraction-chain",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "an ephemeral bytes.Buffer is constructed only to extract its initial value",
		Text: `A freshly constructed bytes.Buffer has offset zero. Constructing one
only to immediately extract the same initial bytes or their string copy adds a
wrapper without changing the extraction contract:

  bytes.NewBuffer(data).Bytes()             -> data
  bytes.NewBuffer(data).String()            -> string(data)
  bytes.NewBufferString(text).Bytes()       -> []byte(text)

The constructor already requires an exact []byte argument (an explicit
conversion from a named slice remains in place), so retaining that argument
preserves the Bytes method's predeclared []byte result type. It also retains the
input slice's pointer, length, capacity, and nilness. NewBuffer(...).String and
string(data) both make the independent byte-to-string value. Finally,
NewBufferString and []byte(text) perform the same string-to-byte copy and
predeclared []byte result type.

The typed method-on-constructor matcher pins bytes.NewBuffer or
bytes.NewBufferString and the exact (*bytes.Buffer).Bytes/String method through
go/types. Aliases work; dot imports, function-valued constructors, user
lookalikes, stored/mutated buffers, arguments of the wrong shape, and promoted
methods stay untouched.

Terminal copy ownership composes redundant Clone removal in one fix:

  bytes.NewBuffer(bytes.Clone(slices.Clone(data))).String() -> string(data)
  bytes.NewBufferString(strings.Clone(text)).Bytes()         -> []byte(text)

A Clone passed to NewBuffer(...).Bytes is RETAINED because that returned slice
may intentionally be an independent snapshot. It becomes removable only when
an immediately surrounding len consumes the bytes without retaining them:

  len(bytes.NewBuffer(bytes.Clone(data)).Bytes()) -> len(data)
  len(bytes.NewBufferString(strings.Clone(s)).Bytes()) -> len(s)

NewBufferString(s).String() is deliberately excluded. Although its bytes are
equal to s, the byte conversion followed by String can detach a short string
from a much larger backing allocation; returning s directly could extend that
allocation's lifetime. Direct NewBuffer(...).String rewrites are limited to
assignment, declaration, and return value positions so established
buffer-String observer rules retain ownership of comparisons, indexing, and
other specialized outer consumers.

The rewrite is BIT-IDENTICAL for race-free ordinary Go programs. Inputs are
evaluated once in the same order; nil/empty slices, capacity and aliasing for
Bytes, invalid UTF-8, named inputs, panics, and result types are preserved.
An untyped nil String input is retained as string([]byte(nil)), avoiding the
invalid string(nil) conversion while preserving the constructor's implicit
[]byte parameter conversion. Fixes that inject len, string, or byte are
withheld when that predeclared identifier is shadowed.
Bare/go/defer call-only contexts receive no conversion fix because a type
conversion is not a legal call statement. Comments keep the finding advisory.
Shared import liveness removes every newly orphaned bytes/strings/slices import,
and nested-Clone ownership reaches the clean fixed point in one -fix pass.`,
		Before: `snapshot := bytes.NewBufferString(strings.Clone(text)).Bytes()
encoded := bytes.NewBuffer(bytes.Clone(data)).String()`,
		After: `snapshot := []byte(text)
encoded := string(data)`,
		MeasuredWin: `On an Apple M2 Pro, benchmarks/ps5094_test.go reduced the
copy-preserving bytes.NewBuffer(bytes.Clone(data)).String() chain from a median
7,315 ns/op, 147,456 B/op, and 2 allocations to 3,558 ns/op, 73,728 B/op, and
1 allocation: about 2.06x faster while removing one 73,728-byte full-input
clone and retaining the required final string copy.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5094",
		Doc:  "ephemeral bytes.Buffer constructor is immediately reduced back to its initial bytes or string",
		Run:  runPS5094,
	},
})

type ps5094Match struct {
	call        typedMethodConstructorCall
	root        *ast.CallExpr
	kept        ast.Expr
	paths       []string
	clone       typedUnaryCallChain
	cloneOK     bool
	constructor string
	method      string
	prefix      []byte
	suffix      []byte
	required    []string
	fixable     bool
}

func runPS5094(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		astutil.WithStack(file, func(node ast.Node, stack []ast.Node) bool {
			outer, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			match, ok := ps5094ExtractionMatch(pass, outer, stack)
			if !ok {
				return true
			}
			cloneLayers := 0
			if match.cloneOK {
				cloneLayers = len(match.clone.calls)
			}
			diagnostic := analysis.Diagnostic{
				Pos:     match.root.Pos(),
				End:     match.root.End(),
				Message: fmt.Sprintf("bytes.%s(...).%s constructs an ephemeral Buffer only to extract its initial value and carries %d throwaway Clone layer(s); use the equivalent builtin conversion or length directly", match.constructor, match.method, cloneLayers),
			}
			if match.fixable && ps5094ReplacementNamesAvailable(pass, &match) {
				if fix, ok := fixReplacedCallScaffoldingPaths(pass, file, match.paths, "replace ephemeral bytes.Buffer extraction chain",
					analysis.TextEdit{Pos: match.root.Pos(), End: match.kept.Pos(), NewText: match.prefix},
					analysis.TextEdit{Pos: match.kept.End(), End: match.root.End(), NewText: match.suffix},
				); ok {
					diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
				}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5094ExtractionMatch(pass *analysis.Pass, outer *ast.CallExpr, stack []ast.Node) (ps5094Match, bool) {
	chain, ok := matchTypedMethodOnPackageConstructor(pass, outer)
	if !ok || len(outer.Args) != 0 || len(chain.constructorCall.Args) != 1 ||
		chain.constructor.Pkg().Path() != "bytes" || chain.method.Pkg().Path() != "bytes" ||
		!typedReceiverNamed(chain.methodSignature, "bytes", "Buffer") {
		return ps5094Match{}, false
	}
	constructor, method := chain.constructor.Name(), chain.method.Name()
	if constructor != "NewBuffer" && constructor != "NewBufferString" || method != "Bytes" && method != "String" {
		return ps5094Match{}, false
	}
	// NewBufferString(...).String may be an intentional backing-storage
	// detachment boundary. Its equal bytes are not enough to justify removing it.
	if constructor == "NewBufferString" && method == "String" {
		return ps5094Match{}, false
	}
	// Specialized outer buffer.String observers own comparisons, indexing,
	// and predicates. Limit the general String extraction to direct value sites.
	if method == "String" && !ps5094DirectValueContext(outer, stack) {
		return ps5094Match{}, false
	}

	root := outer
	prefix, suffix := []byte("[]byte("), []byte(")")
	required := []string{"byte"}
	lenRoot := false
	if method == "String" {
		prefix = []byte("string(")
		required = []string{"string"}
	} else if parent := ps5094SemanticParent(stack); parent != nil {
		if call, ok := parent.(*ast.CallExpr); ok && typedBuiltinName(pass, call.Fun, "len") && len(call.Args) == 1 &&
			!call.Ellipsis.IsValid() && ps2110Unparen(call.Args[0]) == outer {
			root, prefix, required, lenRoot = call, []byte("len("), []string{"len"}, true
		}
	}

	kept := chain.constructorCall.Args[0]
	paths := []string{"bytes"}
	var clone typedUnaryCallChain
	cloneOK := false
	if constructor == "NewBufferString" || method == "String" || lenRoot {
		allowed := isTypedSliceStdlibClone
		if constructor == "NewBufferString" {
			allowed = isTypedStringStdlibClone
		}
		clone, cloneOK = matchTypedUnaryPackageCallChain(pass, kept, allowed)
		if cloneOK {
			kept = clone.base
			paths = append(paths, clone.paths...)
		}
	}
	if constructor == "NewBuffer" && method == "Bytes" && !lenRoot &&
		types.Identical(pass.TypesInfo.TypeOf(kept), types.NewSlice(types.Typ[types.Uint8])) {
		prefix, suffix, required = nil, nil, nil
	}
	if constructor == "NewBuffer" && method == "String" && ps5092Nil(pass, kept) {
		prefix, suffix = []byte("string([]byte("), []byte("))")
		required = []string{"string", "byte"}
	}

	return ps5094Match{
		call: chain, root: root, kept: kept, paths: paths, clone: clone, cloneOK: cloneOK,
		constructor: constructor, method: method, prefix: prefix, suffix: suffix, required: required,
		fixable: lenRoot || !ps5094CallOnlyContext(outer, stack),
	}, true
}

func ps5094ReplacementNamesAvailable(pass *analysis.Pass, match *ps5094Match) bool {
	for _, name := range match.required {
		if !predeclaredInScope(pass, match.root.Pos(), name) {
			return false
		}
	}
	return true
}

func ps5094SemanticParent(stack []ast.Node) ast.Node {
	for index := len(stack) - 1; index >= 0; index-- {
		if _, parenthesis := stack[index].(*ast.ParenExpr); parenthesis {
			continue
		}
		return stack[index]
	}
	return nil
}

func ps5094DirectValueContext(call *ast.CallExpr, stack []ast.Node) bool {
	parent := ps5094SemanticParent(stack)
	switch value := parent.(type) {
	case *ast.AssignStmt:
		for _, expression := range value.Rhs {
			if ps2110Unparen(expression) == call {
				return true
			}
		}
	case *ast.ReturnStmt:
		for _, expression := range value.Results {
			if ps2110Unparen(expression) == call {
				return true
			}
		}
	case *ast.ValueSpec:
		for _, expression := range value.Values {
			if ps2110Unparen(expression) == call {
				return true
			}
		}
	}
	return false
}

func ps5094CallOnlyContext(call *ast.CallExpr, stack []ast.Node) bool {
	parent := ps5094SemanticParent(stack)
	switch value := parent.(type) {
	case *ast.ExprStmt:
		return ps2110Unparen(value.X) == call
	case *ast.GoStmt:
		return value.Call == call
	case *ast.DeferStmt:
		return value.Call == call
	default:
		return false
	}
}
