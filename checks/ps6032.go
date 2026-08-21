package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6032 implements owner issue #746 as a conservative typed candidate
// generator. The corrected owner campaign was negative, so a match is never a
// promotion verdict and never receives an automatic rewrite.
var PS6032 = register(&lint.Check{
	ID:       "PS6032",
	Category: "verify",
	Slug:     "single-consumer-residual-norm-dispatch-boundary",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "an adjacent residual-add and normalization dispatch share a single-consumer intermediate",
		Text: `An accelerator residual add whose returned tensor is consumed
only by an immediately adjacent RMSNorm or LayerNorm is a topology-fusion
candidate. Removing the intermediate command boundary may remove both a
dispatch and a dependency edge, even when isolated elementwise arithmetic
shows little leaf leverage.

This check implements owner issue #746 as a typed multi-call candidate
generator. Within one lexical block it requires:

  - an assignment from a resolved residual-add/elementwise-add method;
  - an immediately following resolved RMSNorm/LayerNorm/normalization method;
  - the same recorder, encoder, graph, stream, command-buffer, or device
    receiver for both calls;
  - a slice/array/buffer/tensor result from the producer;
  - exactly one use of that result in the entire enclosing function; and
  - that sole use occurs in the normalization call.

Adjacency and single-consumer identity keep the finding narrow. An intervening
command, a second use of the intermediate, different receivers, ordinary CPU
helpers, and already fused method names stay silent.

The diagnostic reports one statically removable dispatch/dependency boundary
and requires measured producer/consumer buffer traffic, leaf speedup,
current-parent speedup, profiled event-count delta, and correctness mode.
Dependency-charged stage intervals are not proof of graph leverage. RMS
reduction reassociation must pass an explicit semantic-quality gate in
addition to tolerance parity, and promotion requires a fresh current-parent
exact-digest campaign.

There is NO automatic fix. Kernel fusion changes command topology, buffer
lifetimes, numerical accumulation order, and device code. Those semantics
cannot be preserved mechanically from Go syntax alone.`,
		Before: `tmp := rec.ResidualAdd(hidden, residual)
out := rec.RMSNorm(tmp, weight)`,
		After: `out := rec.ResidualAddRMSNorm(hidden, residual, weight)
// Candidate only: measure traffic, leaf/current-parent ratios, event delta,
// exact digest, and semantic quality before retaining the kernel.`,
		MeasuredWin: `The initial Apple-M2 campaign removed 44 residual-add
boundaries (340 to 296 events) and reported 1.459x. After rebasing onto current
GoAI main, the exact 200-token parent measured 1,205,161,833 ns control versus
1,215,104,042 ns candidate: 0.991818x control/candidate, or 0.825% slower, with
a different all-logit digest. The fusion was removed. This rule therefore
generates candidates but never promotes them from topology or leaf evidence.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6032",
		Doc:  "adjacent accelerator residual-add and normalization calls have a single-consumer intermediate",
		Run:  runPS6032,
	},
})

type ps6032Producer struct {
	call   *ast.CallExpr
	result types.Object
	name   string
	recv   string
	typeOf types.Type
}

type ps6032Consumer struct {
	call *ast.CallExpr
	name string
	recv string
}

func runPS6032(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ps6032Blocks(fn.Body, func(block *ast.BlockStmt) {
				for i := 0; i+1 < len(block.List); i++ {
					producer, ok := ps6032ProducerStmt(pass, block.List[i])
					if !ok || !ps6032SingleUse(pass, fn.Body, producer.result) {
						continue
					}
					consumer, ok := ps6032ConsumerStmt(pass, block.List[i+1], producer.result)
					if !ok || producer.recv == "" || producer.recv != consumer.recv || !ps6022CommandType(producer.typeOf) {
						continue
					}
					pass.Reportf(producer.call.Pos(), "adjacent accelerator %s -> %s calls share single-consumer intermediate %s; one dispatch/dependency boundary is statically removable, but treat this only as a candidate: report producer/consumer traffic, leaf and fresh current-parent ratios, profiled event delta, exact-digest versus tolerance mode, and a semantic-quality gate for reduction reassociation; do not infer graph leverage from dependency-charged intervals", producer.name, consumer.name, producer.result.Name())
				}
			})
		}
	}
	return nil, nil
}

func ps6032Blocks(block *ast.BlockStmt, visit func(*ast.BlockStmt)) {
	visit(block)
	for _, statement := range block.List {
		ast.Inspect(statement, func(node ast.Node) bool {
			if node == block {
				return true
			}
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			child, ok := node.(*ast.BlockStmt)
			if !ok {
				return true
			}
			ps6032Blocks(child, visit)
			return false
		})
	}
}

func ps6032ProducerStmt(pass *analysis.Pass, statement ast.Stmt) (ps6032Producer, bool) {
	assign, ok := statement.(*ast.AssignStmt)
	if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return ps6032Producer{}, false
	}
	id, ok := ps2110Unparen(assign.Lhs[0]).(*ast.Ident)
	if !ok || id.Name == "_" {
		return ps6032Producer{}, false
	}
	call, ok := ps2110Unparen(assign.Rhs[0]).(*ast.CallExpr)
	if !ok {
		return ps6032Producer{}, false
	}
	fn, sig, ok := typedCallee(pass, call.Fun)
	if !ok || sig.Recv() == nil || !ps6032ResidualProducer(fn.Name()) {
		return ps6032Producer{}, false
	}
	result := identObject(pass, id)
	if result == nil || !ps6020DataObject(result) {
		return ps6032Producer{}, false
	}
	recv, recvType := ps6032Receiver(pass, call)
	return ps6032Producer{call: call, result: result, name: fn.Name(), recv: recv, typeOf: recvType}, true
}

func ps6032ConsumerStmt(pass *analysis.Pass, statement ast.Stmt, result types.Object) (ps6032Consumer, bool) {
	call := ps6032StatementCall(statement)
	if call == nil {
		return ps6032Consumer{}, false
	}
	fn, sig, ok := typedCallee(pass, call.Fun)
	if !ok || sig.Recv() == nil || !ps6032NormalizationConsumer(fn.Name()) || !ps6032CallUses(pass, call, result) {
		return ps6032Consumer{}, false
	}
	recv, _ := ps6032Receiver(pass, call)
	return ps6032Consumer{call: call, name: fn.Name(), recv: recv}, true
}

func ps6032StatementCall(statement ast.Stmt) *ast.CallExpr {
	switch value := statement.(type) {
	case *ast.ExprStmt:
		call, _ := ps2110Unparen(value.X).(*ast.CallExpr)
		return call
	case *ast.AssignStmt:
		if len(value.Rhs) != 1 {
			return nil
		}
		call, _ := ps2110Unparen(value.Rhs[0]).(*ast.CallExpr)
		return call
	case *ast.DeclStmt:
		declaration, ok := value.Decl.(*ast.GenDecl)
		if !ok || len(declaration.Specs) != 1 {
			return nil
		}
		spec, ok := declaration.Specs[0].(*ast.ValueSpec)
		if !ok || len(spec.Values) != 1 {
			return nil
		}
		call, _ := ps2110Unparen(spec.Values[0]).(*ast.CallExpr)
		return call
	}
	return nil
}

func ps6032ResidualProducer(name string) bool {
	name = ps6007NormalizeName(name)
	if ps6007ContainsAny(name, "fused", "rmsnorm", "layernorm", "normalization") {
		return false
	}
	return ps6007ContainsAny(name, "residualadd", "addresidual", "elementwiseadd", "biasadd")
}

func ps6032NormalizationConsumer(name string) bool {
	name = ps6007NormalizeName(name)
	if ps6007ContainsAny(name, "fused", "residualadd", "addresidual") {
		return false
	}
	return ps6007ContainsAny(name, "rmsnorm", "layernorm", "normalization", "normalise", "normalize")
}

func ps6032Receiver(pass *analysis.Pass, call *ast.CallExpr) (string, types.Type) {
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return "", nil
	}
	return exprTextRendered(selector.X), pass.TypesInfo.TypeOf(selector.X)
}

func ps6032SingleUse(pass *analysis.Pass, body *ast.BlockStmt, object types.Object) bool {
	uses := 0
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		id, ok := node.(*ast.Ident)
		if ok && pass.TypesInfo.Uses[id] == object {
			uses++
		}
		return uses <= 1
	})
	return uses == 1
}

func ps6032CallUses(pass *analysis.Pass, call *ast.CallExpr, object types.Object) bool {
	found := false
	for _, argument := range call.Args {
		ast.Inspect(argument, func(node ast.Node) bool {
			if found {
				return false
			}
			id, ok := node.(*ast.Ident)
			if ok && pass.TypesInfo.Uses[id] == object {
				found = true
				return false
			}
			return true
		})
		if found {
			break
		}
	}
	return found
}
