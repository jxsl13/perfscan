package checks

import (
	"bytes"
	"go/ast"
	"go/constant"
	"go/format"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6083 implements owner issue #809. It recognizes a deliberately small
// integer-to-float decode whose complete value domain can be materialized as
// an exact, fixed local lookup table.
var PS6083 = register(&lint.Check{
	ID:       "PS6083",
	Category: "arith",
	Slug:     "small-masked-integer-float-lookup",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a small masked integer domain is repeatedly converted to float",
		Text: `A packed-data loop can extract a small unsigned field, apply a fixed
affine decode such as 2*s+1, and convert the result to float32 or float64 on
every iteration. A fixed local table indexed by the same masked field removes
that repeated integer arithmetic and conversion while preserving the exact
destination-typed value.

PS6083 intentionally proves only a compact mechanical subset. The loop must be
a material integer range, slice/array/string range, or canonical zero-based
counted loop. The converted value must be a non-overflowing a*s+b expression
with nonnegative integer constants, where s is either a contiguous low-bit
mask used directly or one immutable local short-declaration initialized from
that mask in the same loop. The masked source must depend on the typed loop
index, have a fixed-width uint8/16/32/64 type, and contain no calls or receives.
uint and uintptr, signed integers, type parameters, casts inside the integer
expression, dynamic or non-contiguous masks, domains larger than 16 entries,
integer overflow, and values not exactly representable in the destination
float type remain silent.

The fix declares one [N]float32 or [N]float64 value immediately before the
loop and replaces only the conversion with a lookup indexed by the unchanged
mask or proven alias. Every entry is generated in the original destination
type. The table is at most 64 bytes for float32 or 128 bytes for float64. Its
literal is constant, so moving it onto a zero-trip path cannot add calls,
side effects, bounds checks, or panics. The original packed-source expression
is still evaluated exactly once at the conversion site. Addressed, reassigned,
captured, or pointer-receiver-exposed aliases are rejected.
Comments inside the conversion, labels, gotos, candidate bodies containing
nested loops, and ambiguous scopes suppress the edit.

This is a source candidate, not a universal speedup. Current compilers may
already simplify some integer conversions, while a local table can add loads
or initialization traffic. Benchmark the real packed caller and inspect its
optimized code before retaining the rewrite. Broader enum decoders, signed or
wrapping arithmetic, and arbitrary expression interpreters are outside this
check.`,
		Before: `for index := range 8 {
	output[index] *= float32(2*((packed>>(3*index))&7) + 1)
}`,
		After: `psFloatLookup := [8]float32{1, 3, 5, 7, 9, 11, 13, 15}
for index := range 8 {
	output[index] *= psFloatLookup[(packed>>(3*index))&7]
}`,
		MeasuredWin: `Owner issue #809 measured the table form after the larger
IQ1_S grid optimization on Apple M2 Pro. Five alternating fresh-process pairs
at 500 ms per benchmark improved the K=4096 leaf from 691.5 ns to 680.6 ns
(-1.58%, p=0.016) and M1/N64/K1024 from 11.56 us to 11.42 us (-1.18%,
p=0.008); M1/N4096/K1024 was statistically neutral (p=0.151). Scalar
controls, bytes, and allocations were unchanged. The repository benchmark is
the exact bounded source shape. On the same Apple M2 Pro with Go 1.27.0
(darwin/arm64, GOMAXPROCS=3), six alternating fresh-process pairs at two
seconds per arm measured a 5.1515 ns/op median before and 4.950 ns/op after
(-3.91%, all six pairs in the same direction), with zero bytes and allocations
for both. These results are specific to the measured compiler and machine;
real packed callers still need their own benchmark.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6083",
		Doc:  "small masked unsigned integer domain repeatedly converted to float",
		Run:  runPS6083,
	},
})

const ps6083MaxEntries = 16

type ps6083Loop struct {
	statement ast.Stmt
	body      *ast.BlockStmt
	index     types.Object
}

type ps6083Alias struct {
	object types.Object
	mask   *ast.BinaryExpr
	bits   int
	domain uint64
}

type ps6083Match struct {
	conversion  *ast.CallExpr
	index       ast.Expr
	loop        ps6083Loop
	destination string
	coefficient uint64
	addend      uint64
	domain      uint64
}

type ps6083Context struct {
	body      *ast.BlockStmt
	parents   map[ast.Node]ast.Node
	nodes     []ast.Node
	usedNames map[string]bool
	hasGoto   bool
	comments  []*ast.CommentGroup
}

func runPS6083(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			context := ps6083PrepareContext(function, file)
			for _, node := range context.nodes {
				loop, ok := ps6083LoopHeader(pass, node)
				if !ok || ps6083ObjectWritten(pass, loop.body, loop.index) || !ps6083StraightLoop(loop.body) {
					continue
				}
				aliases := ps6083Aliases(pass, context, loop)
				match, ok := ps6083LoopMatch(pass, context, loop, aliases)
				if !ok {
					continue
				}
				fix, ok := ps6083Fix(pass, context, match)
				if !ok {
					continue
				}
				pass.Report(analysis.Diagnostic{
					Pos: match.conversion.Pos(),
					End: match.conversion.End(),
					Message: "this " + match.destination + " conversion maps exactly " + strconv.FormatUint(match.domain, 10) +
						" masked unsigned values inside the loop; use an exact destination-typed local lookup table and benchmark the packed caller",
					SuggestedFixes: []analysis.SuggestedFix{fix},
				})
			}
		}
	}
	return nil, nil
}

func ps6083PrepareContext(function *ast.FuncDecl, file *ast.File) *ps6083Context {
	context := &ps6083Context{
		body:      function.Body,
		parents:   make(map[ast.Node]ast.Node),
		usedNames: make(map[string]bool),
		comments:  file.Comments,
	}
	ast.Inspect(function, func(node ast.Node) bool {
		if identifier, ok := node.(*ast.Ident); ok {
			context.usedNames[identifier.Name] = true
		}
		return true
	})
	var stack []ast.Node
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) != 0 {
			context.parents[node] = stack[len(stack)-1]
		}
		context.nodes = append(context.nodes, node)
		if _, literal := node.(*ast.FuncLit); literal {
			return false
		}
		if branch, ok := node.(*ast.BranchStmt); ok && branch.Tok == token.GOTO {
			context.hasGoto = true
		}
		stack = append(stack, node)
		return true
	})
	return context
}

func ps6083LoopHeader(pass *analysis.Pass, node ast.Node) (ps6083Loop, bool) {
	switch loop := node.(type) {
	case *ast.RangeStmt:
		identifier, ok := loop.Key.(*ast.Ident)
		if !ok || identifier.Name == "_" || loop.Value != nil || !ps6083MaterialRange(pass, loop.X) {
			return ps6083Loop{}, false
		}
		object := pass.TypesInfo.Defs[identifier]
		return ps6083Loop{statement: loop, body: loop.Body, index: object}, object != nil
	case *ast.ForStmt:
		initialization, ok := loop.Init.(*ast.AssignStmt)
		if !ok || initialization.Tok != token.DEFINE || len(initialization.Lhs) != 1 || len(initialization.Rhs) != 1 {
			return ps6083Loop{}, false
		}
		identifier, ok := initialization.Lhs[0].(*ast.Ident)
		if !ok || !ps6083IntegerConstant(pass, initialization.Rhs[0], 0) {
			return ps6083Loop{}, false
		}
		object := pass.TypesInfo.Defs[identifier]
		condition, ok := loop.Cond.(*ast.BinaryExpr)
		if object == nil || !ok || condition.Op != token.LSS || !ps6083IsObject(pass, condition.X, object) ||
			ps6083Mentions(pass, condition.Y, object) || !ps6083MaterialBound(pass, condition.Y) {
			return ps6083Loop{}, false
		}
		post, ok := loop.Post.(*ast.IncDecStmt)
		if !ok || post.Tok != token.INC || !ps6083IsObject(pass, post.X, object) {
			return ps6083Loop{}, false
		}
		return ps6083Loop{statement: loop, body: loop.Body, index: object}, true
	}
	return ps6083Loop{}, false
}

func ps6083MaterialRange(pass *analysis.Pass, expression ast.Expr) bool {
	if value := pass.TypesInfo.Types[expression].Value; value != nil {
		bound, ok := constant.Int64Val(value)
		return ok && bound >= 4
	}
	typeOf := pass.TypesInfo.TypeOf(expression)
	if typeOf == nil {
		return false
	}
	switch underlying := types.Unalias(typeOf).Underlying().(type) {
	case *types.Array:
		return underlying.Len() >= 4
	case *types.Slice:
		return true
	case *types.Basic:
		return underlying.Info()&(types.IsInteger|types.IsString) != 0
	}
	return false
}

func ps6083MaterialBound(pass *analysis.Pass, expression ast.Expr) bool {
	value := pass.TypesInfo.Types[expression].Value
	if value == nil {
		return true
	}
	bound, ok := constant.Int64Val(value)
	return ok && bound >= 4
}

func ps6083Aliases(pass *analysis.Pass, context *ps6083Context, loop ps6083Loop) map[types.Object]ps6083Alias {
	aliases := make(map[types.Object]ps6083Alias)
	ps6083InspectDirect(loop.body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 ||
			!ps6083Unconditional(assignment.Rhs[0], loop, context.parents) {
			return true
		}
		identifier, ok := assignment.Lhs[0].(*ast.Ident)
		if !ok || identifier.Name == "_" {
			return true
		}
		object := pass.TypesInfo.Defs[identifier]
		mask, bits, domain, ok := ps6083DirectDomain(pass, assignment.Rhs[0], loop.index)
		if object != nil && ok && ps6083AliasSafe(pass, context, object) {
			aliases[object] = ps6083Alias{object: object, mask: mask, bits: bits, domain: domain}
		}
		return true
	})
	return aliases
}

func ps6083LoopMatch(pass *analysis.Pass, context *ps6083Context, loop ps6083Loop, aliases map[types.Object]ps6083Alias) (ps6083Match, bool) {
	var result ps6083Match
	found := false
	ps6083InspectDirect(loop.body, func(node ast.Node) bool {
		if found {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || !ps6083Unconditional(call, loop, context.parents) {
			return true
		}
		destination, exactLimit, ok := ps6083Destination(pass, call)
		if !ok || ps6083RejectedSource(call.Args[0]) {
			return true
		}
		coefficient, addend, domainExpression, integerType, bits, domain, ok := ps6083Affine(pass, call.Args[0], loop.index, aliases)
		if !ok || !types.Identical(pass.TypesInfo.TypeOf(call.Args[0]), integerType) {
			return true
		}
		maximum, ok := ps6083Maximum(coefficient, addend, domain, exactLimit)
		if !ok || maximum > ps6083UnsignedMaximum(bits) {
			return true
		}
		result = ps6083Match{
			conversion: call, index: domainExpression, loop: loop, destination: destination,
			coefficient: coefficient, addend: addend, domain: domain,
		}
		found = true
		return false
	})
	return result, found
}

func ps6083Affine(pass *analysis.Pass, expression ast.Expr, loopIndex types.Object, aliases map[types.Object]ps6083Alias) (uint64, uint64, ast.Expr, types.Type, int, uint64, bool) {
	coefficient, addend := uint64(1), uint64(0)
	core := ps6083Unparen(expression)
	if addition, ok := core.(*ast.BinaryExpr); ok && addition.Op == token.ADD {
		if value, ok := ps6083UintConstant(pass, addition.X); ok {
			addend, core = value, ps6083Unparen(addition.Y)
		} else if value, ok := ps6083UintConstant(pass, addition.Y); ok {
			addend, core = value, ps6083Unparen(addition.X)
		} else {
			return 0, 0, nil, nil, 0, 0, false
		}
	}
	if multiplication, ok := core.(*ast.BinaryExpr); ok && multiplication.Op == token.MUL {
		if value, ok := ps6083UintConstant(pass, multiplication.X); ok {
			coefficient, core = value, ps6083Unparen(multiplication.Y)
		} else if value, ok := ps6083UintConstant(pass, multiplication.Y); ok {
			coefficient, core = value, ps6083Unparen(multiplication.X)
		} else {
			return 0, 0, nil, nil, 0, 0, false
		}
	}
	if coefficient == 0 {
		return 0, 0, nil, nil, 0, 0, false
	}
	if mask, bits, domain, ok := ps6083DirectDomain(pass, core, loopIndex); ok {
		return coefficient, addend, mask, pass.TypesInfo.TypeOf(mask), bits, domain, true
	}
	identifier, ok := core.(*ast.Ident)
	if !ok {
		return 0, 0, nil, nil, 0, 0, false
	}
	alias, ok := aliases[pass.TypesInfo.Uses[identifier]]
	if !ok || alias.object.Pos() >= expression.Pos() {
		return 0, 0, nil, nil, 0, 0, false
	}
	return coefficient, addend, identifier, alias.object.Type(), alias.bits, alias.domain, true
}

func ps6083DirectDomain(pass *analysis.Pass, expression ast.Expr, loopIndex types.Object) (*ast.BinaryExpr, int, uint64, bool) {
	maskExpression, ok := ps6083Unparen(expression).(*ast.BinaryExpr)
	if !ok || maskExpression.Op != token.AND {
		return nil, 0, 0, false
	}
	source, maskValue := maskExpression.X, maskExpression.Y
	mask, ok := ps6083UintConstant(pass, maskValue)
	if !ok {
		source, maskValue = maskExpression.Y, maskExpression.X
		mask, ok = ps6083UintConstant(pass, maskValue)
	}
	if !ok || mask == 0 || mask&(mask+1) != 0 || mask+1 > ps6083MaxEntries ||
		!ps6083Mentions(pass, source, loopIndex) || ps6083RejectedSource(source) {
		return nil, 0, 0, false
	}
	bits, ok := ps6083FixedUnsignedBits(pass.TypesInfo.TypeOf(maskExpression))
	if !ok || mask > ps6083UnsignedMaximum(bits) || !types.Identical(pass.TypesInfo.TypeOf(source), pass.TypesInfo.TypeOf(maskExpression)) {
		return nil, 0, 0, false
	}
	return maskExpression, bits, mask + 1, true
}

func ps6083Destination(pass *analysis.Pass, call *ast.CallExpr) (string, uint64, bool) {
	if len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return "", 0, false
	}
	identifier, ok := ps6083Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return "", 0, false
	}
	typeName, ok := pass.TypesInfo.Uses[identifier].(*types.TypeName)
	if !ok || typeName.Pkg() != nil {
		return "", 0, false
	}
	switch identifier.Name {
	case "float32":
		return "float32", 1 << 24, true
	case "float64":
		return "float64", 1 << 53, true
	}
	return "", 0, false
}

func ps6083FixedUnsignedBits(typeOf types.Type) (int, bool) {
	if typeOf == nil {
		return 0, false
	}
	basic, ok := types.Unalias(typeOf).Underlying().(*types.Basic)
	if !ok {
		return 0, false
	}
	switch basic.Kind() {
	case types.Uint8:
		return 8, true
	case types.Uint16:
		return 16, true
	case types.Uint32:
		return 32, true
	case types.Uint64:
		return 64, true
	}
	return 0, false
}

func ps6083UnsignedMaximum(bits int) uint64 {
	if bits == 64 {
		return ^uint64(0)
	}
	return uint64(1)<<bits - 1
}

func ps6083Maximum(coefficient, addend, domain, limit uint64) (uint64, bool) {
	if domain == 0 || addend > limit {
		return 0, false
	}
	maximumDomain := domain - 1
	if maximumDomain != 0 && coefficient > (limit-addend)/maximumDomain {
		return 0, false
	}
	return coefficient*maximumDomain + addend, true
}

func ps6083AliasSafe(pass *analysis.Pass, context *ps6083Context, object types.Object) bool {
	safe := true
	ast.Inspect(context.body, func(node ast.Node) bool {
		if node == nil || !safe {
			return false
		}
		switch value := node.(type) {
		case *ast.FuncLit:
			if ps6083Mentions(pass, value.Body, object) {
				safe = false
			}
			return false
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				if identifier, ok := ps6083Unparen(left).(*ast.Ident); ok && pass.TypesInfo.Defs[identifier] == object {
					continue
				}
				if ps6083IsObject(pass, left, object) {
					safe = false
					return false
				}
			}
		case *ast.IncDecStmt:
			if ps6083IsObject(pass, value.X, object) {
				safe = false
				return false
			}
		case *ast.RangeStmt:
			if value.Tok == token.ASSIGN && (ps6083IsObject(pass, value.Key, object) || ps6083IsObject(pass, value.Value, object)) {
				safe = false
				return false
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND && ps6083Mentions(pass, value.X, object) {
				safe = false
				return false
			}
		case *ast.SelectorExpr:
			selection := pass.TypesInfo.Selections[value]
			if selection != nil && ps6083Mentions(pass, value.X, object) {
				signature, _ := selection.Obj().Type().(*types.Signature)
				if signature != nil && signature.Recv() != nil {
					if _, pointer := types.Unalias(signature.Recv().Type()).(*types.Pointer); pointer {
						safe = false
						return false
					}
				}
			}
		}
		return safe
	})
	return safe
}

func ps6083StraightLoop(body *ast.BlockStmt) bool {
	straight := true
	ps6083InspectDirect(body, func(node ast.Node) bool {
		if node != body {
			switch node.(type) {
			case *ast.ForStmt, *ast.RangeStmt, *ast.BranchStmt, *ast.ReturnStmt:
				straight = false
			}
		}
		return straight
	})
	return straight
}

func ps6083ObjectWritten(pass *analysis.Pass, root ast.Node, object types.Object) bool {
	written := false
	ast.Inspect(root, func(node ast.Node) bool {
		if written {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				written = written || ps6083IsObject(pass, left, object)
			}
		case *ast.IncDecStmt:
			written = ps6083IsObject(pass, value.X, object)
		case *ast.RangeStmt:
			if value != root && value.Tok == token.ASSIGN {
				written = ps6083IsObject(pass, value.Key, object) || ps6083IsObject(pass, value.Value, object)
			}
		}
		return !written
	})
	return written
}

func ps6083Unconditional(node ast.Node, loop ps6083Loop, parents map[ast.Node]ast.Node) bool {
	for current := node; current != nil && current != loop.body; current = parents[current] {
		switch parent := parents[current].(type) {
		case *ast.IfStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt, *ast.CaseClause, *ast.CommClause:
			return false
		case *ast.BinaryExpr:
			if (parent.Op == token.LAND || parent.Op == token.LOR) && current == parent.Y {
				return false
			}
		case *ast.BlockStmt:
			if parent != loop.body {
				return false
			}
		}
	}
	return true
}

func ps6083InspectDirect(root ast.Node, inspect func(ast.Node) bool) {
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			return false
		}
		descend := inspect(node)
		if node != root {
			switch node.(type) {
			case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
				return false
			}
		}
		return descend
	})
}

func ps6083RejectedSource(expression ast.Expr) bool {
	rejected := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if rejected {
			return false
		}
		switch value := node.(type) {
		case *ast.CallExpr, *ast.FuncLit:
			rejected = true
		case *ast.UnaryExpr:
			rejected = value.Op == token.ARROW
		}
		return !rejected
	})
	return rejected
}

func ps6083Fix(pass *analysis.Pass, context *ps6083Context, match ps6083Match) (analysis.SuggestedFix, bool) {
	if context.hasGoto || ps6083CommentsOverlap(context.comments, match.conversion.Pos(), match.conversion.End()) {
		return analysis.SuggestedFix{}, false
	}
	if _, labeled := context.parents[match.loop.statement].(*ast.LabeledStmt); labeled {
		return analysis.SuggestedFix{}, false
	}
	var rendered bytes.Buffer
	if err := format.Node(&rendered, pass.Fset, ps6083Unparen(match.index)); err != nil {
		return analysis.SuggestedFix{}, false
	}
	position := pass.Fset.Position(match.conversion.Pos())
	base := "psFloatLookup" + strconv.Itoa(position.Line) + "_" + strconv.Itoa(position.Column)
	name := base
	for suffix := 2; context.usedNames[name]; suffix++ {
		name = base + "_" + strconv.Itoa(suffix)
	}
	context.usedNames[name] = true
	loopPosition := pass.Fset.Position(match.loop.statement.Pos())
	indent := strings.Repeat("\t", max(loopPosition.Column-1, 0))
	var declaration strings.Builder
	declaration.Grow(320)
	declaration.WriteString(name)
	declaration.WriteString(" := [")
	declaration.WriteString(strconv.FormatUint(match.domain, 10))
	declaration.WriteByte(']')
	declaration.WriteString(match.destination)
	declaration.WriteByte('{')
	for index := uint64(0); index < match.domain; index++ {
		if index != 0 {
			declaration.WriteString(", ")
		}
		declaration.WriteString(strconv.FormatUint(match.coefficient*index+match.addend, 10))
	}
	declaration.WriteString("}\n")
	declaration.WriteString(indent)
	return analysis.SuggestedFix{
		Message: "replace the exact small-domain conversion with a destination-typed local lookup",
		TextEdits: []analysis.TextEdit{
			{Pos: match.loop.statement.Pos(), End: match.loop.statement.Pos(), NewText: []byte(declaration.String())},
			{Pos: match.conversion.Pos(), End: match.conversion.End(), NewText: []byte(name + "[" + rendered.String() + "]")},
		},
	}, true
}

func ps6083CommentsOverlap(comments []*ast.CommentGroup, position, end token.Pos) bool {
	for _, comment := range comments {
		if comment.End() > position && comment.Pos() < end {
			return true
		}
	}
	return false
}

func ps6083Mentions(pass *analysis.Pass, node ast.Node, object types.Object) bool {
	found := false
	ast.Inspect(node, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && identObject(pass, identifier) == object {
			found = true
			return false
		}
		return !found
	})
	return found
}

func ps6083IsObject(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := ps6083Unparen(expression).(*ast.Ident)
	return ok && identObject(pass, identifier) == object
}

func ps6083IntegerConstant(pass *analysis.Pass, expression ast.Expr, expected int64) bool {
	value := pass.TypesInfo.Types[ps6083Unparen(expression)].Value
	if value == nil || value.Kind() != constant.Int {
		return false
	}
	actual, ok := constant.Int64Val(value)
	return ok && actual == expected
}

func ps6083UintConstant(pass *analysis.Pass, expression ast.Expr) (uint64, bool) {
	value := pass.TypesInfo.Types[ps6083Unparen(expression)].Value
	if value == nil || value.Kind() != constant.Int {
		return 0, false
	}
	result, exact := constant.Uint64Val(value)
	return result, exact
}

func ps6083Unparen(expression ast.Expr) ast.Expr {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression
		}
		expression = parenthesized.X
	}
}
