package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS4102 reports a full-width encoding/binary decode whose only consumer
// selects bits from the decode's final required byte.
var PS4102 = register(&lint.Check{
	ID:       "PS4102",
	Category: "vector",
	Slug:     "full-width-endian-subfield",
	Level:    lint.LevelAggressive,
	Doc: lint.Documentation{
		Title: "a full-width endian decode used only for a final-byte subfield",
		Text: `A fixed encoding/binary Uint16, Uint32, or Uint64 decode loads every
byte even when its only consumer selects a field wholly contained in the last
byte the decode requires. Load that byte directly and shift or mask it instead.

The candidate rewrite is deliberately limited to the final required byte. An
index at width-1 has the same minimum-length check as the original decode;
choosing an earlier byte could silently remove a short-slice panic. The original
slice expression, including explicit high and max bounds, remains in place, so
its evaluation count, order, side effects, and slice-bound panics are retained.
An explicit uint16, uint32, or uint64 conversion preserves the decode result's
static type; verify that predeclared conversion name is not shadowed at the
rewrite site.

The decode must be a Uint16/32/64 method on the actual standard-library
encoding/binary.LittleEndian or BigEndian value. Import aliases work; a runtime
ByteOrder, NativeEndian, and same-named user methods do not. The supported
consumer is a constant right shift, optionally followed by a contiguous low-bit
mask, or a contiguous low-bit mask alone. For a local binding, the decode must
be its sole initializer and the variable must have exactly one use. Other word
uses, dynamic shifts, cross-byte fields, and direct unsliced buffers stay silent.

This check supersedes PS4001's bulk-copy advice for the exact recognized
subfield shape: heterogeneous records need the one metadata byte, not an unsafe
same-layout alias of the surrounding record.

The isolated Go benchmark is parity on the tested compiler and machine, so this
is advisory rather than an automatic fix. Apply it only where a benchmark of the
real record walk shows a win; a narrower load in assembly is not itself a timing
result.`,
		Before: `word := binary.LittleEndian.Uint32(block[66+group*4:])
scale := word >> 28`,
		After: `word := uint32((block[66+group*4:])[3])
scale := word >> 4`,
		MeasuredWin: `BenchmarkPS4102 (Apple M2 Pro, go1.27.0; six alternating
fresh-process pairs, 2s per arm): the little-endian full decode measured 0.4769
ns/op median versus 0.4776 ns/op for the final-byte form. A data-dependent
big-endian pair measured 2.997 ns/op median versus 2.9525 ns/op, but the
distributions overlapped and paired outcomes included a regression and parity.
All arms measured 0 B/op and 0 allocs/op. Neither result proves a generic timing
win. Compiler output narrows the load and removes the big-endian byte reversal;
benchmark the real memory-access context before changing code.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS4102",
		Doc:  "full-width encoding/binary decode used only for a final-byte subfield",
		Run:  runPS4102,
	},
})

type ps4102Consumer struct {
	base  ast.Expr
	shift uint64
	mask  *uint64
}

type ps4102Match struct {
	call  *ast.CallExpr
	order string
	width int
}

func runPS4102(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		matches := ps4102FileMatches(pass, file)
		for _, match := range matches {
			diagnostic := analysis.Diagnostic{
				Pos: match.call.Pos(),
				End: match.call.End(),
				Message: "binary." + match.order + ".Uint" + strconv.Itoa(match.width) +
					" decodes " + strconv.Itoa(match.width/8) +
					" bytes although its only consumer selects a field from the final required byte; benchmark a direct byte load in context while retaining the decode's short-slice boundary",
			}
			pass.Report(diagnostic)
		}
	}
	return nil, nil
}

// ps4102RecognizedCalls is shared with PS4001 so its bulk-copy advisory does
// not conflict with this narrower exact remedy.
func ps4102RecognizedCalls(pass *analysis.Pass, file *ast.File) map[*ast.CallExpr]bool {
	matches := ps4102FileMatches(pass, file)
	result := make(map[*ast.CallExpr]bool, len(matches))
	for _, match := range matches {
		result[match.call] = true
	}
	return result
}

func ps4102FileMatches(pass *analysis.Pass, file *ast.File) []ps4102Match {
	var inline []ps4102Match
	consumers := make(map[types.Object]ps4102Consumer)

	ast.Inspect(file, func(node ast.Node) bool {
		expr, ok := node.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		consumer, ok := ps4102ParseConsumer(pass, expr)
		if !ok {
			// An explicit mask is part of the consumer contract. If that mask is
			// dynamic or non-contiguous, do not look through it and accidentally
			// report the nested shift as though it were the whole consumer.
			if expr.Op == token.AND && ps4102PotentialBase(pass, expr.X) {
				return false
			}
			return true
		}
		switch base := ps2110Unparen(consumer.base).(type) {
		case *ast.CallExpr:
			if match, ok := ps4102CompleteMatch(pass, consumer, base); ok {
				inline = append(inline, match)
				return false
			}
		case *ast.Ident:
			if object := pass.TypesInfo.Uses[base]; object != nil {
				consumers[object] = consumer
				return false
			}
		}
		return true
	})

	// A short-declared local cannot be referenced from another source file.
	// Count this file's identifiers once, without re-walking package-wide type
	// information for every file and without inheriting the matcher walk's mask
	// pruning (an extra use inside a rejected mask must still disqualify a local).
	useCount := make(map[types.Object]int)
	ast.Inspect(file, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if object := pass.TypesInfo.Uses[identifier]; object != nil {
			useCount[object]++
		}
		return true
	})

	matches := inline
	ast.Inspect(file, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		identifier, ok := assign.Lhs[0].(*ast.Ident)
		if !ok || identifier.Name == "_" {
			return true
		}
		object := pass.TypesInfo.Defs[identifier]
		consumer, ok := consumers[object]
		if !ok || useCount[object] != 1 {
			return true
		}
		call, ok := ps2110Unparen(assign.Rhs[0]).(*ast.CallExpr)
		if !ok {
			return true
		}
		if match, ok := ps4102CompleteMatch(pass, consumer, call); ok {
			matches = append(matches, match)
		}
		return true
	})
	return matches
}

func ps4102ParseConsumer(pass *analysis.Pass, expr *ast.BinaryExpr) (ps4102Consumer, bool) {
	consumer := ps4102Consumer{}
	value := ast.Expr(expr)
	if expr.Op == token.AND {
		mask, ok := ps4102UintConstant(pass, expr.Y)
		if !ok || mask == 0 || mask&(mask+1) != 0 {
			return ps4102Consumer{}, false
		}
		consumer.mask = &mask
		value = ps2110Unparen(expr.X)
	}

	if shift, ok := ps2110Unparen(value).(*ast.BinaryExpr); ok && shift.Op == token.SHR {
		amount, ok := ps4102UintConstant(pass, shift.Y)
		if !ok {
			return ps4102Consumer{}, false
		}
		consumer.base = shift.X
		consumer.shift = amount
		return consumer, true
	}
	if expr.Op == token.AND {
		consumer.base = value
		return consumer, true
	}
	return ps4102Consumer{}, false
}

func ps4102PotentialBase(pass *analysis.Pass, expr ast.Expr) bool {
	if shift, ok := ps2110Unparen(expr).(*ast.BinaryExpr); ok && shift.Op == token.SHR {
		expr = shift.X
	}
	switch base := ps2110Unparen(expr).(type) {
	case *ast.CallExpr:
		_, _, ok := ps4102Decode(pass, base)
		return ok
	case *ast.Ident:
		return true
	default:
		return false
	}
}

func ps4102CompleteMatch(pass *analysis.Pass, consumer ps4102Consumer, call *ast.CallExpr) (ps4102Match, bool) {
	order, width, ok := ps4102Decode(pass, call)
	if !ok {
		return ps4102Match{}, false
	}
	widthBits := uint64(width)
	var byteShift uint64
	switch order {
	case "LittleEndian":
		start := widthBits - 8
		if consumer.shift < start || consumer.shift >= widthBits {
			return ps4102Match{}, false
		}
		byteShift = consumer.shift - start
	case "BigEndian":
		if consumer.mask == nil || consumer.shift >= 8 {
			return ps4102Match{}, false
		}
		byteShift = consumer.shift
		available := uint64(8) - byteShift
		if *consumer.mask >= uint64(1)<<available {
			return ps4102Match{}, false
		}
	default:
		return ps4102Match{}, false
	}
	if consumer.mask != nil {
		available := uint64(8) - byteShift
		if *consumer.mask >= uint64(1)<<available {
			return ps4102Match{}, false
		}
	}
	return ps4102Match{
		call:  call,
		order: order,
		width: width,
	}, true
}

func ps4102Decode(pass *analysis.Pass, call *ast.CallExpr) (order string, width int, ok bool) {
	if len(call.Args) != 1 {
		return "", 0, false
	}
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return "", 0, false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Obj() == nil || selection.Obj().Pkg() == nil ||
		selection.Obj().Pkg().Path() != "encoding/binary" {
		return "", 0, false
	}
	switch selection.Obj().Name() {
	case "Uint16":
		width = 16
	case "Uint32":
		width = 32
	case "Uint64":
		width = 64
	default:
		return "", 0, false
	}

	receiver, ok := ps2110Unparen(selector.X).(*ast.SelectorExpr)
	if !ok {
		return "", 0, false
	}
	receiverObject := pass.TypesInfo.Uses[receiver.Sel]
	variable, ok := receiverObject.(*types.Var)
	if !ok || variable.Pkg() == nil || variable.Pkg().Path() != "encoding/binary" {
		return "", 0, false
	}
	if variable.Name() != "LittleEndian" && variable.Name() != "BigEndian" {
		return "", 0, false
	}

	if _, ok := ps2110Unparen(call.Args[0]).(*ast.SliceExpr); !ok {
		return "", 0, false
	}
	return variable.Name(), width, true
}

func ps4102UintConstant(pass *analysis.Pass, expr ast.Expr) (uint64, bool) {
	value, ok := pass.TypesInfo.Types[ps2110Unparen(expr)]
	if !ok || value.Value == nil || value.Value.Kind() != constant.Int {
		return 0, false
	}
	result, exact := constant.Uint64Val(value.Value)
	return result, exact
}
