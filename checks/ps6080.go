package checks

import (
	"cmp"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"maps"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/lint"
)

// PS6080 implements owner issue #804. It compares typed quant-enum coverage
// across storage, portable decode, and reachable CPU matmul dispatch layers.
var PS6080 = register(&lint.Check{
	ID:       "PS6080",
	Category: "verify",
	Slug:     "decodable-quant-missing-matmul-dispatch",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a storable, decodable quant variant disappears from layered CPU matmul dispatch",
		Text: `A quantization format may have a complete storage contract and a
portable decoder while one or more CPU matrix-multiplication whitelists omit
it. The resulting gap can be more than a missed fast path: a valid value that
other APIs and backends accept may reach an unsupported-type fallback.

This check implements owner issue #804. For each same-package named integer
enum, it derives the eligible format set from constants present in both a
byte/size/layout path and a decode/dequantize/unpack path. It then follows
direct typed calls from CPU matmul/GEMV/matvec entry points and compares every
typed-constant switch, equality guard, and enum-keyed dispatch table reachable
from those entries. Storage, decode, and matmul context propagate through
direct same-package helper calls, so a generic support predicate or
caller-owned scratch decoder remains part of the relevant layer graph.
Neutral helpers reachable from both storage and decode do not count as two
independent eligibility proofs. Public enum constants are bridged to internal
integer IDs only through explicit constant-alias declarations; numeric equality
alone never joins unrelated enums. Function-local or package-level maps count
only when the relevant layer indexes them, the key domain matches the enum or
an explicitly bridged integer ID, and the selected value is statically usable.
CPU/backend dispatch evidence additionally requires a callable map value.

A report requires at least two eligible enum values and at least two reachable
CPU dispatch layers for that enum. The diagnostic names the storage and decode
evidence, every missing layer, the layers that already mention the format, and
architecture/backend matmul evidence when present. Equality complements such
as q != Q4 exclude only the named value, while rejected switch cases, nil map
entries, dead branches, scheduled calls, and dormant tables cannot manufacture
coverage. Constants from another enum, bare numeric coincidence, unresolved
indirect calls, and packages without a layered CPU matmul surface stay silent.
Directly invoked function literals and integer-converted constants remain
visible. Single-case guards still count once the independent storage/decode and
layered-CPU evidence thresholds are met. Ordered range predicates do not
enumerate a finite dispatch set and non-callable enum-keyed metadata maps are
not dispatch evidence.

Attach //perfscan:quant-matmul-read-only to an intentionally non-computable
constant, or //perfscan:quant-matmul-coverage-validated to a constant or
dispatch function whose omission has a documented semantic and benchmark
reason. These are narrow suppressions; a stale comment elsewhere does not
silence the typed variant.

There is NO automatic fix. A tensor decoder does not by itself prove that a
row-dot implementation preserves accumulator width, packed-index semantics,
shape validation, or allocation behavior. Add a portable scalar oracle first,
then separately gate any fused leaf with arbitrary packed rows,
cancellation-heavy inputs, allocation checks, routed end-to-end shapes, and an
unrelated negative control.`,
		Before: `func quantBytes(q QuantType) int {
	switch q { case Q4, IQ2XXS: return blockBytes(q) }
	return 0
}

func dequantize(q QuantType, src []byte) []float32 {
	switch q { case Q4, IQ2XXS: return decode(q, src) }
	return nil
}

func qMatMul(q QuantType) error {
	switch q { case Q4: return q4MatMul() }
	return errUnsupported
}`,
		After: `// First add and validate an exact portable IQ2XXS row path.
// Then include IQ2XXS in every relevant M=1 and all-M support/dispatch layer.
// Retain a scalar oracle and benchmark-gated architecture leaf.`,
		MeasuredWin: `In owner issue #804, IQ2_XXS already had byte sizing,
portable tensor dequantization, golden vectors, and a CUDA-resident matmul path,
but CPU QMatMul omitted it from both M=1 and all-M dispatch layers and returned
"unsupported quant type". Adding the exact portable row path and an Apple
ARM64 fused leaf improved K=4096 by 6.27x, M1/N64/K1024 by 5.98x, and
M1/N4096/K1024 by 4.18x on Apple M2 Pro (p=0.000, n=10), with unchanged
allocations and neutral decoder/control cells.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6080",
		Doc:  "storable and decodable quant variants are absent from layered CPU matmul dispatch",
		Run:  runPS6080,
	},
})

var (
	ps6080InvokedLiteralCaches sync.Map
	ps6080NamedCallbackCaches  sync.Map
	ps6080MayCallbackCaches    sync.Map
	ps6080NamedOrderCaches     sync.Map
	ps6080NamedSafeMapCaches   sync.Map
	ps6080LiteralFuncCaches    sync.Map
	ps6080VariableInitCaches   sync.Map
	ps6080FunctionValueCaches  sync.Map
	ps6080GlobalAliasCaches    sync.Map
	ps6080ReturnFailureCaches  sync.Map
)

type ps6080PackageSentinel struct{}

var ps6080PackagePath = reflect.TypeOf(ps6080PackageSentinel{}).PkgPath()

type ps6080Role uint8

const (
	ps6080StorageRole ps6080Role = 1 << iota
	ps6080DecodeRole
	ps6080MatmulRole
)

var ps6080ContextReplacer = strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "-", "")

type ps6080Function struct {
	declaration           *ast.FuncDecl
	object                *types.Func
	signature             *types.Signature
	body                  *ast.BlockStmt
	name                  string
	context               string
	roles                 ps6080Role
	backend               bool
	validated             bool
	callees               []*types.Func
	cpuCalls              []ps6080CPUCall
	cpuIncoming           bool
	indirect              bool
	scanned               bool
	capturedTargets       map[types.Object][]ps6080NamedFunctionTarget
	returnedLiteralScopes map[*ast.FuncLit]map[types.Object]*ps6080CPUCallScope
}

type ps6080CPUCall struct {
	callee         *types.Func
	scopes         map[types.Object]*ps6080CPUCallScope
	literal        *ast.FuncLit
	literalOwner   *types.Func
	factoryScanned bool
	bindings       map[types.Object][]ps6080NamedFunctionTarget
}

type ps6080CPUCallScope struct {
	enum      *types.TypeName
	fixed     map[string]token.Pos
	allowed   map[string]token.Pos
	source    types.Object
	universal bool
}

type ps6080CPUReachability struct {
	functions map[*types.Func]bool
	scopes    map[*types.Func]map[types.Object]map[string]token.Pos
}

type ps6080BackendOwnerReach struct {
	name  string
	reach ps6080CPUReachability
}

func ps6080FunctionSignature(function *ps6080Function) *types.Signature {
	if function == nil {
		return nil
	}
	if function.signature != nil {
		return function.signature
	}
	if function.object == nil {
		return nil
	}
	signature, _ := function.object.Type().(*types.Signature)
	return signature
}

func ps6080FunctionBody(function *ps6080Function) *ast.BlockStmt {
	if function == nil {
		return nil
	}
	if function.body != nil {
		return function.body
	}
	if function.declaration == nil {
		return nil
	}
	return function.declaration.Body
}

type ps6080Site struct {
	function        *ps6080Function
	kind            string
	mapTable        bool
	callable        bool
	validated       bool
	group           string
	position        token.Pos
	end             token.Pos
	enum            *types.TypeName
	constants       map[*types.Const]token.Pos
	excluded        map[*types.Const]token.Pos
	open            bool
	scope           map[*types.Const]token.Pos
	reachScope      map[*types.Const]token.Pos
	subject         types.Object
	literalScope    *ps6080CPUCallScope
	references      []*types.Func
	referenceRoutes map[*types.Func][]ps6080ReferenceRoute
}

type ps6080ReferenceRoute struct {
	subject      types.Object
	literalScope *ps6080CPUCallScope
}

type ps6080GlobalTable struct {
	kind      string
	callable  bool
	position  token.Pos
	end       token.Pos
	enum      *types.TypeName
	constants map[*types.Const]token.Pos
	excluded  map[*types.Const]token.Pos
	open      bool
}

type ps6080ConstantGroup struct {
	included map[*types.Const]token.Pos
	excluded map[*types.Const]token.Pos
	open     bool
}

type ps6080Evidence struct {
	storage []*ps6080Site
	decode  []*ps6080Site
}

type ps6080Finding struct {
	constant *types.Const
	enum     *types.TypeName
	evidence ps6080Evidence
	missing  []*ps6080Site
	present  []*ps6080Site
	backend  []string
	total    int
}

func runPS6080(pass *analysis.Pass) (any, error) {
	if pass.Pkg != nil && pass.Pkg.Path() == ps6080PackagePath {
		// The analyzer package necessarily contains the storage/decode/matmul
		// vocabulary in PS6080's own implementation. Those checker helpers are
		// not a quantized compute surface, and following their package-wide call
		// graph is both irrelevant and disproportionately expensive.
		return nil, nil
	}
	pass = ps6080ProductionPass(pass)
	cache := &sync.Map{}
	ps6080InvokedLiteralCaches.Store(pass, cache)
	defer ps6080InvokedLiteralCaches.Delete(pass)
	defer ps6080NamedCallbackCaches.Delete(pass)
	defer ps6080MayCallbackCaches.Delete(pass)
	defer ps6080NamedOrderCaches.Delete(pass)
	defer ps6080NamedSafeMapCaches.Delete(pass)
	ps6080LiteralFuncCaches.Store(pass, &sync.Map{})
	defer ps6080LiteralFuncCaches.Delete(pass)
	ps6080VariableInitCaches.Store(pass, &ps6080VariableInitializerCache{})
	defer ps6080VariableInitCaches.Delete(pass)
	ps6080FunctionValueCaches.Store(pass, &ps6080FunctionValueTargetCache{})
	defer ps6080FunctionValueCaches.Delete(pass)
	ps6080GlobalAliasCaches.Store(pass, &sync.Map{})
	defer ps6080GlobalAliasCaches.Delete(pass)
	ps6080ReturnFailureCaches.Store(pass, &ps6080ReturnFailureCache{})
	defer ps6080ReturnFailureCaches.Delete(pass)

	functions := ps6080Functions(pass)
	if len(functions) == 0 {
		return nil, nil
	}
	constantEnums := ps6080ConstantEnums(pass)
	domains := ps6080EnumDomains(constantEnums)
	ps6080PopulateReachableCallees(pass, functions, constantEnums, domains)
	ps6080MarkIndirectFunctionReferences(pass, functions)
	storageReach := ps6080Reachable(functions, func(function *ps6080Function) bool {
		return function.roles&ps6080StorageRole != 0
	})
	decodeReach := ps6080Reachable(functions, func(function *ps6080Function) bool {
		return function.roles&ps6080DecodeRole != 0 && !function.backend
	})
	cpuReach := ps6080CPUReachable(functions, domains, func(function *ps6080Function) bool {
		return ps6080CPURoot(function) && !function.backend
	})
	backendReach := ps6080CPUReachable(functions, domains, func(function *ps6080Function) bool {
		return ps6080CPURoot(function) && function.backend
	})
	if len(storageReach) == 0 || len(decodeReach) == 0 || len(cpuReach.functions) == 0 {
		return nil, nil
	}
	backendOwners := ps6080BackendOwners(functions, domains)

	globalTables := ps6080GlobalTables(pass, constantEnums)
	var sites []*ps6080Site
	for object, function := range functions {
		if !storageReach[object] && !decodeReach[object] &&
			!cpuReach.functions[object] && !backendReach.functions[object] {
			continue
		}
		sites = append(sites, ps6080FunctionSites(pass, function, constantEnums, domains)...)
		sites = append(sites, ps6080ReferencedGlobalSites(
			pass, function, globalTables, constantEnums, domains,
		)...)
	}
	ps6080CanonicalizeSites(sites)
	eligible := ps6080Eligible(sites, storageReach, decodeReach, constantEnums)
	suppressed := ps6080SuppressedConstants(pass)
	var findings []ps6080Finding
	for enum, constants := range eligible {
		if enum.Pkg() != pass.Pkg || len(constants) < 2 {
			continue
		}
		cpuSites := ps6080ScopedRoleSites(sites, enum, cpuReach, domains)
		cpuSites = slices.DeleteFunc(cpuSites, func(site *ps6080Site) bool {
			directStorageOnly := site.function.roles&ps6080StorageRole != 0 &&
				site.function.roles&(ps6080DecodeRole|ps6080MatmulRole) == 0
			return site.function.backend || directStorageOnly || site.mapTable && !site.callable
		})
		if len(cpuSites) < 2 {
			continue
		}
		backendSites := ps6080ScopedRoleSites(sites, enum, backendReach, domains)
		backendSites = slices.DeleteFunc(backendSites, func(site *ps6080Site) bool {
			return site.mapTable && !site.callable
		})
		for constant, evidence := range constants {
			if constant.Pkg() != pass.Pkg || ps6080SuppressedConstant(constant, suppressed, constantEnums) {
				continue
			}
			applicableSites := slices.DeleteFunc(slices.Clone(cpuSites), func(site *ps6080Site) bool {
				return !ps6080SiteApplies(constant, site, cpuSites)
			})
			if len(applicableSites) < 2 {
				continue
			}
			finding := ps6080Finding{constant: constant, enum: enum, evidence: evidence, total: len(applicableSites)}
			for _, site := range applicableSites {
				if ps6080SiteSupports(site, constant) {
					finding.present = append(finding.present, site)
				} else if !site.validated {
					finding.missing = append(finding.missing, site)
				}
			}
			if len(finding.missing) == 0 {
				continue
			}
			finding.backend = ps6080BackendEvidence(constant, backendSites, backendOwners, domains)
			findings = append(findings, finding)
		}
	}
	slices.SortFunc(findings, func(left, right ps6080Finding) int {
		return cmp.Compare(left.constant.Pos(), right.constant.Pos())
	})
	for index := range findings {
		ps6080Report(pass, &findings[index])
	}
	return nil, nil
}

func ps6080ProductionPass(pass *analysis.Pass) *analysis.Pass {
	files := slices.DeleteFunc(slices.Clone(pass.Files), func(file *ast.File) bool {
		return ps6080TestFile(pass, file)
	})
	if len(files) == len(pass.Files) {
		return pass
	}
	production := *pass
	production.Files = files
	return &production
}

func ps6080SiteSupports(site *ps6080Site, constant *types.Const) bool {
	if ps6080ConstantMapHasValue(site.excluded, constant) {
		return false
	}
	if site.open {
		return true
	}
	return ps6080ConstantMapHasValue(site.constants, constant)
}

func ps6080SiteApplies(constant *types.Const, site *ps6080Site, sites []*ps6080Site) bool {
	if len(site.reachScope) > 0 && !ps6080ConstantMapHasValue(site.reachScope, constant) {
		return false
	}
	if len(site.scope) == 0 {
		return true
	}
	if ps6080ConstantMapHasValue(site.scope, constant) {
		return true
	}
	for _, guard := range sites {
		if guard.function.object != site.function.object || guard.kind != "guard" {
			continue
		}
		if ps6080SiteSupports(guard, constant) {
			return false
		}
	}
	return true
}

func ps6080ConstantMapHasValue(constants map[*types.Const]token.Pos, constant *types.Const) bool {
	if constant == nil {
		return false
	}
	value := constant.Val().ExactString()
	for candidate := range constants {
		if candidate.Val().ExactString() == value {
			return true
		}
	}
	return false
}

func ps6080BackendOwners(
	functions map[*types.Func]*ps6080Function,
	domains map[*types.TypeName][]*types.Const,
) []ps6080BackendOwnerReach {
	var owners []ps6080BackendOwnerReach
	for _, root := range functions {
		if !ps6080CPURoot(root) || !root.backend {
			continue
		}
		owners = append(owners, ps6080BackendOwnerReach{
			name: root.name,
			reach: ps6080CPUReachable(functions, domains, func(function *ps6080Function) bool {
				return function == root
			}),
		})
	}
	slices.SortFunc(owners, func(left, right ps6080BackendOwnerReach) int {
		return cmp.Compare(left.name, right.name)
	})
	return owners
}

func ps6080GlobalTables(
	pass *analysis.Pass,
	constantEnums map[*types.Const][]*types.TypeName,
) map[*types.Var][]ps6080GlobalTable {
	result := make(map[*types.Var][]ps6080GlobalTable)
	initWrites := ps6080InitMapWrites(pass)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.VAR {
				continue
			}
			for _, specification := range general.Specs {
				value, ok := specification.(*ast.ValueSpec)
				if !ok || len(value.Names) != len(value.Values) {
					continue
				}
				for index, name := range value.Names {
					variable, ok := pass.TypesInfo.Defs[name].(*types.Var)
					if !ok {
						continue
					}
					mapping, ok := types.Unalias(variable.Type()).Underlying().(*types.Map)
					if !ok {
						continue
					}
					callable := ps6080CallableType(mapping.Elem())
					groups := make(map[*types.TypeName]*ps6080ConstantGroup)
					position, end := token.NoPos, token.NoPos
					var allowedInitWrites map[ast.Node]bool
					initPopulated := false
					initializer := ps2110Unparen(value.Values[index])
					if literal, literalOK := initializer.(*ast.CompositeLit); literalOK {
						position, end = literal.Pos(), literal.End()
						for _, element := range literal.Elts {
							if keyed, ok := element.(*ast.KeyValueExpr); ok {
								ps6080ExpressionConstants(
									pass, keyed.Key, groups, constantEnums,
									ps6080MapValueSupported(pass, keyed.Value, mapping.Elem(), true),
								)
							}
						}
					} else if ps6080MapMakeExpression(pass, initializer) && !variable.Exported() {
						var initialized bool
						groups, allowedInitWrites, position, end, initialized = ps6080InitGlobalMapTable(
							pass, variable, mapping, constantEnums, initWrites[variable],
						)
						if !initialized {
							continue
						}
						initPopulated = true
					} else {
						continue
					}
					for enum, group := range groups {
						if !ps6080MapKeyCompatible(mapping.Key(), enum, group) {
							delete(groups, enum)
						}
					}
					if len(groups) == 0 {
						continue
					}
					if initPopulated && ps6080ExportedPackageMapAlias(pass, variable) {
						continue
					}
					if !ps6080PackageTableStable(pass, variable, allowedInitWrites) {
						continue
					}
					for enum, group := range groups {
						result[variable] = append(result[variable], ps6080GlobalTable{
							kind: "global map table", callable: callable, position: position, end: end,
							enum: enum, constants: group.included, excluded: group.excluded, open: group.open,
						})
					}
				}
			}
		}
	}
	return result
}

func ps6080MapMakeExpression(pass *analysis.Pass, expression ast.Expr) bool {
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || len(call.Args) == 0 || len(call.Args) > 2 {
		return false
	}
	identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok || pass.TypesInfo.ObjectOf(identifier) != types.Universe.Lookup("make") {
		return false
	}
	typeOf := pass.TypesInfo.TypeOf(call)
	if typeOf == nil {
		return false
	}
	if _, mapping := types.Unalias(typeOf).Underlying().(*types.Map); !mapping {
		return false
	}
	if len(call.Args) == 2 {
		capacity := pass.TypesInfo.Types[call.Args[1]].Value
		return capacity != nil && constant.Sign(capacity) >= 0
	}
	return true
}

type ps6080InitMapWrite struct {
	assignment *ast.AssignStmt
	indexed    *ast.IndexExpr
}

type ps6080InitFlowFacts struct {
	blocks      map[token.Pos]*cfg.Block
	reachable   map[*cfg.Block]bool
	guaranteed  map[*cfg.Block]bool
	repeating   map[*cfg.Block]bool
	recoverExit map[*cfg.Block]token.Pos
}

func (facts ps6080InitFlowFacts) accepts(node ast.Node) bool {
	block := facts.blocks[node.Pos()]
	if block == nil || !facts.reachable[block] || !facts.guaranteed[block] || facts.repeating[block] {
		return false
	}
	exit := facts.recoverExit[block]
	return exit == token.NoPos || exit >= node.End()
}

func ps6080InitMapWrites(pass *analysis.Pass) map[*types.Var][]ps6080InitMapWrite {
	result := make(map[*types.Var][]ps6080InitMapWrite)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || function.Recv != nil || function.Name.Name != "init" {
				continue
			}
			type candidate struct {
				table *types.Var
				write ps6080InitMapWrite
			}
			candidates := make([]candidate, 0, len(function.Body.List))
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				assignment, ok := node.(*ast.AssignStmt)
				if !ok || assignment.Tok != token.ASSIGN ||
					len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
					return true
				}
				indexed, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.IndexExpr)
				if !ok {
					return true
				}
				identifier, ok := ps2110Unparen(indexed.X).(*ast.Ident)
				if !ok {
					return true
				}
				table, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
				if !ok || table.Parent() != pass.Pkg.Scope() {
					return true
				}
				if _, mapping := types.Unalias(table.Type()).Underlying().(*types.Map); mapping {
					candidates = append(candidates, candidate{
						table: table,
						write: ps6080InitMapWrite{assignment: assignment, indexed: indexed},
					})
				}
				return true
			})
			if len(candidates) == 0 {
				continue
			}
			candidateAssignments := make(map[*ast.AssignStmt]bool, len(candidates))
			for _, candidate := range candidates {
				candidateAssignments[candidate.write.assignment] = true
			}
			facts := ps6080BuildInitFlowFacts(pass, function.Body, candidateAssignments)
			for _, candidate := range candidates {
				if facts.accepts(candidate.write.assignment) {
					result[candidate.table] = append(result[candidate.table], candidate.write)
				}
			}
		}
	}
	return result
}

func ps6080BuildInitFlowFacts(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	candidates map[*ast.AssignStmt]bool,
) ps6080InitFlowFacts {
	graph := cfg.New(body, ps6080CallMayReturn(pass))
	facts := ps6080InitFlowFacts{
		blocks: make(map[token.Pos]*cfg.Block), reachable: make(map[*cfg.Block]bool),
		guaranteed: make(map[*cfg.Block]bool), repeating: make(map[*cfg.Block]bool),
		recoverExit: make(map[*cfg.Block]token.Pos),
	}
	if len(graph.Blocks) == 0 {
		return facts
	}
	parents := ps6071Parents(body)
	successors := make(map[*cfg.Block][]*cfg.Block, len(graph.Blocks))
	predecessors := make(map[*cfg.Block][]*cfg.Block, len(graph.Blocks))
	for _, block := range graph.Blocks {
		for _, node := range block.Nodes {
			ast.Inspect(node, func(child ast.Node) bool {
				if child != nil && facts.blocks[child.Pos()] == nil {
					facts.blocks[child.Pos()] = block
				}
				return true
			})
		}
		for _, successor := range ps6080FeasibleSuccessors(pass, parents, block) {
			if successor == nil || !successor.Live {
				continue
			}
			successors[block] = append(successors[block], successor)
			predecessors[successor] = append(predecessors[successor], block)
		}
	}
	facts.recoverExit = ps6080InitRecoverExits(pass, body, graph, successors, candidates)
	entry := graph.Blocks[0]
	queue := []*cfg.Block{entry}
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if block == nil || !block.Live || facts.reachable[block] {
			continue
		}
		facts.reachable[block] = true
		queue = append(queue, successors[block]...)
	}
	reachableBlocks := make([]*cfg.Block, 0, len(facts.reachable))
	for _, block := range graph.Blocks {
		if facts.reachable[block] {
			reachableBlocks = append(reachableBlocks, block)
		}
	}
	idom, intersect := ps6080ImmediateDominators(entry, successors, predecessors)
	var commonExit *cfg.Block
	for _, block := range reachableBlocks {
		if len(successors[block]) != 0 && facts.recoverExit[block] == token.NoPos {
			continue
		}
		if commonExit == nil {
			commonExit = block
		} else {
			commonExit = intersect(commonExit, block)
		}
	}
	for block := commonExit; block != nil; block = idom[block] {
		facts.guaranteed[block] = true
		if block == entry {
			break
		}
	}
	facts.repeating = ps6080CyclicBlocks(reachableBlocks, successors)
	return facts
}

func ps6080InitRecoverExits(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	graph *cfg.CFG,
	successors map[*cfg.Block][]*cfg.Block,
	candidates map[*ast.AssignStmt]bool,
) map[*cfg.Block]token.Pos {
	exits := make(map[*cfg.Block]token.Pos)
	cache := ps6080ReturnFailureCacheFor(pass)
	ps6080BuildReturnabilityIndex(pass, cache)
	context := ps6080ReturnabilityContext(body, cache)
	type pathState struct {
		block    *cfg.Block
		recovers bool
	}
	entry := pathState{block: graph.Blocks[0]}
	seen := map[pathState]bool{entry: true}
	queue := []pathState{entry}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		recovers := current.recovers
		blocked := false
		for _, node := range current.block.Nodes {
			assignment, candidate := node.(*ast.AssignStmt)
			var outcome ps6080ExpressionOutcome
			if candidate && candidates[assignment] {
				outcome = ps6080InitMapAssignmentOutcome(pass, assignment, context, cache)
			} else {
				outcome = ps6080NodeOrderedExpressionOutcome(
					pass, node, context, cache,
					cache.returnability.mustReturn, cache.returnability.mustPanic,
				)
			}
			if recovers && !outcome.mustReturn {
				if prior := exits[current.block]; prior == token.NoPos || node.Pos() < prior {
					exits[current.block] = node.Pos()
				}
			}
			if outcome.mustPanic {
				blocked = true
				break
			}
			effects := ps6080NodeReturnabilityEffects(
				pass, node, context, cache,
				func(call *ast.CallExpr) bool {
					return ps6080CallMayReturnWithFacts(
						pass, call, context, cache, cache.returnability.mayReturn,
						make(map[*ast.BlockStmt]bool),
					)
				},
				func(call *ast.CallExpr) bool {
					return ps6080CallMustPanicWithFacts(
						pass, call, context, cache, cache.returnability.mustPanic,
					)
				},
				make(map[*ast.BlockStmt]bool),
			)
			if effects.kills || effects.registersHardNonreturn ||
				effects.registersPanic && !recovers {
				blocked = true
				break
			}
			recovers = recovers || effects.registersRecover
		}
		if blocked {
			continue
		}
		for _, successor := range successors[current.block] {
			next := pathState{block: successor, recovers: recovers}
			if successor.Live && !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return exits
}

func ps6080InitMapAssignmentOutcome(
	pass *analysis.Pass,
	assignment *ast.AssignStmt,
	context *ps6080ReturnabilityBodyContext,
	cache *ps6080ReturnFailureCache,
) ps6080ExpressionOutcome {
	for _, destination := range assignment.Lhs {
		outcome := ps6080AssignmentDestinationOperandsOutcome(
			pass, destination, context, cache,
			cache.returnability.mustReturn, cache.returnability.mustPanic,
		)
		if !outcome.mustReturn {
			return outcome
		}
	}
	return ps6080ExpressionSequenceOutcome(
		pass, assignment.Rhs, context, cache,
		cache.returnability.mustReturn, cache.returnability.mustPanic,
	)
}

func ps6080ImmediateDominators(
	entry *cfg.Block,
	successors map[*cfg.Block][]*cfg.Block,
	predecessors map[*cfg.Block][]*cfg.Block,
) (map[*cfg.Block]*cfg.Block, func(*cfg.Block, *cfg.Block) *cfg.Block) {
	type frame struct {
		block *cfg.Block
		next  int
	}
	seen := map[*cfg.Block]bool{entry: true}
	stack := []frame{{block: entry}}
	postorder := make([]*cfg.Block, 0, len(successors))
	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		if top.next < len(successors[top.block]) {
			successor := successors[top.block][top.next]
			top.next++
			if !seen[successor] {
				seen[successor] = true
				stack = append(stack, frame{block: successor})
			}
			continue
		}
		postorder = append(postorder, top.block)
		stack = stack[:len(stack)-1]
	}
	rpo := slices.Clone(postorder)
	slices.Reverse(rpo)
	order := make(map[*cfg.Block]int, len(rpo))
	for index, block := range rpo {
		order[block] = index
	}
	idom := make(map[*cfg.Block]*cfg.Block, len(rpo))
	idom[entry] = entry
	intersect := func(left, right *cfg.Block) *cfg.Block {
		for left != right {
			for order[left] > order[right] {
				left = idom[left]
			}
			for order[right] > order[left] {
				right = idom[right]
			}
		}
		return left
	}
	for changed := true; changed; {
		changed = false
		for _, block := range rpo[1:] {
			var next *cfg.Block
			for _, predecessor := range predecessors[block] {
				if idom[predecessor] == nil {
					continue
				}
				if next == nil {
					next = predecessor
				} else {
					next = intersect(next, predecessor)
				}
			}
			if next != nil && idom[block] != next {
				idom[block] = next
				changed = true
			}
		}
	}
	return idom, intersect
}

func ps6080CyclicBlocks(
	blocks []*cfg.Block,
	successors map[*cfg.Block][]*cfg.Block,
) map[*cfg.Block]bool {
	cyclic := make(map[*cfg.Block]bool)
	indices := make(map[*cfg.Block]int, len(blocks))
	low := make(map[*cfg.Block]int, len(blocks))
	onStack := make(map[*cfg.Block]bool, len(blocks))
	stack := make([]*cfg.Block, 0, len(blocks))
	nextIndex := 1
	var visit func(*cfg.Block)
	visit = func(block *cfg.Block) {
		indices[block], low[block] = nextIndex, nextIndex
		nextIndex++
		stack = append(stack, block)
		onStack[block] = true
		for _, successor := range successors[block] {
			if indices[successor] == 0 {
				visit(successor)
				low[block] = min(low[block], low[successor])
			} else if onStack[successor] {
				low[block] = min(low[block], indices[successor])
			}
		}
		if low[block] != indices[block] {
			return
		}
		var component []*cfg.Block
		for len(stack) > 0 {
			member := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[member] = false
			component = append(component, member)
			if member == block {
				break
			}
		}
		if len(component) > 1 {
			for _, member := range component {
				cyclic[member] = true
			}
			return
		}
		for _, successor := range successors[block] {
			if successor == block {
				cyclic[block] = true
				return
			}
		}
	}
	for _, block := range blocks {
		if indices[block] == 0 {
			visit(block)
		}
	}
	return cyclic
}

func ps6080ExportedPackageMapAlias(pass *analysis.Pass, table *types.Var) bool {
	for object, alias := range ps6080GlobalAliasInfoFor(pass, table).aliases {
		variable, ok := object.(*types.Var)
		if alias && ok && variable.Parent() == pass.Pkg.Scope() && variable.Exported() {
			return true
		}
	}
	return false
}

func ps6080InitGlobalMapTable(
	pass *analysis.Pass,
	table *types.Var,
	mapping *types.Map,
	constantEnums map[*types.Const][]*types.TypeName,
	initWrites []ps6080InitMapWrite,
) (map[*types.TypeName]*ps6080ConstantGroup, map[ast.Node]bool, token.Pos, token.Pos, bool) {
	groups := make(map[*types.TypeName]*ps6080ConstantGroup)
	writes := make(map[ast.Node]bool, len(initWrites))
	seenKeys := make(map[string]bool, len(initWrites))
	position, end := token.NoPos, token.NoPos
	for _, write := range initWrites {
		constant := ps6080AliasConstant(pass, write.indexed.Index)
		if constant == nil || len(constantEnums[constant]) == 0 {
			return nil, nil, token.NoPos, token.NoPos, false
		}
		key := constant.Val().ExactString()
		if seenKeys[key] {
			return nil, nil, token.NoPos, token.NoPos, false
		}
		seenKeys[key] = true
		ps6080AddConstantObject(
			constant, write.indexed.Index.Pos(), groups, constantEnums,
			ps6080MapValueSupported(pass, write.assignment.Rhs[0], mapping.Elem(), true),
		)
		writes[write.assignment] = true
		if position == token.NoPos || write.assignment.Pos() < position {
			position = write.assignment.Pos()
		}
		end = max(end, write.assignment.End())
	}
	return groups, writes, position, end, len(writes) > 0
}

func ps6080PackageTableStable(
	pass *analysis.Pass,
	table *types.Var,
	allowedInitWrites map[ast.Node]bool,
) bool {
	aliasInfo := ps6080GlobalAliasInfoFor(pass, table)
	aliases := aliasInfo.aliases
	initialAliases := aliasInfo.initialAliases
	for _, file := range pass.Files {
		context := ps6080NewMapAliasContext(pass, table, aliases, initialAliases, file)
		stable := true
		ast.Inspect(file, func(node ast.Node) bool {
			if _, function := node.(*ast.FuncDecl); function {
				return false
			}
			switch value := node.(type) {
			case *ast.AssignStmt:
				for _, left := range value.Lhs {
					if ps6080MapMutationTarget(left, value.Pos(), context) {
						stable = false
						return false
					}
				}
				for index, right := range value.Rhs {
					var destination ast.Expr
					if len(value.Lhs) == len(value.Rhs) {
						destination = value.Lhs[index]
					}
					if ps6080MapAliasEscapes(right, value.Pos(), destination, context) {
						stable = false
						return false
					}
				}
			case *ast.ValueSpec:
				for index, right := range value.Values {
					var destination ast.Expr
					if len(value.Names) == len(value.Values) {
						destination = value.Names[index]
					}
					if ps6080MapAliasEscapes(right, value.Pos(), destination, context) {
						stable = false
						return false
					}
				}
			case *ast.CallExpr:
				if ps6080MapMutationCall(value, context) {
					stable = false
					return false
				}
			case *ast.ReturnStmt:
				for _, result := range value.Results {
					if ps6080MapAliasEscapes(result, value.Pos(), nil, context) {
						stable = false
						return false
					}
				}
			case *ast.SendStmt:
				if ps6080MapAliasEscapes(value.Value, value.Pos(), nil, context) {
					stable = false
					return false
				}
			}
			return stable
		})
		if !stable {
			return false
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Body != nil && !ps6080GlobalTableStableInBodyWithAliases(
				pass, table, function.Body, aliases, initialAliases, allowedInitWrites,
			) {
				return false
			}
		}
	}
	return true
}

func ps6080MapAliases(pass *analysis.Pass, table types.Object, root ast.Node) map[types.Object]bool {
	return ps6080MapAliasesAcross(pass, table, []ast.Node{root})
}

func ps6080MapAliasesAcross(
	pass *analysis.Pass,
	table types.Object,
	roots []ast.Node,
) map[types.Object]bool {
	return ps6080MapAliasesAcrossFrom(pass, map[types.Object]bool{table: true}, roots)
}

func ps6080MapAliasesAcrossFrom(
	pass *analysis.Pass,
	seeds map[types.Object]bool,
	roots []ast.Node,
) map[types.Object]bool {
	aliases := make(map[types.Object]bool, len(seeds))
	for object, alias := range seeds {
		aliases[object] = alias
	}
	changed := true
	for changed {
		changed = false
		for _, root := range roots {
			ast.Inspect(root, func(node ast.Node) bool {
				switch value := node.(type) {
				case *ast.AssignStmt:
					if len(value.Lhs) != len(value.Rhs) {
						return true
					}
					for index, left := range value.Lhs {
						identifier, ok := ps2110Unparen(left).(*ast.Ident)
						if !ok {
							continue
						}
						object := pass.TypesInfo.ObjectOf(identifier)
						if object != nil && !aliases[object] && ps6080MapAliasSource(pass, value.Rhs[index], aliases) {
							aliases[object] = true
							changed = true
						}
					}
				case *ast.ValueSpec:
					if len(value.Names) != len(value.Values) {
						return true
					}
					for index, name := range value.Names {
						object := pass.TypesInfo.Defs[name]
						if object != nil && !aliases[object] && ps6080MapAliasSource(pass, value.Values[index], aliases) {
							aliases[object] = true
							changed = true
						}
					}
				case *ast.RangeStmt:
					composite, ok := ps2110Unparen(value.X).(*ast.CompositeLit)
					if !ok || len(composite.Elts) != 1 || value.Value == nil ||
						!ps6080MapAliasSource(pass, composite.Elts[0], aliases) {
						return true
					}
					identifier, ok := ps2110Unparen(value.Value).(*ast.Ident)
					if !ok || identifier.Name == "_" {
						return true
					}
					object := pass.TypesInfo.ObjectOf(identifier)
					if object != nil && !aliases[object] {
						aliases[object] = true
						changed = true
					}
				}
				return true
			})
		}
	}
	return aliases
}

func ps6080PackageMapAliases(pass *analysis.Pass, table types.Object) map[types.Object]bool {
	aliases := map[types.Object]bool{table: true}
	if table.Parent() != pass.Pkg.Scope() {
		return aliases
	}
	changed := true
	for changed {
		changed = false
		for _, file := range pass.Files {
			for _, declaration := range file.Decls {
				general, ok := declaration.(*ast.GenDecl)
				if !ok || general.Tok != token.VAR {
					continue
				}
				for _, specification := range general.Specs {
					value, ok := specification.(*ast.ValueSpec)
					if !ok || len(value.Names) != len(value.Values) {
						continue
					}
					for index, name := range value.Names {
						object := pass.TypesInfo.Defs[name]
						if object != nil && !aliases[object] && ps6080MapAliasSource(pass, value.Values[index], aliases) {
							aliases[object] = true
							changed = true
						}
					}
				}
			}
		}
	}
	functions := make(map[*types.Func]*ast.FuncDecl)
	var initializers []*ast.FuncDecl
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			if object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func); ok {
				functions[object] = function
			}
			if function.Recv == nil && function.Name.Name == "init" {
				initializers = append(initializers, function)
			}
		}
	}
	slices.SortFunc(initializers, func(left, right *ast.FuncDecl) int {
		return cmp.Compare(left.Pos(), right.Pos())
	})
	functionValues := ps6080FunctionValueTargets(pass)
	stickyUnknown := make(map[types.Object]bool)
	for _, function := range initializers {
		ps6080ApplyInitFunction(
			pass, function, aliases, functions, functionValues, true, false,
			stickyUnknown, make(map[*types.Func]bool),
		)
	}
	return aliases
}

type ps6080GlobalAliasInfo struct {
	aliases        map[types.Object]bool
	initialAliases map[types.Object]bool
}

func ps6080GlobalAliasInfoFor(pass *analysis.Pass, table types.Object) ps6080GlobalAliasInfo {
	if value, cached := ps6080GlobalAliasCaches.Load(pass); cached {
		cache := value.(*sync.Map)
		if value, cached = cache.Load(table); cached {
			return value.(ps6080GlobalAliasInfo)
		}
		computed := ps6080BuildGlobalAliasInfo(pass, table)
		value, _ = cache.LoadOrStore(table, computed)
		return value.(ps6080GlobalAliasInfo)
	}
	return ps6080BuildGlobalAliasInfo(pass, table)
}

func ps6080BuildGlobalAliasInfo(pass *analysis.Pass, table types.Object) ps6080GlobalAliasInfo {
	roots := make([]ast.Node, 0, len(pass.Files))
	for _, file := range pass.Files {
		roots = append(roots, file)
	}
	return ps6080GlobalAliasInfo{
		aliases:        ps6080MapAliasesAcross(pass, table, roots),
		initialAliases: ps6080PackageMapAliases(pass, table),
	}
}

type ps6080FunctionValueTargetCache struct {
	once    sync.Once
	targets map[types.Object]map[*types.Func]bool
}

func ps6080FunctionValueTargets(pass *analysis.Pass) map[types.Object]map[*types.Func]bool {
	if value, cached := ps6080FunctionValueCaches.Load(pass); cached {
		cache := value.(*ps6080FunctionValueTargetCache)
		cache.once.Do(func() {
			cache.targets = ps6080BuildFunctionValueTargets(pass)
		})
		return cache.targets
	}
	return ps6080BuildFunctionValueTargets(pass)
}

func ps6080BuildFunctionValueTargets(pass *analysis.Pass) map[types.Object]map[*types.Func]bool {
	targets := make(map[types.Object]map[*types.Func]bool)
	changed := true
	for changed {
		changed = false
		for _, file := range pass.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				var names []*ast.Ident
				var values []ast.Expr
				switch value := node.(type) {
				case *ast.AssignStmt:
					if len(value.Lhs) != len(value.Rhs) {
						return true
					}
					names = make([]*ast.Ident, len(value.Lhs))
					for index, left := range value.Lhs {
						names[index], _ = ps2110Unparen(left).(*ast.Ident)
					}
					values = value.Rhs
				case *ast.ValueSpec:
					if len(value.Names) != len(value.Values) {
						return true
					}
					names, values = value.Names, value.Values
				default:
					return true
				}
				for index, name := range names {
					if name == nil {
						continue
					}
					object := pass.TypesInfo.ObjectOf(name)
					if object == nil || !ps6080CallableType(object.Type()) {
						continue
					}
					objectTargets := targets[object]
					if objectTargets == nil {
						objectTargets = make(map[*types.Func]bool)
						targets[object] = objectTargets
					}
					for target := range ps6080ExpressionFunctionTargets(pass, values[index], targets) {
						if !objectTargets[target] {
							objectTargets[target] = true
							changed = true
						}
					}
				}
				return true
			})
			for _, declaration := range file.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Body == nil {
					continue
				}
				object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
				if !ok {
					continue
				}
				ast.Inspect(function.Body, func(node ast.Node) bool {
					if _, nested := node.(*ast.FuncLit); nested {
						return false
					}
					returned, ok := node.(*ast.ReturnStmt)
					if !ok {
						return true
					}
					objectTargets := targets[object]
					if objectTargets == nil {
						objectTargets = make(map[*types.Func]bool)
						targets[object] = objectTargets
					}
					for _, result := range returned.Results {
						for target := range ps6080ExpressionFunctionTargets(pass, result, targets) {
							if !objectTargets[target] {
								objectTargets[target] = true
								changed = true
							}
						}
					}
					return true
				})
			}
		}
	}
	return targets
}

func ps6080ExpressionFunctionTargets(
	pass *analysis.Pass,
	expression ast.Expr,
	targets map[types.Object]map[*types.Func]bool,
) map[*types.Func]bool {
	result := make(map[*types.Func]bool)
	if call, ok := ps2110Unparen(expression).(*ast.CallExpr); ok {
		if len(call.Args) == 1 && pass.TypesInfo.Types[call.Fun].IsType() {
			return ps6080ExpressionFunctionTargets(pass, call.Args[0], targets)
		}
		if callee, _, direct := typedCallee(pass, call.Fun); direct {
			for target := range targets[callee] {
				result[target] = true
			}
		}
		if identifier, identifierOK := ps2110Unparen(call.Fun).(*ast.Ident); identifierOK {
			for factory := range targets[pass.TypesInfo.ObjectOf(identifier)] {
				for target := range targets[factory] {
					result[target] = true
				}
			}
		}
		return result
	}
	if callee, _, direct := typedCallee(pass, expression); direct {
		result[callee] = true
	}
	if identifier, ok := ps2110Unparen(expression).(*ast.Ident); ok {
		for target := range targets[pass.TypesInfo.ObjectOf(identifier)] {
			result[target] = true
		}
	}
	return result
}

func ps6080ApplyInitFunction(
	pass *analysis.Pass,
	function *ast.FuncDecl,
	aliases map[types.Object]bool,
	functions map[*types.Func]*ast.FuncDecl,
	functionValues map[types.Object]map[*types.Func]bool,
	outerGuaranteed bool,
	sticky bool,
	stickyUnknown map[types.Object]bool,
	visiting map[*types.Func]bool,
) {
	object, _ := pass.TypesInfo.Defs[function.Name].(*types.Func)
	if object != nil && visiting[object] {
		ps6080InvalidatePackageMapWrites(pass, function.Body, aliases, stickyUnknown, sticky)
		return
	}
	if object != nil {
		visiting[object] = true
		defer delete(visiting, object)
	}
	parents := ps6071Parents(function.Body)
	graph := cfg.New(function.Body, ps6080CallMayReturn(pass))
	invokedLiterals := ps6080InvokedFunctionLiterals(pass, function.Body)
	for literal := range invokedLiterals {
		ps6080InvalidatePackageMapWrites(pass, literal.Body, aliases, stickyUnknown, true)
	}
	known := make(map[types.Object]bool, len(aliases))
	for candidate, alias := range aliases {
		known[candidate] = alias
	}
	apply := func(names []*ast.Ident, values []ast.Expr, node ast.Node) {
		resolved := make([]bool, len(names))
		if len(names) == len(values) {
			for index, value := range values {
				resolved[index] = ps6080MapAliasSource(pass, value, known)
			}
		}
		guaranteed := outerGuaranteed && ps6080CFGNodeGuaranteed(graph, node)
		for index, name := range names {
			if name == nil {
				continue
			}
			candidate := pass.TypesInfo.ObjectOf(name)
			if candidate == nil {
				continue
			}
			known[candidate] = guaranteed && resolved[index]
			if candidate.Parent() == pass.Pkg.Scope() {
				if _, mapping := types.Unalias(candidate.Type()).Underlying().(*types.Map); mapping {
					if sticky {
						stickyUnknown[candidate] = true
					}
					aliases[candidate] = known[candidate] && !stickyUnknown[candidate]
				}
			}
		}
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if node == nil || !ps6080NodeReachable(pass, graph, parents, node) {
			return true
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			names := make([]*ast.Ident, len(value.Lhs))
			for index, left := range value.Lhs {
				names[index], _ = ps2110Unparen(left).(*ast.Ident)
			}
			apply(names, value.Rhs, value)
		case *ast.ValueSpec:
			apply(value.Names, value.Values, value)
		case *ast.CallExpr:
			deferredOrConcurrent := false
			switch parent := parents[value].(type) {
			case *ast.GoStmt:
				deferredOrConcurrent = parent.Call == value
			case *ast.DeferStmt:
				deferredOrConcurrent = parent.Call == value
			}
			callee, _, ok := typedCallee(pass, value.Fun)
			direct := ok && functions[callee] != nil
			var declarations []*ast.FuncDecl
			if direct {
				declarations = append(declarations, functions[callee])
			} else if identifier, identifierOK := ps2110Unparen(value.Fun).(*ast.Ident); identifierOK {
				for target := range functionValues[pass.TypesInfo.ObjectOf(identifier)] {
					if declaration := functions[target]; declaration != nil {
						declarations = append(declarations, declaration)
					}
				}
				slices.SortFunc(declarations, func(left, right *ast.FuncDecl) int {
					return cmp.Compare(left.Pos(), right.Pos())
				})
			}
			var argumentDeclarations []*ast.FuncDecl
			var argumentLiterals []*ast.FuncLit
			for _, argument := range value.Args {
				if literal, literalOK := ps2110Unparen(argument).(*ast.FuncLit); literalOK {
					argumentLiterals = append(argumentLiterals, literal)
					continue
				}
				if target, _, targetOK := typedCallee(pass, argument); targetOK {
					if declaration := functions[target]; declaration != nil {
						argumentDeclarations = append(argumentDeclarations, declaration)
					}
				}
				for target := range ps6080ExpressionFunctionTargets(pass, argument, functionValues) {
					if declaration := functions[target]; declaration != nil {
						argumentDeclarations = append(argumentDeclarations, declaration)
					}
				}
			}
			slices.SortFunc(argumentDeclarations, func(left, right *ast.FuncDecl) int {
				return cmp.Compare(left.Pos(), right.Pos())
			})
			if len(declarations) == 0 && len(argumentDeclarations) == 0 && len(argumentLiterals) == 0 {
				break
			}
			guaranteed := direct && !deferredOrConcurrent && outerGuaranteed &&
				ps6080CFGNodeGuaranteed(graph, value)
			for _, declaration := range declarations {
				if ps6080FunctionReturnsCallable(pass, declaration) {
					ps6080InvalidatePackageMapWrites(
						pass, declaration.Body, aliases, stickyUnknown, true,
					)
				}
				ps6080ApplyInitFunction(
					pass, declaration, aliases, functions, functionValues, guaranteed,
					sticky || deferredOrConcurrent, stickyUnknown, visiting,
				)
			}
			for _, declaration := range argumentDeclarations {
				if ps6080FunctionReturnsCallable(pass, declaration) {
					ps6080InvalidatePackageMapWrites(
						pass, declaration.Body, aliases, stickyUnknown, true,
					)
				}
				ps6080ApplyInitFunction(
					pass, declaration, aliases, functions, functionValues, false,
					sticky || deferredOrConcurrent, stickyUnknown, visiting,
				)
			}
			for _, literal := range argumentLiterals {
				ps6080InvalidatePackageMapWrites(
					pass, literal.Body, aliases, stickyUnknown, sticky || deferredOrConcurrent,
				)
			}
			for candidate, alias := range aliases {
				known[candidate] = alias
			}
		}
		return true
	})
}

func ps6080FunctionReturnsCallable(pass *analysis.Pass, function *ast.FuncDecl) bool {
	object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
	if !ok {
		return false
	}
	signature, ok := object.Type().(*types.Signature)
	if !ok {
		return false
	}
	for result := range signature.Results().Len() {
		if ps6080CallableType(signature.Results().At(result).Type()) {
			return true
		}
	}
	return false
}

func ps6080InvalidatePackageMapWrites(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	aliases map[types.Object]bool,
	stickyUnknown map[types.Object]bool,
	sticky bool,
) {
	ast.Inspect(body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, left := range assignment.Lhs {
			identifier, ok := ps2110Unparen(left).(*ast.Ident)
			if !ok {
				continue
			}
			object := pass.TypesInfo.ObjectOf(identifier)
			if object == nil || object.Parent() != pass.Pkg.Scope() {
				continue
			}
			if _, mapping := types.Unalias(object.Type()).Underlying().(*types.Map); mapping {
				if sticky {
					stickyUnknown[object] = true
				}
				aliases[object] = false
			}
		}
		return true
	})
}

func ps6080CFGNodeGuaranteed(graph *cfg.CFG, node ast.Node) bool {
	if graph == nil || node == nil || len(graph.Blocks) == 0 {
		return false
	}
	target := ps6079CFGBlockAt(graph, node.Pos())
	if target == nil || !target.Live {
		return false
	}
	seen := make(map[*cfg.Block]bool)
	queue := []*cfg.Block{graph.Blocks[0]}
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if block == target || !block.Live || seen[block] {
			continue
		}
		seen[block] = true
		liveSuccessors := 0
		for _, successor := range block.Succs {
			if successor.Live {
				liveSuccessors++
				queue = append(queue, successor)
			}
		}
		if liveSuccessors == 0 {
			return false
		}
	}
	return true
}

func ps6080MapAliasSource(pass *analysis.Pass, expression ast.Expr, aliases map[types.Object]bool) bool {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return aliases[pass.TypesInfo.ObjectOf(value)]
	case *ast.CallExpr:
		return len(value.Args) == 1 && pass.TypesInfo.Types[value.Fun].IsType() &&
			ps6080MapAliasSource(pass, value.Args[0], aliases)
	}
	return false
}

func ps6080MapAliasIdentity(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	if expression == nil || object == nil {
		return false
	}
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return pass.TypesInfo.ObjectOf(value) == object
	case *ast.CallExpr:
		return len(value.Args) == 1 && pass.TypesInfo.Types[value.Fun].IsType() &&
			ps6080MapAliasIdentity(pass, value.Args[0], object)
	}
	return false
}

type ps6080MapAliasBinding struct {
	position        token.Pos
	order           []token.Pos
	expression      ast.Expr
	node            ast.Node
	region          ast.Node
	scope           *ast.FuncLit
	guaranteed      bool
	uninitialized   bool
	alternative     ast.Node
	alternativeLeaf ast.Node
	instance        ast.Node
}

type ps6080MapAliasParameter struct {
	literal *ast.FuncLit
	index   int
}

type ps6080IndexSet []bool

type ps6080InvocationArguments []ps6080IndexSet

type ps6080CallbackMappings []ps6080InvocationArguments

func ps6080GrowIndexSlice[T any](values []T, index int) []T {
	if index < len(values) {
		return values
	}
	values = slices.Grow(values, index+1-len(values))
	return values[:index+1]
}

func ps6080AddIndex(set *ps6080IndexSet, index int) {
	*set = ps6080GrowIndexSlice(*set, index)
	(*set)[index] = true
}

func ps6080HasIndex(set ps6080IndexSet, index int) bool {
	return index >= 0 && index < len(set) && set[index]
}

func ps6080AddInvocationArgument(arguments *ps6080InvocationArguments, argument, parameter int) {
	*arguments = ps6080GrowIndexSlice(*arguments, argument)
	ps6080AddIndex(&(*arguments)[argument], parameter)
}

type ps6080MapAliasContext struct {
	pass                *analysis.Pass
	table               types.Object
	aliases             map[types.Object]bool
	initialAliases      map[types.Object]bool
	parents             map[ast.Node]ast.Node
	bindings            map[types.Object][]ps6080MapAliasBinding
	parameters          map[types.Object]ps6080MapAliasParameter
	invocations         map[*ast.FuncLit]map[*ast.CallExpr]bool
	invocationOrders    map[*ast.FuncLit]map[*ast.CallExpr][][]token.Pos
	invocationArguments map[*ast.FuncLit]map[*ast.CallExpr]ps6080InvocationArguments
	orderedArguments    map[*ast.FuncLit]map[*ast.CallExpr][]ps6080OrderedInvocation
	literalCalls        map[*ast.CallExpr]bool
	callbackArguments   map[*ast.CallExpr]ps6080IndexSet
	rootGraph           *cfg.CFG
	literalGraphs       map[*ast.FuncLit]*cfg.CFG
	tablePosition       token.Pos
	tableBinding        ast.Node
	tableLiteral        ast.Node
	tableContainer      types.Object
	tableAggregate      ast.Expr
	tableField          types.Object
}

func ps6080NewMapAliasContext(
	pass *analysis.Pass,
	table types.Object,
	aliases map[types.Object]bool,
	initialAliases map[types.Object]bool,
	root ast.Node,
) *ps6080MapAliasContext {
	aliases = maps.Clone(aliases)
	context := &ps6080MapAliasContext{
		pass: pass, table: table, aliases: aliases, initialAliases: initialAliases,
		parents: ps6071Parents(root), bindings: make(map[types.Object][]ps6080MapAliasBinding),
		parameters: make(map[types.Object]ps6080MapAliasParameter),
	}
	reachable := func(ast.Node) bool { return true }
	var literalGraphs map[*ast.FuncLit]*cfg.CFG
	if body, ok := root.(*ast.BlockStmt); ok {
		graph := cfg.New(body, ps6080CallMayReturn(pass))
		context.rootGraph = graph
		invoked := ps6080InvokedFunctionLiteralResult(pass, body)
		invokedLiterals := invoked.literals
		context.invocations = invoked.calls
		context.invocationOrders = invoked.orders
		context.invocationArguments = invoked.arguments
		context.orderedArguments = invoked.orderedArgs
		context.literalCalls = invoked.safeCalls
		context.callbackArguments = invoked.safeArguments
		for literal := range invokedLiterals {
			if literal.Type.Params == nil {
				continue
			}
			parameter := 0
			for _, field := range literal.Type.Params.List {
				count := max(1, len(field.Names))
				for index := range count {
					if index < len(field.Names) {
						object := pass.TypesInfo.Defs[field.Names[index]]
						if object != nil {
							if _, mapping := types.Unalias(object.Type()).Underlying().(*types.Map); mapping {
								context.aliases[object] = true
								context.parameters[object] = ps6080MapAliasParameter{
									literal: literal, index: parameter,
								}
							}
						}
					}
					parameter++
				}
			}
		}
		context.aliases = ps6080MapAliasesAcrossFrom(pass, context.aliases, []ast.Node{root})
		literalGraphs = ps6080InvokedLiteralGraphs(pass, invokedLiterals)
		context.literalGraphs = literalGraphs
		reachable = func(node ast.Node) bool {
			return ps6080StaticallyReachable(pass, context.parents, node) && ps6080NodeReachable(
				pass, ps6080GraphForNode(graph, literalGraphs, context.parents, node), context.parents, node,
			)
		}
	}
	record := func(
		object types.Object,
		position token.Pos,
		expression ast.Expr,
		node ast.Node,
		uninitialized bool,
	) {
		if object == nil || object == table || !reachable(node) {
			return
		}
		if ps6080MapAliasIdentity(pass, expression, object) {
			return
		}
		scope := context.containingLiteral(node)
		projectedGuaranteed := ps6080CFGNodeGuaranteed(context.graphForScope(scope), node)
		if !projectedGuaranteed && expression != nil {
			projectedGuaranteed = ps6080CFGNodeGuaranteed(context.graphForScope(scope), expression)
		}
		if !projectedGuaranteed && scope != nil && len(scope.Body.List) > 0 {
			projectedGuaranteed = context.parents[node] == scope.Body && node == scope.Body.List[0]
		}
		region := ps6080EnclosingControlRegion(node, context.parents)
		var alternative ast.Node
		var alternativeLeaf ast.Node
		switch control := region.(type) {
		case *ast.BlockStmt:
			alternative, alternativeLeaf = ps6080IfAlternative(control, context.parents)
		case *ast.CaseClause:
			body, _ := context.parents[control].(*ast.BlockStmt)
			if statement, ok := context.parents[body].(*ast.SwitchStmt); ok &&
				ps6080SwitchHasDefault(statement) {
				alternative, alternativeLeaf = statement, control
			}
		}
		binding := ps6080MapAliasBinding{
			position: position, order: []token.Pos{position}, expression: expression, node: node,
			region: region, scope: scope, guaranteed: true, uninitialized: uninitialized,
			alternative: alternative, alternativeLeaf: alternativeLeaf, instance: alternative,
		}
		context.bindings[object] = append(context.bindings[object], binding)
		if scope == nil {
			return
		}
		type projectedEntry struct {
			order      []token.Pos
			guaranteed bool
			binding    int
		}
		type projectedBucket struct {
			position token.Pos
		}
		projectedByCall := make(map[*ast.CallExpr]map[projectedBucket][]projectedEntry)
		var project func(ps6080MapAliasBinding, *ast.FuncLit, map[*ast.FuncLit]bool)
		project = func(effect ps6080MapAliasBinding, callee *ast.FuncLit, visiting map[*ast.FuncLit]bool) {
			if callee == nil || visiting[callee] {
				return
			}
			visiting[callee] = true
			defer delete(visiting, callee)
			for call := range context.invocations[callee] {
				orders := context.invocationOrders[callee][call]
				if len(orders) == 0 {
					orders = [][]token.Pos{{call.Pos()}}
				}
				for _, invocationOrder := range orders {
					projected := effect
					projected.position = call.Pos()
					projected.order = slices.Concat(invocationOrder, effect.order)
					projected.node = call
					projected.instance = call
					projected.region = ps6080EnclosingControlRegion(call, context.parents)
					projected.scope = context.containingLiteral(call)
					bucket := projectedBucket{position: call.Pos()}
					if len(invocationOrder) > 0 {
						bucket.position = invocationOrder[len(invocationOrder)-1]
					}
					if projectedByCall[call] == nil {
						projectedByCall[call] = make(map[projectedBucket][]projectedEntry)
					}
					entries := projectedByCall[call][bucket]
					entry := -1
					for index := range entries {
						if slices.Equal(entries[index].order, projected.order) {
							entry = index
							projected.guaranteed = entries[index].guaranteed && projected.guaranteed
							if projected.guaranteed == entries[index].guaranteed {
								entry = -2
							}
							break
						}
					}
					if entry == -2 {
						continue
					}
					if entry < 0 && len(entries) < 2 {
						entry = len(entries)
						entries = append(entries, projectedEntry{})
					} else if entry < 0 {
						earliest, latest := 0, 0
						if ps6080PositionOrderBefore(entries[1].order, entries[0].order) {
							earliest = 1
						} else {
							latest = 1
						}
						if ps6080PositionOrderBefore(projected.order, entries[earliest].order) {
							entry = earliest
						} else if ps6080PositionOrderBefore(entries[latest].order, projected.order) {
							entry = latest
						} else {
							continue
						}
					}
					if entries[entry].order == nil {
						entries[entry].binding = len(context.bindings[object])
						context.bindings[object] = append(context.bindings[object], projected)
					} else {
						context.bindings[object][entries[entry].binding] = projected
					}
					entries[entry].order = slices.Clone(projected.order)
					entries[entry].guaranteed = projected.guaranteed
					projectedByCall[call][bucket] = entries
					if projected.scope != nil {
						nested := projected
						nested.guaranteed = nested.guaranteed &&
							ps6080CFGNodeGuaranteed(context.graphForScope(projected.scope), call)
						project(nested, projected.scope, visiting)
					}
				}
			}
		}
		projected := binding
		projected.guaranteed = projectedGuaranteed
		project(projected, scope, make(map[*ast.FuncLit]bool))
	}
	ast.Inspect(root, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			for index, left := range value.Lhs {
				identifier, ok := ps2110Unparen(left).(*ast.Ident)
				if !ok {
					continue
				}
				var expression ast.Expr
				if len(value.Lhs) == len(value.Rhs) {
					expression = value.Rhs[index]
				}
				record(pass.TypesInfo.ObjectOf(identifier), value.Pos(), expression, value, false)
			}
		case *ast.ValueSpec:
			for index, name := range value.Names {
				var expression ast.Expr
				if len(value.Names) == len(value.Values) {
					expression = value.Values[index]
				}
				record(pass.TypesInfo.Defs[name], value.Pos(), expression, value, expression == nil)
			}
		case *ast.RangeStmt:
			if ps6080StaticallyEmptyRange(pass, value) {
				break
			}
			var element ast.Expr
			if composite, ok := ps2110Unparen(value.X).(*ast.CompositeLit); ok && len(composite.Elts) == 1 {
				element = composite.Elts[0]
			}
			if identifier, ok := ps2110Unparen(value.Key).(*ast.Ident); ok && identifier.Name != "_" {
				record(pass.TypesInfo.ObjectOf(identifier), value.Pos(), nil, value, false)
			}
			if identifier, ok := ps2110Unparen(value.Value).(*ast.Ident); ok && identifier.Name != "_" {
				record(pass.TypesInfo.ObjectOf(identifier), value.Pos(), element, value, false)
			}
		}
		return true
	})
	return context
}

func (context *ps6080MapAliasContext) active(expression ast.Expr, position token.Pos) bool {
	return context.activeAt(
		expression, position, []token.Pos{position}, expression, make(map[*ast.FuncLit]bool),
	)
}

func (context *ps6080MapAliasContext) activeAt(
	expression ast.Expr,
	position token.Pos,
	order []token.Pos,
	reference ast.Node,
	invoking map[*ast.FuncLit]bool,
) bool {
	active, _ := context.activeAtWithVisiting(
		expression, position, order, reference, invoking, make(map[types.Object]bool),
	)
	return active
}

func (context *ps6080MapAliasContext) activeAtWithVisiting(
	expression ast.Expr,
	position token.Pos,
	order []token.Pos,
	reference ast.Node,
	invoking map[*ast.FuncLit]bool,
	visiting map[types.Object]bool,
) (bool, bool) {
	if active, resolved := context.activeWithVisiting(
		expression, position, order, reference, invoking, visiting,
	); active || resolved {
		return active, resolved
	}
	object := context.aliasObject(expression)
	if object == nil {
		return false, true
	}
	literal := context.containingLiteral(reference)
	if literal == nil || literal.Pos() <= object.Pos() && object.Pos() < literal.End() || invoking[literal] {
		return false, literal == nil || literal.Pos() <= object.Pos() && object.Pos() < literal.End()
	}
	invoking[literal] = true
	defer delete(invoking, literal)
	resolved := false
	for call := range context.invocations[literal] {
		orders := context.invocationOrders[literal][call]
		if len(orders) == 0 {
			orders = [][]token.Pos{{call.Pos()}}
		}
		for _, invocationOrder := range orders {
			active, callResolved := context.activeAtWithVisiting(
				expression, call.Pos(), invocationOrder, call, invoking, visiting,
			)
			if active {
				return true, true
			}
			resolved = resolved || callResolved
		}
	}
	return false, resolved
}

func (context *ps6080MapAliasContext) aliasObject(expression ast.Expr) types.Object {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return context.pass.TypesInfo.ObjectOf(value)
	case *ast.CallExpr:
		if len(value.Args) == 1 && context.pass.TypesInfo.Types[value.Fun].IsType() {
			return context.aliasObject(value.Args[0])
		}
	}
	return nil
}

func (context *ps6080MapAliasContext) containingLiteral(node ast.Node) *ast.FuncLit {
	for current := node; current != nil; current = context.parents[current] {
		if literal, ok := current.(*ast.FuncLit); ok {
			return literal
		}
	}
	return nil
}

func (context *ps6080MapAliasContext) activeWithVisiting(
	expression ast.Expr,
	position token.Pos,
	order []token.Pos,
	reference ast.Node,
	invoking map[*ast.FuncLit]bool,
	visiting map[types.Object]bool,
) (bool, bool) {
	if tableExpression, ok := context.tableLiteral.(ast.Expr); ok &&
		ps2110Unparen(expression) == ps2110Unparen(tableExpression) {
		return true, true
	}
	if selector, ok := ps2110Unparen(expression).(*ast.SelectorExpr); ok && context.tableField != nil &&
		context.pass.TypesInfo.ObjectOf(selector.Sel) == context.tableField {
		if identifier, direct := ps2110Unparen(selector.X).(*ast.Ident); direct &&
			context.pass.TypesInfo.ObjectOf(identifier) == context.tableContainer {
			return true, true
		}
		if context.tableAggregate != nil &&
			ps2110Unparen(selector.X) == ps2110Unparen(context.tableAggregate) {
			return true, true
		}
	}
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		object := context.pass.TypesInfo.ObjectOf(value)
		if context.table != nil && object == context.table {
			if context.tableBinding != nil && context.within(reference, context.tableBinding) &&
				!context.within(reference, context.tableLiteral) {
				return false, true
			}
			active := context.tablePosition == token.NoPos || context.tablePosition < position
			return active, active
		}
		if object == nil || !context.aliases[object] {
			return false, true
		}
		if visiting[object] {
			return false, true
		}
		visibleRegions := make(map[ast.Node]bool)
		for current := reference; current != nil; current = context.parents[current] {
			if ps6080ControlRegion(current, context.parents) {
				visibleRegions[current] = true
			}
		}
		var latest *ps6080MapAliasBinding
		referenceScope := context.containingLiteral(reference)
		for index := range context.bindings[object] {
			binding := &context.bindings[object][index]
			if binding.scope == referenceScope && context.bindingBeforeQuery(binding, position, order, reference) &&
				(binding.region == nil || visibleRegions[binding.region]) &&
				(latest == nil || ps6080MapAliasBindingBefore(latest, binding)) {
				latest = binding
			}
		}
		visiting[object] = true
		collectiveKill := context.collectiveNilKill(object, position, order, reference, latest)
		if collectiveKill != nil {
			latest = nil
		}
		active := context.initialAliases[object]
		parameterWritten := collectiveKill != nil ||
			context.parameterWrittenBefore(object, position, order, reference)
		resolved := active || latest != nil || parameterWritten || collectiveKill != nil
		if collectiveKill != nil {
			active = false
		}
		if parameter, ok := context.parameters[object]; ok && !parameterWritten &&
			!invoking[parameter.literal] {
			resolved = true
			invoking[parameter.literal] = true
			for call := range context.invocations[parameter.literal] {
				occurrences := context.orderedArguments[parameter.literal][call]
				if len(occurrences) == 0 {
					orders := context.invocationOrders[parameter.literal][call]
					if len(orders) == 0 {
						orders = [][]token.Pos{{call.Pos()}}
					}
					for _, invocationOrder := range orders {
						occurrences = append(occurrences, ps6080OrderedInvocation{
							order: invocationOrder, arguments: context.invocationArguments[parameter.literal][call],
						})
					}
				}
				for _, occurrence := range occurrences {
					if position == call.Pos() &&
						!ps6080PositionOrderPrefix(occurrence.order, order) {
						continue
					}
					var arguments ps6080IndexSet
					ps6080AddIndex(&arguments, parameter.index)
					if occurrence.arguments != nil {
						if parameter.index < len(occurrence.arguments) {
							arguments = occurrence.arguments[parameter.index]
						} else {
							arguments = nil
						}
					}
					for argument, present := range arguments {
						if !present {
							continue
						}
						argumentActive := false
						if argument < len(call.Args) {
							argumentActive, _ = context.activeAtWithVisiting(
								call.Args[argument], call.Pos(), occurrence.order, call, invoking, visiting,
							)
						}
						if argumentActive {
							active = true
							break
						}
					}
				}
				if active {
					break
				}
			}
			delete(invoking, parameter.literal)
		}
		if latest != nil {
			previousActive := false
			if !latest.guaranteed {
				delete(visiting, object)
				previousActive, _ = context.activeAtWithVisiting(
					value, latest.position, latest.order, latest.node, invoking, visiting,
				)
				visiting[object] = true
			}
			if latest.expression == nil {
				active = false
			} else {
				active, _ = context.activeAtWithVisiting(
					latest.expression, latest.position, latest.order, reference, invoking, visiting,
				)
			}
			active = active || previousActive
		}
		for index := range context.bindings[object] {
			binding := &context.bindings[object][index]
			if binding.scope != referenceScope || !context.bindingBeforeQuery(binding, position, order, reference) ||
				(collectiveKill != nil && (binding == collectiveKill ||
					ps6080MapAliasBindingBefore(binding, collectiveKill))) ||
				(latest != nil && !ps6080MapAliasBindingBefore(latest, binding)) ||
				binding.region == nil || visibleRegions[binding.region] ||
				ps6080MutuallyExclusiveControlRegions(binding.region, reference, context.parents) {
				continue
			}
			if binding.expression != nil {
				bindingActive, _ := context.activeAtWithVisiting(
					binding.expression, binding.position, binding.order, reference, invoking, visiting,
				)
				active = active || bindingActive
			}
			resolved = true
			if active {
				break
			}
		}
		delete(visiting, object)
		return active, resolved
	case *ast.CallExpr:
		if len(value.Args) == 1 && context.pass.TypesInfo.Types[value.Fun].IsType() {
			return context.activeAtWithVisiting(
				value.Args[0], position, order, reference, invoking, visiting,
			)
		}
	}
	return false, true
}

func (context *ps6080MapAliasContext) collectiveNilKill(
	object types.Object,
	position token.Pos,
	order []token.Pos,
	reference ast.Node,
	latest *ps6080MapAliasBinding,
) *ps6080MapAliasBinding {
	type alternativeKey struct {
		alternative ast.Node
		instance    ast.Node
	}
	type alternativeState struct {
		leaves map[ast.Node]bool
		latest *ps6080MapAliasBinding
	}
	states := make(map[alternativeKey]alternativeState, len(context.bindings[object]))
	referenceScope := context.containingLiteral(reference)
	for index := range context.bindings[object] {
		binding := &context.bindings[object][index]
		if binding.scope != referenceScope || binding.alternative == nil || binding.instance == nil ||
			context.within(reference, binding.alternative) ||
			!context.provenInactive(binding.expression, binding.position, binding.order, binding.node,
				make(map[types.Object]bool)) ||
			!context.bindingBeforeQuery(binding, position, order, reference) {
			continue
		}
		key := alternativeKey{alternative: binding.alternative, instance: binding.instance}
		state := states[key]
		if state.leaves == nil {
			state.leaves = make(map[ast.Node]bool)
		}
		state.leaves[binding.alternativeLeaf] = true
		if state.latest == nil || ps6080MapAliasBindingBefore(state.latest, binding) {
			state.latest = binding
		}
		states[key] = state
	}
	var result *ps6080MapAliasBinding
	for key, state := range states {
		if !ps6080AlternativeCovered(key.alternative, state.leaves) {
			continue
		}
		dominating := *state.latest
		dominating.node = key.instance
		if key.instance == key.alternative {
			switch statement := key.alternative.(type) {
			case *ast.IfStmt:
				dominating.node = statement.Cond
			case *ast.SwitchStmt:
				dominating.node = ps6080SwitchEntryNode(statement)
			}
		}
		if key.instance != reference && !context.bindingDominatesQuery(&dominating, reference) ||
			latest != nil && !(latest.alternative == key.alternative && latest.instance == key.instance) &&
				!ps6080MapAliasBindingBefore(latest, state.latest) {
			continue
		}
		if result == nil || ps6080MapAliasBindingBefore(result, state.latest) {
			result = state.latest
		}
	}
	return result
}

func (context *ps6080MapAliasContext) provenInactive(
	expression ast.Expr,
	position token.Pos,
	order []token.Pos,
	reference ast.Node,
	visiting map[types.Object]bool,
) bool {
	if ps6080NilExpression(context.pass, expression) {
		return true
	}
	switch value := ps2110Unparen(expression).(type) {
	case *ast.CompositeLit:
		_, mapping := types.Unalias(context.pass.TypesInfo.TypeOf(value)).Underlying().(*types.Map)
		tableExpression, _ := context.tableLiteral.(ast.Expr)
		return mapping && (tableExpression == nil || ps2110Unparen(value) != ps2110Unparen(tableExpression))
	case *ast.CallExpr:
		if len(value.Args) == 1 && context.pass.TypesInfo.Types[value.Fun].IsType() {
			return context.provenInactive(value.Args[0], position, order, reference, visiting)
		}
		identifier, direct := ps2110Unparen(value.Fun).(*ast.Ident)
		return direct && context.pass.TypesInfo.ObjectOf(identifier) == types.Universe.Lookup("make")
	case *ast.Ident:
		object := context.pass.TypesInfo.ObjectOf(value)
		if object == nil || object == context.table || context.initialAliases[object] || visiting[object] {
			return false
		}
		visiting[object] = true
		defer delete(visiting, object)
		referenceScope := context.containingLiteral(reference)
		var latest *ps6080MapAliasBinding
		for index := range context.bindings[object] {
			binding := &context.bindings[object][index]
			if binding.scope == referenceScope &&
				context.bindingBeforeQuery(binding, position, order, reference) &&
				(latest == nil || ps6080MapAliasBindingBefore(latest, binding)) {
				latest = binding
			}
		}
		return latest != nil && latest.expression != nil && context.provenInactive(
			latest.expression, latest.position, latest.order, latest.node, visiting,
		)
	}
	return false
}

func (context *ps6080MapAliasContext) parameterWrittenBefore(
	object types.Object,
	position token.Pos,
	order []token.Pos,
	reference ast.Node,
) bool {
	referenceScope := context.containingLiteral(reference)
	for index := range context.bindings[object] {
		binding := &context.bindings[object][index]
		if binding.scope == referenceScope &&
			binding.guaranteed &&
			context.bindingBeforeQuery(binding, position, order, reference) &&
			context.bindingDominatesQuery(binding, reference) &&
			!ps6080MutuallyExclusiveControlRegions(binding.region, reference, context.parents) {
			return true
		}
	}
	return false
}

func (context *ps6080MapAliasContext) within(node, ancestor ast.Node) bool {
	for current := node; current != nil; current = context.parents[current] {
		if current == ancestor {
			return true
		}
	}
	return false
}

func ps6080MapAliasBindingBefore(left, right *ps6080MapAliasBinding) bool {
	if left.position != right.position {
		return left.position < right.position
	}
	length := min(len(left.order), len(right.order))
	for index := range length {
		if left.order[index] != right.order[index] {
			return left.order[index] < right.order[index]
		}
	}
	return len(left.order) < len(right.order)
}

func ps6080PositionOrderBefore(left, right []token.Pos) bool {
	length := min(len(left), len(right))
	for index := range length {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return len(left) < len(right)
}

func ps6080PositionOrderPrefix(prefix, order []token.Pos) bool {
	return len(prefix) <= len(order) && slices.Equal(prefix, order[:len(prefix)])
}

func ps6080MapAliasBindingBeforeQuery(
	binding *ps6080MapAliasBinding,
	position token.Pos,
	order []token.Pos,
) bool {
	if binding.position != position {
		return binding.position < position
	}
	length := min(len(binding.order), len(order))
	for index := range length {
		if binding.order[index] != order[index] {
			return binding.order[index] < order[index]
		}
	}
	return len(binding.order) < len(order)
}

func (context *ps6080MapAliasContext) bindingBeforeQuery(
	binding *ps6080MapAliasBinding,
	position token.Pos,
	order []token.Pos,
	reference ast.Node,
) bool {
	lexicallyBefore := ps6080MapAliasBindingBeforeQuery(binding, position, order)
	if binding.node == nil || reference == nil ||
		binding.scope != context.containingLiteral(reference) {
		return lexicallyBefore
	}
	graph := context.graphForScope(binding.scope)
	if graph == nil {
		return lexicallyBefore
	}
	from := ps6079CFGBlockAt(graph, binding.node.Pos())
	to := ps6079CFGBlockAt(graph, reference.Pos())
	if from == nil || to == nil || from == to {
		return lexicallyBefore
	}
	seen := map[*cfg.Block]bool{from: true}
	queue := slices.Clone(ps6080FeasibleSuccessors(context.pass, context.parents, from))
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if block == to {
			return true
		}
		if block == nil || seen[block] || !block.Live {
			continue
		}
		seen[block] = true
		queue = append(queue, ps6080FeasibleSuccessors(context.pass, context.parents, block)...)
	}
	return false
}

func (context *ps6080MapAliasContext) bindingDominatesQuery(
	binding *ps6080MapAliasBinding,
	reference ast.Node,
) bool {
	graph := context.graphForScope(binding.scope)
	if graph == nil || len(graph.Blocks) == 0 || binding.node == nil || reference == nil {
		return false
	}
	from := ps6079CFGBlockAt(graph, binding.node.Pos())
	to := ps6079CFGBlockAt(graph, reference.Pos())
	if from == nil || to == nil {
		return false
	}
	if from == to {
		return binding.node.Pos() < reference.Pos()
	}
	seen := make(map[*cfg.Block]bool)
	queue := []*cfg.Block{graph.Blocks[0]}
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if block == nil || block == from || seen[block] || !block.Live {
			continue
		}
		if block == to {
			return false
		}
		seen[block] = true
		queue = append(queue, ps6080FeasibleSuccessors(context.pass, context.parents, block)...)
	}
	return true
}

func (context *ps6080MapAliasContext) graphForScope(scope *ast.FuncLit) *cfg.CFG {
	if scope == nil {
		return context.rootGraph
	}
	return context.literalGraphs[scope]
}

func ps6080MapMutationTarget(
	expression ast.Expr,
	position token.Pos,
	context *ps6080MapAliasContext,
) bool {
	if identifier, ok := ps2110Unparen(expression).(*ast.Ident); ok {
		object := context.pass.TypesInfo.ObjectOf(identifier)
		if context.tableContainer != nil && object == context.tableContainer {
			return context.tablePosition < position
		}
		return context.table != nil && object == context.table && context.active(identifier, position)
	}
	mutated := false
	ast.Inspect(expression, func(node ast.Node) bool {
		candidate, ok := node.(ast.Expr)
		if ok && context.active(candidate, position) {
			mutated = true
			return false
		}
		return !mutated
	})
	return mutated
}

func ps6080MapMutationCall(call *ast.CallExpr, context *ps6080MapAliasContext) bool {
	return ps6080MapMutationCallWithSafeArguments(call, context, nil)
}

func ps6080MapMutationCallWithSafeArguments(
	call *ast.CallExpr,
	context *ps6080MapAliasContext,
	safeArguments ps6080IndexSet,
) bool {
	if context.pass.TypesInfo.Types[call.Fun].IsType() {
		return false
	}
	if identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident); ok {
		object := context.pass.TypesInfo.ObjectOf(identifier)
		switch object {
		case types.Universe.Lookup("len"):
			return false
		case types.Universe.Lookup("delete"), types.Universe.Lookup("clear"):
			return len(call.Args) > 0 && context.active(call.Args[0], call.Pos())
		}
	}
	if ps6080MapAliasEscapes(call.Fun, call.Pos(), nil, context) {
		return true
	}
	for index, argument := range call.Args {
		if !ps6080HasIndex(safeArguments, index) &&
			ps6080MapAliasEscapes(argument, call.Pos(), nil, context) {
			return true
		}
	}
	return false
}

func ps6080MapAliasEscapes(
	expression ast.Expr,
	position token.Pos,
	destination ast.Expr,
	context *ps6080MapAliasContext,
) bool {
	if context.active(expression, position) {
		identifier, ok := ps2110Unparen(destination).(*ast.Ident)
		if !ok {
			return true
		}
		object := context.pass.TypesInfo.ObjectOf(identifier)
		if object == nil {
			return identifier.Name != "_"
		}
		_, mapping := types.Unalias(object.Type()).Underlying().(*types.Map)
		return !mapping
	}
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(expression, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	escaped := false
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || !context.active(identifier, position) {
			return !escaped
		}
		current := ast.Expr(identifier)
		for {
			switch parent := parents[current].(type) {
			case *ast.ParenExpr:
				current = parent
				continue
			case *ast.CallExpr:
				if len(parent.Args) == 1 && parent.Args[0] == current && context.pass.TypesInfo.Types[parent.Fun].IsType() {
					current = parent
					continue
				}
			}
			break
		}
		switch parent := parents[current].(type) {
		case *ast.IndexExpr:
			if parent.X == current {
				return true
			}
		case *ast.CallExpr:
			callee, direct := ps2110Unparen(parent.Fun).(*ast.Ident)
			if direct && context.pass.TypesInfo.ObjectOf(callee) == types.Universe.Lookup("len") {
				return true
			}
		case *ast.BinaryExpr:
			if (parent.Op == token.EQL || parent.Op == token.NEQ) &&
				((parent.X == current && ps6080NilExpression(context.pass, parent.Y)) ||
					(parent.Y == current && ps6080NilExpression(context.pass, parent.X))) {
				return true
			}
		}
		escaped = true
		return false
	})
	return escaped
}

func ps6080NilExpression(pass *analysis.Pass, expression ast.Expr) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && pass.TypesInfo.ObjectOf(identifier) == types.Universe.Lookup("nil")
}

func ps6080ReferencedGlobalSites(
	pass *analysis.Pass,
	function *ps6080Function,
	tables map[*types.Var][]ps6080GlobalTable,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
) []*ps6080Site {
	if len(tables) == 0 {
		return nil
	}
	body := ps6080FunctionBody(function)
	parents := ps6071Parents(body)
	graph := cfg.New(body, ps6080CallMayReturn(pass))
	invoked := ps6080InvokedFunctionLiteralResult(pass, body)
	invokedLiterals := invoked.literals
	literalGraphs := ps6080InvokedLiteralGraphs(pass, invokedLiterals)
	type tableContext struct {
		variable *types.Var
		context  *ps6080MapAliasContext
		stable   bool
	}
	contexts := make([]tableContext, 0, len(tables))
	for variable := range tables {
		aliasInfo := ps6080GlobalAliasInfoFor(pass, variable)
		aliases := aliasInfo.aliases
		initialAliases := aliasInfo.initialAliases
		contexts = append(contexts, tableContext{
			variable: variable,
			context: ps6080NewMapAliasContext(
				pass, variable, aliases, initialAliases, body,
			),
			stable: ps6080GlobalTableStableInBodyWithAliases(
				pass, variable, body, aliases, initialAliases, nil,
			),
		})
	}
	slices.SortFunc(contexts, func(left, right tableContext) int {
		return cmp.Compare(left.variable.Pos(), right.variable.Pos())
	})
	var result []*ps6080Site
	ast.Inspect(body, func(node ast.Node) bool {
		if literal, nested := node.(*ast.FuncLit); nested {
			return invokedLiterals[literal]
		}
		index, ok := node.(*ast.IndexExpr)
		if !ok ||
			!ps6080NodeReachable(pass, ps6080GraphForNode(graph, literalGraphs, parents, index), parents, index) ||
			!ps6080DynamicDispatchIndex(pass, index.Index) {
			return true
		}
		for _, candidate := range contexts {
			if !candidate.stable || !candidate.context.active(index.X, index.Pos()) {
				continue
			}
			for _, table := range tables[candidate.variable] {
				subject, _ := ps6080EnumSubject(pass, index.Index, table.enum)
				resolvedSubject := ps6080ResolvedSiteSubject(
					pass, function, subject, index, table.enum, parents,
				)
				site := &ps6080Site{
					function: function, kind: table.kind, mapTable: true, callable: table.callable,
					validated: function.validated,
					position:  table.position, end: table.end,
					enum: table.enum, constants: table.constants, excluded: table.excluded, open: table.open,
					subject:    resolvedSubject,
					references: []*types.Func{function.object},
					referenceRoutes: map[*types.Func][]ps6080ReferenceRoute{
						function.object: {{subject: resolvedSubject}},
					},
				}
				if literal := ps6080ContainingLiteral(index, parents); literal != nil && resolvedSubject != nil {
					site.referenceRoutes[function.object][0].literalScope = ps6080CPUInvocationScope(
						pass, function, literal, resolvedSubject, table.enum, parents, invoked,
						constantEnums, domains, make(map[*ast.FuncLit]bool),
					)
					aliases := ps6080ReturnedScopeObjects(
						pass, function, resolvedSubject, index, table.enum, parents,
					)
					for current := ast.Node(index); current != nil; current = parents[current] {
						returnedLiteral, nested := current.(*ast.FuncLit)
						if !nested {
							continue
						}
						returnedScopes := function.returnedLiteralScopes[returnedLiteral]
						for _, alias := range aliases {
							site.referenceRoutes[function.object][0].literalScope =
								ps6080MergeCPUArgumentScopes(
									site.referenceRoutes[function.object][0].literalScope,
									returnedScopes[alias],
								)
						}
					}
				}
				result = append(result, site)
			}
		}
		return true
	})
	return result
}

func ps6080GlobalTableStableInBodyWithAliases(
	pass *analysis.Pass,
	table types.Object,
	body *ast.BlockStmt,
	aliases map[types.Object]bool,
	initialAliases map[types.Object]bool,
	allowedWrites map[ast.Node]bool,
) bool {
	parents := ps6071Parents(body)
	graph := cfg.New(body, ps6080CallMayReturn(pass))
	invokedLiterals := ps6080InvokedFunctionLiterals(pass, body)
	literalGraphs := ps6080InvokedLiteralGraphs(pass, invokedLiterals)
	context := ps6080NewMapAliasContext(pass, table, aliases, initialAliases, body)
	stable := true
	ast.Inspect(body, func(node ast.Node) bool {
		if literal, nested := node.(*ast.FuncLit); nested {
			return invokedLiterals[literal]
		}
		if node != nil && !ps6080NodeReachable(
			pass, ps6080GraphForNode(graph, literalGraphs, parents, node), parents, node,
		) {
			return true
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			if !allowedWrites[value] {
				for _, left := range value.Lhs {
					if ps6080MapMutationTarget(left, value.Pos(), context) {
						stable = false
						return false
					}
				}
			}
			for index, right := range value.Rhs {
				var destination ast.Expr
				if len(value.Lhs) == len(value.Rhs) {
					destination = value.Lhs[index]
				}
				if ps6080MapAliasEscapes(right, value.Pos(), destination, context) {
					stable = false
					return false
				}
			}
		case *ast.ValueSpec:
			for index, right := range value.Values {
				var destination ast.Expr
				if len(value.Names) == len(value.Values) {
					destination = value.Names[index]
				}
				if ps6080MapAliasEscapes(right, value.Pos(), destination, context) {
					stable = false
					return false
				}
			}
		case *ast.CallExpr:
			if ps6080MapMutationCall(value, context) {
				stable = false
				return false
			}
		case *ast.ReturnStmt:
			for _, result := range value.Results {
				if ps6080MapAliasEscapes(result, value.Pos(), nil, context) {
					stable = false
					return false
				}
			}
		case *ast.SendStmt:
			if ps6080MapAliasEscapes(value.Value, value.Pos(), nil, context) {
				stable = false
				return false
			}
		case *ast.IncDecStmt:
			if ps6080MapMutationTarget(value.X, value.Pos(), context) {
				stable = false
				return false
			}
		}
		return stable
	})
	return stable
}

func ps6080SuppressedConstant(
	constant *types.Const,
	suppressed map[*types.Const]bool,
	constantEnums map[*types.Const][]*types.TypeName,
) bool {
	if suppressed[constant] {
		return true
	}
	value := constant.Val().ExactString()
	for alias := range suppressed {
		if alias.Val().ExactString() != value {
			continue
		}
		if types.Identical(alias.Type(), constant.Type()) || slices.ContainsFunc(constantEnums[constant], func(enum *types.TypeName) bool {
			return slices.Contains(constantEnums[alias], enum)
		}) {
			return true
		}
	}
	return false
}

func ps6080Functions(pass *analysis.Pass) map[*types.Func]*ps6080Function {
	result := make(map[*types.Func]*ps6080Function)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if !ok {
				continue
			}
			signature, _ := object.Type().(*types.Signature)
			context := strings.ToLower(function.Name.Name)
			result[object] = &ps6080Function{
				declaration: function,
				object:      object,
				signature:   signature,
				body:        function.Body,
				name:        function.Name.Name,
				context:     context,
				roles:       ps6080Roles(function.Name.Name),
				backend:     ps6080FunctionBackendContext(pass.Pkg, function.Name.Name, signature),
				validated:   ps6080CoverageValidatedComments(function.Doc),
			}
		}
	}
	return result
}

func ps6080TestFile(pass *analysis.Pass, file *ast.File) bool {
	if pass == nil || pass.Fset == nil || file == nil {
		return false
	}
	return strings.HasSuffix(pass.Fset.PositionFor(file.Pos(), false).Filename, "_test.go")
}

func ps6080RecordReturnedLiteralScopes(
	functions map[*types.Func]*ps6080Function,
	literal *ast.FuncLit,
	ownerObject *types.Func,
	scopes map[types.Object]*ps6080CPUCallScope,
) bool {
	owner := functions[ownerObject]
	if literal == nil || owner == nil {
		return false
	}
	if owner.returnedLiteralScopes == nil {
		owner.returnedLiteralScopes =
			make(map[*ast.FuncLit]map[types.Object]*ps6080CPUCallScope)
	}
	literalScopes := owner.returnedLiteralScopes[literal]
	if literalScopes == nil {
		literalScopes = make(map[types.Object]*ps6080CPUCallScope)
		owner.returnedLiteralScopes[literal] = literalScopes
	}
	changed := false
	for subject, scope := range scopes {
		merged := ps6080MergeCPUArgumentScopes(literalScopes[subject], scope)
		if !ps6080CPUCallScopesEqual(literalScopes[subject], merged) {
			changed = true
		}
		literalScopes[subject] = merged
	}
	return changed
}

func ps6080CPUCallScopesEqual(left, right *ps6080CPUCallScope) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.enum != right.enum || left.source != right.source ||
		left.universal != right.universal || len(left.fixed) != len(right.fixed) ||
		len(left.allowed) != len(right.allowed) {
		return false
	}
	for value, position := range left.fixed {
		if right.fixed[value] != position {
			return false
		}
	}
	for value, position := range left.allowed {
		if right.allowed[value] != position {
			return false
		}
	}
	return true
}

func ps6080CallableReceiverBindings(
	pass *analysis.Pass,
	caller *ps6080Function,
	target ps6080NamedFunctionTarget,
	call *ast.CallExpr,
	parents map[ast.Node]ast.Node,
) map[types.Object][]ps6080NamedFunctionTarget {
	signature, _ := target.function.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return nil
	}
	receiver := target.receiver
	var query ast.Node = call
	if target.receiverQuery != nil {
		query = target.receiverQuery
	}
	if target.methodExpression {
		if len(call.Args) == 0 {
			return nil
		}
		receiver = call.Args[0]
		query = call
	}
	if receiver == nil {
		return nil
	}
	receiverCaller := caller
	receiverParents := parents
	if target.receiverCaller != nil {
		receiverCaller = target.receiverCaller
		receiverParents = ps6071Parents(ps6080FunctionBody(receiverCaller))
	}
	if ps6080PointerReceiverType(signature.Recv().Type()) &&
		ps6080ReceiverFieldsMayChangeAfter(pass, receiverCaller, receiver, query.End()) {
		return nil
	}
	resolve := func(expression ast.Expr, resultIndex int) []ps6080NamedFunctionTarget {
		if resultIndex < 0 {
			if resolved, static := ps6080StaticNamedFunctionTarget(
				pass, receiverCaller, expression, query, receiverParents,
			); static {
				return []ps6080NamedFunctionTarget{resolved}
			}
		}
		return ps6080PossibleNamedFunctionTargetsVisiting(
			pass, receiverCaller, expression, query, receiverParents, resultIndex,
			make(map[ps6080FactorySummaryKey]int),
			make(map[ps6080FactorySummaryKey][]ps6080NamedFunctionTarget),
			target.captures, make(map[ps6080FactorySummaryKey]bool),
			make(map[types.Object]bool),
		)
	}
	bindings := make(map[types.Object][]ps6080NamedFunctionTarget)
	if ps6080CallableReceiverType(signature.Recv().Type()) {
		expression := receiver
		seen := make(map[types.Object]bool)
		for {
			switch value := ps2110Unparen(expression).(type) {
			case *ast.Ident:
				variable, ok := pass.TypesInfo.ObjectOf(value).(*types.Var)
				if !ok || seen[variable] {
					goto receiverResolved
				}
				initializer := ps6080StableLocalInitializer(
					pass, receiverCaller, variable, query, receiverParents,
				)
				if initializer == nil {
					goto receiverResolved
				}
				seen[variable] = true
				expression = initializer
			case *ast.StarExpr:
				expression = value.X
			case *ast.UnaryExpr:
				if value.Op != token.AND {
					goto receiverResolved
				}
				expression = value.X
			case *ast.CallExpr:
				if len(value.Args) != 1 || !pass.TypesInfo.Types[value.Fun].IsType() {
					goto receiverResolved
				}
				expression = value.Args[0]
			default:
				goto receiverResolved
			}
		}
	receiverResolved:
		if targets := resolve(expression, -1); len(targets) > 0 {
			bindings[signature.Recv()] = targets
		}
	}
	for _, path := range ps6080CallableReceiverFieldPaths(signature.Recv().Type()) {
		values := ps6080CallableReceiverFieldValues(
			pass, receiverCaller, receiver, query, receiverParents, path,
		)
		var targets []ps6080NamedFunctionTarget
		indices := make(map[ps6080NamedFunctionTargetKey]int)
		for _, value := range values {
			for _, target := range resolve(value.expression, value.resultIndex) {
				key := ps6080FunctionTargetKey(target)
				if index, exists := indices[key]; exists {
					ps6080MergeNamedFunctionTarget(&targets[index], target)
					continue
				}
				indices[key] = len(targets)
				targets = append(targets, target)
			}
		}
		if len(targets) > 0 {
			bindings[path[0]] = targets
		}
	}
	if len(bindings) == 0 {
		return nil
	}
	return bindings
}

func ps6080CallableReceiverType(receiver types.Type) bool {
	receiver = types.Unalias(receiver)
	if pointer, ok := receiver.(*types.Pointer); ok {
		receiver = types.Unalias(pointer.Elem())
	}
	return ps6080CallableType(receiver)
}

func ps6080PointerReceiverType(receiver types.Type) bool {
	_, pointer := types.Unalias(receiver).(*types.Pointer)
	return pointer
}

func ps6080ReceiverFieldsMayChangeAfter(
	pass *analysis.Pass,
	caller *ps6080Function,
	receiver ast.Expr,
	after token.Pos,
) bool {
	object := ps6080ExpressionRootObject(pass, receiver)
	body := ps6080FunctionBody(caller)
	if object == nil || body == nil {
		return false
	}
	type event struct{ node ast.Node }
	invoked := ps6080InvokedFunctionLiterals(pass, body)
	var events []event
	ast.Inspect(body, func(node ast.Node) bool {
		if literal, nested := node.(*ast.FuncLit); nested {
			return invoked[literal]
		}
		switch node.(type) {
		case *ast.AssignStmt, *ast.IncDecStmt, *ast.CallExpr:
			events = append(events, event{node: node})
		}
		return true
	})
	slices.SortFunc(events, func(left, right event) int {
		return cmp.Compare(left.node.Pos(), right.node.Pos())
	})
	aliases := map[types.Object]bool{object: true}
	for _, candidate := range events {
		switch value := candidate.node.(type) {
		case *ast.AssignStmt:
			rightAliases := make([]bool, len(value.Lhs))
			if len(value.Lhs) == len(value.Rhs) {
				for index, expression := range value.Rhs {
					rightAliases[index] = aliases[ps6080ExpressionRootObject(pass, expression)]
				}
			}
			for _, expression := range value.Lhs {
				if candidate.node.Pos() > after &&
					aliases[ps6080ExpressionRootObject(pass, expression)] {
					if _, direct := ps2110Unparen(expression).(*ast.Ident); !direct {
						return true
					}
				}
			}
			for index, expression := range value.Lhs {
				identifier, direct := ps2110Unparen(expression).(*ast.Ident)
				if !direct {
					continue
				}
				assigned := pass.TypesInfo.ObjectOf(identifier)
				if assigned == nil {
					continue
				}
				if assigned == object && candidate.node.Pos() <= after {
					aliases[assigned] = true
					continue
				}
				_, pointerAlias := types.Unalias(assigned.Type()).(*types.Pointer)
				if index < len(rightAliases) && rightAliases[index] && pointerAlias {
					aliases[assigned] = true
				} else {
					delete(aliases, assigned)
				}
			}
		case *ast.IncDecStmt:
			if candidate.node.Pos() > after && aliases[ps6080ExpressionRootObject(pass, value.X)] {
				return true
			}
		case *ast.CallExpr:
			if candidate.node.Pos() <= after {
				continue
			}
			if selector, ok := ps2110Unparen(value.Fun).(*ast.SelectorExpr); ok &&
				ps6080PointerMethodOnObjects(pass, selector, aliases) {
				return true
			}
			for _, argument := range value.Args {
				if !aliases[ps6080ExpressionRootObject(pass, argument)] {
					continue
				}
				argumentType := pass.TypesInfo.TypeOf(argument)
				if argumentType != nil {
					if _, pointer := types.Unalias(argumentType).(*types.Pointer); pointer {
						return true
					}
				}
			}
		}
	}
	return false
}

func ps6080ExpressionRootObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	for expression != nil {
		switch value := ps2110Unparen(expression).(type) {
		case *ast.Ident:
			return pass.TypesInfo.ObjectOf(value)
		case *ast.SelectorExpr:
			expression = value.X
		case *ast.IndexExpr:
			expression = value.X
		case *ast.IndexListExpr:
			expression = value.X
		case *ast.StarExpr:
			expression = value.X
		case *ast.UnaryExpr:
			if value.Op != token.AND {
				return nil
			}
			expression = value.X
		case *ast.CallExpr:
			if len(value.Args) != 1 || !pass.TypesInfo.Types[value.Fun].IsType() {
				return nil
			}
			expression = value.Args[0]
		default:
			return nil
		}
	}
	return nil
}

func ps6080CallableReceiverFieldPaths(receiver types.Type) [][]*types.Var {
	return ps6080CallableReceiverFieldPathsVisiting(receiver, nil, make(map[types.Type]bool))
}

func ps6080CallableReceiverFieldPathsVisiting(
	receiver types.Type,
	prefix []*types.Var,
	visiting map[types.Type]bool,
) [][]*types.Var {
	receiver = types.Unalias(receiver)
	if pointer, ok := receiver.(*types.Pointer); ok {
		receiver = types.Unalias(pointer.Elem())
	}
	if visiting[receiver] {
		return nil
	}
	structure, ok := receiver.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	visiting[receiver] = true
	defer delete(visiting, receiver)
	var result [][]*types.Var
	for index := range structure.NumFields() {
		field := structure.Field(index)
		path := append(slices.Clone(prefix), field)
		if ps6080CallableReceiverType(field.Type()) {
			result = append(result, path)
			continue
		}
		result = append(
			result,
			ps6080CallableReceiverFieldPathsVisiting(field.Type(), path, visiting)...,
		)
	}
	return result
}

func ps6080CallableReceiverFieldValues(
	pass *analysis.Pass,
	caller *ps6080Function,
	receiver ast.Expr,
	query ast.Node,
	parents map[ast.Node]ast.Node,
	path []*types.Var,
) []ps6080ReceiverFieldValue {
	values := []ps6080ReceiverFieldValue{{expression: receiver, resultIndex: -1}}
	for _, field := range path {
		var next []ps6080ReceiverFieldValue
		for _, value := range values {
			if value.resultIndex >= 0 {
				continue
			}
			next = append(next, ps6080CallableReceiverDirectFieldValues(
				pass, caller, value.expression, query, parents, field,
			)...)
		}
		values = next
		if len(values) == 0 {
			return nil
		}
	}
	return values
}

func ps6080CallableReceiverDirectFieldValues(
	pass *analysis.Pass,
	caller *ps6080Function,
	receiver ast.Expr,
	query ast.Node,
	parents map[ast.Node]ast.Node,
	field *types.Var,
) []ps6080ReceiverFieldValue {
	fieldIndex := -1
	receiverType := pass.TypesInfo.TypeOf(receiver)
	if receiverType == nil {
		return nil
	}
	receiverType = types.Unalias(receiverType)
	if pointer, ok := receiverType.(*types.Pointer); ok {
		receiverType = types.Unalias(pointer.Elem())
	}
	if structure, ok := receiverType.Underlying().(*types.Struct); ok {
		for index := range structure.NumFields() {
			if structure.Field(index) == field {
				fieldIndex = index
				break
			}
		}
	}
	if fieldIndex < 0 {
		return nil
	}
	effect := ps6080ReceiverFieldAssignmentBefore(pass, caller, receiver, query, field)
	if effect.unknown {
		return nil
	}
	result := slices.Clone(effect.values)
	if !effect.touched || !effect.guaranteed {
		if initial := ps6080CallableReceiverInitialFieldExpression(
			pass, caller, receiver, query, parents, field, fieldIndex,
		); initial != nil {
			duplicate := false
			for _, value := range result {
				if value.resultIndex < 0 &&
					ps6080EquivalentExpressions(pass, value.expression, initial) {
					duplicate = true
					break
				}
			}
			if !duplicate {
				result = append(result, ps6080ReceiverFieldValue{
					expression: initial, resultIndex: -1,
				})
			}
		}
	}
	return result
}

func ps6080CallableReceiverInitialFieldExpression(
	pass *analysis.Pass,
	caller *ps6080Function,
	receiver ast.Expr,
	query ast.Node,
	parents map[ast.Node]ast.Node,
	field *types.Var,
	fieldIndex int,
) ast.Expr {
	seen := make(map[types.Object]bool)
	for receiver != nil {
		switch value := ps2110Unparen(receiver).(type) {
		case *ast.Ident:
			variable, ok := pass.TypesInfo.ObjectOf(value).(*types.Var)
			if !ok || seen[variable] {
				return nil
			}
			seen[variable] = true
			receiver = ps6080StableLocalInitializer(pass, caller, variable, query, parents)
		case *ast.UnaryExpr:
			if value.Op != token.AND {
				return nil
			}
			receiver = value.X
		case *ast.StarExpr:
			receiver = value.X
		case *ast.CallExpr:
			if len(value.Args) != 1 || !pass.TypesInfo.Types[value.Fun].IsType() {
				return nil
			}
			receiver = value.Args[0]
		case *ast.CompositeLit:
			positional := 0
			for _, element := range value.Elts {
				if keyed, ok := element.(*ast.KeyValueExpr); ok {
					identifier, direct := ps2110Unparen(keyed.Key).(*ast.Ident)
					if direct && pass.TypesInfo.ObjectOf(identifier) == field {
						return keyed.Value
					}
					continue
				}
				if positional == fieldIndex {
					return element
				}
				positional++
			}
			return nil
		default:
			return nil
		}
	}
	return nil
}

func ps6080ReceiverFieldAssignmentValues(
	pass *analysis.Pass,
	assignment *ast.AssignStmt,
) []ps6080ReceiverFieldValue {
	if assignment == nil || len(assignment.Lhs) == 0 {
		return nil
	}
	result := make([]ps6080ReceiverFieldValue, len(assignment.Lhs))
	if len(assignment.Lhs) == len(assignment.Rhs) {
		for index, expression := range assignment.Rhs {
			result[index] = ps6080ReceiverFieldValue{
				expression: expression, resultIndex: -1,
			}
		}
		return result
	}
	if len(assignment.Rhs) != 1 {
		return nil
	}
	tuple, tupleResult := pass.TypesInfo.TypeOf(assignment.Rhs[0]).(*types.Tuple)
	if !tupleResult || tuple.Len() != len(assignment.Lhs) {
		return nil
	}
	for index := range assignment.Lhs {
		result[index] = ps6080ReceiverFieldValue{
			expression: assignment.Rhs[0], resultIndex: index,
		}
	}
	return result
}

func ps6080ReceiverFieldAssignmentBefore(
	pass *analysis.Pass,
	caller *ps6080Function,
	receiver ast.Expr,
	query ast.Node,
	field *types.Var,
) ps6080ReceiverFieldEffect {
	body := ps6080FunctionBody(caller)
	root := ps6080ExpressionRootObject(pass, receiver)
	if body == nil || root == nil || query == nil {
		return ps6080ReceiverFieldEffect{}
	}
	parents := ps6071Parents(body)
	graph := cfg.New(body, ps6080CallMayReturn(pass))
	invoked := ps6080InvokedFunctionLiteralResult(pass, body)
	type fieldEvent struct {
		position   token.Pos
		assignment *ast.AssignStmt
		call       *ast.CallExpr
	}
	var events []fieldEvent
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil || node.Pos() >= query.Pos() {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			// Go evaluates every operand on both sides before it performs the
			// assignment. Place the transfer after calls nested in those operands.
			events = append(events, fieldEvent{position: value.End(), assignment: value})
		case *ast.CallExpr:
			// A containing call runs after calls in its function and arguments.
			events = append(events, fieldEvent{position: value.End(), call: value})
		}
		return true
	})
	slices.SortFunc(events, func(left, right fieldEvent) int {
		if order := cmp.Compare(left.position, right.position); order != 0 {
			return order
		}
		if left.assignment != nil && right.call != nil {
			return 1
		}
		if left.call != nil && right.assignment != nil {
			return -1
		}
		return 0
	})
	calledLiterals := make(map[*ast.CallExpr][]*ast.FuncLit)
	for literal, calls := range invoked.calls {
		for call := range calls {
			calledLiterals[call] = append(calledLiterals[call], literal)
		}
	}
	mustExecute := func(node ast.Node) (bool, bool) {
		if node == nil || !ps6080StaticallyReachable(pass, parents, node) ||
			!ps6080NodeReachable(pass, graph, parents, node) {
			return false, false
		}
		return true, !ps6080CFGEntryMayReachWithoutAssignments(
			pass, graph, parents, query.Pos(), []token.Pos{node.Pos()},
		)
	}
	aliases := map[types.Object]bool{root: true}
	aliasKills := make(map[types.Object][]token.Pos)
	var reachableWrites []ps6080FunctionAssignment
	var guaranteedPositions []token.Pos
	var unknownPositions []token.Pos
	touched := false
	for _, event := range events {
		reachable, mandatory := false, false
		if event.assignment != nil {
			reachable, mandatory = mustExecute(event.assignment)
		}
		if event.call != nil {
			reachable, _ = mustExecute(event.call)
			if !reachable {
				continue
			}
			callAliases := maps.Clone(aliases)
			for alias := range callAliases {
				if !ps6080AliasMayReachPosition(
					pass, graph, parents, aliases, aliasKills, alias, event.call.Pos(),
				) {
					delete(callAliases, alias)
				}
			}
			for _, literal := range calledLiterals[event.call] {
				effect := ps6080InvokedReceiverFieldWrite(
					pass, literal, callAliases, field,
				)
				if !effect.touched {
					continue
				}
				touched = true
				if effect.unknown {
					unknownPositions = append(unknownPositions, event.call.Pos())
				}
				for _, value := range effect.values {
					reachableWrites = append(reachableWrites, ps6080FunctionAssignment{
						node: event.call, expression: value.expression,
						resultIndex: value.resultIndex,
					})
				}
				if effect.guaranteed {
					guaranteedPositions = append(guaranteedPositions, event.call.Pos())
				}
			}
			if target, named := ps6080StaticNamedCallee(
				pass, caller, event.call, parents,
			); named {
				calleeAliases, calleeArguments := ps6080NamedReceiverFieldAliases(
					pass, target, event.call, callAliases,
				)
				if len(calleeAliases) > 0 {
					effect := ps6080InvokedNamedReceiverFieldWrite(
						pass, target.function, calleeAliases, field,
					)
					if effect.touched {
						touched = true
						if effect.unknown {
							unknownPositions = append(unknownPositions, event.call.Pos())
						}
						declaration := ps6080NamedFunctionDeclaration(pass, target.function)
						for _, value := range effect.values {
							value.expression = ps6080NamedReceiverFieldArgument(
								pass, value.expression, calleeArguments, declaration.Body,
							)
							reachableWrites = append(reachableWrites, ps6080FunctionAssignment{
								node: event.call, expression: value.expression,
								resultIndex: value.resultIndex,
							})
						}
						if effect.guaranteed {
							guaranteedPositions = append(guaranteedPositions, event.call.Pos())
						}
					}
				}
			}
			continue
		}
		assignment := event.assignment
		assignmentValues := ps6080ReceiverFieldAssignmentValues(pass, assignment)
		if !reachable || assignment == nil || assignmentValues == nil {
			continue
		}
		rightAliases := make([]bool, len(assignmentValues))
		for index, value := range assignmentValues {
			right := ps6080ExpressionRootObject(pass, value.expression)
			if value.resultIndex >= 0 {
				continue
			}
			rightAliases[index] = ps6080AliasMayReachPosition(
				pass, graph, parents, aliases, aliasKills, right, assignment.Pos(),
			)
		}
		for index, left := range assignment.Lhs {
			if identifier, direct := ps2110Unparen(left).(*ast.Ident); direct {
				assigned := pass.TypesInfo.ObjectOf(identifier)
				if assigned == nil {
					continue
				}
				if assigned == root {
					if !rightAliases[index] {
						if mandatory {
							clear(aliases)
							clear(aliasKills)
							reachableWrites = nil
							guaranteedPositions = nil
							unknownPositions = nil
							touched = false
						} else {
							for alias := range aliases {
								if alias != root {
									aliasKills[alias] = append(
										aliasKills[alias], assignment.Pos(),
									)
								}
							}
						}
					}
					aliases[assigned] = true
					continue
				}
				_, pointerAlias := types.Unalias(assigned.Type()).(*types.Pointer)
				if pointerAlias && rightAliases[index] {
					aliases[assigned] = true
					if mandatory {
						delete(aliasKills, assigned)
					}
				} else if mandatory {
					delete(aliases, assigned)
					delete(aliasKills, assigned)
				} else {
					aliasKills[assigned] = append(aliasKills[assigned], assignment.Pos())
				}
				continue
			}
			selector, selected := ps2110Unparen(left).(*ast.SelectorExpr)
			if !selected || pass.TypesInfo.ObjectOf(selector.Sel) != field ||
				!ps6080AliasMayReachPosition(
					pass, graph, parents, aliases, aliasKills,
					ps6080ExpressionRootObject(pass, selector.X), assignment.Pos(),
				) {
				continue
			}
			touched = true
			reachableWrites = append(reachableWrites, ps6080FunctionAssignment{
				node: assignment, expression: assignmentValues[index].expression,
				resultIndex: assignmentValues[index].resultIndex,
			})
			guaranteedPositions = append(guaranteedPositions, assignment.Pos())
		}
	}
	if !touched {
		return ps6080ReceiverFieldEffect{}
	}
	for _, position := range unknownPositions {
		if ps6080CFGPositionMayReachWithoutAssignments(
			pass, graph, parents, position, query.Pos(), guaranteedPositions,
		) {
			return ps6080ReceiverFieldEffect{touched: true, unknown: true}
		}
	}
	return ps6080ReceiverFieldEffect{
		values: ps6080FinalReceiverFieldValuesBefore(
			pass, graph, parents, reachableWrites, guaranteedPositions, query.Pos(),
		),
		touched: true,
		guaranteed: !ps6080CFGEntryMayReachWithoutAssignments(
			pass, graph, parents, query.Pos(), guaranteedPositions,
		),
	}
}

func ps6080FinalReceiverFieldValuesBefore(
	pass *analysis.Pass,
	graph *cfg.CFG,
	parents map[ast.Node]ast.Node,
	writes []ps6080FunctionAssignment,
	guaranteedPositions []token.Pos,
	query token.Pos,
) []ps6080ReceiverFieldValue {
	var result []ps6080ReceiverFieldValue
	for _, write := range writes {
		if !ps6080CFGPositionMayReachWithoutAssignments(
			pass, graph, parents, write.node.Pos(), query, guaranteedPositions,
		) {
			continue
		}
		duplicate := false
		for _, value := range result {
			if value.resultIndex == write.resultIndex &&
				ps6080EquivalentExpressions(pass, value.expression, write.expression) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, ps6080ReceiverFieldValue{
				expression: write.expression, resultIndex: write.resultIndex,
			})
		}
	}
	return result
}

func ps6080AliasMayReachPosition(
	pass *analysis.Pass,
	graph *cfg.CFG,
	parents map[ast.Node]ast.Node,
	aliases map[types.Object]bool,
	kills map[types.Object][]token.Pos,
	object types.Object,
	position token.Pos,
) bool {
	return aliases[object] && (len(kills[object]) == 0 ||
		ps6080CFGEntryMayReachWithoutAssignments(pass, graph, parents, position, kills[object]))
}

type ps6080ReceiverFieldEffect struct {
	values     []ps6080ReceiverFieldValue
	touched    bool
	guaranteed bool
	unknown    bool
}

type ps6080ReceiverFieldValue struct {
	expression  ast.Expr
	resultIndex int
}

func ps6080InvokedReceiverFieldWrite(
	pass *analysis.Pass,
	literal *ast.FuncLit,
	aliases map[types.Object]bool,
	field *types.Var,
) ps6080ReceiverFieldEffect {
	return ps6080InvokedReceiverFieldWriteVisiting(
		pass, literal, aliases, field, make(map[ast.Node]bool),
	)
}

func ps6080InvokedNamedReceiverFieldWrite(
	pass *analysis.Pass,
	function *types.Func,
	aliases map[types.Object]bool,
	field *types.Var,
) ps6080ReceiverFieldEffect {
	declaration := ps6080NamedFunctionDeclaration(pass, function)
	if declaration == nil {
		// An external body receiving the receiver alias cannot be proven read-only.
		return ps6080ReceiverFieldEffect{touched: len(aliases) > 0, unknown: len(aliases) > 0}
	}
	return ps6080InvokedReceiverFieldWriteBodyVisiting(
		pass, declaration, declaration.Body, aliases, field, make(map[ast.Node]bool),
	)
}

func ps6080InvokedReceiverFieldWriteVisiting(
	pass *analysis.Pass,
	literal *ast.FuncLit,
	aliases map[types.Object]bool,
	field *types.Var,
	visiting map[ast.Node]bool,
) ps6080ReceiverFieldEffect {
	if literal == nil {
		return ps6080ReceiverFieldEffect{}
	}
	return ps6080InvokedReceiverFieldWriteBodyVisiting(
		pass, literal, literal.Body, aliases, field, visiting,
	)
}

func ps6080InvokedReceiverFieldWriteBodyVisiting(
	pass *analysis.Pass,
	identity ast.Node,
	body *ast.BlockStmt,
	aliases map[types.Object]bool,
	field *types.Var,
	visiting map[ast.Node]bool,
) ps6080ReceiverFieldEffect {
	if identity == nil || body == nil || visiting[identity] {
		return ps6080ReceiverFieldEffect{}
	}
	visiting[identity] = true
	defer delete(visiting, identity)
	parents := ps6071Parents(body)
	graph := cfg.New(body, ps6080CallMayReturn(pass))
	localAliases := maps.Clone(aliases)
	aliasKills := make(map[types.Object][]token.Pos)
	var writes []ps6080FunctionAssignment
	var guaranteedPositions []token.Pos
	var unknownPositions []token.Pos
	type setterEvent struct {
		position   token.Pos
		assignment *ast.AssignStmt
		call       *ast.CallExpr
	}
	var events []setterEvent
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			// Alias transfers happen only after calls in both assignment operands.
			events = append(events, setterEvent{position: value.End(), assignment: value})
		case *ast.CallExpr:
			// Nested calls execute before their containing call.
			events = append(events, setterEvent{position: value.End(), call: value})
		}
		return true
	})
	slices.SortFunc(events, func(left, right setterEvent) int {
		if order := cmp.Compare(left.position, right.position); order != 0 {
			return order
		}
		if left.assignment != nil && right.call != nil {
			return 1
		}
		if left.call != nil && right.assignment != nil {
			return -1
		}
		return 0
	})
	invoked := ps6080InvokedFunctionLiteralResult(pass, body)
	calledLiterals := make(map[*ast.CallExpr][]*ast.FuncLit)
	for nested, calls := range invoked.calls {
		for call := range calls {
			calledLiterals[call] = append(calledLiterals[call], nested)
		}
	}
	caller := &ps6080Function{body: body}
	if declaration, named := identity.(*ast.FuncDecl); named {
		caller.declaration = declaration
		caller.object, _ = pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	}
	for _, event := range events {
		if event.call != nil {
			call := event.call
			if !ps6080StaticallyReachable(pass, parents, call) ||
				!ps6080NodeReachable(pass, graph, parents, call) {
				continue
			}
			switch parent := parents[call].(type) {
			case *ast.DeferStmt:
				if parent.Call == call {
					continue
				}
			case *ast.GoStmt:
				if parent.Call == call {
					continue
				}
			}
			callAliases := maps.Clone(localAliases)
			for alias := range callAliases {
				if !ps6080AliasMayReachPosition(
					pass, graph, parents, localAliases, aliasKills, alias, call.Pos(),
				) {
					delete(callAliases, alias)
				}
			}
			for _, nested := range calledLiterals[call] {
				effect := ps6080InvokedReceiverFieldWriteVisiting(
					pass, nested, callAliases, field, visiting,
				)
				if !effect.touched {
					continue
				}
				if effect.unknown {
					unknownPositions = append(unknownPositions, call.Pos())
				}
				for _, value := range effect.values {
					writes = append(writes, ps6080FunctionAssignment{
						node: call, expression: value.expression, resultIndex: value.resultIndex,
					})
				}
				if effect.guaranteed {
					guaranteedPositions = append(guaranteedPositions, call.Pos())
				}
			}
			if target, named := ps6080StaticNamedCallee(pass, caller, call, parents); named {
				calleeAliases, calleeArguments := ps6080NamedReceiverFieldAliases(
					pass, target, call, callAliases,
				)
				if len(calleeAliases) > 0 {
					declaration := ps6080NamedFunctionDeclaration(pass, target.function)
					if declaration == nil {
						unknownPositions = append(unknownPositions, call.Pos())
						continue
					}
					effect := ps6080InvokedReceiverFieldWriteBodyVisiting(
						pass, declaration, declaration.Body, calleeAliases, field, visiting,
					)
					if effect.unknown {
						unknownPositions = append(unknownPositions, call.Pos())
					}
					for _, value := range effect.values {
						value.expression = ps6080NamedReceiverFieldArgument(
							pass, value.expression, calleeArguments, declaration.Body,
						)
						writes = append(writes, ps6080FunctionAssignment{
							node: call, expression: value.expression,
							resultIndex: value.resultIndex,
						})
					}
					if effect.guaranteed {
						guaranteedPositions = append(guaranteedPositions, call.Pos())
					}
				}
			}
			continue
		}
		assignment := event.assignment
		assignmentValues := ps6080ReceiverFieldAssignmentValues(pass, assignment)
		if assignment == nil || assignmentValues == nil ||
			!ps6080StaticallyReachable(pass, parents, assignment) ||
			!ps6080NodeReachable(pass, graph, parents, assignment) {
			continue
		}
		mandatory := ps6080CFGMustExecuteAnyAssignment(
			pass, graph, parents, []token.Pos{assignment.Pos()},
		)
		rightAliases := make([]bool, len(assignmentValues))
		for index, value := range assignmentValues {
			if value.resultIndex >= 0 {
				continue
			}
			rightAliases[index] = ps6080AliasMayReachPosition(
				pass, graph, parents, localAliases, aliasKills,
				ps6080ExpressionRootObject(pass, value.expression), assignment.Pos(),
			)
		}
		for index, left := range assignment.Lhs {
			if identifier, direct := ps2110Unparen(left).(*ast.Ident); direct {
				assigned := pass.TypesInfo.ObjectOf(identifier)
				if assigned == nil {
					continue
				}
				_, pointerAlias := types.Unalias(assigned.Type()).(*types.Pointer)
				if pointerAlias && rightAliases[index] {
					localAliases[assigned] = true
					if mandatory {
						delete(aliasKills, assigned)
					}
				} else if mandatory {
					delete(localAliases, assigned)
					delete(aliasKills, assigned)
				} else {
					aliasKills[assigned] = append(aliasKills[assigned], assignment.Pos())
				}
				continue
			}
			selector, selected := ps2110Unparen(left).(*ast.SelectorExpr)
			if selected && pass.TypesInfo.ObjectOf(selector.Sel) == field &&
				ps6080AliasMayReachPosition(
					pass, graph, parents, localAliases, aliasKills,
					ps6080ExpressionRootObject(pass, selector.X), assignment.Pos(),
				) {
				writes = append(writes, ps6080FunctionAssignment{
					node: assignment, expression: assignmentValues[index].expression,
					resultIndex: assignmentValues[index].resultIndex,
				})
				guaranteedPositions = append(guaranteedPositions, assignment.Pos())
			}
		}
	}
	// Events are already in evaluation order. Sorting by token.Pos would put a
	// containing assignment before a call in its RHS, reversing Go execution.
	for _, position := range unknownPositions {
		if ps6080CFGPositionMayReachExitWithoutAssignments(
			pass, graph, parents, position, guaranteedPositions,
		) {
			return ps6080ReceiverFieldEffect{touched: true, unknown: true}
		}
	}
	if len(writes) == 0 {
		return ps6080ReceiverFieldEffect{}
	}
	return ps6080ReceiverFieldEffect{
		values: ps6080FinalReceiverFieldValues(
			pass, graph, parents, writes, guaranteedPositions,
		),
		touched: true,
		guaranteed: ps6080CFGMustExecuteAnyAssignment(
			pass, graph, parents, guaranteedPositions,
		),
	}
}

func ps6080FinalReceiverFieldValues(
	pass *analysis.Pass,
	graph *cfg.CFG,
	parents map[ast.Node]ast.Node,
	writes []ps6080FunctionAssignment,
	guaranteedPositions []token.Pos,
) []ps6080ReceiverFieldValue {
	var result []ps6080ReceiverFieldValue
	for _, write := range writes {
		if !ps6080CFGPositionMayReachExitWithoutAssignments(
			pass, graph, parents, write.node.Pos(), guaranteedPositions,
		) {
			continue
		}
		duplicate := false
		for _, value := range result {
			if value.resultIndex == write.resultIndex &&
				ps6080EquivalentExpressions(pass, value.expression, write.expression) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, ps6080ReceiverFieldValue{
				expression: write.expression, resultIndex: write.resultIndex,
			})
		}
	}
	return result
}

func ps6080NamedFunctionDeclaration(
	pass *analysis.Pass,
	function *types.Func,
) *ast.FuncDecl {
	if function == nil || function.Pkg() != pass.Pkg {
		return nil
	}
	for _, file := range pass.Files {
		for _, candidate := range file.Decls {
			declaration, ok := candidate.(*ast.FuncDecl)
			if ok && pass.TypesInfo.Defs[declaration.Name] == function {
				return declaration
			}
		}
	}
	return nil
}

func ps6080NamedReceiverFieldAliases(
	pass *analysis.Pass,
	target ps6080NamedFunctionTarget,
	call *ast.CallExpr,
	callerAliases map[types.Object]bool,
) (map[types.Object]bool, map[types.Object]ast.Expr) {
	result := make(map[types.Object]bool)
	arguments := make(map[types.Object]ast.Expr)
	if target.function == nil || call == nil {
		return result, arguments
	}
	signature, ok := target.function.Type().(*types.Signature)
	if !ok {
		return result, arguments
	}
	bind := func(parameter types.Object, expression ast.Expr) {
		if parameter == nil || expression == nil {
			return
		}
		arguments[parameter] = expression
		if callerAliases[ps6080ExpressionRootObject(pass, expression)] {
			result[parameter] = true
		}
	}
	argument := 0
	if receiver := signature.Recv(); receiver != nil {
		switch {
		case target.receiver != nil:
			bind(receiver, target.receiver)
		case target.methodExpression && len(call.Args) > 0:
			bind(receiver, call.Args[0])
			argument = 1
		}
	}
	parameters := signature.Params()
	for index := 0; index < parameters.Len() && argument < len(call.Args); index++ {
		bind(parameters.At(index), call.Args[argument])
		argument++
		if signature.Variadic() && index == parameters.Len()-1 {
			for ; argument < len(call.Args); argument++ {
				bind(parameters.At(index), call.Args[argument])
			}
		}
	}
	return result, arguments
}

func ps6080NamedReceiverFieldArgument(
	pass *analysis.Pass,
	expression ast.Expr,
	arguments map[types.Object]ast.Expr,
	body *ast.BlockStmt,
) ast.Expr {
	if body == nil {
		return expression
	}
	parents := ps6071Parents(body)
	caller := &ps6080Function{body: body}
	seen := make(map[types.Object]bool)
	for expression != nil {
		switch value := ps2110Unparen(expression).(type) {
		case *ast.CallExpr:
			if len(value.Args) != 1 || !pass.TypesInfo.Types[value.Fun].IsType() {
				return expression
			}
			expression = value.Args[0]
		case *ast.Ident:
			object := pass.TypesInfo.ObjectOf(value)
			variable, local := object.(*types.Var)
			if !local || seen[variable] {
				return expression
			}
			seen[variable] = true
			initializer := ps6080StableLocalInitializer(
				pass, caller, variable, expression, parents,
			)
			if initializer != nil {
				expression = initializer
				continue
			}
			if argument := arguments[object]; argument != nil &&
				!ps6080ObjectMayMutateBefore(pass, body, object, expression) {
				return argument
			}
			return expression
		default:
			return expression
		}
	}
	return nil
}

func ps6080ObjectMayMutateBefore(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	object types.Object,
	query ast.Node,
) bool {
	if body == nil || object == nil || query == nil {
		return false
	}
	objects := map[types.Object]bool{object: true}
	parents := ps6071Parents(body)
	graph := cfg.New(body, ps6080CallMayReturn(pass))
	mutated := false
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil || mutated || node.Pos() >= query.Pos() {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if !ps6080StaticallyReachable(pass, parents, node) ||
			!ps6080NodeReachable(pass, graph, parents, node) ||
			!ps6080CFGPositionMayReachWithoutAssignments(
				pass, graph, parents, node.Pos(), query.Pos(), nil,
			) {
			return true
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			mutated = ps6080AssignmentTouchesObjects(pass, value, objects)
		case *ast.IncDecStmt, *ast.RangeStmt:
			if statement, ok := node.(ast.Stmt); ok {
				mutated = ps6080StatementsWriteObjects(pass, []ast.Stmt{statement}, objects)
			}
		case *ast.UnaryExpr:
			identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
			mutated = value.Op == token.AND && direct &&
				pass.TypesInfo.ObjectOf(identifier) == object
		case *ast.SelectorExpr:
			mutated = ps6080PointerMethodOnObjects(pass, value, objects)
		}
		return !mutated
	})
	return mutated
}

func ps6080PopulateReachableCallees(
	pass *analysis.Pass,
	functions map[*types.Func]*ps6080Function,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
) {
	namedCallbacks := ps6080NamedCallbackParameters(pass)
	queued := make(map[*types.Func]bool)
	var queue []*types.Func
	mergeCallableBindings := func(
		object *types.Func,
		bindings map[types.Object][]ps6080NamedFunctionTarget,
	) {
		function := functions[object]
		if function == nil || len(bindings) == 0 {
			return
		}
		var changed bool
		function.capturedTargets, changed = ps6080MergeFunctionTargetBindings(
			function.capturedTargets, bindings,
		)
		if changed && function.scanned {
			function.callees = nil
			function.cpuCalls = nil
			function.scanned = false
			queue = append(queue, object)
		}
	}
	recordReturnedLiteralScopes := func(
		literal *ast.FuncLit,
		ownerObject *types.Func,
		scopes map[types.Object]*ps6080CPUCallScope,
	) {
		if !ps6080RecordReturnedLiteralScopes(functions, literal, ownerObject, scopes) {
			return
		}
		owner := functions[ownerObject]
		if owner != nil && owner.scanned {
			owner.callees = nil
			owner.cpuCalls = nil
			owner.scanned = false
			queue = append(queue, ownerObject)
		}
	}
	for object, function := range functions {
		if function.roles != 0 {
			queued[object] = true
			queue = append(queue, object)
		}
	}
	for len(queue) > 0 {
		object := queue[0]
		queue = queue[1:]
		function := functions[object]
		function.scanned = true
		seen := make(map[*types.Func]bool)
		body := ps6080FunctionBody(function)
		parents := ps6071Parents(body)
		graph := cfg.New(body, ps6080CallMayReturn(pass))
		invoked := ps6080InvokedFunctionLiteralResult(pass, body)
		invokedLiterals := invoked.literals
		literalGraphs := ps6080InvokedLiteralGraphs(pass, invokedLiterals)
		ast.Inspect(body, func(node ast.Node) bool {
			if literal, nested := node.(*ast.FuncLit); nested {
				return invokedLiterals[literal]
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if !ps6080NodeReachable(pass, ps6080GraphForNode(graph, literalGraphs, parents, call), parents, call) {
				return true
			}
			target, ok := ps6080StaticNamedCallee(pass, function, call, parents)
			callee := target.function
			if ok && callee.Pkg() == pass.Pkg && functions[callee] != nil {
				mergeCallableBindings(
					callee, ps6080CallableReceiverBindings(pass, function, target, call, parents),
				)
				if callee != object {
					functions[callee].cpuIncoming = true
				}
				if callee != object && !seen[callee] {
					seen[callee] = true
					function.callees = append(function.callees, callee)
				}
				function.cpuCalls = append(function.cpuCalls, ps6080CPUCall{
					callee: callee,
					scopes: ps6080CPUCallScopes(
						pass, function, callee, call, parents, invoked, constantEnums, domains, &target,
					),
				})
				if callee != object && !queued[callee] {
					queued[callee] = true
					queue = append(queue, callee)
				}
				for _, callback := range ps6080NamedCallbackCalls(
					pass, function, callee, call, parents, invoked, namedCallbacks,
					constantEnums, domains,
				) {
					if callback.factoryScanned {
						recordReturnedLiteralScopes(
							callback.literal, callback.literalOwner, callback.scopes,
						)
					}
					mergeCallableBindings(callback.callee, callback.bindings)
					if callback.callee == object || functions[callback.callee] == nil {
						continue
					}
					functions[callback.callee].cpuIncoming = true
					if !seen[callback.callee] {
						seen[callback.callee] = true
						function.callees = append(function.callees, callback.callee)
					}
					function.cpuCalls = append(function.cpuCalls, callback)
					if !queued[callback.callee] {
						queued[callback.callee] = true
						queue = append(queue, callback.callee)
					}
				}
			}
			if !ok {
				possibleTargets := ps6080PossibleNamedFunctionTargets(
					pass, function, call.Fun, call, parents,
				)
				for _, possible := range possibleTargets {
					callee = possible.function
					if possible.literal != nil && possible.factoryScanned {
						recordReturnedLiteralScopes(
							possible.literal, possible.literalOwner,
							ps6080CPUCallScopes(
								pass, function, callee, call, parents, invoked, constantEnums, domains,
								&possible,
							),
						)
					}
					calleeInfo := functions[callee]
					if possible.literal != nil && possible.factoryResult && !possible.factoryScanned &&
						calleeInfo == nil {
						signature, _ := pass.TypesInfo.TypeOf(possible.literal).(*types.Signature)
						calleeInfo = &ps6080Function{
							declaration:     function.declaration,
							object:          callee,
							signature:       signature,
							body:            possible.literal.Body,
							name:            "anonymous function",
							context:         function.context,
							capturedTargets: ps6080CloneFunctionTargetBindings(possible.captures),
						}
						functions[callee] = calleeInfo
					}
					if possible.literal != nil && possible.factoryResult && !possible.factoryScanned &&
						calleeInfo != nil {
						var capturesChanged bool
						calleeInfo.capturedTargets, capturesChanged = ps6080MergeFunctionTargetBindings(
							calleeInfo.capturedTargets, possible.captures,
						)
						if capturesChanged && calleeInfo.scanned {
							calleeInfo.callees = nil
							calleeInfo.cpuCalls = nil
							calleeInfo.scanned = false
							queue = append(queue, callee)
						}
					}
					mergeCallableBindings(
						callee,
						ps6080CallableReceiverBindings(pass, function, possible, call, parents),
					)
					if callee == nil || callee.Pkg() != pass.Pkg || calleeInfo == nil {
						continue
					}
					if callee != object {
						calleeInfo.cpuIncoming = true
					}
					if callee != object && !seen[callee] {
						seen[callee] = true
						function.callees = append(function.callees, callee)
					}
					function.cpuCalls = append(function.cpuCalls, ps6080CPUCall{
						callee: callee,
						scopes: ps6080CPUCallScopes(
							pass, function, callee, call, parents, invoked, constantEnums, domains,
							&possible,
						),
					})
					if callee != object && !queued[callee] {
						queued[callee] = true
						queue = append(queue, callee)
					}
				}
			}
			return true
		})
	}
}

func ps6080NamedCallbackCalls(
	pass *analysis.Pass,
	caller *ps6080Function,
	callee *types.Func,
	call *ast.CallExpr,
	parents map[ast.Node]ast.Node,
	invoked *ps6080InvokedLiteralResult,
	callbacks map[*types.Func]ps6080CallbackMappings,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
) []ps6080CPUCall {
	var result []ps6080CPUCall
	mayCallbacks := ps6080MayNamedCallbackSites(pass)
	for callbackParameter, mapping := range callbacks[callee] {
		if mapping == nil || callbackParameter >= len(call.Args) {
			continue
		}
		if callbackParameter < len(mayCallbacks[callee]) && len(mayCallbacks[callee][callbackParameter]) > 0 {
			continue
		}
		target, ok := ps6080StaticNamedFunctionTarget(
			pass, caller, call.Args[callbackParameter], call, parents,
		)
		if !ok || target.function.Pkg() != pass.Pkg {
			continue
		}
		signature, _ := target.function.Type().(*types.Signature)
		callbackSignature, _ := pass.TypesInfo.TypeOf(call.Args[callbackParameter]).(*types.Signature)
		if signature == nil {
			continue
		}
		expectedParameters := signature.Params().Len()
		if target.methodExpression {
			expectedParameters++
		}
		if signature.Recv() != nil && target.receiver == nil && !target.methodExpression ||
			callbackSignature == nil || callbackSignature.Params().Len() != expectedParameters {
			continue
		}
		scopes := make(map[types.Object]*ps6080CPUCallScope, signature.Params().Len()+1)
		mappedScope := func(index int, enum *types.TypeName) *ps6080CPUCallScope {
			var scope *ps6080CPUCallScope
			if index < len(mapping) {
				for sourceIndex, present := range mapping[index] {
					if !present || sourceIndex >= len(call.Args) {
						continue
					}
					addition := ps6080CPUArgumentScope(
						pass, caller, call.Args[sourceIndex], call, enum, parents, invoked,
						constantEnums, domains, make(map[*ast.FuncLit]bool),
					)
					scope = ps6080MergeCPUArgumentScopes(scope, addition)
				}
			}
			if scope == nil {
				scope = &ps6080CPUCallScope{enum: enum, universal: true}
			}
			return scope
		}
		argumentOffset := 0
		if receiver := signature.Recv(); receiver != nil {
			if enum, enumType := ps6080EnumType(receiver.Type()); enumType {
				if target.methodExpression {
					scopes[receiver] = mappedScope(0, enum)
				} else {
					receiverQuery := target.receiverQuery
					if receiverQuery == nil {
						receiverQuery = call
					}
					scopes[receiver] = ps6080CPUArgumentScope(
						pass, caller, target.receiver, receiverQuery, enum, parents, invoked,
						constantEnums, domains, make(map[*ast.FuncLit]bool),
					)
				}
			}
			if target.methodExpression {
				argumentOffset = 1
			}
		}
		for targetIndex := range signature.Params().Len() {
			parameter := signature.Params().At(targetIndex)
			enum, enumType := ps6080EnumType(parameter.Type())
			if !enumType {
				continue
			}
			scopes[parameter] = mappedScope(targetIndex+argumentOffset, enum)
		}
		result = append(result, ps6080CPUCall{
			callee: target.function, scopes: scopes,
			bindings: ps6080CallableReceiverBindings(pass, caller, target, call, parents),
		})
	}
	for callbackParameter, sites := range mayCallbacks[callee] {
		if callbackParameter >= len(call.Args) {
			continue
		}
		target, ok := ps6080StaticNamedFunctionTarget(
			pass, caller, call.Args[callbackParameter], call, parents,
		)
		if !ok || target.function.Pkg() != pass.Pkg {
			continue
		}
		signature, _ := target.function.Type().(*types.Signature)
		callbackSignature, _ := pass.TypesInfo.TypeOf(call.Args[callbackParameter]).(*types.Signature)
		if signature == nil {
			continue
		}
		expectedParameters := signature.Params().Len()
		if target.methodExpression {
			expectedParameters++
		}
		if signature.Recv() != nil && target.receiver == nil && !target.methodExpression ||
			callbackSignature == nil || callbackSignature.Params().Len() != expectedParameters {
			continue
		}
		dispatcher, _ := callee.Type().(*types.Signature)
		for _, site := range sites {
			result = append(result, ps6080CPUCall{
				callee:   target.function,
				bindings: ps6080CallableReceiverBindings(pass, caller, target, call, parents),
				scopes: ps6080NamedCallbackSiteScopes(
					pass, caller, call, parents, invoked, site, dispatcher, signature,
					target.receiver, target.receiverQuery, target.methodExpression,
					constantEnums, domains,
				),
			})
		}
	}
	callbackParameters := make([]bool, max(len(callbacks[callee]), len(mayCallbacks[callee])))
	for index, mapping := range callbacks[callee] {
		callbackParameters[index] = mapping != nil
	}
	for index, sites := range mayCallbacks[callee] {
		callbackParameters[index] = callbackParameters[index] || len(sites) > 0
	}
	for callbackParameter, callback := range callbackParameters {
		if !callback || callbackParameter >= len(call.Args) {
			continue
		}
		if _, resolved := ps6080StaticNamedFunctionTarget(
			pass, caller, call.Args[callbackParameter], call, parents,
		); resolved {
			continue
		}
		for _, target := range ps6080PossibleNamedFunctionTargets(
			pass, caller, call.Args[callbackParameter], call, parents,
		) {
			if target.function == nil || target.function.Pkg() != pass.Pkg {
				continue
			}
			var scopeSets []map[types.Object]*ps6080CPUCallScope
			var mapping ps6080InvocationArguments
			if callbackParameter < len(callbacks[callee]) {
				mapping = callbacks[callee][callbackParameter]
			}
			var sites []*ps6080MayCallbackSite
			if callbackParameter < len(mayCallbacks[callee]) {
				sites = mayCallbacks[callee][callbackParameter]
			}
			if mapping != nil && len(sites) == 0 {
				if scopes, valid := ps6080NamedCallbackMappingScopes(
					pass, caller, call, call.Args[callbackParameter], parents, invoked, mapping,
					target, constantEnums, domains,
				); valid {
					scopeSets = append(scopeSets, scopes)
				}
			}
			if len(sites) > 0 {
				dispatcher, _ := callee.Type().(*types.Signature)
				targetSignature, _ := target.function.Type().(*types.Signature)
				if targetSignature != nil {
					for _, site := range sites {
						scopeSets = append(scopeSets, ps6080NamedCallbackSiteScopes(
							pass, caller, call, parents, invoked, site, dispatcher, targetSignature,
							target.receiver, target.receiverQuery, target.methodExpression,
							constantEnums, domains,
						))
					}
				}
			}
			if len(scopeSets) == 0 {
				scopeSets = append(scopeSets, ps6080UniversalCPUCallScopes(target.function))
			}
			for _, scopes := range scopeSets {
				result = append(result, ps6080CPUCall{
					callee: target.function, scopes: scopes, literal: target.literal,
					literalOwner: target.literalOwner, factoryScanned: target.factoryScanned,
					bindings: ps6080CallableReceiverBindings(pass, caller, target, call, parents),
				})
			}
		}
	}
	return result
}

func ps6080NamedCallbackMappingScopes(
	pass *analysis.Pass,
	caller *ps6080Function,
	call *ast.CallExpr,
	callbackExpression ast.Expr,
	parents map[ast.Node]ast.Node,
	invoked *ps6080InvokedLiteralResult,
	mapping ps6080InvocationArguments,
	target ps6080NamedFunctionTarget,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
) (map[types.Object]*ps6080CPUCallScope, bool) {
	signature, _ := target.function.Type().(*types.Signature)
	callbackSignature, _ := pass.TypesInfo.TypeOf(callbackExpression).(*types.Signature)
	if signature == nil {
		return nil, false
	}
	expectedParameters := signature.Params().Len()
	if target.methodExpression {
		expectedParameters++
	}
	if signature.Recv() != nil && target.receiver == nil && !target.methodExpression ||
		callbackSignature == nil || callbackSignature.Params().Len() != expectedParameters {
		return nil, false
	}
	scopes := make(map[types.Object]*ps6080CPUCallScope, signature.Params().Len()+1)
	mappedScope := func(index int, enum *types.TypeName) *ps6080CPUCallScope {
		var scope *ps6080CPUCallScope
		if index < len(mapping) {
			for sourceIndex, present := range mapping[index] {
				if !present || sourceIndex >= len(call.Args) {
					continue
				}
				addition := ps6080CPUArgumentScope(
					pass, caller, call.Args[sourceIndex], call, enum, parents, invoked,
					constantEnums, domains, make(map[*ast.FuncLit]bool),
				)
				scope = ps6080MergeCPUArgumentScopes(scope, addition)
			}
		}
		if scope == nil {
			scope = &ps6080CPUCallScope{enum: enum, universal: true}
		}
		return scope
	}
	argumentOffset := 0
	if receiver := signature.Recv(); receiver != nil {
		if enum, enumType := ps6080EnumType(receiver.Type()); enumType {
			if target.methodExpression {
				scopes[receiver] = mappedScope(0, enum)
			} else {
				receiverQuery := target.receiverQuery
				if receiverQuery == nil {
					receiverQuery = call
				}
				scopes[receiver] = ps6080CPUArgumentScope(
					pass, caller, target.receiver, receiverQuery, enum, parents, invoked,
					constantEnums, domains, make(map[*ast.FuncLit]bool),
				)
			}
		}
		if target.methodExpression {
			argumentOffset = 1
		}
	}
	for targetIndex := range signature.Params().Len() {
		parameter := signature.Params().At(targetIndex)
		enum, enumType := ps6080EnumType(parameter.Type())
		if enumType {
			scopes[parameter] = mappedScope(targetIndex+argumentOffset, enum)
		}
	}
	return scopes, true
}

func ps6080NamedCallbackSiteScopes(
	pass *analysis.Pass,
	caller *ps6080Function,
	call *ast.CallExpr,
	parents map[ast.Node]ast.Node,
	invoked *ps6080InvokedLiteralResult,
	site *ps6080MayCallbackSite,
	dispatcher *types.Signature,
	target *types.Signature,
	receiver ast.Expr,
	receiverQuery ast.Node,
	methodExpression bool,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
) map[types.Object]*ps6080CPUCallScope {
	result := make(map[types.Object]*ps6080CPUCallScope)
	compose := func(enum *types.TypeName, inner *ps6080CPUCallScope) *ps6080CPUCallScope {
		for forwardIndex := range site.forwarding {
			forward := &site.forwarding[forwardIndex]
			forwardInvoked := forward.invoked
			if forwardInvoked == nil {
				forwardInvoked = ps6080InvokedFunctionLiteralResult(pass, forward.function.body)
			}
			inner = ps6080ComposeNamedCallbackScope(
				pass, forward.function, forward.call, forward.parents, forwardInvoked,
				forward.dispatcher, enum, inner, constantEnums, domains,
			)
		}
		return ps6080ComposeNamedCallbackScope(
			pass, caller, call, parents, invoked, dispatcher, enum, inner,
			constantEnums, domains,
		)
	}
	argumentOffset := 0
	if target.Recv() != nil {
		if enum, enumType := ps6080EnumType(target.Recv().Type()); enumType {
			if methodExpression {
				if len(site.call.Args) == 0 {
					result[target.Recv()] = &ps6080CPUCallScope{enum: enum, universal: true}
				} else {
					siteInvoked := site.invoked
					if siteInvoked == nil {
						siteInvoked = ps6080InvokedFunctionLiteralResult(pass, site.function.body)
					}
					inner := ps6080CPUArgumentScope(
						pass, site.function, site.call.Args[0], site.call, enum,
						site.parents, siteInvoked, constantEnums, domains, make(map[*ast.FuncLit]bool),
					)
					result[target.Recv()] = compose(enum, inner)
				}
				argumentOffset = 1
			} else {
				if receiverQuery == nil {
					receiverQuery = call
				}
				result[target.Recv()] = ps6080CPUArgumentScope(
					pass, caller, receiver, receiverQuery, enum, parents, invoked,
					constantEnums, domains, make(map[*ast.FuncLit]bool),
				)
			}
		}
	}
	limit := min(target.Params().Len(), len(site.call.Args)-argumentOffset)
	for index := range limit {
		parameter := target.Params().At(index)
		enum, enumType := ps6080EnumType(parameter.Type())
		if !enumType {
			continue
		}
		siteInvoked := site.invoked
		if siteInvoked == nil {
			siteInvoked = ps6080InvokedFunctionLiteralResult(pass, site.function.body)
		}
		inner := ps6080CPUArgumentScope(
			pass, site.function, site.call.Args[index+argumentOffset], site.call, enum, site.parents, siteInvoked,
			constantEnums, domains, make(map[*ast.FuncLit]bool),
		)
		result[parameter] = compose(enum, inner)
	}
	for index := limit; index < target.Params().Len(); index++ {
		parameter := target.Params().At(index)
		if enum, enumType := ps6080EnumType(parameter.Type()); enumType {
			result[parameter] = &ps6080CPUCallScope{enum: enum, universal: true}
		}
	}
	return result
}

func ps6080ComposeNamedCallbackScope(
	pass *analysis.Pass,
	caller *ps6080Function,
	call *ast.CallExpr,
	parents map[ast.Node]ast.Node,
	invoked *ps6080InvokedLiteralResult,
	dispatcher *types.Signature,
	enum *types.TypeName,
	inner *ps6080CPUCallScope,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
) *ps6080CPUCallScope {
	if inner == nil || inner.universal {
		return &ps6080CPUCallScope{enum: enum, universal: true}
	}
	result := &ps6080CPUCallScope{
		enum: inner.enum, fixed: ps6080CPUUnionValues(nil, inner.fixed),
	}
	if inner.source == nil {
		return result
	}
	sourceIndex := -1
	if dispatcher != nil {
		for index := range dispatcher.Params().Len() {
			if dispatcher.Params().At(index) == inner.source {
				sourceIndex = index
				break
			}
		}
	}
	if sourceIndex < 0 || sourceIndex >= len(call.Args) {
		if inner.allowed == nil {
			return &ps6080CPUCallScope{enum: inner.enum, universal: true}
		}
		result.fixed = ps6080CPUUnionValues(result.fixed, inner.allowed)
		return result
	}
	outer := ps6080CPUArgumentScope(
		pass, caller, call.Args[sourceIndex], call, inner.enum, parents, invoked,
		constantEnums, domains, make(map[*ast.FuncLit]bool),
	)
	if outer == nil || outer.universal {
		if inner.allowed == nil {
			return &ps6080CPUCallScope{enum: inner.enum, universal: true}
		}
		result.fixed = ps6080CPUUnionValues(result.fixed, inner.allowed)
		return result
	}
	outerFixed := outer.fixed
	if inner.allowed != nil {
		outerFixed = ps6080CPUIntersectValues(outerFixed, inner.allowed)
	}
	result.fixed = ps6080CPUUnionValues(result.fixed, outerFixed)
	if outer.source != nil {
		result.source = outer.source
		switch {
		case outer.allowed == nil:
			result.allowed = inner.allowed
		case inner.allowed == nil:
			result.allowed = outer.allowed
		default:
			result.allowed = ps6080CPUIntersectValues(outer.allowed, inner.allowed)
		}
	}
	return result
}

type ps6080NamedFunctionTarget struct {
	function          *types.Func
	literal           *ast.FuncLit
	factoryResult     bool
	factoryScanned    bool
	literalOwner      *types.Func
	captures          map[types.Object][]ps6080NamedFunctionTarget
	receiver          ast.Expr
	receiverQuery     ast.Node
	receiverCaller    *ps6080Function
	receiverArgument  ast.Expr
	receiverCall      *ast.CallExpr
	receiverArgCaller *ps6080Function
	methodExpression  bool
}

type ps6080NamedFunctionTargetKey struct {
	function          *types.Func
	receiver          token.Pos
	receiverQuery     token.Pos
	receiverArgument  token.Pos
	receiverCall      token.Pos
	receiverArgCaller *types.Func
	methodExpression  bool
}

func ps6080FunctionTargetKey(target ps6080NamedFunctionTarget) ps6080NamedFunctionTargetKey {
	return ps6080NamedFunctionTargetKey{
		function: target.function, receiver: ps6080NodePosition(target.receiver),
		receiverQuery:    ps6080NodePosition(target.receiverQuery),
		receiverArgument: ps6080NodePosition(target.receiverArgument),
		receiverCall:     ps6080CallPosition(target.receiverCall),
		receiverArgCaller: func() *types.Func {
			if target.receiverArgCaller == nil {
				return nil
			}
			return target.receiverArgCaller.object
		}(),
		methodExpression: target.methodExpression,
	}
}

func ps6080CallPosition(call *ast.CallExpr) token.Pos {
	if call == nil {
		return token.NoPos
	}
	return call.Pos()
}

type ps6080FactorySummaryKey struct {
	function       *types.Func
	selectedResult int
	arguments      string
}

func ps6080LiteralFunctionTarget(
	pass *analysis.Pass,
	literal *ast.FuncLit,
) ps6080NamedFunctionTarget {
	value, cached := ps6080LiteralFuncCaches.Load(pass)
	if !cached {
		value = &sync.Map{}
	}
	cache := value.(*sync.Map)
	if function, ok := cache.Load(literal); ok {
		return ps6080NamedFunctionTarget{
			function: function.(*types.Func), literal: literal,
		}
	}
	signature, _ := pass.TypesInfo.TypeOf(literal).(*types.Signature)
	function := types.NewFunc(
		literal.Pos(), pass.Pkg, "ps6080Literal"+strconv.Itoa(int(literal.Pos())), signature,
	)
	actual, _ := cache.LoadOrStore(literal, function)
	return ps6080NamedFunctionTarget{
		function: actual.(*types.Func), literal: literal,
	}
}

func ps6080CloneFunctionTargetBindings(
	bindings map[types.Object][]ps6080NamedFunctionTarget,
) map[types.Object][]ps6080NamedFunctionTarget {
	if len(bindings) == 0 {
		return nil
	}
	result := make(map[types.Object][]ps6080NamedFunctionTarget, len(bindings))
	for object, targets := range bindings {
		result[object] = slices.Clone(targets)
	}
	return result
}

func ps6080MergeFunctionTargetBindings(
	destination map[types.Object][]ps6080NamedFunctionTarget,
	source map[types.Object][]ps6080NamedFunctionTarget,
) (map[types.Object][]ps6080NamedFunctionTarget, bool) {
	changed := false
	if destination == nil && len(source) > 0 {
		destination = make(map[types.Object][]ps6080NamedFunctionTarget, len(source))
	}
	for object, targets := range source {
		indices := make(map[ps6080NamedFunctionTargetKey]int, len(destination[object]))
		for index, target := range destination[object] {
			indices[ps6080FunctionTargetKey(target)] = index
		}
		for _, target := range targets {
			if target.function == nil {
				continue
			}
			key := ps6080FunctionTargetKey(target)
			if index, exists := indices[key]; exists {
				if ps6080MergeNamedFunctionTarget(&destination[object][index], target) {
					changed = true
				}
				continue
			}
			indices[key] = len(destination[object])
			destination[object] = append(destination[object], target)
			changed = true
		}
	}
	return destination, changed
}

func ps6080MergeNamedFunctionTarget(
	destination *ps6080NamedFunctionTarget,
	source ps6080NamedFunctionTarget,
) bool {
	changed := false
	if source.factoryResult && !destination.factoryResult {
		destination.factoryResult = true
		changed = true
	}
	if source.factoryScanned && !destination.factoryScanned {
		destination.factoryScanned = true
		changed = true
	}
	if destination.literalOwner == nil && source.literalOwner != nil {
		destination.literalOwner = source.literalOwner
		changed = true
	}
	if destination.receiverCaller == nil && source.receiverCaller != nil {
		destination.receiverCaller = source.receiverCaller
		changed = true
	}
	if destination.receiverArgument == nil && source.receiverArgument != nil {
		destination.receiverArgument = source.receiverArgument
		destination.receiverCall = source.receiverCall
		destination.receiverArgCaller = source.receiverArgCaller
		changed = true
	}
	var capturesChanged bool
	destination.captures, capturesChanged = ps6080MergeFunctionTargetBindings(
		destination.captures, source.captures,
	)
	return changed || capturesChanged
}

type ps6080FunctionAssignment struct {
	node        ast.Node
	expression  ast.Expr
	resultIndex int
}

func ps6080FunctionAssignments(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	object types.Object,
) []ps6080FunctionAssignment {
	var result []ps6080FunctionAssignment
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch assignment := node.(type) {
		case *ast.AssignStmt:
			if len(assignment.Lhs) != len(assignment.Rhs) {
				if len(assignment.Rhs) == 1 {
					if tuple, tupleResult := pass.TypesInfo.TypeOf(assignment.Rhs[0]).(*types.Tuple); tupleResult && tuple.Len() == len(assignment.Lhs) {
						for index, left := range assignment.Lhs {
							identifier, direct := ps2110Unparen(left).(*ast.Ident)
							if direct && pass.TypesInfo.ObjectOf(identifier) == object {
								result = append(result, ps6080FunctionAssignment{
									node: assignment, expression: assignment.Rhs[0], resultIndex: index,
								})
							}
						}
					}
				}
				return true
			}
			for index, left := range assignment.Lhs {
				identifier, direct := ps2110Unparen(left).(*ast.Ident)
				if direct && pass.TypesInfo.ObjectOf(identifier) == object {
					result = append(result, ps6080FunctionAssignment{
						node: assignment, expression: assignment.Rhs[index], resultIndex: -1,
					})
				}
			}
		case *ast.ValueSpec:
			if len(assignment.Names) != len(assignment.Values) {
				if len(assignment.Values) == 1 {
					if tuple, tupleResult := pass.TypesInfo.TypeOf(assignment.Values[0]).(*types.Tuple); tupleResult && tuple.Len() == len(assignment.Names) {
						for index, name := range assignment.Names {
							if pass.TypesInfo.Defs[name] == object {
								result = append(result, ps6080FunctionAssignment{
									node: assignment, expression: assignment.Values[0], resultIndex: index,
								})
							}
						}
					}
				}
				return true
			}
			for index, name := range assignment.Names {
				if pass.TypesInfo.Defs[name] == object {
					result = append(result, ps6080FunctionAssignment{
						node: assignment, expression: assignment.Values[index], resultIndex: -1,
					})
				}
			}
		case *ast.RangeStmt:
			identifier, direct := ps2110Unparen(assignment.Value).(*ast.Ident)
			if !direct || pass.TypesInfo.ObjectOf(identifier) != object {
				return true
			}
			literal, direct := ps2110Unparen(assignment.X).(*ast.CompositeLit)
			if !direct {
				return true
			}
			for _, element := range literal.Elts {
				expression := element
				if keyed, keyedElement := element.(*ast.KeyValueExpr); keyedElement {
					expression = keyed.Value
				}
				result = append(result, ps6080FunctionAssignment{
					node: assignment.X, expression: expression, resultIndex: -1,
				})
			}
		}
		return true
	})
	return result
}

func ps6080NonIdentityFunctionAssignments(
	pass *analysis.Pass,
	object types.Object,
	assignments []ps6080FunctionAssignment,
) []ps6080FunctionAssignment {
	result := assignments[:0]
	for _, assignment := range assignments {
		identifier, direct := ps2110Unparen(assignment.expression).(*ast.Ident)
		if assignment.resultIndex < 0 && direct && pass.TypesInfo.ObjectOf(identifier) == object {
			continue
		}
		result = append(result, assignment)
	}
	return result
}

func ps6080CFGPositionMayReachWithoutAssignments(
	pass *analysis.Pass,
	graph *cfg.CFG,
	parents map[ast.Node]ast.Node,
	from token.Pos,
	to token.Pos,
	assignments []token.Pos,
) bool {
	fromBlock := ps6079CFGBlockAt(graph, from)
	toBlock := ps6079CFGBlockAt(graph, to)
	if fromBlock == nil || toBlock == nil || !fromBlock.Live || !toBlock.Live {
		return false
	}
	clear := func(block *cfg.Block, after, before token.Pos) bool {
		for _, position := range assignments {
			if position <= after || before.IsValid() && position >= before {
				continue
			}
			if ps6079CFGBlockAt(graph, position) == block {
				return false
			}
		}
		return true
	}
	if fromBlock == toBlock {
		return from < to && clear(fromBlock, from, to)
	}
	if !clear(fromBlock, from, token.NoPos) {
		return false
	}
	seen := map[*cfg.Block]bool{fromBlock: true}
	queue := ps6080FeasibleSuccessors(pass, parents, fromBlock)
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if block == nil || !block.Live || seen[block] {
			continue
		}
		seen[block] = true
		if block == toBlock {
			if clear(block, token.NoPos, to) {
				return true
			}
			continue
		}
		if !clear(block, token.NoPos, token.NoPos) {
			continue
		}
		queue = append(queue, ps6080FeasibleSuccessors(pass, parents, block)...)
	}
	return false
}

func ps6080CFGEntryMayReachWithoutAssignments(
	pass *analysis.Pass,
	graph *cfg.CFG,
	parents map[ast.Node]ast.Node,
	to token.Pos,
	assignments []token.Pos,
) bool {
	if graph == nil || len(graph.Blocks) == 0 {
		return false
	}
	entry := graph.Blocks[0]
	toBlock := ps6079CFGBlockAt(graph, to)
	if entry == nil || toBlock == nil || !entry.Live || !toBlock.Live {
		return false
	}
	clear := func(block *cfg.Block, before token.Pos) bool {
		for _, position := range assignments {
			if before.IsValid() && position >= before {
				continue
			}
			if ps6079CFGBlockAt(graph, position) == block {
				return false
			}
		}
		return true
	}
	seen := make(map[*cfg.Block]bool)
	queue := []*cfg.Block{entry}
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if block == nil || !block.Live || seen[block] {
			continue
		}
		seen[block] = true
		if block == toBlock {
			if clear(block, to) {
				return true
			}
			continue
		}
		if clear(block, token.NoPos) {
			queue = append(queue, ps6080FeasibleSuccessors(pass, parents, block)...)
		}
	}
	return false
}

func ps6080CFGPositionMayReachExitWithoutAssignments(
	pass *analysis.Pass,
	graph *cfg.CFG,
	parents map[ast.Node]ast.Node,
	from token.Pos,
	assignments []token.Pos,
) bool {
	fromBlock := ps6079CFGBlockAt(graph, from)
	if fromBlock == nil || !fromBlock.Live {
		return false
	}
	clearAfter := func(block *cfg.Block, after token.Pos) bool {
		for _, position := range assignments {
			if position > after && ps6079CFGBlockAt(graph, position) == block {
				return false
			}
		}
		return true
	}
	type state struct {
		block *cfg.Block
		after token.Pos
	}
	queue := []state{{block: fromBlock, after: from}}
	seen := make(map[*cfg.Block]bool)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.block == nil || !current.block.Live || seen[current.block] ||
			!clearAfter(current.block, current.after) {
			continue
		}
		seen[current.block] = true
		successors := ps6080FeasibleSuccessors(pass, parents, current.block)
		liveSuccessors := 0
		for _, successor := range successors {
			if successor.Live {
				liveSuccessors++
				queue = append(queue, state{block: successor})
			}
		}
		if liveSuccessors == 0 {
			return true
		}
	}
	return false
}

func ps6080CFGMustExecuteAnyAssignment(
	pass *analysis.Pass,
	graph *cfg.CFG,
	parents map[ast.Node]ast.Node,
	assignments []token.Pos,
) bool {
	if graph == nil || len(graph.Blocks) == 0 || !graph.Blocks[0].Live {
		return false
	}
	type state struct {
		block   *cfg.Block
		written bool
	}
	seen := make(map[state]bool)
	queue := []state{{block: graph.Blocks[0]}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.block == nil || !current.block.Live || seen[current] {
			continue
		}
		seen[current] = true
		for _, position := range assignments {
			if ps6079CFGBlockAt(graph, position) == current.block {
				current.written = true
				break
			}
		}
		successors := ps6080FeasibleSuccessors(pass, parents, current.block)
		liveSuccessors := 0
		for _, successor := range successors {
			if successor.Live {
				liveSuccessors++
				queue = append(queue, state{block: successor, written: current.written})
			}
		}
		if liveSuccessors == 0 && !current.written {
			return false
		}
	}
	return true
}

func ps6080LiteralRootInvocationPositions(
	invoked *ps6080InvokedLiteralResult,
	literal *ast.FuncLit,
	excluded []token.Pos,
) []token.Pos {
	var result []token.Pos
	for _, orders := range invoked.orders[literal] {
		for _, order := range orders {
			if len(order) == 0 || slices.Contains(excluded, order[0]) || slices.Contains(result, order[0]) {
				continue
			}
			result = append(result, order[0])
		}
	}
	slices.Sort(result)
	return result
}

func ps6080LiteralInvocationArgumentsAt(
	pass *analysis.Pass,
	invoked *ps6080InvokedLiteralResult,
	literal *ast.FuncLit,
	parameter int,
	root token.Pos,
) []ast.Expr {
	if parameter < 0 {
		return nil
	}
	var result []ast.Expr
	appendArgument := func(expression ast.Expr) {
		if expression == nil {
			return
		}
		for _, existing := range result {
			if existing.Pos() == expression.Pos() {
				return
			}
		}
		result = append(result, expression)
	}
	literalType := pass.TypesInfo.TypeOf(literal)
	for call := range invoked.calls[literal] {
		matching := false
		for _, order := range invoked.orders[literal][call] {
			matching = matching || len(order) > 0 && order[0] == root
		}
		if !matching {
			continue
		}
		callType := pass.TypesInfo.TypeOf(call.Fun)
		if literalType != nil && callType != nil &&
			types.Identical(literalType.Underlying(), callType.Underlying()) && parameter < len(call.Args) {
			appendArgument(call.Args[parameter])
		}
		for _, ordered := range invoked.orderedArgs[literal][call] {
			if len(ordered.order) == 0 || ordered.order[0] != root {
				continue
			}
			for argument, parameters := range ordered.arguments {
				if ps6080HasIndex(parameters, parameter) && argument < len(call.Args) {
					appendArgument(call.Args[argument])
				}
			}
		}
	}
	return result
}

func ps6080ObjectAddressTaken(pass *analysis.Pass, root ast.Node, object types.Object) bool {
	taken := false
	objects := map[types.Object]bool{object: true}
	ast.Inspect(root, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.UnaryExpr:
			identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
			taken = value.Op == token.AND && direct && pass.TypesInfo.ObjectOf(identifier) == object
		case *ast.SelectorExpr:
			taken = ps6080PointerMethodOnObjects(pass, value, objects)
		}
		return !taken
	})
	return taken
}

func ps6080ImmutablePackageFunctionInitializer(
	pass *analysis.Pass,
	variable *types.Var,
) (ast.Expr, bool) {
	var initializer ast.Expr
	mutable := false
	for _, file := range pass.Files {
		if ps6080ObjectAddressTaken(pass, file, variable) {
			return nil, false
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.AssignStmt:
				for _, left := range value.Lhs {
					identifier, direct := ps2110Unparen(left).(*ast.Ident)
					if direct && pass.TypesInfo.ObjectOf(identifier) == variable {
						mutable = true
						return false
					}
				}
			case *ast.ValueSpec:
				if len(value.Names) != len(value.Values) {
					for _, name := range value.Names {
						if pass.TypesInfo.Defs[name] == variable {
							mutable = true
							return false
						}
					}
					return true
				}
				for index, name := range value.Names {
					if pass.TypesInfo.Defs[name] == variable {
						initializer = value.Values[index]
					}
				}
			}
			return !mutable
		})
		if mutable {
			return nil, false
		}
	}
	return initializer, initializer != nil
}

func ps6080StaticNamedFunctionTarget(
	pass *analysis.Pass,
	caller *ps6080Function,
	expression ast.Expr,
	query ast.Node,
	parents map[ast.Node]ast.Node,
) (ps6080NamedFunctionTarget, bool) {
	seen := make(map[types.Object]bool)
	for {
		switch value := ps2110Unparen(expression).(type) {
		case *ast.Ident:
			object := pass.TypesInfo.ObjectOf(value)
			if function, ok := object.(*types.Func); ok {
				return ps6080NamedFunctionTarget{function: function}, true
			}
			variable, ok := object.(*types.Var)
			if !ok || seen[variable] {
				return ps6080NamedFunctionTarget{}, false
			}
			seen[variable] = true
			expression = ps6080StableLocalInitializer(pass, caller, variable, query, parents)
			if expression == nil {
				return ps6080NamedFunctionTarget{}, false
			}
		case *ast.SelectorExpr:
			function, ok := pass.TypesInfo.ObjectOf(value.Sel).(*types.Func)
			if !ok {
				return ps6080NamedFunctionTarget{}, false
			}
			result := ps6080NamedFunctionTarget{function: function}
			if selection := pass.TypesInfo.Selections[value]; selection != nil {
				switch selection.Kind() {
				case types.MethodVal:
					result.receiver = value.X
					result.receiverQuery = value
				case types.MethodExpr:
					result.methodExpression = true
				}
			}
			return result, true
		case *ast.CallExpr:
			if len(value.Args) != 1 || !pass.TypesInfo.Types[value.Fun].IsType() {
				return ps6080NamedFunctionTarget{}, false
			}
			expression = value.Args[0]
		default:
			return ps6080NamedFunctionTarget{}, false
		}
	}
}

func ps6080PossibleNamedFunctionTargets(
	pass *analysis.Pass,
	caller *ps6080Function,
	expression ast.Expr,
	query ast.Node,
	parents map[ast.Node]ast.Node,
) []ps6080NamedFunctionTarget {
	var capturedTargets map[types.Object][]ps6080NamedFunctionTarget
	if caller != nil {
		capturedTargets = caller.capturedTargets
	}
	return ps6080PossibleNamedFunctionTargetsVisiting(
		pass, caller, expression, query, parents, -1,
		make(map[ps6080FactorySummaryKey]int),
		make(map[ps6080FactorySummaryKey][]ps6080NamedFunctionTarget), capturedTargets,
		make(map[ps6080FactorySummaryKey]bool),
		make(map[types.Object]bool),
	)
}

func ps6080LiteralCaptureBindings(
	pass *analysis.Pass,
	caller *ps6080Function,
	literal *ast.FuncLit,
	query ast.Node,
	parents map[ast.Node]ast.Node,
	factoryVisiting map[ps6080FactorySummaryKey]int,
	factoryCache map[ps6080FactorySummaryKey][]ps6080NamedFunctionTarget,
	parameterTargets map[types.Object][]ps6080NamedFunctionTarget,
	factoryCyclic map[ps6080FactorySummaryKey]bool,
	captureVisiting map[types.Object]bool,
) map[types.Object][]ps6080NamedFunctionTarget {
	var captures map[types.Object][]ps6080NamedFunctionTarget
	seen := make(map[types.Object]bool)
	ast.Inspect(literal.Body, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		variable, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
		if !ok || seen[variable] ||
			literal.Pos() <= variable.Pos() && variable.Pos() < literal.End() ||
			!ps6080CallableType(variable.Type()) {
			return true
		}
		seen[variable] = true
		if captureVisiting[variable] {
			return true
		}
		initializer := ps6080StableLocalInitializer(pass, caller, variable, query, parents)
		if initializer != nil && initializer.Pos() == literal.Pos() {
			return true
		}
		captureVisiting[variable] = true
		targets := ps6080PossibleNamedFunctionTargetsVisiting(
			pass, caller, identifier, query, parents, -1, factoryVisiting, factoryCache,
			parameterTargets, factoryCyclic, captureVisiting,
		)
		delete(captureVisiting, variable)
		if len(targets) > 0 {
			if captures == nil {
				captures = make(map[types.Object][]ps6080NamedFunctionTarget)
			}
			captures[variable] = targets
		}
		return true
	})
	return captures
}

func ps6080PossibleNamedFunctionTargetsVisiting(
	pass *analysis.Pass,
	caller *ps6080Function,
	expression ast.Expr,
	query ast.Node,
	parents map[ast.Node]ast.Node,
	selectedResult int,
	factoryVisiting map[ps6080FactorySummaryKey]int,
	factoryCache map[ps6080FactorySummaryKey][]ps6080NamedFunctionTarget,
	parameterTargets map[types.Object][]ps6080NamedFunctionTarget,
	factoryCyclic map[ps6080FactorySummaryKey]bool,
	captureVisiting map[types.Object]bool,
) []ps6080NamedFunctionTarget {
	seenVariables := make(map[types.Object]bool)
	body := ps6080FunctionBody(caller)
	invoked := ps6080InvokedFunctionLiteralResult(pass, body)
	graph := cfg.New(body, ps6080CallMayReturn(pass))
	literalGraphs := ps6080InvokedLiteralGraphs(pass, invoked.literals)
	type callLiteralPosition struct {
		position token.Pos
		literal  *ast.FuncLit
	}
	var callLiterals []callLiteralPosition
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok {
			callLiterals = append(callLiterals, callLiteralPosition{
				position: call.Pos(), literal: ps6080ContainingLiteral(call, parents),
			})
		}
		return true
	})
	slices.SortFunc(callLiterals, func(left, right callLiteralPosition) int {
		return cmp.Compare(left.position, right.position)
	})
	callLiteralAt := func(position token.Pos) (*ast.FuncLit, bool) {
		index, found := slices.BinarySearchFunc(
			callLiterals, position,
			func(candidate callLiteralPosition, target token.Pos) int {
				return cmp.Compare(candidate.position, target)
			},
		)
		if !found {
			return nil, false
		}
		return callLiterals[index].literal, true
	}
	var deferredCalls []token.Pos
	var concurrentCalls []token.Pos
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch parent := parents[call].(type) {
		case *ast.DeferStmt:
			if parent.Call == call {
				deferredCalls = append(deferredCalls, call.Pos())
			}
		case *ast.GoStmt:
			if parent.Call == call {
				concurrentCalls = append(concurrentCalls, call.Pos())
			}
		}
		return true
	})
	mustExclude := append(slices.Clone(deferredCalls), concurrentCalls...)
	var result []ps6080NamedFunctionTarget
	resultIndices := make(map[ps6080NamedFunctionTargetKey]int)
	addTarget := func(target ps6080NamedFunctionTarget) {
		if target.function == nil {
			return
		}
		key := ps6080FunctionTargetKey(target)
		if index, exists := resultIndices[key]; exists {
			ps6080MergeNamedFunctionTarget(&result[index], target)
			return
		}
		resultIndices[key] = len(result)
		result = append(result, target)
	}
	var resolve func(ast.Expr, int)
	resolve = func(candidate ast.Expr, resultIndex int) {
		switch value := ps2110Unparen(candidate).(type) {
		case *ast.StarExpr:
			resolve(value.X, resultIndex)
		case *ast.UnaryExpr:
			if value.Op == token.AND {
				resolve(value.X, resultIndex)
			}
		case *ast.IndexExpr:
			if target, ok := ps6080TypedNamedFunctionTarget(pass, value); ok {
				addTarget(target)
			}
		case *ast.IndexListExpr:
			if target, ok := ps6080TypedNamedFunctionTarget(pass, value); ok {
				addTarget(target)
			}
		case *ast.CallExpr:
			if len(value.Args) == 1 && pass.TypesInfo.Types[value.Fun].IsType() {
				resolve(value.Args[0], -1)
				return
			}
			var factories []ps6080NamedFunctionTarget
			if factory, static := ps6080StaticNamedCallee(
				pass, caller, value, parents,
			); static {
				factories = append(factories, factory)
			} else {
				factories = ps6080PossibleNamedFunctionTargetsVisiting(
					pass, caller, value.Fun, value, parents, -1, factoryVisiting, factoryCache,
					parameterTargets, factoryCyclic, captureVisiting,
				)
			}
			for _, factory := range factories {
				for _, target := range ps6080ReachableFactoryTargets(
					pass, factory, value, resultIndex, caller, parents,
					factoryVisiting, factoryCache, parameterTargets, factoryCyclic,
					captureVisiting,
				) {
					addTarget(target)
				}
			}
		case *ast.FuncLit:
			target := ps6080LiteralFunctionTarget(pass, value)
			target.captures = ps6080LiteralCaptureBindings(
				pass, caller, value, query, parents, factoryVisiting, factoryCache,
				parameterTargets, factoryCyclic, captureVisiting,
			)
			addTarget(target)
		case *ast.SelectorExpr:
			selected := pass.TypesInfo.ObjectOf(value.Sel)
			if field, ok := selected.(*types.Var); ok {
				binding := field
				if first := ps6080SelectorFirstField(pass, value); first != nil {
					binding = first
				}
				for _, target := range parameterTargets[binding] {
					addTarget(target)
				}
				return
			}
			function, ok := selected.(*types.Func)
			if !ok {
				return
			}
			target := ps6080NamedFunctionTarget{function: function}
			if selection := pass.TypesInfo.Selections[value]; selection != nil {
				switch selection.Kind() {
				case types.MethodVal:
					target.receiver = value.X
					target.receiverQuery = value
				case types.MethodExpr:
					target.methodExpression = true
				}
			}
			addTarget(target)
		case *ast.Ident:
			object := pass.TypesInfo.ObjectOf(value)
			if function, ok := object.(*types.Func); ok {
				addTarget(ps6080NamedFunctionTarget{function: function})
				return
			}
			entryTargets := parameterTargets[object]
			variable, ok := object.(*types.Var)
			if !ok {
				for _, target := range entryTargets {
					addTarget(target)
				}
				return
			}
			if seenVariables[variable] {
				return
			}
			seenVariables[variable] = true
			if variable.Parent() == pass.Pkg.Scope() {
				initializer, immutable := ps6080ImmutablePackageFunctionInitializer(pass, variable)
				if immutable {
					resolve(initializer, -1)
				}
				return
			}
			if ps6080ObjectAddressTaken(pass, body, variable) {
				return
			}

			outerAssignments := ps6080NonIdentityFunctionAssignments(
				pass, variable, ps6080FunctionAssignments(pass, body, variable),
			)
			outerPositions := make([]token.Pos, 0, len(outerAssignments))
			for _, assignment := range outerAssignments {
				outerPositions = append(outerPositions, assignment.node.Pos())
			}
			literalAssignments := make(map[*ast.FuncLit][]ps6080FunctionAssignment)
			literalPositions := make(map[*ast.FuncLit][]token.Pos, len(invoked.literals))
			allPositions := slices.Clone(outerPositions)
			for literal := range invoked.literals {
				assignments := ps6080NonIdentityFunctionAssignments(
					pass, variable, ps6080FunctionAssignments(pass, literal.Body, variable),
				)
				if len(assignments) == 0 {
					continue
				}
				literalAssignments[literal] = assignments
				positions := make([]token.Pos, 0, len(assignments))
				for _, assignment := range assignments {
					positions = append(positions, assignment.node.Pos())
				}
				literalPositions[literal] = positions
				if ps6080CFGMustExecuteAnyAssignment(
					pass, literalGraphs[literal], parents, positions,
				) {
					allPositions = append(
						allPositions,
						ps6080LiteralRootInvocationPositions(invoked, literal, mustExclude)...,
					)
				}
			}
			queryLiteral := ps6080ContainingLiteral(query, parents)
			queryEntryMayReach := func() bool {
				if queryLiteral == nil {
					return ps6080CFGEntryMayReachWithoutAssignments(
						pass, graph, parents, query.Pos(), allPositions,
					)
				}
				if !ps6080CFGEntryMayReachWithoutAssignments(
					pass, literalGraphs[queryLiteral], parents, query.Pos(),
					literalPositions[queryLiteral],
				) {
					return false
				}
				for _, orders := range invoked.orders[queryLiteral] {
					for _, order := range orders {
						if len(order) == 0 || slices.Contains(mustExclude, order[0]) {
							continue
						}
						clear := true
						for _, invocation := range order {
							container, known := callLiteralAt(invocation)
							if !known {
								clear = false
								break
							}
							invocationGraph := graph
							positions := allPositions
							if container != nil {
								invocationGraph = literalGraphs[container]
								positions = literalPositions[container]
							}
							if !ps6080CFGEntryMayReachWithoutAssignments(
								pass, invocationGraph, parents, invocation, positions,
							) {
								clear = false
								break
							}
						}
						if clear {
							return true
						}
					}
				}
				return false
			}
			entryMayReach := ps6080CFGEntryMayReachWithoutAssignments(
				pass, graph, parents, query.Pos(), allPositions,
			)
			if len(outerAssignments) == 0 && len(literalAssignments) == 0 {
				entryMayReach = true
			} else if queryLiteral != nil {
				entryMayReach = queryEntryMayReach()
			}
			if entryMayReach && ps6080CorrelatedGuardMustAssignBefore(
				pass, caller, variable, query, parents,
			) {
				entryMayReach = false
			}
			if len(entryTargets) > 0 && entryMayReach {
				for _, target := range entryTargets {
					addTarget(target)
				}
			}
			literalAssignmentMayReachQuery := func(literal *ast.FuncLit, from token.Pos) bool {
				if queryLiteral == nil || !ps6080NodeWithin(query, literal.Body) {
					return false
				}
				if literal == queryLiteral {
					return ps6080CFGPositionMayReachWithoutAssignments(
						pass, literalGraphs[literal], parents, from, query.Pos(),
						literalPositions[literal],
					)
				}
				if !ps6080CFGEntryMayReachWithoutAssignments(
					pass, literalGraphs[queryLiteral], parents, query.Pos(),
					literalPositions[queryLiteral],
				) {
					return false
				}
				for _, orders := range invoked.orders[queryLiteral] {
					for _, order := range orders {
						if len(order) == 0 || slices.Contains(mustExclude, order[0]) {
							continue
						}
						for start, invocation := range order {
							container, known := callLiteralAt(invocation)
							if !known || container != literal ||
								!ps6080CFGPositionMayReachWithoutAssignments(
									pass, literalGraphs[literal], parents, from, invocation,
									literalPositions[literal],
								) {
								continue
							}
							clear := true
							for _, nestedInvocation := range order[start+1:] {
								container, known := callLiteralAt(nestedInvocation)
								if !known || container == nil ||
									!ps6080CFGEntryMayReachWithoutAssignments(
										pass, literalGraphs[container], parents, nestedInvocation,
										literalPositions[container],
									) {
									clear = false
									break
								}
							}
							if clear {
								return true
							}
						}
					}
				}
				return false
			}
			for _, assignment := range outerAssignments {
				if assignment.node.Pos() >= query.Pos() ||
					!ps6080StaticallyReachable(pass, parents, assignment.node) ||
					!ps6080CFGPositionMayReachWithoutAssignments(
						pass, graph, parents, assignment.node.Pos(), query.Pos(), allPositions,
					) {
					continue
				}
				resolve(assignment.expression, assignment.resultIndex)
			}
			for literal, assignments := range literalAssignments {
				literalGraph := literalGraphs[literal]
				for _, assignment := range assignments {
					if !ps6080StaticallyReachable(pass, parents, assignment.node) ||
						!ps6080NodeReachable(pass, literalGraph, parents, assignment.node) {
						continue
					}
					if literalAssignmentMayReachQuery(literal, assignment.node.Pos()) {
						resolve(assignment.expression, assignment.resultIndex)
						continue
					}
					if !ps6080CFGPositionMayReachExitWithoutAssignments(
						pass, literalGraph, parents, assignment.node.Pos(), literalPositions[literal],
					) {
						continue
					}
					parameter := ps6080LiteralParameterExpressionIndex(
						pass, caller, literal, assignment.expression, assignment.node, parents,
					)
					for _, invocation := range ps6080LiteralRootInvocationPositions(
						invoked, literal, deferredCalls,
					) {
						if invocation < query.Pos() && ps6080CFGPositionMayReachWithoutAssignments(
							pass, graph, parents, invocation, query.Pos(), allPositions,
						) {
							if parameter < 0 {
								resolve(assignment.expression, assignment.resultIndex)
								break
							} else {
								for _, argument := range ps6080LiteralInvocationArgumentsAt(
									pass, invoked, literal, parameter, invocation,
								) {
									resolve(argument, -1)
								}
							}
						}
					}
				}
			}
		}
	}
	resolve(expression, selectedResult)
	slices.SortFunc(result, func(left, right ps6080NamedFunctionTarget) int {
		return cmp.Compare(left.function.Pos(), right.function.Pos())
	})
	return result
}

func ps6080SelectorFirstField(pass *analysis.Pass, selector *ast.SelectorExpr) *types.Var {
	if _, explicit := ps2110Unparen(selector.X).(*ast.SelectorExpr); !explicit {
		selection := pass.TypesInfo.Selections[selector]
		if selection != nil && selection.Kind() == types.FieldVal && len(selection.Index()) > 1 {
			receiver := types.Unalias(selection.Recv())
			if pointer, ok := receiver.(*types.Pointer); ok {
				receiver = types.Unalias(pointer.Elem())
			}
			if structure, ok := receiver.Underlying().(*types.Struct); ok {
				index := selection.Index()[0]
				if 0 <= index && index < structure.NumFields() {
					return structure.Field(index)
				}
			}
		}
	}
	var first *types.Var
	for selector != nil {
		field, ok := pass.TypesInfo.ObjectOf(selector.Sel).(*types.Var)
		if !ok {
			return first
		}
		first = field
		selector, _ = ps2110Unparen(selector.X).(*ast.SelectorExpr)
	}
	return first
}

func ps6080FactoryParameterArgument(
	pass *analysis.Pass,
	factoryTarget ps6080NamedFunctionTarget,
	call *ast.CallExpr,
	parameter int,
	parameterCount int,
) (ast.Expr, int, bool) {
	if factoryTarget.methodExpression && len(call.Args) == 1 {
		if tuple, ok := pass.TypesInfo.TypeOf(call.Args[0]).(*types.Tuple); ok &&
			tuple.Len() == parameterCount+1 {
			return call.Args[0], parameter + 1, true
		}
	}
	offset := 0
	if factoryTarget.methodExpression {
		offset = 1
	}
	if len(call.Args) < offset {
		return nil, -1, false
	}
	arguments := call.Args[offset:]
	if len(arguments) == 1 {
		if tuple, ok := pass.TypesInfo.TypeOf(arguments[0]).(*types.Tuple); ok &&
			parameter < tuple.Len() && tuple.Len() == parameterCount {
			return arguments[0], parameter, true
		}
	}
	if parameter >= len(arguments) {
		return nil, -1, false
	}
	return arguments[parameter], -1, true
}

func ps6080WriteFactoryTargetSummary(
	builder *strings.Builder,
	target ps6080NamedFunctionTarget,
	visiting map[*types.Func]bool,
) {
	if target.function == nil {
		return
	}
	builder.WriteString(strconv.Itoa(int(target.function.Pos())))
	builder.WriteByte(':')
	builder.WriteString(strconv.Itoa(int(ps6080NodePosition(target.receiver))))
	builder.WriteByte(':')
	builder.WriteString(strconv.Itoa(int(ps6080NodePosition(target.receiverQuery))))
	if target.methodExpression {
		builder.WriteByte('m')
	}
	if visiting[target.function] || len(target.captures) == 0 {
		builder.WriteByte(',')
		return
	}
	visiting[target.function] = true
	builder.WriteByte('{')
	objects := make([]types.Object, 0, len(target.captures))
	for object := range target.captures {
		objects = append(objects, object)
	}
	slices.SortFunc(objects, func(left, right types.Object) int {
		if order := cmp.Compare(left.Pos(), right.Pos()); order != 0 {
			return order
		}
		return strings.Compare(left.Name(), right.Name())
	})
	builder.Grow(len(objects) * 24)
	for _, object := range objects {
		builder.WriteString(strconv.Itoa(int(object.Pos())))
		builder.WriteByte('=')
		targets := slices.Clone(target.captures[object])
		slices.SortFunc(targets, func(left, right ps6080NamedFunctionTarget) int {
			return cmp.Compare(left.function.Pos(), right.function.Pos())
		})
		for _, captured := range targets {
			ps6080WriteFactoryTargetSummary(builder, captured, visiting)
		}
		builder.WriteByte(';')
	}
	builder.WriteByte('}')
	builder.WriteByte(',')
	delete(visiting, target.function)
}

func ps6080FactorySummaryArguments(
	signature *types.Signature,
	parameterTargets map[types.Object][]ps6080NamedFunctionTarget,
) string {
	if signature == nil {
		return ""
	}
	var builder strings.Builder
	builder.Grow((len(parameterTargets) + signature.Params().Len() + 1) * 48)
	objects := make([]types.Object, 0, signature.Params().Len()+1)
	present := make(map[types.Object]bool, signature.Params().Len()+1)
	if signature.Recv() != nil {
		objects = append(objects, signature.Recv())
		present[signature.Recv()] = true
	}
	for index := range signature.Params().Len() {
		object := signature.Params().At(index)
		objects = append(objects, object)
		present[object] = true
	}
	var captures []types.Object
	for object := range parameterTargets {
		if !present[object] {
			captures = append(captures, object)
		}
	}
	slices.SortFunc(captures, func(left, right types.Object) int {
		if order := cmp.Compare(left.Pos(), right.Pos()); order != 0 {
			return order
		}
		return strings.Compare(left.Name(), right.Name())
	})
	objects = append(objects, captures...)
	for index, object := range objects {
		builder.WriteByte('|')
		builder.WriteString(strconv.Itoa(index))
		builder.WriteByte('=')
		targets := slices.Clone(parameterTargets[object])
		slices.SortFunc(targets, func(left, right ps6080NamedFunctionTarget) int {
			if order := cmp.Compare(left.function.Pos(), right.function.Pos()); order != 0 {
				return order
			}
			if order := cmp.Compare(
				ps6080NodePosition(left.receiver), ps6080NodePosition(right.receiver),
			); order != 0 {
				return order
			}
			return cmp.Compare(
				ps6080NodePosition(left.receiverQuery), ps6080NodePosition(right.receiverQuery),
			)
		})
		for _, target := range targets {
			ps6080WriteFactoryTargetSummary(&builder, target, make(map[*types.Func]bool))
		}
	}
	return builder.String()
}

func ps6080FactoryCallArgumentSummary(
	pass *analysis.Pass,
	factoryTarget ps6080NamedFunctionTarget,
	call *ast.CallExpr,
	signature *types.Signature,
) string {
	if signature == nil {
		return ""
	}
	var builder strings.Builder
	builder.Grow(signature.Params().Len() * 16)
	for index := range signature.Params().Len() {
		argument, resultIndex, ok := ps6080FactoryParameterArgument(
			pass, factoryTarget, call, index, signature.Params().Len(),
		)
		if !ok || ps6080CallableType(signature.Params().At(index).Type()) {
			continue
		}
		builder.WriteByte('#')
		builder.WriteString(strconv.Itoa(index))
		builder.WriteByte(':')
		builder.WriteString(strconv.Itoa(resultIndex))
		builder.WriteByte('=')
		if value := pass.TypesInfo.Types[argument].Value; value != nil {
			builder.WriteString(value.ExactString())
		} else {
			builder.WriteString(strconv.Itoa(int(argument.Pos())))
		}
	}
	return builder.String()
}

func ps6080NodePosition(node ast.Node) token.Pos {
	if node == nil {
		return token.NoPos
	}
	return node.Pos()
}

func ps6080DeferredLiteralAssigns(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	parents map[ast.Node]ast.Node,
	object types.Object,
) bool {
	invoked := ps6080InvokedFunctionLiteralResult(pass, body)
	scheduled := make(map[*ast.FuncLit]bool)
	var queue []*ast.FuncLit
	for literal, calls := range invoked.calls {
		for call := range calls {
			if deferred, ok := parents[call].(*ast.DeferStmt); ok && deferred.Call == call {
				scheduled[literal] = true
				queue = append(queue, literal)
				break
			}
		}
	}
	for len(queue) > 0 {
		literal := queue[0]
		queue = queue[1:]
		if len(ps6080FunctionAssignments(pass, literal.Body, object)) > 0 {
			return true
		}
		for candidate, calls := range invoked.calls {
			if scheduled[candidate] {
				continue
			}
			for call := range calls {
				if ps6080NodeWithin(call, literal.Body) {
					scheduled[candidate] = true
					queue = append(queue, candidate)
					break
				}
			}
		}
	}
	return false
}

func ps6080ReachableFactoryTargets(
	pass *analysis.Pass,
	factoryTarget ps6080NamedFunctionTarget,
	call *ast.CallExpr,
	selectedResult int,
	caller *ps6080Function,
	callerParents map[ast.Node]ast.Node,
	visiting map[ps6080FactorySummaryKey]int,
	factoryCache map[ps6080FactorySummaryKey][]ps6080NamedFunctionTarget,
	callerParameterTargets map[types.Object][]ps6080NamedFunctionTarget,
	factoryCyclic map[ps6080FactorySummaryKey]bool,
	captureVisiting map[types.Object]bool,
) []ps6080NamedFunctionTarget {
	factory := factoryTarget.function
	if factory == nil {
		return nil
	}
	signature, _ := factory.Type().(*types.Signature)
	parameterTargets := ps6080CloneFunctionTargetBindings(factoryTarget.captures)
	parameterConstants := make(map[types.Object]constant.Value)
	if parameterTargets == nil {
		parameterTargets = make(map[types.Object][]ps6080NamedFunctionTarget)
	}
	var factoryReceiver ast.Expr
	var factoryReceiverQuery ast.Node = call
	factoryReceiverResult := -1
	if signature != nil && signature.Recv() != nil {
		if factoryTarget.methodExpression {
			if len(call.Args) > 0 {
				factoryReceiver = call.Args[0]
				if len(call.Args) == 1 {
					if tuple, ok := pass.TypesInfo.TypeOf(factoryReceiver).(*types.Tuple); ok &&
						tuple.Len() == signature.Params().Len()+1 {
						factoryReceiverResult = 0
					}
				}
			}
		} else {
			factoryReceiver = factoryTarget.receiver
			if factoryTarget.receiverQuery != nil {
				factoryReceiverQuery = factoryTarget.receiverQuery
			}
		}
		if factoryReceiver != nil {
			if factoryReceiverResult < 0 {
				if value := pass.TypesInfo.Types[factoryReceiver].Value; value != nil {
					parameterConstants[signature.Recv()] = value
				}
			}
			resolved := false
			if factoryReceiverResult < 0 {
				if target, static := ps6080StaticNamedFunctionTarget(
					pass, caller, factoryReceiver, factoryReceiverQuery, callerParents,
				); static {
					parameterTargets[signature.Recv()] = []ps6080NamedFunctionTarget{target}
					resolved = true
				}
			}
			if !resolved {
				parameterTargets[signature.Recv()] =
					ps6080PossibleNamedFunctionTargetsVisiting(
						pass, caller, factoryReceiver, factoryReceiverQuery, callerParents,
						factoryReceiverResult, visiting,
						factoryCache, callerParameterTargets, factoryCyclic, captureVisiting,
					)
			}
		}
	}
	if signature != nil {
		for index := range signature.Params().Len() {
			argument, resultIndex, ok := ps6080FactoryParameterArgument(
				pass, factoryTarget, call, index, signature.Params().Len(),
			)
			if !ok {
				continue
			}
			if resultIndex < 0 {
				if value := pass.TypesInfo.Types[argument].Value; value != nil {
					parameterConstants[signature.Params().At(index)] = value
				}
			}
			if resultIndex < 0 {
				if target, static := ps6080StaticNamedFunctionTarget(
					pass, caller, argument, call, callerParents,
				); static {
					parameterTargets[signature.Params().At(index)] =
						[]ps6080NamedFunctionTarget{target}
					continue
				}
			}
			parameterTargets[signature.Params().At(index)] =
				ps6080PossibleNamedFunctionTargetsVisiting(
					pass, caller, argument, call, callerParents, resultIndex, visiting, factoryCache,
					callerParameterTargets, factoryCyclic, captureVisiting,
				)
		}
	}
	cacheKey := ps6080FactorySummaryKey{
		function: factory, selectedResult: selectedResult,
		arguments: ps6080FactorySummaryArguments(signature, parameterTargets) +
			ps6080FactoryCallArgumentSummary(pass, factoryTarget, call, signature),
	}
	if cached, ok := factoryCache[cacheKey]; ok {
		return slices.Clone(cached)
	}
	if cycleStart, activeCycle := visiting[cacheKey]; activeCycle {
		for active, depth := range visiting {
			if depth >= cycleStart {
				factoryCyclic[active] = true
			}
		}
		return nil
	}
	visiting[cacheKey] = len(visiting) + 1
	defer delete(visiting, cacheKey)
	var declaration *ast.FuncDecl
	var body *ast.BlockStmt
	var resultFields *ast.FieldList
	if factoryTarget.literal != nil {
		body = factoryTarget.literal.Body
		resultFields = factoryTarget.literal.Type.Results
	} else {
		for _, file := range pass.Files {
			for _, candidate := range file.Decls {
				function, ok := candidate.(*ast.FuncDecl)
				if ok && pass.TypesInfo.Defs[function.Name] == factory {
					declaration = function
					break
				}
			}
			if declaration != nil {
				break
			}
		}
		if declaration != nil {
			body = declaration.Body
			resultFields = declaration.Type.Results
		}
	}
	if body == nil {
		if !factoryCyclic[cacheKey] {
			factoryCache[cacheKey] = nil
		}
		return nil
	}
	info := &ps6080Function{
		declaration: declaration, object: factory, signature: signature, body: body,
		capturedTargets: factoryTarget.captures,
	}
	parents := ps6071Parents(body)
	graph := cfg.New(body, ps6080CallMayReturn(pass))
	factoryInvoked := ps6080InvokedFunctionLiterals(pass, body)
	var result []ps6080NamedFunctionTarget
	resultIndices := make(map[ps6080NamedFunctionTargetKey]int)
	addTargets := func(targets []ps6080NamedFunctionTarget) {
		for _, target := range targets {
			if target.receiver != nil && target.function != nil {
				target.receiverCaller = info
				targetSignature, _ := target.function.Type().(*types.Signature)
				if targetSignature != nil && targetSignature.Recv() != nil {
					if _, enumReceiver := ps6080EnumType(targetSignature.Recv().Type()); enumReceiver {
						parameter := ps6080FunctionParameterExpressionIndex(
							pass, info, target.receiver, target.receiverQuery, parents,
						)
						switch {
						case parameter == signature.Params().Len() && signature.Recv() != nil &&
							factoryReceiver != nil && factoryReceiverResult < 0:
							target.receiverArgument = factoryReceiver
							target.receiverCall = call
							target.receiverArgCaller = caller
						case parameter >= 0 && parameter < signature.Params().Len():
							argument, resultIndex, ok := ps6080FactoryParameterArgument(
								pass, factoryTarget, call, parameter, signature.Params().Len(),
							)
							if ok && resultIndex < 0 {
								target.receiverArgument = argument
								target.receiverCall = call
								target.receiverArgCaller = caller
							}
						}
					}
				}
				target.captures, _ = ps6080MergeFunctionTargetBindings(
					target.captures, parameterTargets,
				)
			}
			if target.literal != nil {
				target.factoryResult = true
				target.factoryScanned = target.factoryScanned || factoryInvoked[target.literal]
				if target.factoryScanned && target.literalOwner == nil {
					target.literalOwner = factory
				}
			}
			if target.function == nil {
				continue
			}
			key := ps6080FunctionTargetKey(target)
			if index, exists := resultIndices[key]; exists {
				ps6080MergeNamedFunctionTarget(&result[index], target)
				continue
			}
			resultIndices[key] = len(result)
			result = append(result, target)
		}
	}
	var namedResults []ast.Expr
	var namedResultObjects []types.Object
	if resultFields != nil {
		for _, field := range resultFields.List {
			if len(field.Names) == 0 {
				namedResults = append(namedResults, nil)
				namedResultObjects = append(namedResultObjects, nil)
				continue
			}
			for _, name := range field.Names {
				namedResults = append(namedResults, name)
				namedResultObjects = append(namedResultObjects, pass.TypesInfo.ObjectOf(name))
			}
		}
	}
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		returned, ok := node.(*ast.ReturnStmt)
		if !ok || !ps6080NodeReachable(pass, graph, parents, returned) ||
			!ps6080StaticallyReachable(pass, parents, returned) ||
			!ps6080FactoryReturnReachable(pass, body, returned, parents, parameterConstants) {
			return true
		}
		nakedReturn := len(returned.Results) == 0
		expressions := returned.Results
		if nakedReturn {
			expressions = namedResults
		}
		type projection struct {
			expression       ast.Expr
			logicalResult    int
			expressionResult int
		}
		projections := make([]projection, 0, len(expressions))
		if len(expressions) == 1 {
			if tuple, tupleResult := pass.TypesInfo.TypeOf(expressions[0]).(*types.Tuple); tupleResult {
				for index := range tuple.Len() {
					projections = append(projections, projection{
						expression: expressions[0], logicalResult: index, expressionResult: index,
					})
				}
			}
		}
		if len(projections) == 0 {
			for index, expression := range expressions {
				projections = append(projections, projection{
					expression: expression, logicalResult: index, expressionResult: -1,
				})
			}
		}
		for _, projected := range projections {
			if selectedResult >= 0 && projected.logicalResult != selectedResult {
				continue
			}
			expression := projected.expression
			if expression == nil {
				continue
			}
			if projected.logicalResult < len(namedResultObjects) &&
				namedResultObjects[projected.logicalResult] != nil &&
				ps6080DeferredLiteralAssigns(
					pass, body, parents, namedResultObjects[projected.logicalResult],
				) {
				continue
			}
			if parameter := ps6080FunctionParameterExpressionIndex(
				pass, info, expression, returned, parents,
			); parameter >= 0 {
				if signature != nil {
					switch {
					case parameter < signature.Params().Len():
						addTargets(parameterTargets[signature.Params().At(parameter)])
					case parameter == signature.Params().Len() && signature.Recv() != nil:
						addTargets(parameterTargets[signature.Recv()])
					}
				}
				continue
			}
			if projected.expressionResult < 0 {
				if target, static := ps6080StaticNamedFunctionTarget(
					pass, info, expression, returned, parents,
				); static {
					addTargets([]ps6080NamedFunctionTarget{target})
					continue
				}
			}
			addTargets(ps6080PossibleNamedFunctionTargetsVisiting(
				pass, info, expression, returned, parents, projected.expressionResult,
				visiting, factoryCache, parameterTargets, factoryCyclic, captureVisiting,
			))
		}
		return true
	})
	slices.SortFunc(result, func(left, right ps6080NamedFunctionTarget) int {
		return cmp.Compare(left.function.Pos(), right.function.Pos())
	})
	if !factoryCyclic[cacheKey] {
		factoryCache[cacheKey] = slices.Clone(result)
	}
	return result
}

func ps6080FactoryReturnReachable(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	returned *ast.ReturnStmt,
	parents map[ast.Node]ast.Node,
	parameters map[types.Object]constant.Value,
) bool {
	if len(parameters) == 0 {
		return true
	}
	effective := maps.Clone(parameters)
	for object := range effective {
		if ps6080FactoryParameterWrittenBefore(pass, body, returned, object) {
			delete(effective, object)
		}
	}
	var statement ast.Stmt = returned
	for current := ast.Node(returned); current != nil; current = parents[current] {
		if candidate, ok := current.(ast.Stmt); ok {
			statement = candidate
		}
		if conditional, ok := current.(*ast.IfStmt); ok && len(effective) > 0 {
			value, fixed := ps6080FactoryBoolValue(pass, conditional.Cond, effective)
			if fixed {
				switch {
				case ps6080NodeWithin(returned, conditional.Body) && !value:
					return false
				case conditional.Else != nil && ps6080NodeWithin(returned, conditional.Else) && value:
					return false
				}
			}
		}
		if switched, ok := current.(*ast.SwitchStmt); ok && len(effective) > 0 {
			selected, fixed := ps6080FactorySelectedSwitchClause(pass, switched, effective)
			if fixed {
				var containing *ast.CaseClause
				for _, candidate := range switched.Body.List {
					clause, clauseOK := candidate.(*ast.CaseClause)
					if clauseOK && ps6080NodeWithin(returned, clause) {
						containing = clause
						break
					}
				}
				if selected != containing {
					return false
				}
			}
		}
		block, ok := parents[current].(*ast.BlockStmt)
		if !ok || statement == nil {
			continue
		}
		index := slices.Index(block.List, statement)
		if index < 0 {
			continue
		}
		for _, priorStatement := range block.List[:index] {
			priorConstants := maps.Clone(parameters)
			for object := range priorConstants {
				if ps6080FactoryParameterWrittenBefore(
					pass, body, priorStatement, object,
				) {
					delete(priorConstants, object)
				}
			}
			if len(priorConstants) == 0 {
				continue
			}
			switch prior := priorStatement.(type) {
			case *ast.IfStmt:
				if prior.Init != nil {
					continue
				}
				value, fixed := ps6080FactoryBoolValue(pass, prior.Cond, priorConstants)
				if !fixed {
					continue
				}
				selected := []ast.Stmt(nil)
				if value {
					selected = prior.Body.List
				} else if prior.Else != nil {
					selected = []ast.Stmt{prior.Else}
				}
				if len(selected) > 0 && ps6080StatementsTerminate(pass, selected) {
					return false
				}
			case *ast.SwitchStmt:
				selected, fixed := ps6080FactorySelectedSwitchClause(pass, prior, priorConstants)
				if fixed && selected != nil && ps6080StatementsTerminate(pass, selected.Body) {
					return false
				}
			}
		}
	}
	return true
}

func ps6080FactoryParameterWrittenBefore(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	before ast.Node,
	object types.Object,
) bool {
	written := false
	beforePosition := before.Pos()
	switch value := before.(type) {
	case *ast.IfStmt:
		beforePosition = value.Cond.Pos()
	case *ast.SwitchStmt:
		beforePosition = ps6080SwitchEntryNode(value).Pos()
	}
	invoked := ps6080InvokedFunctionLiteralResult(pass, body)
	parents := ps6071Parents(body)
	graph := cfg.New(body, ps6080CallMayReturn(pass))
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil || written || node.Pos() >= beforePosition {
			return false
		}
		if literal, nested := node.(*ast.FuncLit); nested {
			for call := range invoked.calls[literal] {
				if call.Pos() >= beforePosition ||
					!ps6080CFGPositionMayReachWithoutAssignments(
						pass, graph, parents, call.Pos(), beforePosition, nil,
					) {
					continue
				}
				switch parent := parents[call].(type) {
				case *ast.DeferStmt:
					if parent.Call == call {
						continue
					}
				case *ast.GoStmt:
					if parent.Call == call {
						continue
					}
				}
				written = ps6080StatementsWriteObjects(
					pass, literal.Body.List, map[types.Object]bool{object: true},
				)
				return false
			}
			return false
		}
		if !ps6080CFGPositionMayReachWithoutAssignments(
			pass, graph, parents, node.Pos(), beforePosition, nil,
		) {
			return true
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				identifier, direct := ps2110Unparen(left).(*ast.Ident)
				if direct && pass.TypesInfo.ObjectOf(identifier) == object {
					written = true
					break
				}
			}
		case *ast.IncDecStmt:
			identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
			written = direct && pass.TypesInfo.ObjectOf(identifier) == object
		case *ast.RangeStmt:
			for _, expression := range []ast.Expr{value.Key, value.Value} {
				identifier, direct := ps2110Unparen(expression).(*ast.Ident)
				if direct && pass.TypesInfo.ObjectOf(identifier) == object {
					written = true
					break
				}
			}
		case *ast.UnaryExpr:
			identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
			written = value.Op == token.AND && direct && pass.TypesInfo.ObjectOf(identifier) == object
		}
		return !written
	})
	return written
}

func ps6080FactorySelectedSwitchClause(
	pass *analysis.Pass,
	statement *ast.SwitchStmt,
	parameters map[types.Object]constant.Value,
) (*ast.CaseClause, bool) {
	if statement == nil || statement.Init != nil {
		return nil, false
	}
	astSafe := true
	ast.Inspect(statement.Body, func(node ast.Node) bool {
		if branch, ok := node.(*ast.BranchStmt); ok && branch.Tok == token.FALLTHROUGH {
			astSafe = false
			return false
		}
		return astSafe
	})
	if !astSafe {
		return nil, false
	}
	tag := constant.MakeBool(true)
	if statement.Tag != nil {
		var fixed bool
		tag, fixed = ps6080FactoryConstantValue(pass, statement.Tag, parameters)
		if !fixed {
			return nil, false
		}
	}
	var defaultClause *ast.CaseClause
	for _, candidate := range statement.Body.List {
		clause, ok := candidate.(*ast.CaseClause)
		if !ok {
			return nil, false
		}
		if len(clause.List) == 0 {
			defaultClause = clause
			continue
		}
		for _, expression := range clause.List {
			value, fixed := ps6080FactoryConstantValue(pass, expression, parameters)
			if !fixed {
				return nil, false
			}
			if ps6080FactoryConstantsEqual(tag, value) {
				return clause, true
			}
		}
	}
	return defaultClause, true
}

func ps6080FactoryConstantsEqual(left, right constant.Value) (equal bool) {
	defer func() {
		if recover() != nil {
			equal = false
		}
	}()
	return constant.Compare(left, token.EQL, right)
}

func ps6080FactoryBoolValue(
	pass *analysis.Pass,
	expression ast.Expr,
	parameters map[types.Object]constant.Value,
) (bool, bool) {
	value, fixed := ps6080FactoryConstantValue(pass, expression, parameters)
	if !fixed || value.Kind() != constant.Bool {
		return false, false
	}
	return constant.BoolVal(value), true
}

func ps6080FactoryConstantValue(
	pass *analysis.Pass,
	expression ast.Expr,
	parameters map[types.Object]constant.Value,
) (result constant.Value, fixed bool) {
	defer func() {
		if recover() != nil {
			result = nil
			fixed = false
		}
	}()
	expression = ps2110Unparen(expression)
	if value := pass.TypesInfo.Types[expression].Value; value != nil {
		return value, true
	}
	switch value := expression.(type) {
	case *ast.Ident:
		result := parameters[pass.TypesInfo.ObjectOf(value)]
		return result, result != nil
	case *ast.UnaryExpr:
		operand, fixed := ps6080FactoryConstantValue(pass, value.X, parameters)
		if !fixed {
			return nil, false
		}
		switch value.Op {
		case token.ADD, token.SUB, token.XOR, token.NOT:
			return constant.UnaryOp(value.Op, operand, 0), true
		default:
			return nil, false
		}
	case *ast.BinaryExpr:
		left, leftFixed := ps6080FactoryConstantValue(pass, value.X, parameters)
		right, rightFixed := ps6080FactoryConstantValue(pass, value.Y, parameters)
		if !leftFixed || !rightFixed {
			return nil, false
		}
		switch value.Op {
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			return constant.MakeBool(constant.Compare(left, value.Op, right)), true
		case token.LAND:
			return constant.MakeBool(constant.BoolVal(left) && constant.BoolVal(right)), true
		case token.LOR:
			return constant.MakeBool(constant.BoolVal(left) || constant.BoolVal(right)), true
		case token.ADD, token.SUB, token.MUL, token.QUO, token.REM,
			token.AND, token.OR, token.XOR, token.SHL, token.SHR, token.AND_NOT:
			return constant.BinaryOp(left, value.Op, right), true
		default:
			return nil, false
		}
	}
	return nil, false
}

func ps6080TypedNamedFunctionTarget(
	pass *analysis.Pass,
	expression ast.Expr,
) (ps6080NamedFunctionTarget, bool) {
	function, _, ok := typedCallee(pass, expression)
	if !ok {
		return ps6080NamedFunctionTarget{}, false
	}
	result := ps6080NamedFunctionTarget{function: function}
	callable := ps2110Unparen(expression)
	switch value := callable.(type) {
	case *ast.IndexExpr:
		callable = ps2110Unparen(value.X)
	case *ast.IndexListExpr:
		callable = ps2110Unparen(value.X)
	}
	selector, selectorOK := callable.(*ast.SelectorExpr)
	if !selectorOK {
		return result, true
	}
	if selection := pass.TypesInfo.Selections[selector]; selection != nil {
		switch selection.Kind() {
		case types.MethodVal:
			result.receiver = selector.X
			result.receiverQuery = selector
		case types.MethodExpr:
			result.methodExpression = true
		}
	}
	return result, true
}

func ps6080StaticNamedCallee(
	pass *analysis.Pass,
	caller *ps6080Function,
	call *ast.CallExpr,
	parents map[ast.Node]ast.Node,
) (ps6080NamedFunctionTarget, bool) {
	if result, ok := ps6080TypedNamedFunctionTarget(pass, call.Fun); ok {
		return result, true
	}
	return ps6080StaticNamedFunctionTarget(pass, caller, call.Fun, call, parents)
}

func ps6080CPURoot(function *ps6080Function) bool {
	return function.roles&ps6080MatmulRole != 0 &&
		(function.object.Exported() || !function.cpuIncoming || function.indirect)
}

func ps6080MarkIndirectFunctionReferences(
	pass *analysis.Pass,
	functions map[*types.Func]*ps6080Function,
) {
	callbacks := ps6080NamedCallbackParameters(pass)
	mayCallbacks := ps6080MayNamedCallbackSites(pass)
	parents := make(map[ast.Node]ast.Node)
	for _, file := range pass.Files {
		for child, parent := range ps6071Parents(file) {
			parents[child] = parent
		}
	}
	type reachInfo struct {
		graph         *cfg.CFG
		invoked       *ps6080InvokedLiteralResult
		literalGraphs map[*ast.FuncLit]*cfg.CFG
	}
	reachability := make(map[*types.Func]*reachInfo)
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			object, functionObject := pass.TypesInfo.ObjectOf(identifier).(*types.Func)
			function := functions[object]
			if !functionObject || function == nil || pass.TypesInfo.Defs[identifier] == object {
				return true
			}
			if !ps6080StaticallyReachable(pass, parents, identifier) {
				return true
			}
			var owner *types.Func
			var declaration *ast.FuncDecl
			for current := ast.Node(identifier); current != nil; current = parents[current] {
				candidate, ok := current.(*ast.FuncDecl)
				if !ok {
					continue
				}
				declaration = candidate
				owner, _ = pass.TypesInfo.Defs[declaration.Name].(*types.Func)
				if enclosing := functions[owner]; enclosing != nil && !enclosing.scanned {
					return true
				}
				break
			}
			if owner != nil && declaration != nil {
				info := reachability[owner]
				if info == nil {
					invoked := ps6080InvokedFunctionLiteralResult(pass, declaration.Body)
					info = &reachInfo{
						graph: cfg.New(declaration.Body, ps6080CallMayReturn(pass)), invoked: invoked,
						literalGraphs: ps6080InvokedLiteralGraphs(pass, invoked.literals),
					}
					reachability[owner] = info
				}
				if literal := ps6080ContainingLiteral(identifier, parents); literal != nil &&
					!info.invoked.literals[literal] {
					return true
				}
				graph := ps6080GraphForNode(info.graph, info.literalGraphs, parents, identifier)
				if !ps6080NodeReachable(pass, graph, parents, identifier) {
					return true
				}
			}
			for current := ast.Node(identifier); current != nil; current = parents[current] {
				parent := parents[current]
				if call, directCall := parent.(*ast.CallExpr); directCall && call.Fun == current {
					callee, _, resolved := typedCallee(pass, call.Fun)
					if resolved && callee == object {
						return true
					}
					break
				}
				switch parent.(type) {
				case *ast.SelectorExpr, *ast.IndexExpr, *ast.IndexListExpr, *ast.ParenExpr:
					continue
				}
				break
			}
			if ps6080SafeFunctionAliasReference(pass, identifier, parents, callbacks, mayCallbacks) ||
				ps6080SafeNamedCallbackReference(pass, identifier, parents, callbacks, mayCallbacks) {
				return true
			}
			function.indirect = true
			return true
		})
	}
}

func ps6080SafeNamedCallbackReference(
	pass *analysis.Pass,
	reference *ast.Ident,
	parents map[ast.Node]ast.Node,
	callbacks map[*types.Func]ps6080CallbackMappings,
	mayCallbacks map[*types.Func][][]*ps6080MayCallbackSite,
) bool {
	current := ast.Node(reference)
	for {
		parent := parents[current]
		if paren, ok := parent.(*ast.ParenExpr); ok && paren.X == current {
			current = parent
			continue
		}
		if selector, ok := parent.(*ast.SelectorExpr); ok && selector.Sel == current {
			current = parent
			continue
		}
		call, ok := parent.(*ast.CallExpr)
		if !ok {
			return false
		}
		if len(call.Args) == 1 && call.Args[0] == current && pass.TypesInfo.Types[call.Fun].IsType() {
			current = call
			continue
		}
		argument := slices.Index(call.Args, current.(ast.Expr))
		if argument < 0 {
			return false
		}
		callee, _, direct := typedCallee(pass, call.Fun)
		if !direct {
			return false
		}
		return argument < len(callbacks[callee]) && callbacks[callee][argument] != nil ||
			argument < len(mayCallbacks[callee]) && len(mayCallbacks[callee][argument]) > 0
	}
}

func ps6080SafeFunctionAliasReference(
	pass *analysis.Pass,
	reference *ast.Ident,
	parents map[ast.Node]ast.Node,
	callbacks map[*types.Func]ps6080CallbackMappings,
	mayCallbacks map[*types.Func][][]*ps6080MayCallbackSite,
) bool {
	alias := ps6080FunctionAliasTarget(pass, reference, parents)
	return ps6080SafeFunctionAliasObject(
		pass, alias, parents, callbacks, mayCallbacks, make(map[*types.Var]bool),
	)
}

func ps6080FunctionAliasTarget(
	pass *analysis.Pass,
	reference *ast.Ident,
	parents map[ast.Node]ast.Node,
) *types.Var {
	current := ast.Node(reference)
	for {
		parent := parents[current]
		if paren, ok := parent.(*ast.ParenExpr); ok && paren.X == current {
			current = parent
			continue
		}
		if selector, ok := parent.(*ast.SelectorExpr); ok && selector.Sel == current {
			current = parent
			continue
		}
		if call, ok := parent.(*ast.CallExpr); ok && len(call.Args) == 1 &&
			call.Args[0] == current && pass.TypesInfo.Types[call.Fun].IsType() {
			current = parent
			continue
		}
		break
	}
	var alias *types.Var
	switch parent := parents[current].(type) {
	case *ast.AssignStmt:
		index := slices.Index(parent.Rhs, current.(ast.Expr))
		if index >= 0 && len(parent.Lhs) == len(parent.Rhs) {
			identifier, direct := ps2110Unparen(parent.Lhs[index]).(*ast.Ident)
			if direct {
				alias, _ = pass.TypesInfo.ObjectOf(identifier).(*types.Var)
			}
		}
	case *ast.ValueSpec:
		index := slices.Index(parent.Values, current.(ast.Expr))
		if index >= 0 && len(parent.Names) == len(parent.Values) {
			alias, _ = pass.TypesInfo.Defs[parent.Names[index]].(*types.Var)
		}
	}
	if alias == nil || !ps6080CallableType(alias.Type()) {
		return nil
	}
	return alias
}

func ps6080SafeFunctionAliasObject(
	pass *analysis.Pass,
	alias *types.Var,
	parents map[ast.Node]ast.Node,
	callbacks map[*types.Func]ps6080CallbackMappings,
	mayCallbacks map[*types.Func][][]*ps6080MayCallbackSite,
	visiting map[*types.Var]bool,
) bool {
	if alias == nil || visiting[alias] {
		return false
	}
	visiting[alias] = true
	defer delete(visiting, alias)
	used := false
	safe := true
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			if !safe {
				return false
			}
			identifier, ok := node.(*ast.Ident)
			if !ok || pass.TypesInfo.ObjectOf(identifier) != alias {
				return true
			}
			if pass.TypesInfo.Defs[identifier] == alias {
				return true
			}
			current := ast.Node(identifier)
			for {
				parent := parents[current]
				if paren, ok := parent.(*ast.ParenExpr); ok && paren.X == current {
					current = parent
					continue
				}
				call, directCall := parent.(*ast.CallExpr)
				if directCall && call.Fun == current {
					used = true
					return true
				}
				if ps6080SafeNamedCallbackReference(pass, identifier, parents, callbacks, mayCallbacks) {
					used = true
					return true
				}
				if target := ps6080FunctionAliasTarget(pass, identifier, parents); target != nil &&
					ps6080SafeFunctionAliasObject(pass, target, parents, callbacks, mayCallbacks, visiting) {
					used = true
					return true
				}
				safe = false
				return false
			}
		})
	}
	return safe && used
}

func ps6080CPUCallScopes(
	pass *analysis.Pass,
	caller *ps6080Function,
	callee *types.Func,
	call *ast.CallExpr,
	parents map[ast.Node]ast.Node,
	invoked *ps6080InvokedLiteralResult,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
	target *ps6080NamedFunctionTarget,
) map[types.Object]*ps6080CPUCallScope {
	signature, _ := callee.Type().(*types.Signature)
	if signature == nil {
		return nil
	}
	result := make(map[types.Object]*ps6080CPUCallScope)
	argumentOffset := 0
	if signature.Recv() != nil {
		var receiver ast.Expr
		if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
			selection := pass.TypesInfo.Selections[selector]
			switch {
			case selection != nil && selection.Kind() == types.MethodVal:
				receiver = selector.X
			case selection != nil && selection.Kind() == types.MethodExpr && len(call.Args) > 0:
				receiver = call.Args[0]
				argumentOffset = 1
			}
		}
		if receiver == nil && target != nil {
			switch {
			case target.receiver != nil:
				receiver = target.receiver
			case target.methodExpression && len(call.Args) > 0:
				receiver = call.Args[0]
				argumentOffset = 1
			}
		}
		if enum, enumType := ps6080EnumType(signature.Recv().Type()); enumType && receiver != nil {
			receiverQuery := ast.Node(call)
			if target != nil && target.receiver == receiver && target.receiverQuery != nil {
				receiverQuery = target.receiverQuery
			}
			receiverCaller := caller
			receiverParents := parents
			receiverInvoked := invoked
			if target != nil && target.receiverArgument != nil && target.receiverArgCaller != nil {
				receiver = target.receiverArgument
				receiverCaller = target.receiverArgCaller
				receiverQuery = target.receiverCall
				receiverParents = ps6071Parents(ps6080FunctionBody(receiverCaller))
				receiverInvoked = ps6080InvokedFunctionLiteralResult(
					pass, ps6080FunctionBody(receiverCaller),
				)
			}
			result[signature.Recv()] = ps6080CPUArgumentScope(
				pass, receiverCaller, receiver, receiverQuery, enum, receiverParents, receiverInvoked,
				constantEnums, domains, make(map[*ast.FuncLit]bool),
			)
		}
	}
	limit := min(signature.Params().Len(), len(call.Args)-argumentOffset)
	for index := range limit {
		parameter := signature.Params().At(index)
		enum, enumType := ps6080EnumType(parameter.Type())
		if !enumType {
			continue
		}
		end := index + argumentOffset + 1
		if signature.Variadic() && index == signature.Params().Len()-1 {
			end = len(call.Args)
		}
		for argumentIndex := index + argumentOffset; argumentIndex < end; argumentIndex++ {
			scope := ps6080CPUArgumentScope(
				pass, caller, call.Args[argumentIndex], call, enum, parents, invoked,
				constantEnums, domains, make(map[*ast.FuncLit]bool),
			)
			result[parameter] = ps6080MergeCPUArgumentScopes(result[parameter], scope)
		}
	}
	for current := ast.Node(call); current != nil; current = parents[current] {
		literal, nested := current.(*ast.FuncLit)
		if !nested || len(caller.returnedLiteralScopes[literal]) == 0 {
			continue
		}
		returnedScopes := caller.returnedLiteralScopes[literal]
		for index := range limit {
			parameter := signature.Params().At(index)
			enum, enumType := ps6080EnumType(parameter.Type())
			if !enumType {
				continue
			}
			end := index + argumentOffset + 1
			if signature.Variadic() && index == signature.Params().Len()-1 {
				end = len(call.Args)
			}
			for argumentIndex := index + argumentOffset; argumentIndex < end; argumentIndex++ {
				for _, subject := range ps6080ReturnedScopeSubjects(
					pass, caller, call.Args[argumentIndex], call, enum, parents,
				) {
					if returnedScope := returnedScopes[subject]; returnedScope != nil {
						result[parameter] = ps6080MergeCPUArgumentScopes(
							result[parameter], returnedScope,
						)
					}
				}
			}
		}
	}
	return result
}

func ps6080ReturnedScopeSubjects(
	pass *analysis.Pass,
	caller *ps6080Function,
	argument ast.Expr,
	query ast.Node,
	enum *types.TypeName,
	parents map[ast.Node]ast.Node,
) []types.Object {
	subject, unique := ps6080EnumSubject(pass, argument, enum)
	if !unique {
		return nil
	}
	return ps6080ReturnedScopeObjects(pass, caller, subject, query, enum, parents)
}

func ps6080ReturnedScopeObjects(
	pass *analysis.Pass,
	caller *ps6080Function,
	subject types.Object,
	query ast.Node,
	enum *types.TypeName,
	parents map[ast.Node]ast.Node,
) []types.Object {
	seen := make(map[types.Object]bool)
	var result []types.Object
	for subject != nil && !seen[subject] {
		seen[subject] = true
		result = append(result, subject)
		variable, ok := subject.(*types.Var)
		if !ok {
			break
		}
		initializer := ps6080StableLocalInitializer(pass, caller, variable, query, parents)
		if initializer == nil {
			break
		}
		var unique bool
		subject, unique = ps6080EnumSubject(pass, initializer, enum)
		if !unique {
			break
		}
	}
	return result
}

func ps6080UniversalCPUCallScopes(function *types.Func) map[types.Object]*ps6080CPUCallScope {
	signature, _ := function.Type().(*types.Signature)
	if signature == nil {
		return nil
	}
	result := make(map[types.Object]*ps6080CPUCallScope)
	if receiver := signature.Recv(); receiver != nil {
		if enum, enumType := ps6080EnumType(receiver.Type()); enumType {
			result[receiver] = &ps6080CPUCallScope{enum: enum, universal: true}
		}
	}
	for index := range signature.Params().Len() {
		parameter := signature.Params().At(index)
		if enum, enumType := ps6080EnumType(parameter.Type()); enumType {
			result[parameter] = &ps6080CPUCallScope{enum: enum, universal: true}
		}
	}
	return result
}

func ps6080MergeCPUArgumentScopes(
	left, right *ps6080CPUCallScope,
) *ps6080CPUCallScope {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	distinctSources := left.source != nil && right.source != nil && left.source != right.source
	if distinctSources && left.allowed != nil && right.allowed != nil {
		fixed := ps6080CPUUnionValues(left.fixed, right.fixed)
		fixed = ps6080CPUUnionValues(fixed, left.allowed)
		fixed = ps6080CPUUnionValues(fixed, right.allowed)
		return &ps6080CPUCallScope{enum: left.enum, fixed: fixed}
	}
	if left.universal || right.universal || distinctSources {
		return &ps6080CPUCallScope{enum: left.enum, universal: true}
	}
	result := &ps6080CPUCallScope{
		enum:  left.enum,
		fixed: ps6080CPUUnionValues(left.fixed, right.fixed),
	}
	switch {
	case left.source == nil:
		result.source = right.source
		result.allowed = right.allowed
	case right.source == nil:
		result.source = left.source
		result.allowed = left.allowed
	default:
		result.source = left.source
		if left.allowed != nil && right.allowed != nil {
			result.allowed = ps6080CPUUnionValues(left.allowed, right.allowed)
		}
	}
	return result
}

func ps6080CPUArgumentScope(
	pass *analysis.Pass,
	caller *ps6080Function,
	argument ast.Expr,
	query ast.Node,
	enum *types.TypeName,
	parents map[ast.Node]ast.Node,
	invoked *ps6080InvokedLiteralResult,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
	visiting map[*ast.FuncLit]bool,
) *ps6080CPUCallScope {
	if value := pass.TypesInfo.Types[argument].Value; value != nil {
		return &ps6080CPUCallScope{
			enum: enum, fixed: map[string]token.Pos{value.ExactString(): argument.Pos()},
		}
	}
	subject, resolved, fixed := ps6080CPUArgumentSubject(pass, caller, argument, query, enum, parents)
	if fixed != nil {
		return &ps6080CPUCallScope{
			enum: enum, fixed: map[string]token.Pos{fixed.ExactString(): argument.Pos()},
		}
	}
	if !resolved {
		return &ps6080CPUCallScope{enum: enum, universal: true}
	}
	return ps6080CPUSubjectScopeAtNode(
		pass, caller, subject, query, enum, parents, invoked, constantEnums, domains, visiting,
	)
}

func ps6080CPUArgumentSubject(
	pass *analysis.Pass,
	caller *ps6080Function,
	argument ast.Expr,
	query ast.Node,
	enum *types.TypeName,
	parents map[ast.Node]ast.Node,
) (types.Object, bool, constant.Value) {
	identifier, direct := ps2110Unparen(argument).(*ast.Ident)
	if direct {
		if object, variable := pass.TypesInfo.ObjectOf(identifier).(*types.Var); variable {
			literal := ps6080ContainingLiteral(query, parents)
			captured := literal != nil && !(literal.Pos() <= object.Pos() && object.Pos() < literal.End())
			if !captured {
				initializer := ps6080StableLocalInitializer(pass, caller, object, query, parents)
				if initializer == nil {
					goto directSubject
				}
				if value := pass.TypesInfo.Types[initializer].Value; value != nil {
					return nil, false, value
				}
				if subject, unique := ps6080EnumSubject(pass, initializer, enum); unique {
					if !ps6080ObjectMayChangeBetween(
						pass, ps6080FunctionBody(caller), subject, initializer.End(), query.Pos(),
					) {
						return subject, true, nil
					}
				}
			}
		}
	}

directSubject:
	subject, unique := ps6080EnumSubject(pass, argument, enum)
	return subject, unique, nil
}

func ps6080StableLocalInitializer(
	pass *analysis.Pass,
	caller *ps6080Function,
	object *types.Var,
	query ast.Node,
	parents map[ast.Node]ast.Node,
) ast.Expr {
	type frame struct {
		statements []ast.Stmt
		stop       ast.Stmt
	}
	var frames []frame
	var statement ast.Stmt
	for current := query; current != nil; current = parents[current] {
		if candidate, ok := current.(ast.Stmt); ok {
			statement = candidate
		}
		switch parent := parents[current].(type) {
		case *ast.BlockStmt:
			if statement != nil {
				frames = append(frames, frame{statements: parent.List, stop: statement})
			}
		case *ast.CaseClause:
			if statement != nil {
				frames = append(frames, frame{statements: parent.Body, stop: statement})
			}
		}
	}
	if len(frames) == 0 {
		return nil
	}
	slices.Reverse(frames)
	var initializer ast.Expr
	for _, current := range frames {
		for _, candidate := range current.statements {
			if candidate == current.stop || candidate.Pos() >= current.stop.Pos() {
				break
			}
			if expression, assigned, safe := ps6080DirectLocalAssignment(pass, candidate, object); assigned {
				if !safe {
					return nil
				}
				initializer = expression
				continue
			}
			if ps6080StatementMayMutateObject(pass, candidate, object) {
				return nil
			}
		}
	}
	return initializer
}

func ps6080DirectLocalAssignment(
	pass *analysis.Pass,
	statement ast.Stmt,
	object *types.Var,
) (ast.Expr, bool, bool) {
	switch value := statement.(type) {
	case *ast.AssignStmt:
		matched := -1
		for index, left := range value.Lhs {
			identifier, direct := ps2110Unparen(left).(*ast.Ident)
			if !direct || pass.TypesInfo.ObjectOf(identifier) != object {
				continue
			}
			matched = index
		}
		if matched >= 0 {
			if value.Tok != token.ASSIGN && value.Tok != token.DEFINE || len(value.Lhs) != len(value.Rhs) {
				return nil, true, false
			}
			return value.Rhs[matched], true, true
		}
	case *ast.DeclStmt:
		general, _ := value.Decl.(*ast.GenDecl)
		if general == nil {
			break
		}
		for _, specification := range general.Specs {
			values, _ := specification.(*ast.ValueSpec)
			if values == nil {
				continue
			}
			for index, name := range values.Names {
				if pass.TypesInfo.Defs[name] != object {
					continue
				}
				if len(values.Names) != len(values.Values) {
					return nil, true, false
				}
				return values.Values[index], true, true
			}
		}
	}
	return nil, false, true
}

func ps6080StatementMayMutateObject(
	pass *analysis.Pass,
	statement ast.Stmt,
	object types.Object,
) bool {
	objects := map[types.Object]bool{object: true}
	unsafe := false
	ast.Inspect(statement, func(node ast.Node) bool {
		if node == nil || unsafe {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			unsafe = ps6080AssignmentTouchesObjects(pass, value, objects)
		case *ast.IncDecStmt, *ast.RangeStmt:
			if candidate, ok := node.(ast.Stmt); ok {
				unsafe = ps6080StatementsWriteObjects(pass, []ast.Stmt{candidate}, objects)
			}
		case *ast.UnaryExpr:
			identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
			unsafe = value.Op == token.AND && direct && pass.TypesInfo.ObjectOf(identifier) == object
		case *ast.SelectorExpr:
			unsafe = ps6080PointerMethodOnObjects(pass, value, objects)
		}
		return !unsafe
	})
	return unsafe
}

func ps6080NodeMayMutateObjects(
	pass *analysis.Pass,
	node ast.Node,
	objects map[types.Object]bool,
) bool {
	if node == nil || len(objects) == 0 {
		return false
	}
	unsafe := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		if candidate == nil || unsafe {
			return false
		}
		switch value := candidate.(type) {
		case *ast.AssignStmt:
			unsafe = ps6080AssignmentTouchesObjects(pass, value, objects)
		case *ast.IncDecStmt, *ast.RangeStmt:
			if statement, ok := candidate.(ast.Stmt); ok {
				unsafe = ps6080StatementsWriteObjects(pass, []ast.Stmt{statement}, objects)
			}
		case *ast.UnaryExpr:
			identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
			unsafe = value.Op == token.AND && direct && objects[pass.TypesInfo.ObjectOf(identifier)]
		case *ast.SelectorExpr:
			unsafe = ps6080PointerMethodOnObjects(pass, value, objects)
		}
		return !unsafe
	})
	return unsafe
}

func ps6080CPUSubjectScopeAtNode(
	pass *analysis.Pass,
	caller *ps6080Function,
	subject types.Object,
	node ast.Node,
	enum *types.TypeName,
	parents map[ast.Node]ast.Node,
	invoked *ps6080InvokedLiteralResult,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
	visiting map[*ast.FuncLit]bool,
) *ps6080CPUCallScope {
	values, constrained, unstable := ps6080CPUContextValues(
		pass, caller, node, subject, enum, parents, constantEnums, domains,
	)
	if unstable {
		return &ps6080CPUCallScope{enum: enum, universal: true}
	}
	if constrained {
		return &ps6080CPUCallScope{enum: enum, source: subject, allowed: values}
	}
	literal := ps6080ContainingLiteral(node, parents)
	if literal != nil {
		return ps6080CPUInvocationScope(
			pass, caller, literal, subject, enum, parents, invoked,
			constantEnums, domains, visiting,
		)
	}
	return &ps6080CPUCallScope{enum: enum, source: subject}
}

func ps6080CPUContextValues(
	pass *analysis.Pass,
	caller *ps6080Function,
	node ast.Node,
	subject types.Object,
	enum *types.TypeName,
	parents map[ast.Node]ast.Node,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
) (map[string]token.Pos, bool, bool) {
	allowed := ps6080CPUFullDomain(domains[enum])
	subjects := ps6080CPUEquivalentSubjects(pass, caller, subject, node, enum, parents)
	constrained := false
	for current := node; current != nil; current = parents[current] {
		if _, boundary := current.(*ast.FuncLit); boundary {
			break
		}
		switch parent := parents[current].(type) {
		case *ast.BinaryExpr:
			if !ps6080NodeWithin(node, parent.Y) || parent.Op != token.LAND && parent.Op != token.LOR {
				continue
			}
			if ps6080NodeMayMutateObjects(pass, parent.X, subjects) {
				return allowed, false, true
			}
			trueValues, falseValues, relevant := ps6080CPUPredicateValues(
				pass, parent.X, subjects, enum, constantEnums, domains,
			)
			if !relevant {
				continue
			}
			candidate := trueValues
			if parent.Op == token.LOR {
				candidate = falseValues
			}
			allowed = ps6080CPUIntersectValues(allowed, candidate)
			constrained = true
		case *ast.IfStmt:
			var branch *bool
			var region ast.Node
			if ps6080NodeWithin(node, parent.Body) {
				truth := true
				branch = &truth
				region = parent.Body
			} else if parent.Else != nil && ps6080NodeWithin(node, parent.Else) {
				truth := false
				branch = &truth
				region = parent.Else
			}
			if branch == nil {
				continue
			}
			if ps6080ObjectMayChangeBeforeNode(pass, region, subject, node) ||
				ps6080NodeMayMutateObjects(pass, parent.Cond, subjects) {
				return allowed, false, true
			}
			trueValues, falseValues, relevant := ps6080CPUPredicateValues(
				pass, parent.Cond, subjects, enum, constantEnums, domains,
			)
			if !relevant {
				continue
			}
			candidate := falseValues
			if *branch {
				candidate = trueValues
			}
			allowed = ps6080CPUIntersectValues(allowed, candidate)
			constrained = true
		case *ast.CaseClause:
			body, _ := parents[parent].(*ast.BlockStmt)
			switchStmt, _ := parents[body].(*ast.SwitchStmt)
			if switchStmt == nil {
				continue
			}
			if !ps6080CaseBodyContains(parent, node) {
				candidate, relevant, mutation := ps6080CPUSwitchExpressionPrerequisites(
					pass, switchStmt, parent, node, subjects, enum, constantEnums, domains,
				)
				if mutation {
					return allowed, false, true
				}
				if relevant {
					allowed = ps6080CPUIntersectValues(allowed, candidate)
					constrained = true
				}
				continue
			}
			if ps6080ObjectMayChangeBeforeNode(pass, parent, subject, node) {
				return allowed, false, true
			}
			candidate, relevant, mutation := ps6080CPUSwitchClauseValues(
				pass, switchStmt, parent, subjects, enum, constantEnums, domains,
			)
			if mutation {
				return allowed, false, true
			}
			if relevant {
				allowed = ps6080CPUIntersectValues(allowed, candidate)
				constrained = true
			}
		}
	}
	return allowed, constrained, false
}

func ps6080CPUSwitchExpressionPrerequisites(
	pass *analysis.Pass,
	switchStmt *ast.SwitchStmt,
	clause *ast.CaseClause,
	node ast.Node,
	subjects map[types.Object]bool,
	enum *types.TypeName,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
) (map[string]token.Pos, bool, bool) {
	if switchStmt.Tag != nil {
		return ps6080CPUFullDomain(domains[enum]), false, false
	}
	allowed := ps6080CPUFullDomain(domains[enum])
	relevant := false
	for _, statement := range switchStmt.Body.List {
		current, ok := statement.(*ast.CaseClause)
		if !ok {
			return ps6080CPUFullDomain(domains[enum]), false, false
		}
		for _, expression := range current.List {
			if ps6080NodeWithin(node, expression) {
				return allowed, relevant, false
			}
			if ps6080NodeMayMutateObjects(pass, expression, subjects) {
				return ps6080CPUFullDomain(domains[enum]), false, true
			}
			_, falsity, expressionRelevant := ps6080CPUPredicateValues(
				pass, expression, subjects, enum, constantEnums, domains,
			)
			if expressionRelevant {
				allowed = ps6080CPUIntersectValues(allowed, falsity)
				relevant = true
			}
		}
		if current == clause {
			break
		}
	}
	return ps6080CPUFullDomain(domains[enum]), false, false
}

func ps6080CPUEquivalentSubjects(
	pass *analysis.Pass,
	caller *ps6080Function,
	subject types.Object,
	node ast.Node,
	enum *types.TypeName,
	parents map[ast.Node]ast.Node,
) map[types.Object]bool {
	result := map[types.Object]bool{subject: true}
	variables := make(map[*types.Var]bool)
	ast.Inspect(ps6080FunctionBody(caller), func(candidate ast.Node) bool {
		identifier, ok := candidate.(*ast.Ident)
		if !ok {
			return true
		}
		variable, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
		if ok {
			variableEnum, enumType := ps6080EnumType(variable.Type())
			if enumType && variableEnum == enum {
				variables[variable] = true
			}
		}
		return true
	})
	for changed := true; changed; {
		changed = false
		for variable := range variables {
			if result[variable] {
				continue
			}
			initializer := ps6080StableLocalInitializer(pass, caller, variable, node, parents)
			if initializer == nil {
				continue
			}
			initializerSubject, unique := ps6080EnumSubject(pass, initializer, enum)
			if !unique || !result[initializerSubject] || ps6080ObjectMayChangeBetween(
				pass, ps6080FunctionBody(caller), initializerSubject, initializer.End(), node.Pos(),
			) {
				continue
			}
			result[variable] = true
			changed = true
		}
	}
	return result
}

func ps6080CPUPredicateValues(
	pass *analysis.Pass,
	expression ast.Expr,
	subjects map[types.Object]bool,
	enum *types.TypeName,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
) (map[string]token.Pos, map[string]token.Pos, bool) {
	full := func() map[string]token.Pos { return ps6080CPUFullDomain(domains[enum]) }
	expression = ps2110Unparen(expression)
	if value := pass.TypesInfo.Types[expression].Value; value != nil && value.Kind() == constant.Bool {
		if constant.BoolVal(value) {
			return full(), make(map[string]token.Pos), false
		}
		return make(map[string]token.Pos), full(), false
	}
	switch value := expression.(type) {
	case *ast.UnaryExpr:
		if value.Op == token.NOT {
			truth, falsity, relevant := ps6080CPUPredicateValues(
				pass, value.X, subjects, enum, constantEnums, domains,
			)
			return falsity, truth, relevant
		}
	case *ast.BinaryExpr:
		if value.Op == token.LAND || value.Op == token.LOR {
			leftTrue, leftFalse, leftRelevant := ps6080CPUPredicateValues(
				pass, value.X, subjects, enum, constantEnums, domains,
			)
			rightTrue, rightFalse, rightRelevant := ps6080CPUPredicateValues(
				pass, value.Y, subjects, enum, constantEnums, domains,
			)
			if value.Op == token.LAND {
				return ps6080CPUIntersectValues(leftTrue, rightTrue),
					ps6080CPUUnionValues(leftFalse, rightFalse), leftRelevant || rightRelevant
			}
			return ps6080CPUUnionValues(leftTrue, rightTrue),
				ps6080CPUIntersectValues(leftFalse, rightFalse), leftRelevant || rightRelevant
		}
		if value.Op == token.EQL || value.Op == token.NEQ {
			groups, _ := ps6080GuardExpression(pass, expression, constantEnums)
			conditionSubject, unique := ps6080EnumSubject(pass, expression, enum)
			if !unique || !subjects[conditionSubject] || groups[enum] == nil {
				return full(), full(), false
			}
			truth := ps6080CPUGroupValues(groups[enum], domains[enum])
			return truth, ps6080CPUDifferenceValues(full(), truth), true
		}
	}
	return full(), full(), false
}

func ps6080CPUSwitchClauseValues(
	pass *analysis.Pass,
	switchStmt *ast.SwitchStmt,
	clause *ast.CaseClause,
	subjects map[types.Object]bool,
	enum *types.TypeName,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
) (map[string]token.Pos, bool, bool) {
	full := ps6080CPUFullDomain(domains[enum])
	var result map[string]token.Pos
	relevant := false
	clauses := switchStmt.Body.List
	index := slices.Index(clauses, ast.Stmt(clause))
	start := index
	for start > 0 {
		prior, ok := clauses[start-1].(*ast.CaseClause)
		if !ok || len(prior.Body) == 0 {
			break
		}
		branch, ok := prior.Body[len(prior.Body)-1].(*ast.BranchStmt)
		if !ok || branch.Tok != token.FALLTHROUGH {
			break
		}
		start--
	}
	for candidateIndex := start; candidateIndex <= index; candidateIndex++ {
		candidate, ok := clauses[candidateIndex].(*ast.CaseClause)
		if !ok {
			return full, false, false
		}
		if candidateIndex < index && ps6080NodeMayMutateObjects(pass, candidate, subjects) {
			return full, false, true
		}
		values, candidateRelevant := ps6080CPUSingleSwitchClauseValues(
			pass, switchStmt, candidate, subjects, enum, constantEnums, domains,
		)
		if !candidateRelevant {
			return full, false, false
		}
		result = ps6080CPUUnionValues(result, values)
		relevant = true
	}
	return result, relevant, false
}

func ps6080CPUSingleSwitchClauseValues(
	pass *analysis.Pass,
	switchStmt *ast.SwitchStmt,
	clause *ast.CaseClause,
	subjects map[types.Object]bool,
	enum *types.TypeName,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
) (map[string]token.Pos, bool) {
	full := ps6080CPUFullDomain(domains[enum])
	if switchStmt.Tag != nil {
		switchSubject, unique := ps6080EnumSubject(pass, switchStmt.Tag, enum)
		if !unique || !subjects[switchSubject] {
			return full, false
		}
		if len(clause.List) > 0 {
			values := make(map[string]token.Pos, len(clause.List))
			for _, expression := range clause.List {
				value := pass.TypesInfo.Types[expression].Value
				if value == nil {
					return full, false
				}
				values[value.ExactString()] = expression.Pos()
			}
			return values, true
		}
		explicit := make(map[string]token.Pos)
		for _, statement := range switchStmt.Body.List {
			current, ok := statement.(*ast.CaseClause)
			if !ok {
				return full, false
			}
			for _, expression := range current.List {
				value := pass.TypesInfo.Types[expression].Value
				if value == nil {
					return full, false
				}
				explicit[value.ExactString()] = expression.Pos()
			}
		}
		return ps6080CPUDifferenceValues(full, explicit), true
	}
	if len(clause.List) > 0 {
		values := make(map[string]token.Pos)
		for _, expression := range clause.List {
			truth, _, expressionRelevant := ps6080CPUPredicateValues(
				pass, expression, subjects, enum, constantEnums, domains,
			)
			if !expressionRelevant {
				return full, false
			}
			values = ps6080CPUUnionValues(values, truth)
		}
		return values, true
	}
	values := full
	for _, statement := range switchStmt.Body.List {
		current, ok := statement.(*ast.CaseClause)
		if !ok {
			return full, false
		}
		for _, expression := range current.List {
			_, falsity, expressionRelevant := ps6080CPUPredicateValues(
				pass, expression, subjects, enum, constantEnums, domains,
			)
			if !expressionRelevant {
				return full, false
			}
			values = ps6080CPUIntersectValues(values, falsity)
		}
	}
	return values, true
}

func ps6080CPUGroupValues(
	group *ps6080ConstantGroup,
	domain []*types.Const,
) map[string]token.Pos {
	result := make(map[string]token.Pos)
	for _, constant := range domain {
		value := constant.Val().ExactString()
		supported := group.open
		for excluded := range group.excluded {
			if excluded.Val().ExactString() == value {
				supported = false
				break
			}
		}
		if !group.open {
			for included := range group.included {
				if included.Val().ExactString() == value {
					supported = true
					break
				}
			}
		}
		if supported {
			result[value] = constant.Pos()
		}
	}
	return result
}

func ps6080CPUFullDomain(domain []*types.Const) map[string]token.Pos {
	result := make(map[string]token.Pos, len(domain))
	for _, constant := range domain {
		result[constant.Val().ExactString()] = constant.Pos()
	}
	return result
}

func ps6080CPUUnionValues(left, right map[string]token.Pos) map[string]token.Pos {
	result := make(map[string]token.Pos, len(left)+len(right))
	for value, position := range left {
		result[value] = position
	}
	for value, position := range right {
		if prior, exists := result[value]; !exists || position < prior {
			result[value] = position
		}
	}
	return result
}

func ps6080CPUIntersectValues(left, right map[string]token.Pos) map[string]token.Pos {
	result := make(map[string]token.Pos, min(len(left), len(right)))
	for value, position := range left {
		if other, exists := right[value]; exists {
			result[value] = min(position, other)
		}
	}
	return result
}

func ps6080CPUDifferenceValues(left, right map[string]token.Pos) map[string]token.Pos {
	result := make(map[string]token.Pos, len(left))
	for value, position := range left {
		if _, removed := right[value]; !removed {
			result[value] = position
		}
	}
	return result
}

func ps6080CaseBodyContains(clause *ast.CaseClause, node ast.Node) bool {
	return len(clause.Body) > 0 && clause.Body[0].Pos() <= node.Pos() && node.End() <= clause.Body[len(clause.Body)-1].End()
}

func ps6080ContainingLiteral(node ast.Node, parents map[ast.Node]ast.Node) *ast.FuncLit {
	for current := node; current != nil; current = parents[current] {
		if literal, ok := current.(*ast.FuncLit); ok {
			return literal
		}
	}
	return nil
}

func ps6080CPUInvocationScope(
	pass *analysis.Pass,
	caller *ps6080Function,
	literal *ast.FuncLit,
	subject types.Object,
	enum *types.TypeName,
	parents map[ast.Node]ast.Node,
	invoked *ps6080InvokedLiteralResult,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
	visiting map[*ast.FuncLit]bool,
) *ps6080CPUCallScope {
	if visiting[literal] || invoked == nil || len(invoked.calls[literal]) == 0 {
		return &ps6080CPUCallScope{enum: enum, universal: true}
	}
	visiting[literal] = true
	defer delete(visiting, literal)

	parameter := ps6080LiteralParameterIndex(pass, literal, subject)
	var result *ps6080CPUCallScope
	for invocation := range invoked.calls[literal] {
		var scope *ps6080CPUCallScope
		if parameter >= 0 {
			if projected, ok := ps6080MayCallbackLiteralScope(
				pass, caller, literal, subject, enum, invocation, parents, invoked,
				constantEnums, domains,
			); ok {
				scope = projected
				result = ps6080MergeCPUArgumentScopes(result, scope)
				if result.universal {
					return result
				}
				continue
			}
			literalType := pass.TypesInfo.TypeOf(literal)
			invocationType := pass.TypesInfo.TypeOf(invocation.Fun)
			if parameter >= len(invocation.Args) || literalType == nil || invocationType == nil ||
				!types.Identical(types.Unalias(literalType), types.Unalias(invocationType)) {
				return &ps6080CPUCallScope{enum: enum, universal: true}
			}
			scope = ps6080CPUArgumentScope(
				pass, caller, invocation.Args[parameter], invocation, enum, parents, invoked,
				constantEnums, domains, visiting,
			)
		} else {
			if ps6080ObjectMayChangeBeforeNode(pass, literal.Body, subject, invocation) {
				return &ps6080CPUCallScope{enum: enum, universal: true}
			}
			if variable, ok := subject.(*types.Var); ok {
				initializer := ps6080StableLocalInitializer(pass, caller, variable, invocation, parents)
				if initializer != nil {
					if value := pass.TypesInfo.Types[initializer].Value; value != nil {
						scope = &ps6080CPUCallScope{
							enum:  enum,
							fixed: map[string]token.Pos{value.ExactString(): initializer.Pos()},
						}
					} else if resolved, unique := ps6080EnumSubject(pass, initializer, enum); unique {
						scope = ps6080CPUSubjectScopeAtNode(
							pass, caller, resolved, invocation, enum, parents, invoked,
							constantEnums, domains, visiting,
						)
					}
				}
			}
			if scope == nil {
				scope = ps6080CPUSubjectScopeAtNode(
					pass, caller, subject, invocation, enum, parents, invoked,
					constantEnums, domains, visiting,
				)
			}
		}
		result = ps6080MergeCPUArgumentScopes(result, scope)
		if result.universal {
			return result
		}
	}
	if result == nil {
		return &ps6080CPUCallScope{enum: enum, universal: true}
	}
	return result
}

func ps6080MayCallbackLiteralScope(
	pass *analysis.Pass,
	caller *ps6080Function,
	literal *ast.FuncLit,
	subject types.Object,
	enum *types.TypeName,
	invocation *ast.CallExpr,
	parents map[ast.Node]ast.Node,
	invoked *ps6080InvokedLiteralResult,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
) (*ps6080CPUCallScope, bool) {
	callee, _, direct := typedCallee(pass, invocation.Fun)
	if !direct {
		return nil, false
	}
	dispatcher, _ := callee.Type().(*types.Signature)
	target, _ := pass.TypesInfo.TypeOf(literal).(*types.Signature)
	if target == nil {
		return nil, false
	}
	mayCallbacks := ps6080MayNamedCallbackSites(pass)
	for index, argument := range invocation.Args {
		if !ps6080ExpressionResolvesLiteral(pass, caller, argument, invocation, parents, literal) ||
			index >= len(mayCallbacks[callee]) {
			continue
		}
		var result *ps6080CPUCallScope
		for _, site := range mayCallbacks[callee][index] {
			scopes := ps6080NamedCallbackSiteScopes(
				pass, caller, invocation, parents, invoked, site, dispatcher, target,
				nil, nil, false, constantEnums, domains,
			)
			result = ps6080MergeCPUArgumentScopes(result, scopes[subject])
		}
		if result == nil {
			result = &ps6080CPUCallScope{enum: enum, universal: true}
		}
		return result, true
	}
	return nil, false
}

func ps6080ExpressionResolvesLiteral(
	pass *analysis.Pass,
	caller *ps6080Function,
	expression ast.Expr,
	query ast.Node,
	parents map[ast.Node]ast.Node,
	literal *ast.FuncLit,
) bool {
	resolved, ok := ps6080StaticFunctionLiteral(pass, caller, expression, query, parents)
	return ok && resolved == literal
}

func ps6080StaticFunctionLiteral(
	pass *analysis.Pass,
	caller *ps6080Function,
	expression ast.Expr,
	query ast.Node,
	parents map[ast.Node]ast.Node,
) (*ast.FuncLit, bool) {
	seen := make(map[types.Object]bool)
	for {
		switch value := ps2110Unparen(expression).(type) {
		case *ast.FuncLit:
			return value, true
		case *ast.CallExpr:
			if len(value.Args) != 1 || !pass.TypesInfo.Types[value.Fun].IsType() {
				return nil, false
			}
			expression = value.Args[0]
		case *ast.Ident:
			variable, ok := pass.TypesInfo.ObjectOf(value).(*types.Var)
			if !ok || seen[variable] {
				return nil, false
			}
			seen[variable] = true
			expression = ps6080StableLocalInitializer(pass, caller, variable, query, parents)
			if expression == nil {
				return nil, false
			}
		default:
			return nil, false
		}
	}
}

func ps6080CallbackLiteralInvocations(
	pass *analysis.Pass,
	function *ps6080Function,
	parents map[ast.Node]ast.Node,
) map[*ast.FuncLit][]*ast.CallExpr {
	result := make(map[*ast.FuncLit][]*ast.CallExpr)
	ast.Inspect(function.body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || pass.TypesInfo.Types[call.Fun].IsType() {
			return true
		}
		literal, resolved := ps6080StaticFunctionLiteral(
			pass, function, call.Fun, call, parents,
		)
		if resolved {
			result[literal] = append(result[literal], call)
		}
		return true
	})
	return result
}

func ps6080CallbackLiteralReachable(
	pass *analysis.Pass,
	outer *cfg.CFG,
	literalGraphs map[*ast.FuncLit]*cfg.CFG,
	parents map[ast.Node]ast.Node,
	invocations map[*ast.FuncLit][]*ast.CallExpr,
	literal *ast.FuncLit,
	guaranteed bool,
	visiting map[*ast.FuncLit]bool,
) bool {
	if literal == nil || visiting[literal] {
		return false
	}
	visiting[literal] = true
	defer delete(visiting, literal)
	for _, invocation := range invocations[literal] {
		enclosing := ps6080ContainingLiteral(invocation, parents)
		graph := outer
		if enclosing != nil {
			graph = literalGraphs[enclosing]
			if graph == nil {
				graph = cfg.New(enclosing.Body, ps6080CallMayReturn(pass))
				literalGraphs[enclosing] = graph
			}
		}
		if !ps6080NodeReachable(pass, graph, parents, invocation) ||
			guaranteed && !ps6080CFGNodeGuaranteed(graph, invocation) {
			continue
		}
		if enclosing == nil || ps6080CallbackLiteralReachable(
			pass, outer, literalGraphs, parents, invocations, enclosing,
			guaranteed, visiting,
		) {
			return true
		}
	}
	return false
}

func ps6080LiteralParameterIndex(pass *analysis.Pass, literal *ast.FuncLit, subject types.Object) int {
	signature, _ := pass.TypesInfo.TypeOf(literal).(*types.Signature)
	if signature == nil {
		return -1
	}
	for index := range signature.Params().Len() {
		if signature.Params().At(index) == subject {
			return index
		}
	}
	return -1
}

func ps6080LiteralParameterExpressionIndex(
	pass *analysis.Pass,
	caller *ps6080Function,
	literal *ast.FuncLit,
	expression ast.Expr,
	query ast.Node,
	parents map[ast.Node]ast.Node,
) int {
	seen := make(map[types.Object]bool)
	for {
		switch value := ps2110Unparen(expression).(type) {
		case *ast.CallExpr:
			if len(value.Args) != 1 || !pass.TypesInfo.Types[value.Fun].IsType() {
				return -1
			}
			expression = value.Args[0]
		case *ast.Ident:
			object := pass.TypesInfo.ObjectOf(value)
			if parameter := ps6080LiteralParameterIndex(pass, literal, object); parameter >= 0 {
				return parameter
			}
			variable, ok := object.(*types.Var)
			if !ok || seen[variable] {
				return -1
			}
			seen[variable] = true
			expression = ps6080StableLocalInitializer(pass, caller, variable, query, parents)
			if expression == nil {
				return -1
			}
		default:
			return -1
		}
	}
}

func ps6080FunctionParameterExpressionIndex(
	pass *analysis.Pass,
	function *ps6080Function,
	expression ast.Expr,
	query ast.Node,
	parents map[ast.Node]ast.Node,
) int {
	seen := make(map[types.Object]bool)
	for {
		switch value := ps2110Unparen(expression).(type) {
		case *ast.CallExpr:
			if len(value.Args) != 1 || !pass.TypesInfo.Types[value.Fun].IsType() {
				return -1
			}
			expression = value.Args[0]
		case *ast.Ident:
			object := pass.TypesInfo.ObjectOf(value)
			if function.signature != nil {
				if function.signature.Recv() == object {
					return function.signature.Params().Len()
				}
				for index := range function.signature.Params().Len() {
					if function.signature.Params().At(index) == object {
						return index
					}
				}
			}
			variable, ok := object.(*types.Var)
			if !ok || seen[variable] {
				return -1
			}
			seen[variable] = true
			expression = ps6080StableLocalInitializer(pass, function, variable, query, parents)
			if expression == nil {
				return -1
			}
		default:
			return -1
		}
	}
}

func ps6080ObjectMayChangeBeforeNode(
	pass *analysis.Pass,
	region ast.Node,
	object types.Object,
	node ast.Node,
) bool {
	if region == nil || object == nil || node == nil {
		return true
	}
	objects := map[types.Object]bool{object: true}
	changed := false
	ast.Inspect(region, func(candidate ast.Node) bool {
		if candidate == nil || changed {
			return false
		}
		if candidate.Pos() >= node.Pos() {
			return false
		}
		switch value := candidate.(type) {
		case *ast.AssignStmt:
			changed = ps6080AssignmentTouchesObjects(pass, value, objects)
		case *ast.IncDecStmt:
			identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
			changed = direct && pass.TypesInfo.ObjectOf(identifier) == object
		case *ast.RangeStmt:
			changed = ps6080StatementsWriteObjects(pass, []ast.Stmt{value}, objects)
		case *ast.UnaryExpr:
			identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
			changed = value.Op == token.AND && direct && pass.TypesInfo.ObjectOf(identifier) == object
		case *ast.SelectorExpr:
			changed = ps6080PointerMethodOnObjects(pass, value, objects)
		}
		return !changed
	})
	return changed
}

func ps6080ObjectMayChangeBetween(
	pass *analysis.Pass,
	region ast.Node,
	object types.Object,
	start token.Pos,
	end token.Pos,
) bool {
	if region == nil || object == nil || start == token.NoPos || end == token.NoPos || end <= start {
		return true
	}
	objects := map[types.Object]bool{object: true}
	changed := false
	ast.Inspect(region, func(candidate ast.Node) bool {
		if candidate == nil || changed {
			return false
		}
		if candidate.End() <= start || candidate.Pos() >= end {
			return false
		}
		switch value := candidate.(type) {
		case *ast.AssignStmt:
			changed = ps6080AssignmentTouchesObjects(pass, value, objects)
		case *ast.IncDecStmt:
			identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
			changed = direct && pass.TypesInfo.ObjectOf(identifier) == object
		case *ast.RangeStmt:
			changed = ps6080StatementsWriteObjects(pass, []ast.Stmt{value}, objects)
		case *ast.UnaryExpr:
			identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
			changed = value.Op == token.AND && direct && pass.TypesInfo.ObjectOf(identifier) == object
		case *ast.SelectorExpr:
			changed = ps6080PointerMethodOnObjects(pass, value, objects)
		}
		return !changed
	})
	return changed
}

func ps6080StaticallyEmptyRange(pass *analysis.Pass, statement *ast.RangeStmt) bool {
	typeOf := pass.TypesInfo.TypeOf(statement.X)
	if typeOf == nil {
		return false
	}
	switch value := typeOf.Underlying().(type) {
	case *types.Array:
		return value.Len() == 0
	case *types.Pointer:
		array, ok := value.Elem().Underlying().(*types.Array)
		return ok && array.Len() == 0
	case *types.Basic:
		constantValue := pass.TypesInfo.Types[statement.X].Value
		if constantValue == nil {
			return false
		}
		if value.Info()&types.IsString != 0 {
			return constant.StringVal(constantValue) == ""
		}
		if value.Info()&types.IsInteger != 0 {
			return constant.Sign(constantValue) <= 0
		}
	}
	if call, ok := ps2110Unparen(statement.X).(*ast.CallExpr); ok {
		switch typeOf.Underlying().(type) {
		case *types.Slice, *types.Map, *types.Chan, *types.Signature:
			if len(call.Args) == 1 && pass.TypesInfo.Types[call.Fun].IsType() {
				if ps6080NilExpression(pass, call.Args[0]) {
					return true
				}
			}
			if identifier, direct := ps2110Unparen(call.Fun).(*ast.Ident); direct {
				builtin, builtinCall := pass.TypesInfo.ObjectOf(identifier).(*types.Builtin)
				if builtinCall && builtin.Name() == "make" {
					switch typeOf.Underlying().(type) {
					case *types.Map:
						return true
					case *types.Slice:
						if len(call.Args) >= 2 {
							length := pass.TypesInfo.Types[call.Args[1]].Value
							return length != nil && length.Kind() == constant.Int && constant.Sign(length) == 0
						}
					}
				}
			}
		}
	}
	literal, ok := ps2110Unparen(statement.X).(*ast.CompositeLit)
	if !ok || len(literal.Elts) != 0 {
		return false
	}
	switch typeOf.Underlying().(type) {
	case *types.Slice, *types.Map:
		return true
	}
	return false
}

func ps6080SwitchClauseReachable(
	pass *analysis.Pass,
	parents map[ast.Node]ast.Node,
	clause *ast.CaseClause,
) bool {
	switchBody, ok := parents[clause].(*ast.BlockStmt)
	if !ok {
		return true
	}
	switchStmt, ok := parents[switchBody].(*ast.SwitchStmt)
	if !ok {
		return true
	}
	tag := constant.MakeBool(true)
	if switchStmt.Tag != nil {
		tag = pass.TypesInfo.Types[switchStmt.Tag].Value
		if tag == nil {
			return true
		}
	}
	selected := -1
	fallback := -1
	for index, statement := range switchStmt.Body.List {
		candidate, valid := statement.(*ast.CaseClause)
		if !valid {
			return true
		}
		if len(candidate.List) == 0 {
			fallback = index
			continue
		}
		for _, expression := range candidate.List {
			value := pass.TypesInfo.Types[expression].Value
			if value == nil {
				return true
			}
			if constant.Compare(tag, token.EQL, value) {
				selected = index
				break
			}
		}
		if selected >= 0 {
			break
		}
	}
	if selected < 0 {
		selected = fallback
	}
	current := slices.Index(switchStmt.Body.List, ast.Stmt(clause))
	if selected < 0 || current < selected {
		return false
	}
	for index := selected; index < current; index++ {
		candidate := switchStmt.Body.List[index].(*ast.CaseClause)
		if len(candidate.Body) == 0 {
			return false
		}
		branch, ok := candidate.Body[len(candidate.Body)-1].(*ast.BranchStmt)
		if !ok || branch.Tok != token.FALLTHROUGH {
			return false
		}
	}
	return true
}

func ps6080StaticallyCompletingEmptyRange(
	pass *analysis.Pass,
	statement *ast.RangeStmt,
) bool {
	if !ps6080StaticallyEmptyRange(pass, statement) {
		return false
	}
	typeOf := pass.TypesInfo.TypeOf(statement.X)
	if typeOf == nil {
		return false
	}
	switch typeOf.Underlying().(type) {
	case *types.Chan, *types.Signature:
		return false
	default:
		return true
	}
}

func ps6080ExactInterfaceDynamicType(
	pass *analysis.Pass,
	expression ast.Expr,
) (dynamic types.Type, nilValue bool, known bool) {
	expression = ps2110Unparen(expression)
	if ps6080NilExpression(pass, expression) {
		return nil, true, true
	}
	conversion, ok := expression.(*ast.CallExpr)
	if !ok || len(conversion.Args) != 1 || !pass.TypesInfo.Types[conversion.Fun].IsType() {
		return nil, false, false
	}
	target := pass.TypesInfo.TypeOf(conversion.Fun)
	if target == nil {
		return nil, false, false
	}
	if _, interfaceType := types.Unalias(target).Underlying().(*types.Interface); !interfaceType {
		return nil, false, false
	}
	argument := ps2110Unparen(conversion.Args[0])
	if ps6080NilExpression(pass, argument) {
		return nil, true, true
	}
	dynamic = pass.TypesInfo.TypeOf(argument)
	if dynamic == nil {
		return nil, false, false
	}
	if basic, ok := dynamic.(*types.Basic); ok && basic.Info()&types.IsUntyped != 0 {
		dynamic = types.Default(dynamic)
	}
	if ps6080TypeHasFreeParameter(dynamic, nil) {
		return nil, false, false
	}
	if _, interfaceType := types.Unalias(dynamic).Underlying().(*types.Interface); interfaceType {
		return ps6080ExactInterfaceDynamicType(pass, argument)
	}
	return dynamic, false, true
}

func ps6080TypeHasFreeParameter(t types.Type, seen map[types.Type]bool) bool {
	if t == nil {
		return false
	}
	if seen[t] {
		return false
	}
	if seen == nil {
		seen = make(map[types.Type]bool)
	}
	seen[t] = true
	switch value := t.(type) {
	case *types.TypeParam:
		return true
	case *types.Named:
		arguments := value.TypeArgs()
		if parameters := value.TypeParams(); parameters != nil && parameters.Len() > 0 &&
			(arguments == nil || arguments.Len() == 0) {
			return true
		}
		if arguments != nil {
			for index := range arguments.Len() {
				if ps6080TypeHasFreeParameter(arguments.At(index), seen) {
					return true
				}
			}
		}
		return ps6080TypeHasFreeParameter(value.Underlying(), seen)
	case *types.Alias:
		return ps6080TypeHasFreeParameter(types.Unalias(value), seen)
	case *types.Pointer:
		return ps6080TypeHasFreeParameter(value.Elem(), seen)
	case *types.Slice:
		return ps6080TypeHasFreeParameter(value.Elem(), seen)
	case *types.Array:
		return ps6080TypeHasFreeParameter(value.Elem(), seen)
	case *types.Map:
		return ps6080TypeHasFreeParameter(value.Key(), seen) ||
			ps6080TypeHasFreeParameter(value.Elem(), seen)
	case *types.Chan:
		return ps6080TypeHasFreeParameter(value.Elem(), seen)
	case *types.Struct:
		for index := range value.NumFields() {
			if ps6080TypeHasFreeParameter(value.Field(index).Type(), seen) {
				return true
			}
		}
	case *types.Tuple:
		for index := range value.Len() {
			if ps6080TypeHasFreeParameter(value.At(index).Type(), seen) {
				return true
			}
		}
	case *types.Signature:
		if parameters := value.TypeParams(); parameters != nil && parameters.Len() > 0 {
			return true
		}
		if receiver := value.Recv(); receiver != nil &&
			ps6080TypeHasFreeParameter(receiver.Type(), seen) {
			return true
		}
		return ps6080TypeHasFreeParameter(value.Params(), seen) ||
			ps6080TypeHasFreeParameter(value.Results(), seen)
	case *types.Interface:
		for index := range value.NumExplicitMethods() {
			if ps6080TypeHasFreeParameter(value.ExplicitMethod(index).Type(), seen) {
				return true
			}
		}
		for index := range value.NumEmbeddeds() {
			if ps6080TypeHasFreeParameter(value.EmbeddedType(index), seen) {
				return true
			}
		}
	case *types.Union:
		for index := range value.Len() {
			if ps6080TypeHasFreeParameter(value.Term(index).Type(), seen) {
				return true
			}
		}
	}
	return false
}

func ps6080TypeSwitchClauseReachable(
	pass *analysis.Pass,
	parents map[ast.Node]ast.Node,
	clause *ast.CaseClause,
) bool {
	switchBody, ok := parents[clause].(*ast.BlockStmt)
	if !ok {
		return true
	}
	switchStmt, ok := parents[switchBody].(*ast.TypeSwitchStmt)
	if !ok {
		return true
	}
	var assertion *ast.TypeAssertExpr
	switch value := switchStmt.Assign.(type) {
	case *ast.ExprStmt:
		assertion, _ = ps2110Unparen(value.X).(*ast.TypeAssertExpr)
	case *ast.AssignStmt:
		if len(value.Rhs) == 1 {
			assertion, _ = ps2110Unparen(value.Rhs[0]).(*ast.TypeAssertExpr)
		}
	}
	if assertion == nil {
		return true
	}
	dynamic, nilValue, known := ps6080ExactInterfaceDynamicType(pass, assertion.X)
	if !known {
		return true
	}
	selected := -1
	fallback := -1
	for index, statement := range switchStmt.Body.List {
		candidate, valid := statement.(*ast.CaseClause)
		if !valid {
			return true
		}
		if len(candidate.List) == 0 {
			fallback = index
			continue
		}
		for _, expression := range candidate.List {
			if ps6080NilExpression(pass, expression) {
				if nilValue {
					selected = index
				}
			} else if !nilValue {
				caseType := pass.TypesInfo.TypeOf(expression)
				if caseType == nil {
					return true
				}
				if ps6080TypeHasFreeParameter(caseType, nil) {
					return true
				}
				if _, interfaceType := types.Unalias(caseType).Underlying().(*types.Interface); interfaceType {
					if types.AssignableTo(dynamic, caseType) {
						selected = index
					}
				} else if types.Identical(types.Unalias(dynamic), types.Unalias(caseType)) {
					selected = index
				}
			}
			if selected >= 0 {
				break
			}
		}
		if selected >= 0 {
			break
		}
	}
	if selected < 0 {
		selected = fallback
	}
	return slices.Index(switchStmt.Body.List, ast.Stmt(clause)) == selected
}

func ps6080TypeSwitchSelectionKnown(
	pass *analysis.Pass,
	parents map[ast.Node]ast.Node,
	clause *ast.CaseClause,
) bool {
	switchBody, ok := parents[clause].(*ast.BlockStmt)
	if !ok {
		return false
	}
	switchStmt, ok := parents[switchBody].(*ast.TypeSwitchStmt)
	if !ok {
		return false
	}
	var assertion *ast.TypeAssertExpr
	switch value := switchStmt.Assign.(type) {
	case *ast.ExprStmt:
		assertion, _ = ps2110Unparen(value.X).(*ast.TypeAssertExpr)
	case *ast.AssignStmt:
		if len(value.Rhs) == 1 {
			assertion, _ = ps2110Unparen(value.Rhs[0]).(*ast.TypeAssertExpr)
		}
	}
	if assertion == nil {
		return false
	}
	_, _, known := ps6080ExactInterfaceDynamicType(pass, assertion.X)
	return known
}

func ps6080DefinitelyNilChannel(pass *analysis.Pass, expression ast.Expr) bool {
	conversion, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || len(conversion.Args) != 1 || !pass.TypesInfo.Types[conversion.Fun].IsType() {
		return false
	}
	target := pass.TypesInfo.TypeOf(conversion.Fun)
	if target == nil {
		return false
	}
	if _, channel := types.Unalias(target).Underlying().(*types.Chan); !channel {
		return false
	}
	return ps6080NilExpression(pass, conversion.Args[0])
}

func ps6080CommClauseDefinitelyDisabled(pass *analysis.Pass, clause *ast.CommClause) bool {
	var channel ast.Expr
	switch communication := clause.Comm.(type) {
	case *ast.SendStmt:
		channel = communication.Chan
	case *ast.ExprStmt:
		receive, ok := ps2110Unparen(communication.X).(*ast.UnaryExpr)
		if ok && receive.Op == token.ARROW {
			channel = receive.X
		}
	case *ast.AssignStmt:
		if len(communication.Rhs) == 1 {
			receive, ok := ps2110Unparen(communication.Rhs[0]).(*ast.UnaryExpr)
			if ok && receive.Op == token.ARROW {
				channel = receive.X
			}
		}
	}
	return channel != nil && ps6080DefinitelyNilChannel(pass, channel)
}

func ps6080CommClauseSelectionRequired(
	parents map[ast.Node]ast.Node,
	clause *ast.CommClause,
	node ast.Node,
) bool {
	for _, statement := range clause.Body {
		for current := node; current != nil && current != clause; current = parents[current] {
			if current == statement {
				return true
			}
		}
	}
	assignment, ok := clause.Comm.(*ast.AssignStmt)
	if !ok {
		return false
	}
	if node == assignment {
		return true
	}
	for _, left := range assignment.Lhs {
		for current := node; current != nil && current != assignment; current = parents[current] {
			if current == left {
				return true
			}
		}
	}
	return false
}

func ps6080StaticallyReachable(
	pass *analysis.Pass,
	parents map[ast.Node]ast.Node,
	node ast.Node,
) bool {
	for current := node; current != nil; current = parents[current] {
		if clause, ok := current.(*ast.CaseClause); ok {
			if !ps6080SwitchClauseReachable(pass, parents, clause) ||
				!ps6080TypeSwitchClauseReachable(pass, parents, clause) {
				return false
			}
		}
		if clause, ok := current.(*ast.CommClause); ok &&
			ps6080CommClauseSelectionRequired(parents, clause, node) &&
			ps6080CommClauseDefinitelyDisabled(pass, clause) {
			return false
		}
		if block, ok := current.(*ast.BlockStmt); ok {
			if statement, ranged := parents[block].(*ast.RangeStmt); ranged &&
				statement.Body == block && ps6080StaticallyEmptyRange(pass, statement) {
				return false
			}
		}
	}
	return true
}

func ps6080FeasibleSuccessors(
	pass *analysis.Pass,
	parents map[ast.Node]ast.Node,
	block *cfg.Block,
) []*cfg.Block {
	successors := block.Succs
	if len(successors) == 2 && successors[0].Kind == cfg.KindSwitchCaseBody {
		clause, _ := successors[0].Stmt.(*ast.CaseClause)
		if clause != nil && ps6080TypeSwitchSelectionKnown(pass, parents, clause) {
			if ps6080TypeSwitchClauseReachable(pass, parents, clause) {
				return successors[:1]
			}
			return successors[1:2]
		}
	}
	if len(successors) == 2 && len(block.Nodes) > 0 {
		if condition, ok := block.Nodes[len(block.Nodes)-1].(ast.Expr); ok {
			value := pass.TypesInfo.Types[condition].Value
			if value != nil && value.Kind() == constant.Bool {
				index := 1
				if constant.BoolVal(value) {
					index = 0
				}
				return successors[index : index+1]
			}
			clause, inSwitch := parents[condition].(*ast.CaseClause)
			if inSwitch && value != nil {
				switchBody, _ := parents[clause].(*ast.BlockStmt)
				switchStmt, _ := parents[switchBody].(*ast.SwitchStmt)
				if switchStmt != nil && switchStmt.Tag != nil {
					tag := pass.TypesInfo.Types[switchStmt.Tag].Value
					if tag != nil {
						index := 1
						if constant.Compare(tag, token.EQL, value) {
							index = 0
						}
						return successors[index : index+1]
					}
				}
			}
		}
	}
	if len(successors) == 2 && block.Kind == cfg.KindRangeLoop {
		if statement, ok := block.Stmt.(*ast.RangeStmt); ok &&
			ps6080StaticallyCompletingEmptyRange(pass, statement) {
			return successors[1:2]
		}
	}
	feasible := make([]*cfg.Block, 0, len(successors))
	for _, successor := range successors {
		switch successor.Kind {
		case cfg.KindSelectCaseBody:
			clause, _ := successor.Stmt.(*ast.CommClause)
			if clause != nil && ps6080CommClauseDefinitelyDisabled(pass, clause) {
				continue
			}
		case cfg.KindSwitchCaseBody:
			clause, _ := successor.Stmt.(*ast.CaseClause)
			if clause != nil && (!ps6080SwitchClauseReachable(pass, parents, clause) ||
				!ps6080TypeSwitchClauseReachable(pass, parents, clause)) {
				continue
			}
		}
		feasible = append(feasible, successor)
	}
	return feasible
}

type ps6080InvokedLiteralResult struct {
	literals      map[*ast.FuncLit]bool
	calls         map[*ast.FuncLit]map[*ast.CallExpr]bool
	orders        map[*ast.FuncLit]map[*ast.CallExpr][][]token.Pos
	arguments     map[*ast.FuncLit]map[*ast.CallExpr]ps6080InvocationArguments
	safeCalls     map[*ast.CallExpr]bool
	safeArguments map[*ast.CallExpr]ps6080IndexSet
	orderedArgs   map[*ast.FuncLit]map[*ast.CallExpr][]ps6080OrderedInvocation
}

type ps6080OrderedInvocation struct {
	order     []token.Pos
	arguments ps6080InvocationArguments
}

func ps6080EmptyInvokedLiteralResult() *ps6080InvokedLiteralResult {
	return &ps6080InvokedLiteralResult{
		literals:      make(map[*ast.FuncLit]bool),
		calls:         make(map[*ast.FuncLit]map[*ast.CallExpr]bool),
		orders:        make(map[*ast.FuncLit]map[*ast.CallExpr][][]token.Pos),
		arguments:     make(map[*ast.FuncLit]map[*ast.CallExpr]ps6080InvocationArguments),
		safeCalls:     make(map[*ast.CallExpr]bool),
		safeArguments: make(map[*ast.CallExpr]ps6080IndexSet),
		orderedArgs:   make(map[*ast.FuncLit]map[*ast.CallExpr][]ps6080OrderedInvocation),
	}
}

func ps6080AddOrderedInvocation(
	result *ps6080InvokedLiteralResult,
	literal *ast.FuncLit,
	call *ast.CallExpr,
	order []token.Pos,
	arguments ps6080InvocationArguments,
) {
	ps6080AddInvocationOrder(result, literal, call, order)
	if result.orderedArgs[literal] == nil {
		result.orderedArgs[literal] = make(map[*ast.CallExpr][]ps6080OrderedInvocation)
	}
	for _, existing := range result.orderedArgs[literal][call] {
		if slices.Equal(existing.order, order) && ps6080InvocationArgumentsEqual(existing.arguments, arguments) {
			return
		}
	}
	result.orderedArgs[literal][call] = append(
		result.orderedArgs[literal][call],
		ps6080OrderedInvocation{order: slices.Clone(order), arguments: arguments},
	)
}

func ps6080InvocationArgumentsEqual(left, right ps6080InvocationArguments) bool {
	if len(left) != len(right) {
		return false
	}
	for argument, leftParameters := range left {
		if !slices.Equal(leftParameters, right[argument]) {
			return false
		}
	}
	return true
}

func ps6080AddInvocationOrder(
	result *ps6080InvokedLiteralResult,
	literal *ast.FuncLit,
	call *ast.CallExpr,
	order []token.Pos,
) {
	if result.orders[literal] == nil {
		result.orders[literal] = make(map[*ast.CallExpr][][]token.Pos)
	}
	for _, existing := range result.orders[literal][call] {
		if slices.Equal(existing, order) {
			return
		}
	}
	result.orders[literal][call] = append(
		result.orders[literal][call], slices.Clone(order),
	)
}

func ps6080InvokedFunctionLiteralResult(
	pass *analysis.Pass,
	body *ast.BlockStmt,
) *ps6080InvokedLiteralResult {
	value, cached := ps6080InvokedLiteralCaches.Load(pass)
	if !cached {
		return ps6080ComputeInvokedFunctionLiterals(pass, body)
	}
	cache := value.(*sync.Map)
	if value, cached = cache.Load(body); cached {
		return value.(*ps6080InvokedLiteralResult)
	}
	computed := ps6080ComputeInvokedFunctionLiterals(pass, body)
	value, _ = cache.LoadOrStore(body, computed)
	return value.(*ps6080InvokedLiteralResult)
}

func ps6080InvokedFunctionLiterals(pass *analysis.Pass, body *ast.BlockStmt) map[*ast.FuncLit]bool {
	return ps6080InvokedFunctionLiteralResult(pass, body).literals
}

func ps6080NamedCallbackParameters(pass *analysis.Pass) map[*types.Func]ps6080CallbackMappings {
	if cached, ok := ps6080NamedCallbackCaches.Load(pass); ok {
		return cached.(map[*types.Func]ps6080CallbackMappings)
	}
	type callbackCall struct {
		caller     *types.Func
		callee     *types.Func
		arguments  []ast.Expr
		parameters map[types.Object]int
		function   *ps6080Function
		parents    map[ast.Node]ast.Node
		call       *ast.CallExpr
	}
	result := make(map[*types.Func]ps6080CallbackMappings)
	var calls []callbackCall
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if !ok {
				continue
			}
			signature, _ := object.Type().(*types.Signature)
			parameters := make(map[types.Object]int)
			if signature != nil {
				for index := range signature.Params().Len() {
					parameter := signature.Params().At(index)
					parameters[parameter] = index
				}
			}
			if len(parameters) == 0 {
				continue
			}
			parents := ps6071Parents(function.Body)
			graph := cfg.New(function.Body, ps6080CallMayReturn(pass))
			literalGraphs := make(map[*ast.FuncLit]*cfg.CFG)
			info := &ps6080Function{
				declaration: function, object: object, signature: signature, body: function.Body,
			}
			literalInvocations := ps6080CallbackLiteralInvocations(pass, info, parents)
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if literal, nested := node.(*ast.FuncLit); nested {
					return len(literalInvocations[literal]) > 0
				}
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				callGraph := graph
				if literal := ps6080ContainingLiteral(call, parents); literal != nil {
					callGraph = literalGraphs[literal]
					if callGraph == nil {
						callGraph = cfg.New(literal.Body, ps6080CallMayReturn(pass))
						literalGraphs[literal] = callGraph
					}
					if !ps6080CallbackLiteralReachable(
						pass, graph, literalGraphs, parents, literalInvocations, literal,
						true, make(map[*ast.FuncLit]bool),
					) {
						return true
					}
				}
				if !ps6080NodeReachable(pass, callGraph, parents, call) ||
					!ps6080CFGNodeGuaranteed(callGraph, call) {
					return true
				}
				switch parent := parents[call].(type) {
				case *ast.GoStmt:
					if parent.Call == call {
						return true
					}
				case *ast.DeferStmt:
					if parent.Call == call {
						return true
					}
				}
				if index, parameter := ps6080CallbackParameterIndex(
					pass, info, call.Fun, call, parents, parameters,
				); parameter && ps6080CallableType(signature.Params().At(index).Type()) {
					mapping := result[object]
					mapping = ps6080GrowIndexSlice(mapping, index)
					if mapping[index] == nil {
						mapping[index] = ps6080InvocationArguments{}
					}
					for argument, expression := range call.Args {
						if source, forwarded := ps6080CallbackParameterIndex(
							pass, info, expression, call, parents, parameters,
						); forwarded {
							ps6080AddInvocationArgument(&mapping[index], argument, source)
						}
					}
					result[object] = mapping
					return true
				}
				callee, _, direct := typedCallee(pass, call.Fun)
				if direct {
					calls = append(calls, callbackCall{
						caller: object, callee: callee, arguments: call.Args, parameters: parameters,
						function: info, parents: parents, call: call,
					})
				}
				return true
			})
		}
	}
	changed := true
	for changed {
		changed = false
		for _, call := range calls {
			for index, calleeMapping := range result[call.callee] {
				if calleeMapping == nil || index >= len(call.arguments) {
					continue
				}
				parameter, forwarded := ps6080CallbackParameterIndex(
					pass, call.function, call.arguments[index], call.call,
					call.parents, call.parameters,
				)
				if !forwarded || !ps6080CallableType(call.function.signature.Params().At(parameter).Type()) {
					continue
				}
				callerMapping := result[call.caller]
				if parameter >= len(callerMapping) || callerMapping[parameter] == nil {
					callerMapping = ps6080GrowIndexSlice(callerMapping, parameter)
					callerMapping[parameter] = ps6080InvocationArguments{}
					changed = true
				}
				for argument, calleeParameters := range calleeMapping {
					for calleeParameter, present := range calleeParameters {
						if !present {
							continue
						}
						if calleeParameter >= len(call.arguments) {
							continue
						}
						source, mapped := ps6080CallbackParameterIndex(
							pass, call.function, call.arguments[calleeParameter], call.call,
							call.parents, call.parameters,
						)
						if !mapped {
							continue
						}
						mapping := &callerMapping[parameter]
						alreadyMapped := argument < len(*mapping) &&
							ps6080HasIndex((*mapping)[argument], source)
						if !alreadyMapped {
							ps6080AddInvocationArgument(mapping, argument, source)
							changed = true
						}
					}
				}
				result[call.caller] = callerMapping
			}
		}
	}
	value, _ := ps6080NamedCallbackCaches.LoadOrStore(pass, result)
	return value.(map[*types.Func]ps6080CallbackMappings)
}

type ps6080MayCallbackSite struct {
	call       *ast.CallExpr
	function   *ps6080Function
	parents    map[ast.Node]ast.Node
	invoked    *ps6080InvokedLiteralResult
	forwarding []ps6080MayCallbackForward
	order      []token.Pos
}

type ps6080MayCallbackForward struct {
	call       *ast.CallExpr
	function   *ps6080Function
	parents    map[ast.Node]ast.Node
	invoked    *ps6080InvokedLiteralResult
	dispatcher *types.Signature
}

func ps6080CallbackParameterIndex(
	pass *analysis.Pass,
	function *ps6080Function,
	expression ast.Expr,
	query ast.Node,
	parents map[ast.Node]ast.Node,
	parameters map[types.Object]int,
) (int, bool) {
	seen := make(map[types.Object]bool)
	for {
		switch value := ps2110Unparen(expression).(type) {
		case *ast.Ident:
			object := pass.TypesInfo.ObjectOf(value)
			if index, parameter := parameters[object]; parameter {
				return index, true
			}
			variable, ok := object.(*types.Var)
			if !ok || seen[variable] {
				return 0, false
			}
			seen[variable] = true
			expression = ps6080StableLocalInitializer(pass, function, variable, query, parents)
			if expression == nil {
				return 0, false
			}
		case *ast.CallExpr:
			if len(value.Args) != 1 || !pass.TypesInfo.Types[value.Fun].IsType() {
				return 0, false
			}
			expression = value.Args[0]
		default:
			return 0, false
		}
	}
}

func ps6080MayNamedCallbackSites(pass *analysis.Pass) map[*types.Func][][]*ps6080MayCallbackSite {
	if cached, ok := ps6080MayCallbackCaches.Load(pass); ok {
		return cached.(map[*types.Func][][]*ps6080MayCallbackSite)
	}
	type forwardingCall struct {
		caller     *types.Func
		callee     *types.Func
		call       *ast.CallExpr
		function   *ps6080Function
		parents    map[ast.Node]ast.Node
		parameters map[types.Object]int
	}
	result := make(map[*types.Func][][]*ps6080MayCallbackSite)
	var forwarding []forwardingCall
	functionCount := 0
	appendSite := func(function *types.Func, parameter int, site *ps6080MayCallbackSite) bool {
		sites := ps6080GrowIndexSlice(result[function], parameter)
		for _, existing := range sites[parameter] {
			if slices.Equal(existing.order, site.order) {
				return false
			}
		}
		sites[parameter] = append(sites[parameter], site)
		result[function] = sites
		return true
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if !ok {
				continue
			}
			functionCount++
			signature, _ := object.Type().(*types.Signature)
			if signature == nil || signature.Params().Len() == 0 {
				continue
			}
			parameters := make(map[types.Object]int)
			for index := range signature.Params().Len() {
				parameter := signature.Params().At(index)
				if ps6080CallableType(parameter.Type()) {
					parameters[parameter] = index
				}
			}
			if len(parameters) == 0 {
				continue
			}
			parents := ps6071Parents(function.Body)
			graph := cfg.New(function.Body, ps6080CallMayReturn(pass))
			literalGraphs := make(map[*ast.FuncLit]*cfg.CFG)
			info := &ps6080Function{
				declaration: function, object: object, signature: signature, body: function.Body,
			}
			literalInvocations := ps6080CallbackLiteralInvocations(pass, info, parents)
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if literal, nested := node.(*ast.FuncLit); nested {
					return len(literalInvocations[literal]) > 0
				}
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				callGraph := graph
				if literal := ps6080ContainingLiteral(call, parents); literal != nil {
					callGraph = literalGraphs[literal]
					if callGraph == nil {
						callGraph = cfg.New(literal.Body, ps6080CallMayReturn(pass))
						literalGraphs[literal] = callGraph
					}
					if !ps6080CallbackLiteralReachable(
						pass, graph, literalGraphs, parents, literalInvocations, literal,
						false, make(map[*ast.FuncLit]bool),
					) {
						return true
					}
				}
				if !ps6080NodeReachable(pass, callGraph, parents, call) {
					return true
				}
				if index, callback := ps6080CallbackParameterIndex(
					pass, info, call.Fun, call, parents, parameters,
				); callback {
					appendSite(object, index, &ps6080MayCallbackSite{
						call: call, function: info, parents: parents, order: []token.Pos{call.Pos()},
					})
					return true
				}
				if callee, _, direct := typedCallee(pass, call.Fun); direct {
					forwarding = append(forwarding, forwardingCall{
						caller: object, callee: callee, call: call, function: info,
						parents: parents, parameters: parameters,
					})
				}
				return true
			})
		}
	}
	changed := true
	for changed {
		changed = false
		for _, forward := range forwarding {
			dispatcher, _ := forward.callee.Type().(*types.Signature)
			for calleeParameter, sites := range result[forward.callee] {
				if calleeParameter >= len(forward.call.Args) {
					continue
				}
				callerParameter, forwarded := ps6080CallbackParameterIndex(
					pass, forward.function, forward.call.Args[calleeParameter], forward.call,
					forward.parents, forward.parameters,
				)
				if !forwarded {
					continue
				}
				for _, site := range sites {
					if len(site.order) >= functionCount {
						continue
					}
					copy := *site
					copy.forwarding = slices.Clone(site.forwarding)
					copy.forwarding = append(copy.forwarding, ps6080MayCallbackForward{
						call: forward.call, function: forward.function,
						parents: forward.parents, dispatcher: dispatcher,
					})
					copy.order = append(slices.Clone(site.order), forward.call.Pos())
					changed = appendSite(forward.caller, callerParameter, &copy) || changed
				}
			}
		}
	}
	value, _ := ps6080MayCallbackCaches.LoadOrStore(pass, result)
	return value.(map[*types.Func][][]*ps6080MayCallbackSite)
}

type ps6080NamedCallbackInvocation struct {
	order     []token.Pos
	arguments ps6080InvocationArguments
}

func ps6080NamedCallbackInvocations(pass *analysis.Pass) map[*types.Func][][]ps6080NamedCallbackInvocation {
	if cached, ok := ps6080NamedOrderCaches.Load(pass); ok {
		return cached.(map[*types.Func][][]ps6080NamedCallbackInvocation)
	}
	type forwardingCall struct {
		caller     *types.Func
		callee     *types.Func
		position   token.Pos
		arguments  []ast.Expr
		parameters map[types.Object]int
	}
	result := make(map[*types.Func][][]ps6080NamedCallbackInvocation)
	var forwarding []forwardingCall
	functionCount := 0
	appendInvocation := func(
		function *types.Func,
		parameter int,
		invocation ps6080NamedCallbackInvocation,
	) bool {
		invocations := ps6080GrowIndexSlice(result[function], parameter)
		for _, existing := range invocations[parameter] {
			if slices.Equal(existing.order, invocation.order) &&
				ps6080InvocationArgumentsEqual(existing.arguments, invocation.arguments) {
				return false
			}
		}
		invocations[parameter] = append(
			invocations[parameter], ps6080NamedCallbackInvocation{
				order: slices.Clone(invocation.order), arguments: invocation.arguments,
			},
		)
		result[function] = invocations
		return true
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if !ok {
				continue
			}
			functionCount++
			signature, _ := object.Type().(*types.Signature)
			parameters := make(map[types.Object]int)
			if signature != nil {
				for index := range signature.Params().Len() {
					parameters[signature.Params().At(index)] = index
				}
			}
			if len(parameters) == 0 {
				continue
			}
			parents := ps6071Parents(function.Body)
			graph := cfg.New(function.Body, ps6080CallMayReturn(pass))
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				call, ok := node.(*ast.CallExpr)
				if !ok || !ps6080NodeReachable(pass, graph, parents, call) ||
					!ps6080CFGNodeGuaranteed(graph, call) {
					return true
				}
				switch parent := parents[call].(type) {
				case *ast.GoStmt:
					if parent.Call == call {
						return true
					}
				case *ast.DeferStmt:
					if parent.Call == call {
						return true
					}
				}
				if identifier, direct := ps2110Unparen(call.Fun).(*ast.Ident); direct {
					parameterObject := pass.TypesInfo.ObjectOf(identifier)
					if index, parameter := parameters[parameterObject]; parameter &&
						ps6080CallableType(parameterObject.Type()) {
						var arguments ps6080InvocationArguments
						for argument, expression := range call.Args {
							identifier, direct := ps2110Unparen(expression).(*ast.Ident)
							if !direct {
								continue
							}
							if source, forwarded := parameters[pass.TypesInfo.ObjectOf(identifier)]; forwarded {
								ps6080AddInvocationArgument(&arguments, argument, source)
							}
						}
						appendInvocation(object, index, ps6080NamedCallbackInvocation{
							order: []token.Pos{call.Pos()}, arguments: arguments,
						})
						return true
					}
				}
				if callee, _, direct := typedCallee(pass, call.Fun); direct {
					forwarding = append(forwarding, forwardingCall{
						caller: object, callee: callee, position: call.Pos(),
						arguments: call.Args, parameters: parameters,
					})
				}
				return true
			})
		}
	}
	changed := true
	for changed {
		changed = false
		for _, call := range forwarding {
			for parameter, invocations := range result[call.callee] {
				if parameter >= len(call.arguments) {
					continue
				}
				identifier, direct := ps2110Unparen(call.arguments[parameter]).(*ast.Ident)
				if !direct {
					continue
				}
				source, forwarded := call.parameters[pass.TypesInfo.ObjectOf(identifier)]
				if !forwarded {
					continue
				}
				for _, invocation := range invocations {
					if len(invocation.order) > functionCount {
						continue
					}
					var arguments ps6080InvocationArguments
					for argument, calleeParameters := range invocation.arguments {
						for calleeParameter, present := range calleeParameters {
							if !present {
								continue
							}
							if calleeParameter >= len(call.arguments) {
								continue
							}
							identifier, direct := ps2110Unparen(call.arguments[calleeParameter]).(*ast.Ident)
							if !direct {
								continue
							}
							callerParameter, mapped := call.parameters[pass.TypesInfo.ObjectOf(identifier)]
							if !mapped {
								continue
							}
							ps6080AddInvocationArgument(&arguments, argument, callerParameter)
						}
					}
					forwardedOrder := append([]token.Pos{call.position}, invocation.order...)
					changed = appendInvocation(call.caller, source, ps6080NamedCallbackInvocation{
						order: forwardedOrder, arguments: arguments,
					}) || changed
				}
			}
		}
	}
	value, _ := ps6080NamedOrderCaches.LoadOrStore(pass, result)
	return value.(map[*types.Func][][]ps6080NamedCallbackInvocation)
}

func ps6080NamedCallbackSafeMapParameters(pass *analysis.Pass) map[*types.Func]ps6080IndexSet {
	if cached, ok := ps6080NamedSafeMapCaches.Load(pass); ok {
		return cached.(map[*types.Func]ps6080IndexSet)
	}
	type functionInfo struct {
		object      *types.Func
		declaration *ast.FuncDecl
		parameters  map[types.Object]int
		parents     map[ast.Node]ast.Node
	}
	summaries := ps6080NamedCallbackParameters(pass)
	var functions []functionInfo
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			object, _ := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if len(summaries[object]) == 0 {
				continue
			}
			signature, _ := object.Type().(*types.Signature)
			parameters := make(map[types.Object]int)
			if signature != nil {
				for index := range signature.Params().Len() {
					parameters[signature.Params().At(index)] = index
				}
			}
			functions = append(functions, functionInfo{
				object: object, declaration: function, parameters: parameters,
				parents: ps6071Parents(function.Body),
			})
		}
	}
	result := make(map[*types.Func]ps6080IndexSet)
	changed := true
	for changed {
		changed = false
		for _, function := range functions {
			var candidates ps6080IndexSet
			for _, mapping := range summaries[function.object] {
				for _, parameters := range mapping {
					for parameter, present := range parameters {
						if present {
							ps6080AddIndex(&candidates, parameter)
						}
					}
				}
			}
			signature, _ := function.object.Type().(*types.Signature)
			for parameter, candidate := range candidates {
				if !candidate {
					continue
				}
				if signature == nil || parameter >= signature.Params().Len() ||
					ps6080HasIndex(result[function.object], parameter) {
					continue
				}
				parameterObject := signature.Params().At(parameter)
				if _, mapping := types.Unalias(parameterObject.Type()).Underlying().(*types.Map); !mapping {
					continue
				}
				safe := true
				ast.Inspect(function.declaration.Body, func(node ast.Node) bool {
					if !safe {
						return false
					}
					identifier, ok := node.(*ast.Ident)
					if !ok || pass.TypesInfo.ObjectOf(identifier) != parameterObject {
						return true
					}
					var expression ast.Expr = identifier
					for {
						parent, wrapped := function.parents[expression].(*ast.ParenExpr)
						if !wrapped {
							break
						}
						expression = parent
					}
					call, argument := function.parents[expression].(*ast.CallExpr)
					if !argument {
						safe = false
						return false
					}
					argumentIndex := slices.Index(call.Args, expression)
					if argumentIndex < 0 {
						safe = false
						return false
					}
					allowed := false
					if callback, direct := ps2110Unparen(call.Fun).(*ast.Ident); direct {
						if callbackIndex, found := function.parameters[pass.TypesInfo.ObjectOf(callback)]; found {
							mapping := summaries[function.object]
							if callbackIndex < len(mapping) && argumentIndex < len(mapping[callbackIndex]) {
								allowed = ps6080HasIndex(mapping[callbackIndex][argumentIndex], parameter)
							}
						}
					}
					if !allowed {
						if callee, _, direct := typedCallee(pass, call.Fun); direct {
							allowed = ps6080HasIndex(result[callee], argumentIndex)
						}
					}
					if !allowed {
						safe = false
						return false
					}
					return true
				})
				if safe {
					parameters := result[function.object]
					ps6080AddIndex(&parameters, parameter)
					result[function.object] = parameters
					changed = true
				}
			}
		}
	}
	value, _ := ps6080NamedSafeMapCaches.LoadOrStore(pass, result)
	return value.(map[*types.Func]ps6080IndexSet)
}

func ps6080NamedCallSafeMapArguments(
	call *ast.CallExpr,
	structurallySafe ps6080IndexSet,
	callbacks ps6080CallbackMappings,
	resolvedCallbacks ps6080IndexSet,
) ps6080IndexSet {
	var result ps6080IndexSet
	for mapParameter, structurallySafeParameter := range structurallySafe {
		if !structurallySafeParameter {
			continue
		}
		forwarded := false
		safe := true
		for callbackParameter, arguments := range callbacks {
			for _, parameters := range arguments {
				if !ps6080HasIndex(parameters, mapParameter) {
					continue
				}
				forwarded = true
				if callbackParameter >= len(call.Args) ||
					!ps6080HasIndex(resolvedCallbacks, callbackParameter) {
					safe = false
				}
			}
		}
		if forwarded && safe {
			ps6080AddIndex(&result, mapParameter)
		}
	}
	return result
}

func ps6080ComputeInvokedFunctionLiterals(
	pass *analysis.Pass,
	body *ast.BlockStmt,
) *ps6080InvokedLiteralResult {
	hasLiteral := false
	ast.Inspect(body, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			hasLiteral = true
			return false
		}
		return !hasLiteral
	})
	if !hasLiteral {
		return ps6080EmptyInvokedLiteralResult()
	}

	type binding struct {
		expression ast.Expr
		node       ast.Node
		scope      *ast.BlockStmt
	}
	type scanContext struct {
		root       *ast.BlockStmt
		literal    *ast.FuncLit
		invocation *ast.CallExpr
		parents    map[*scanContext]bool
	}
	type contextKey struct {
		literal    *ast.FuncLit
		invocation *ast.CallExpr
	}
	type resolutionKey struct {
		context   *scanContext
		reference ast.Node
		object    types.Object
	}
	type contextFlowKey struct {
		context *scanContext
		object  types.Object
		stop    ast.Node
	}
	type contextObjectKey struct {
		context *scanContext
		object  types.Object
	}
	type contextBinding struct {
		binding *binding
		context *scanContext
	}
	type targetSet map[*ast.FuncLit]bool
	type contextBindingSet map[contextBinding]bool
	type resolutionState struct {
		targets     map[resolutionKey]targetSet
		targetsBusy map[resolutionKey]bool
		flows       map[contextFlowKey]contextBindingSet
		flowsBusy   map[contextFlowKey]bool
		objectsBusy map[contextObjectKey]bool
	}

	bindings := make(map[types.Object][]binding)
	escapedCallables := make(map[types.Object]bool)
	parents := ps6071Parents(body)
	needsLiteralResolution := false
	ast.Inspect(body, func(node ast.Node) bool {
		literal, ok := node.(*ast.FuncLit)
		if !ok {
			return !needsLiteralResolution
		}
		var expression ast.Node = literal
		parent := parents[expression]
		for {
			if _, wrapped := parent.(*ast.ParenExpr); !wrapped {
				break
			}
			expression = parent
			parent = parents[expression]
		}
		call, argument := parent.(*ast.CallExpr)
		if !argument || call.Fun == expression {
			needsLiteralResolution = true
			return false
		}
		needsLiteralResolution = true
		return false
	})
	if !needsLiteralResolution {
		return ps6080EmptyInvokedLiteralResult()
	}
	bindingScope := func(node ast.Node) *ast.BlockStmt {
		for current := node; current != nil; current = parents[current] {
			if literal, ok := current.(*ast.FuncLit); ok {
				return literal.Body
			}
		}
		return body
	}
	record := func(object types.Object, expression ast.Expr, node ast.Node) {
		if object == nil || !ps6080CallableType(object.Type()) {
			return
		}
		bindings[object] = append(bindings[object], binding{
			expression: expression,
			node:       node,
			scope:      bindingScope(node),
		})
	}
	containingFlowNode := func(node ast.Node) ast.Node {
		for current := node; current != nil; current = parents[current] {
			switch current.(type) {
			case ast.Stmt, *ast.ValueSpec:
				return current
			}
		}
		return node
	}
	rangeValue := func(expression ast.Expr) ast.Expr {
		expression = ps2110Unparen(expression)
		literal, ok := expression.(*ast.CompositeLit)
		if !ok || len(literal.Elts) != 1 {
			return nil
		}
		element := literal.Elts[0]
		if keyed, ok := element.(*ast.KeyValueExpr); ok {
			element = keyed.Value
		}
		return element
	}
	ast.Inspect(body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			for index, left := range value.Lhs {
				identifier, ok := ps2110Unparen(left).(*ast.Ident)
				if !ok {
					continue
				}
				var expression ast.Expr
				if len(value.Lhs) == len(value.Rhs) {
					expression = value.Rhs[index]
				}
				record(pass.TypesInfo.ObjectOf(identifier), expression, value)
			}
		case *ast.ValueSpec:
			for index, name := range value.Names {
				var expression ast.Expr
				if len(value.Names) == len(value.Values) {
					expression = value.Values[index]
				}
				record(pass.TypesInfo.Defs[name], expression, value)
			}
		case *ast.RangeStmt:
			if ps6080StaticallyEmptyRange(pass, value) || value.Value == nil {
				break
			}
			identifier, direct := ps2110Unparen(value.Value).(*ast.Ident)
			if !direct {
				break
			}
			record(pass.TypesInfo.ObjectOf(identifier), rangeValue(value.X), identifier)
		case *ast.UnaryExpr:
			if value.Op != token.AND {
				break
			}
			identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
			if direct {
				object := pass.TypesInfo.ObjectOf(identifier)
				if ps6080StaticallyReachable(pass, parents, value) {
					escapedCallables[object] = true
				}
				record(object, nil, containingFlowNode(value))
			}
		}
		return true
	})
	statementIndexes := make(map[ast.Stmt]int, len(body.List))
	for index, statement := range body.List {
		statementIndexes[statement] = index
	}
	topLevelStatement := func(node ast.Node) ast.Stmt {
		for current := node; current != nil && current != body; current = parents[current] {
			statement, ok := current.(ast.Stmt)
			if ok && parents[current] == body {
				return statement
			}
		}
		return nil
	}
	directTopLevelBinding := func(candidate *binding) (ast.Stmt, bool) {
		switch node := candidate.node.(type) {
		case *ast.AssignStmt:
			return node, parents[node] == body
		case *ast.ValueSpec:
			statement := topLevelStatement(node)
			_, direct := statement.(*ast.DeclStmt)
			return statement, direct
		default:
			return nil, false
		}
	}
	var simpleGraph *cfg.CFG
	definitionDominates := func(graph *cfg.CFG, definition ast.Node, call *ast.CallExpr) bool {
		if graph == nil || definition == nil || call == nil || len(graph.Blocks) == 0 {
			return false
		}
		definitionBlock := ps6079CFGBlockAt(graph, definition.Pos())
		callBlock := ps6079CFGBlockAt(graph, call.Pos())
		if definitionBlock == nil || callBlock == nil || !definitionBlock.Live || !callBlock.Live {
			return false
		}
		if definitionBlock == callBlock {
			return definition.Pos() < call.Pos()
		}
		seen := make(map[*cfg.Block]bool)
		pending := []*cfg.Block{graph.Blocks[0]}
		for len(pending) > 0 {
			block := pending[0]
			pending = pending[1:]
			if block == definitionBlock || seen[block] || !block.Live {
				continue
			}
			if block == callBlock {
				return false
			}
			seen[block] = true
			pending = append(pending, ps6080FeasibleSuccessors(pass, parents, block)...)
		}
		return true
	}
	simpleTargets := make(map[*ast.FuncLit]bool)
	simpleCalls := make(map[*ast.FuncLit]map[*ast.CallExpr]bool)
	simpleSafeCalls := make(map[*ast.CallExpr]bool)
	for object, candidates := range bindings {
		if escapedCallables[object] {
			continue
		}
		var literal *ast.FuncLit
		var literalBinding *binding
		valid := true
		for index := range candidates {
			candidate := &candidates[index]
			if candidate.expression == nil {
				if _, declaration := candidate.node.(*ast.ValueSpec); !declaration {
					valid = false
				}
				continue
			}
			value, direct := ps2110Unparen(candidate.expression).(*ast.FuncLit)
			if !direct || literal != nil {
				valid = false
				break
			}
			literal = value
			literalBinding = candidate
		}
		if !valid || literal == nil {
			continue
		}
		assignment, direct := directTopLevelBinding(literalBinding)
		if !direct {
			continue
		}
		assignmentIndex := statementIndexes[assignment]
		for index := range candidates {
			candidate := &candidates[index]
			if candidate == literalBinding {
				continue
			}
			declaration, direct := directTopLevelBinding(candidate)
			if !direct || statementIndexes[declaration] > assignmentIndex {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		nested := false
		ast.Inspect(literal.Body, func(node ast.Node) bool {
			value, found := node.(*ast.FuncLit)
			if !found {
				return !nested
			}
			var expression ast.Node = value
			parent := parents[expression]
			for {
				if _, wrapped := parent.(*ast.ParenExpr); !wrapped {
					break
				}
				expression = parent
				parent = parents[expression]
			}
			call, argument := parent.(*ast.CallExpr)
			if !argument || call.Fun == expression {
				nested = true
				return false
			}
			_, _, named := typedCallee(pass, call.Fun)
			if !named {
				nested = true
			}
			return false
		})
		if nested {
			continue
		}
		calls := make(map[*ast.CallExpr]bool)
		ast.Inspect(body, func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok || pass.TypesInfo.Types[call.Fun].IsType() {
				return true
			}
			identifier, direct := ps2110Unparen(call.Fun).(*ast.Ident)
			if !direct || pass.TypesInfo.ObjectOf(identifier) != object {
				return true
			}
			statement := topLevelStatement(call)
			if statement == nil || statementIndexes[statement] <= assignmentIndex ||
				!ps6080StaticallyReachable(pass, parents, call) {
				return true
			}
			if simpleGraph == nil {
				simpleGraph = cfg.New(body, ps6080CallMayReturn(pass))
			}
			if !ps6080NodeReachable(pass, simpleGraph, parents, call) {
				return true
			}
			if !definitionDominates(simpleGraph, literalBinding.node, call) {
				return true
			}
			calls[call] = true
			return true
		})
		if len(calls) > 0 {
			simpleTargets[literal] = true
			simpleCalls[literal] = calls
			for call := range calls {
				simpleSafeCalls[call] = true
			}
		}
	}
	if len(simpleTargets) > 0 {
		complete := true
		ast.Inspect(body, func(node ast.Node) bool {
			literal, ok := node.(*ast.FuncLit)
			if !ok {
				return complete
			}
			if simpleTargets[literal] {
				return false
			}
			var expression ast.Node = literal
			parent := parents[expression]
			for {
				if _, wrapped := parent.(*ast.ParenExpr); !wrapped {
					break
				}
				expression = parent
				parent = parents[expression]
			}
			call, argument := parent.(*ast.CallExpr)
			if !argument || call.Fun == expression {
				complete = false
				return false
			}
			complete = false
			return false
		})
		if complete {
			result := ps6080EmptyInvokedLiteralResult()
			result.literals = simpleTargets
			result.calls = simpleCalls
			result.safeCalls = simpleSafeCalls
			for literal, calls := range simpleCalls {
				for call := range calls {
					ps6080AddInvocationOrder(result, literal, call, []token.Pos{call.Pos()})
				}
			}
			return result
		}
	}
	literalsByBody := make(map[*ast.BlockStmt]*ast.FuncLit)
	ast.Inspect(body, func(node ast.Node) bool {
		if literal, ok := node.(*ast.FuncLit); ok {
			literalsByBody[literal.Body] = literal
		}
		return true
	})
	literalWrites := make(map[*ast.FuncLit]map[types.Object]bool)
	for object, candidates := range bindings {
		for index := range candidates {
			literal := literalsByBody[candidates[index].scope]
			if literal == nil || literal.Pos() <= object.Pos() && object.Pos() < literal.End() {
				continue
			}
			if literalWrites[literal] == nil {
				literalWrites[literal] = make(map[types.Object]bool)
			}
			literalWrites[literal][object] = true
		}
	}
	returningLiterals := make(map[*ast.FuncLit]bool)
	var staticLiteralTargets func(ast.Expr, map[types.Object]bool) targetSet
	staticLiteralTargets = func(expression ast.Expr, seen map[types.Object]bool) targetSet {
		result := make(targetSet)
		switch value := ps2110Unparen(expression).(type) {
		case *ast.FuncLit:
			result[value] = true
		case *ast.Ident:
			object := pass.TypesInfo.ObjectOf(value)
			if object == nil || seen[object] {
				return result
			}
			seen[object] = true
			for index := range bindings[object] {
				for literal := range staticLiteralTargets(bindings[object][index].expression, seen) {
					result[literal] = true
				}
			}
			delete(seen, object)
		case *ast.CallExpr:
			if len(value.Args) == 1 && pass.TypesInfo.Types[value.Fun].IsType() {
				return staticLiteralTargets(value.Args[0], seen)
			}
			for _, argument := range value.Args {
				for literal := range staticLiteralTargets(argument, seen) {
					result[literal] = true
				}
			}
			for factory := range staticLiteralTargets(value.Fun, seen) {
				if returningLiterals[factory] {
					continue
				}
				returningLiterals[factory] = true
				ast.Inspect(factory.Body, func(node ast.Node) bool {
					if _, nested := node.(*ast.FuncLit); nested {
						return false
					}
					returned, ok := node.(*ast.ReturnStmt)
					if !ok {
						return true
					}
					for _, expression := range returned.Results {
						for literal := range staticLiteralTargets(expression, seen) {
							result[literal] = true
						}
					}
					return false
				})
				delete(returningLiterals, factory)
			}
		case *ast.CompositeLit:
			for _, element := range value.Elts {
				if keyed, ok := element.(*ast.KeyValueExpr); ok {
					element = keyed.Value
				}
				for literal := range staticLiteralTargets(element, seen) {
					result[literal] = true
				}
			}
		}
		return result
	}
	literalDependencies := make(map[*ast.FuncLit]targetSet, len(literalsByBody))
	for _, literal := range literalsByBody {
		dependencies := make(targetSet)
		ast.Inspect(literal.Body, func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok || pass.TypesInfo.Types[call.Fun].IsType() {
				return true
			}
			for target := range staticLiteralTargets(call.Fun, make(map[types.Object]bool)) {
				dependencies[target] = true
			}
			for _, argument := range call.Args {
				for target := range staticLiteralTargets(argument, make(map[types.Object]bool)) {
					dependencies[target] = true
				}
			}
			return true
		})
		literalDependencies[literal] = dependencies
	}
	changed := true
	for changed {
		changed = false
		for literal, dependencies := range literalDependencies {
			writes := literalWrites[literal]
			if writes == nil {
				writes = make(map[types.Object]bool)
				literalWrites[literal] = writes
			}
			for dependency := range dependencies {
				for object := range literalWrites[dependency] {
					if !writes[object] {
						writes[object] = true
						changed = true
					}
				}
			}
		}
	}
	allLiteralWrites := make(map[types.Object]bool)
	for _, writes := range literalWrites {
		for object := range writes {
			allLiteralWrites[object] = true
		}
	}
	var mayCallNonLiteral func(ast.Expr, map[types.Object]bool) bool
	mayCallNonLiteral = func(expression ast.Expr, seen map[types.Object]bool) bool {
		switch value := ps2110Unparen(expression).(type) {
		case *ast.FuncLit:
			return false
		case *ast.Ident:
			object := pass.TypesInfo.ObjectOf(value)
			if _, named := object.(*types.Func); named {
				return true
			}
			if object == nil || seen[object] {
				return object != nil
			}
			seen[object] = true
			defer delete(seen, object)
			for index := range bindings[object] {
				if bindings[object][index].expression == nil ||
					mayCallNonLiteral(bindings[object][index].expression, seen) {
					return true
				}
			}
			return false
		case *ast.CallExpr:
			if len(value.Args) == 1 && pass.TypesInfo.Types[value.Fun].IsType() {
				return mayCallNonLiteral(value.Args[0], seen)
			}
		}
		return true
	}

	mergeTargets := func(destination, source targetSet) {
		for literal := range source {
			destination[literal] = true
		}
	}

	result := ps6080EmptyInvokedLiteralResult()
	graphs := make(map[*ast.BlockStmt]*cfg.CFG)
	if simpleGraph != nil {
		graphs[body] = simpleGraph
	}
	graphBlocks := make(map[*cfg.CFG]map[token.Pos]*cfg.Block)
	blockAt := func(graph *cfg.CFG, position token.Pos) *cfg.Block {
		if graph == nil {
			return nil
		}
		blocks := graphBlocks[graph]
		if blocks == nil {
			blocks = make(map[token.Pos]*cfg.Block)
			for _, block := range graph.Blocks {
				for _, node := range block.Nodes {
					ast.Inspect(node, func(child ast.Node) bool {
						if child != nil {
							if _, exists := blocks[child.Pos()]; !exists {
								blocks[child.Pos()] = block
							}
						}
						return true
					})
				}
			}
			graphBlocks[graph] = blocks
		}
		return blocks[position]
	}
	nodeReachable := func(
		graph *cfg.CFG,
		contextParents map[ast.Node]ast.Node,
		node ast.Node,
	) bool {
		if graph == nil || node == nil {
			return false
		}
		block := blockAt(graph, node.Pos())
		found, live := block != nil, block != nil && block.Live
		if !found {
			for _, candidate := range graph.Blocks {
				for _, cfgNode := range candidate.Nodes {
					if node.Pos() <= cfgNode.Pos() && cfgNode.End() <= node.End() {
						found = true
						live = live || candidate.Live
					}
				}
			}
		}
		return ps6080NodeReachableWithLiveness(pass, contextParents, node, found, live)
	}
	contexts := make(map[contextKey]*scanContext)
	children := make(map[*scanContext]map[*scanContext]bool)
	queued := make(map[*scanContext]bool)
	var queue []*scanContext
	markDirty := func(root *scanContext) {
		seen := make(map[*scanContext]bool)
		pending := []*scanContext{root}
		for len(pending) > 0 {
			context := pending[0]
			pending = pending[1:]
			if seen[context] {
				continue
			}
			seen[context] = true
			if !queued[context] {
				queued[context] = true
				queue = append(queue, context)
			}
			for child := range children[context] {
				pending = append(pending, child)
			}
		}
	}
	escapeCallable := func(object types.Object) {
		if object == nil || escapedCallables[object] {
			return
		}
		escapedCallables[object] = true
		for context := range queued {
			markDirty(context)
		}
	}
	enqueue := func(parent *scanContext, literal *ast.FuncLit, call *ast.CallExpr) {
		key := contextKey{literal: literal, invocation: call}
		context := contexts[key]
		if context == nil {
			context = &scanContext{
				root:       literal.Body,
				literal:    literal,
				invocation: call,
				parents:    make(map[*scanContext]bool),
			}
			contexts[key] = context
		}
		if context.parents[parent] {
			return
		}
		context.parents[parent] = true
		if children[parent] == nil {
			children[parent] = make(map[*scanContext]bool)
		}
		children[parent][context] = true
		markDirty(parent)
		for _, candidate := range contexts {
			if candidate.literal == literal {
				markDirty(candidate)
			}
		}
	}

	parameterArgument := func(context *scanContext, object types.Object) (ast.Expr, bool) {
		if context.literal == nil || context.invocation == nil || context.literal.Type.Params == nil {
			return nil, false
		}
		parameter := 0
		for _, field := range context.literal.Type.Params.List {
			count := len(field.Names)
			if count == 0 {
				count = 1
			}
			for index := range count {
				if index < len(field.Names) && pass.TypesInfo.Defs[field.Names[index]] == object {
					if parameter < len(context.invocation.Args) {
						return context.invocation.Args[parameter], true
					}
					return nil, true
				}
				parameter++
			}
		}
		return nil, false
	}
	contextsMayRunSequentially := func(parent *scanContext, left, right *ast.CallExpr) bool {
		graph := graphs[parent.root]
		if graph == nil {
			graph = cfg.New(parent.root, ps6080CallMayReturn(pass))
			graphs[parent.root] = graph
		}
		leftBlock := blockAt(graph, left.Pos())
		rightBlock := blockAt(graph, right.Pos())
		if leftBlock == nil || rightBlock == nil || !leftBlock.Live || !rightBlock.Live {
			return false
		}
		if leftBlock == rightBlock {
			return true
		}
		reachable := func(from, to *cfg.Block) bool {
			seen := make(map[*cfg.Block]bool)
			queue := []*cfg.Block{from}
			for len(queue) > 0 {
				block := queue[0]
				queue = queue[1:]
				if block == to {
					return true
				}
				if seen[block] {
					continue
				}
				seen[block] = true
				for _, successor := range ps6080FeasibleSuccessors(pass, parents, block) {
					if successor.Live {
						queue = append(queue, successor)
					}
				}
			}
			return false
		}
		return reachable(leftBlock, rightBlock) || reachable(rightBlock, leftBlock)
	}
	type contextPair struct {
		left  *scanContext
		right *scanContext
	}
	var contextsMayBothExecute func(*scanContext, *scanContext, map[contextPair]bool) bool
	contextsMayBothExecute = func(
		left *scanContext,
		right *scanContext,
		visiting map[contextPair]bool,
	) bool {
		pair := contextPair{left: left, right: right}
		if visiting[pair] {
			return false
		}
		visiting[pair] = true
		defer delete(visiting, pair)
		for leftParent := range left.parents {
			for rightParent := range right.parents {
				if leftParent == rightParent {
					if contextsMayRunSequentially(leftParent, left.invocation, right.invocation) {
						return true
					}
					continue
				}
				if contextsMayBothExecute(leftParent, rightParent, visiting) {
					return true
				}
			}
		}
		return false
	}
	var contextMayRepeat func(*scanContext, map[*scanContext]bool) bool
	contextMayRepeat = func(context *scanContext, visiting map[*scanContext]bool) bool {
		if context == nil || context.literal == nil {
			return false
		}
		if visiting[context] {
			return true
		}
		visiting[context] = true
		defer delete(visiting, context)
		for parent := range context.parents {
			if ps6079PositionInLoop(parent.root, context.invocation.Pos()) ||
				contextMayRepeat(parent, visiting) {
				return true
			}
			for _, other := range contexts {
				if other == context || other.literal != context.literal {
					continue
				}
				if contextsMayBothExecute(context, other, make(map[contextPair]bool)) {
					return true
				}
			}
		}
		return false
	}

	cloneContextBindings := func(source contextBindingSet) contextBindingSet {
		result := make(contextBindingSet, len(source))
		for value := range source {
			result[value] = true
		}
		return result
	}
	mergeContextBindings := func(destination, source contextBindingSet) bool {
		changed := false
		for value := range source {
			if !destination[value] {
				destination[value] = true
				changed = true
			}
		}
		return changed
	}
	bindingForNode := func(context *scanContext, object types.Object, node ast.Node) *binding {
		for index := len(bindings[object]) - 1; index >= 0; index-- {
			candidate := &bindings[object][index]
			if candidate.scope == context.root && candidate.node == node {
				return candidate
			}
		}
		return nil
	}
	callsInEvaluationOrder := func(node ast.Node) []*ast.CallExpr {
		var calls []*ast.CallExpr
		astutil.Apply(node, func(cursor *astutil.Cursor) bool {
			_, nested := cursor.Node().(*ast.FuncLit)
			return !nested
		}, func(cursor *astutil.Cursor) bool {
			call, ok := cursor.Node().(*ast.CallExpr)
			if ok && !pass.TypesInfo.Types[call.Fun].IsType() {
				calls = append(calls, call)
			}
			return true
		})
		return calls
	}

	var resolveObject func(*scanContext, ast.Node, types.Object, *resolutionState) targetSet
	var resolveExpression func(*scanContext, ast.Node, ast.Expr, *resolutionState) targetSet
	var analyzeContext func(*scanContext, types.Object, ast.Node, *resolutionState) contextBindingSet
	resolveExpression = func(
		context *scanContext,
		reference ast.Node,
		expression ast.Expr,
		state *resolutionState,
	) targetSet {
		result := make(targetSet)
		switch value := ps2110Unparen(expression).(type) {
		case *ast.FuncLit:
			result[value] = true
		case *ast.Ident:
			mergeTargets(result, resolveObject(
				context, reference, pass.TypesInfo.ObjectOf(value), state,
			))
		case *ast.CallExpr:
			if len(value.Args) == 1 && pass.TypesInfo.Types[value.Fun].IsType() {
				mergeTargets(result, resolveExpression(
					context, reference, value.Args[0], state,
				))
			}
		}
		return result
	}
	analyzeContext = func(
		context *scanContext,
		object types.Object,
		stop ast.Node,
		state *resolutionState,
	) contextBindingSet {
		key := contextFlowKey{context: context, object: object, stop: stop}
		if cached := state.flows[key]; cached != nil {
			return cloneContextBindings(cached)
		}
		objectKey := contextObjectKey{context: context, object: object}
		if state.flowsBusy[key] || state.objectsBusy[objectKey] {
			return contextBindingSet{{}: true}
		}
		state.flowsBusy[key] = true
		state.objectsBusy[objectKey] = true
		defer delete(state.flowsBusy, key)
		defer delete(state.objectsBusy, objectKey)

		graph := graphs[context.root]
		if graph == nil {
			graph = cfg.New(context.root, ps6080CallMayReturn(pass))
			graphs[context.root] = graph
		}
		if len(graph.Blocks) == 0 || !graph.Blocks[0].Live {
			return make(contextBindingSet)
		}
		transfer := func(values contextBindingSet, node ast.Node) contextBindingSet {
			scheduled := make(map[*ast.CallExpr]bool)
			switch value := node.(type) {
			case *ast.GoStmt:
				scheduled[value.Call] = true
			case *ast.DeferStmt:
				scheduled[value.Call] = true
			}
			for _, call := range callsInEvaluationOrder(node) {
				if scheduled[call] {
					continue
				}
				targets := resolveExpression(context, call, call.Fun, state)
				if len(targets) == 0 {
					continue
				}
				next := make(contextBindingSet)
				for target := range targets {
					enqueue(context, target, call)
					child := contexts[contextKey{literal: target, invocation: call}]
					effects := analyzeContext(child, object, nil, state)
					for effect := range effects {
						if effect.binding == nil {
							mergeContextBindings(next, values)
						} else {
							next[effect] = true
						}
					}
				}
				values = next
			}
			if candidate := bindingForNode(context, object, node); candidate != nil {
				values = contextBindingSet{{binding: candidate, context: context}: true}
			}
			return values
		}

		states := map[*cfg.Block]contextBindingSet{
			graph.Blocks[0]: {{binding: nil, context: nil}: true},
		}
		pending := []*cfg.Block{graph.Blocks[0]}
		inQueue := map[*cfg.Block]bool{graph.Blocks[0]: true}
		for len(pending) > 0 {
			block := pending[0]
			pending = pending[1:]
			inQueue[block] = false
			values := cloneContextBindings(states[block])
			for _, node := range block.Nodes {
				values = transfer(values, node)
			}
			if len(values) == 0 {
				continue
			}
			for _, successor := range ps6080FeasibleSuccessors(pass, parents, block) {
				if !successor.Live {
					continue
				}
				if states[successor] == nil {
					states[successor] = cloneContextBindings(values)
					if !inQueue[successor] {
						inQueue[successor] = true
						pending = append(pending, successor)
					}
				} else if mergeContextBindings(states[successor], values) && !inQueue[successor] {
					inQueue[successor] = true
					pending = append(pending, successor)
				}
			}
		}

		result := make(contextBindingSet)
		if stop != nil {
			target := blockAt(graph, stop.Pos())
			if values := states[target]; target != nil && values != nil {
				result = cloneContextBindings(values)
				for _, node := range target.Nodes {
					if node.Pos() <= stop.Pos() && stop.End() <= node.End() {
						break
					}
					result = transfer(result, node)
				}
			}
		} else {
			for _, block := range graph.Blocks {
				if !block.Live || block.Return() == nil || states[block] == nil {
					continue
				}
				values := cloneContextBindings(states[block])
				for _, node := range block.Nodes {
					values = transfer(values, node)
				}
				mergeContextBindings(result, values)
			}
		}
		state.flows[key] = cloneContextBindings(result)
		return result
	}
	resolveObject = func(
		context *scanContext,
		reference ast.Node,
		object types.Object,
		state *resolutionState,
	) targetSet {
		result := make(targetSet)
		if context == nil || reference == nil || object == nil {
			return result
		}
		if escapedCallables[object] {
			return result
		}
		key := resolutionKey{context: context, reference: reference, object: object}
		if resolved := state.targets[key]; resolved != nil {
			mergeTargets(result, resolved)
			return result
		}
		if state.targetsBusy[key] {
			return result
		}
		state.targetsBusy[key] = true
		defer delete(state.targetsBusy, key)

		values := analyzeContext(context, object, reference, state)
		captured := context.literal != nil &&
			!(context.literal.Pos() <= object.Pos() && object.Pos() < context.literal.End())
		if captured && contextMayRepeat(context, make(map[*scanContext]bool)) {
			contextParents := ps6071Parents(context.root)
			for index := range bindings[object] {
				candidate := &bindings[object][index]
				if candidate.scope == context.root &&
					ps6080StaticallyReachable(pass, parents, candidate.node) &&
					nodeReachable(graphs[context.root], contextParents, candidate.node) {
					values[contextBinding{binding: candidate, context: context}] = true
				}
			}
		}
		for value := range values {
			if value.binding != nil {
				mergeTargets(result, resolveExpression(
					value.context, value.binding.node, value.binding.expression, state,
				))
				continue
			}
			if argument, parameter := parameterArgument(context, object); parameter {
				for parent := range context.parents {
					mergeTargets(result, resolveExpression(
						parent, context.invocation, argument, state,
					))
				}
				continue
			}
			for parent := range context.parents {
				mergeTargets(result, resolveObject(
					parent, context.invocation, object, state,
				))
			}
		}
		state.targets[key] = result
		return result
	}
	namedCallbacks := ps6080NamedCallbackParameters(pass)
	mayCallbacks := ps6080MayNamedCallbackSites(pass)
	namedInvocations := ps6080NamedCallbackInvocations(pass)
	scan := func(context *scanContext, found func(*ast.FuncLit, *ast.CallExpr)) {
		root := context.root
		contextParents := ps6071Parents(root)
		graph := graphs[root]
		if graph == nil {
			graph = cfg.New(root, ps6080CallMayReturn(pass))
			graphs[root] = graph
		}
		ast.Inspect(root, func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if !nodeReachable(graph, contextParents, call) {
				return true
			}
			if !ps6080StaticallyReachable(pass, parents, call) {
				return true
			}
			if pass.TypesInfo.Types[call.Fun].IsType() {
				return true
			}
			state := &resolutionState{
				targets:     make(map[resolutionKey]targetSet),
				targetsBusy: make(map[resolutionKey]bool),
				flows:       make(map[contextFlowKey]contextBindingSet),
				flowsBusy:   make(map[contextFlowKey]bool),
				objectsBusy: make(map[contextObjectKey]bool),
			}
			targets := resolveExpression(context, call, call.Fun, state)
			for literal := range targets {
				enqueue(context, literal, call)
				ps6080AddInvocationOrder(result, literal, call, []token.Pos{call.Pos()})
				if found != nil {
					found(literal, call)
				}
			}
			if found != nil && len(targets) > 0 && !mayCallNonLiteral(call.Fun, make(map[types.Object]bool)) {
				result.safeCalls[call] = true
			}
			if callee, _, direct := typedCallee(pass, call.Fun); direct {
				for index, mapping := range namedCallbacks[callee] {
					if mapping == nil || index >= len(call.Args) {
						continue
					}
					argumentTargets := resolveExpression(context, call, call.Args[index], state)
					mergeTargets(
						argumentTargets, staticLiteralTargets(call.Args[index], make(map[types.Object]bool)),
					)
					if len(argumentTargets) > 0 &&
						!mayCallNonLiteral(call.Args[index], make(map[types.Object]bool)) {
						safeArguments := result.safeArguments[call]
						ps6080AddIndex(&safeArguments, index)
						result.safeArguments[call] = safeArguments
					}
					var invocations []ps6080NamedCallbackInvocation
					if index < len(namedInvocations[callee]) {
						invocations = namedInvocations[callee][index]
					}
					if len(invocations) == 0 {
						invocations = []ps6080NamedCallbackInvocation{{arguments: mapping}}
					}
					for literal := range argumentTargets {
						enqueue(context, literal, call)
						for _, invocation := range invocations {
							order := append([]token.Pos{call.Pos()}, invocation.order...)
							ps6080AddOrderedInvocation(result, literal, call, order, invocation.arguments)
						}
						if found != nil {
							found(literal, call)
							if result.arguments[literal] == nil {
								result.arguments[literal] =
									make(map[*ast.CallExpr]ps6080InvocationArguments)
							}
							result.arguments[literal][call] = mapping
						}
					}
				}
				for index, sites := range mayCallbacks[callee] {
					if len(sites) == 0 || index >= len(call.Args) {
						continue
					}
					argumentTargets := resolveExpression(context, call, call.Args[index], state)
					mergeTargets(
						argumentTargets, staticLiteralTargets(call.Args[index], make(map[types.Object]bool)),
					)
					if len(argumentTargets) > 0 &&
						!mayCallNonLiteral(call.Args[index], make(map[types.Object]bool)) {
						safeArguments := result.safeArguments[call]
						ps6080AddIndex(&safeArguments, index)
						result.safeArguments[call] = safeArguments
					}
					for literal := range argumentTargets {
						enqueue(context, literal, call)
						for _, site := range sites {
							order := append([]token.Pos{call.Pos()}, site.order...)
							ps6080AddInvocationOrder(result, literal, call, order)
						}
						if found != nil {
							found(literal, call)
						}
					}
				}
			}
			if len(targets) == 0 && ps6080CallableType(pass.TypesInfo.TypeOf(call.Fun)) {
				if _, _, named := typedCallee(pass, call.Fun); !named {
					for object := range allLiteralWrites {
						escapeCallable(object)
					}
				}
			}
			if len(targets) == 0 || mayCallNonLiteral(call.Fun, make(map[types.Object]bool)) {
				for _, argument := range call.Args {
					argumentTargets := resolveExpression(context, call, argument, state)
					mergeTargets(argumentTargets, staticLiteralTargets(argument, make(map[types.Object]bool)))
					for literal := range argumentTargets {
						for object := range literalWrites[literal] {
							escapeCallable(object)
						}
					}
					if len(argumentTargets) == 0 && ps6080CallableType(pass.TypesInfo.TypeOf(argument)) {
						for object := range allLiteralWrites {
							escapeCallable(object)
						}
					}
				}
			}
			return true
		})
	}

	outer := &scanContext{root: body, parents: make(map[*scanContext]bool)}
	graphs[body] = cfg.New(body, ps6080CallMayReturn(pass))
	markDirty(outer)
	for {
		for len(queue) > 0 {
			context := queue[0]
			queue = queue[1:]
			queued[context] = false
			scan(context, nil)
		}

		result = ps6080EmptyInvokedLiteralResult()
		reachable := map[*scanContext]bool{outer: true}
		pending := []*scanContext{outer}
		for len(pending) > 0 {
			context := pending[0]
			pending = pending[1:]
			scan(context, func(literal *ast.FuncLit, call *ast.CallExpr) {
				result.literals[literal] = true
				if result.calls[literal] == nil {
					result.calls[literal] = make(map[*ast.CallExpr]bool)
				}
				result.calls[literal][call] = true
				child := contexts[contextKey{literal: literal, invocation: call}]
				if child != nil && !reachable[child] {
					reachable[child] = true
					pending = append(pending, child)
				}
			})
		}
		if len(queue) == 0 {
			return result
		}
	}
}

func ps6080Roles(context string) ps6080Role {
	context = ps6080WithoutDirectives(context)
	normalized := ps6080ContextReplacer.Replace(ps6007NormalizeName(context))
	var roles ps6080Role
	if ps6007ContainsAny(normalized, "dequant", "decode", "unpack", "decompress") {
		roles |= ps6080DecodeRole
	}
	if ps6080HasStorageIdentifier(context) {
		roles |= ps6080StorageRole
	}
	if ps6080HasMatmulIdentifier(context) ||
		ps6080HasAlternativeMatmulName(context) ||
		strings.Contains(normalized, "quant") && strings.Contains(normalized, "rowdot") {
		roles |= ps6080MatmulRole
	}
	return roles
}

func ps6080HasStorageIdentifier(name string) bool {
	words := ps6080IdentifierWords(name)
	if ps6080HasJoinedStorageMarker(words) {
		return true
	}
	storageContext, quantContext := false, false
	for _, word := range words {
		switch word {
		case "storage", "layout", "stride":
			storageContext = true
		case "quant", "quantized", "block", "row":
			quantContext = true
		}
	}
	return storageContext && quantContext
}

func ps6080HasJoinedStorageMarker(words []string) bool {
	const longest = len("bytesperblock")
	for start := range words {
		var joined strings.Builder
		joined.Grow(longest)
		for _, word := range words[start:] {
			joined.WriteString(word)
			switch joined.String() {
			case "bytesize", "blocksize", "rowsize", "typesize", "quantsize", "quantbytes", "quantbyte",
				"blockbytes", "rowbytes", "typebytes", "bytesperblock", "bytesperrow":
				return true
			}
			if joined.Len() >= longest {
				break
			}
		}
	}
	return false
}

func ps6080HasMatmulIdentifier(name string) bool {
	words := ps6080IdentifierWords(name)
	for start := range words {
		var combined strings.Builder
		combined.Grow(len("matrixmultiply"))
		for _, word := range words[start:] {
			combined.WriteString(word)
			switch combined.String() {
			case "qmatmul", "matmul", "matrixmultiply", "gemv", "matvec":
				return true
			}
			if combined.Len() >= len("matrixmultiply") {
				break
			}
		}
	}
	return false
}

func ps6080HasAlternativeMatmulName(name string) bool {
	normalized := ps6080ContextReplacer.Replace(ps6007NormalizeName(name))
	if !strings.Contains(normalized, "gemm") && !strings.Contains(normalized, "mulmat") {
		return false
	}
	words := ps6080IdentifierWords(name)
	for index, word := range words {
		if word == "gemm" || word == "qgemm" || word == "mulmat" || word == "qmulmat" {
			return true
		}
		if word != "mul" || index+1 >= len(words) || words[index+1] != "mat" {
			continue
		}
		return true
	}
	return false
}

func ps6080IdentifierWords(name string) []string {
	runes := []rune(name)
	var result []string
	start := -1
	flush := func(end int) {
		if start >= 0 && start < end {
			result = append(result, strings.ToLower(string(runes[start:end])))
		}
		start = -1
	}
	for index, current := range runes {
		if !unicode.IsLetter(current) && !unicode.IsDigit(current) {
			flush(index)
			continue
		}
		if start < 0 {
			start = index
			continue
		}
		previous := runes[index-1]
		if unicode.IsDigit(current) != unicode.IsDigit(previous) {
			flush(index)
			start = index
			continue
		}
		nextLower := index+1 < len(runes) && unicode.IsLower(runes[index+1])
		if unicode.IsUpper(current) &&
			(unicode.IsLower(previous) || unicode.IsDigit(previous) || unicode.IsUpper(previous) && nextLower) {
			flush(index)
			start = index
		}
	}
	flush(len(runes))
	return result
}

func ps6080BackendContext(context string) bool {
	context = ps6080WithoutDirectives(context)
	for _, marker := range [...]string{"cuda", "metal", "vulkan", "opencl", "rocm", "mps", "gpu", "accelerator"} {
		if ps6080HasIdentifierMarker(context, marker) {
			return true
		}
	}
	return false
}

func ps6080HasIdentifierMarker(name, marker string) bool {
	runes := []rune(name)
	markerRunes := []rune(marker)
	for start := 0; start+len(markerRunes) <= len(runes); start++ {
		end := start + len(markerRunes)
		if !strings.EqualFold(string(runes[start:end]), marker) ||
			!ps6080IdentifierMarkerStart(runes, start) ||
			!ps6080IdentifierMarkerEnd(runes, end) {
			continue
		}
		return true
	}
	return false
}

func ps6080IdentifierMarkerStart(runes []rune, start int) bool {
	if start == 0 || !unicode.IsLetter(runes[start-1]) && !unicode.IsDigit(runes[start-1]) {
		return true
	}
	current, previous := runes[start], runes[start-1]
	if unicode.IsDigit(current) != unicode.IsDigit(previous) {
		return true
	}
	return unicode.IsUpper(current) &&
		(unicode.IsLower(previous) || start+1 < len(runes) && unicode.IsLower(runes[start+1]))
}

func ps6080IdentifierMarkerEnd(runes []rune, end int) bool {
	if end == len(runes) || !unicode.IsLetter(runes[end]) && !unicode.IsDigit(runes[end]) {
		return true
	}
	return unicode.IsDigit(runes[end]) != unicode.IsDigit(runes[end-1]) || unicode.IsUpper(runes[end])
}

func ps6080FunctionBackendContext(pkg *types.Package, name string, signature *types.Signature) bool {
	if ps6080BackendContext(name) {
		return true
	}
	if pkg != nil && (ps6080BackendContext(pkg.Name()) || ps6080BackendContext(path.Base(pkg.Path()))) {
		return true
	}
	if signature == nil || signature.Recv() == nil {
		return false
	}
	receiver := types.Unalias(signature.Recv().Type())
	if pointer, ok := receiver.(*types.Pointer); ok {
		receiver = types.Unalias(pointer.Elem())
	}
	named, ok := receiver.(*types.Named)
	return ok && named.Obj() != nil && ps6080BackendContext(named.Obj().Name())
}

func ps6080WithoutDirectives(context string) string {
	lines := strings.Split(context, "\n")
	lines = slices.DeleteFunc(lines, ps6080ValidatedText)
	return strings.Join(lines, "\n")
}

func ps6080Reachable(functions map[*types.Func]*ps6080Function, seed func(*ps6080Function) bool) map[*types.Func]bool {
	result := make(map[*types.Func]bool)
	var queue []*types.Func
	for object, function := range functions {
		if seed(function) {
			result[object] = true
			queue = append(queue, object)
		}
	}
	for len(queue) > 0 {
		object := queue[0]
		queue = queue[1:]
		for _, callee := range functions[object].callees {
			if !result[callee] {
				result[callee] = true
				queue = append(queue, callee)
			}
		}
	}
	return result
}

func ps6080CPUReachable(
	functions map[*types.Func]*ps6080Function,
	domains map[*types.TypeName][]*types.Const,
	seed func(*ps6080Function) bool,
) ps6080CPUReachability {
	result := ps6080CPUReachability{
		functions: make(map[*types.Func]bool),
		scopes:    make(map[*types.Func]map[types.Object]map[string]token.Pos),
	}
	queued := make(map[*types.Func]bool)
	var queue []*types.Func
	enqueue := func(object *types.Func) {
		if !queued[object] {
			queued[object] = true
			queue = append(queue, object)
		}
	}
	for object, function := range functions {
		if !seed(function) {
			continue
		}
		result.functions[object] = true
		result.scopes[object] = make(map[types.Object]map[string]token.Pos)
		enqueue(object)
	}
	for len(queue) > 0 {
		object := queue[0]
		queue = queue[1:]
		queued[object] = false
		for _, call := range functions[object].cpuCalls {
			type evaluatedScope struct {
				values  map[string]token.Pos
				limited bool
			}
			evaluated := make(map[types.Object]evaluatedScope, len(call.scopes))
			feasible := true
			for target, callScope := range call.scopes {
				values, limited := ps6080CPUReachCallValues(
					callScope, result.scopes[object][callScope.source], domains[callScope.enum],
				)
				evaluated[target] = evaluatedScope{values: values, limited: limited}
				if limited && len(values) == 0 {
					feasible = false
				}
			}
			if !feasible {
				continue
			}
			newCallee := !result.functions[call.callee]
			calleeChanged := newCallee
			result.functions[call.callee] = true
			if newCallee {
				result.scopes[call.callee] = make(map[types.Object]map[string]token.Pos)
				for target, scope := range evaluated {
					if scope.limited {
						result.scopes[call.callee][target] = scope.values
					}
				}
			} else {
				for target, prior := range result.scopes[call.callee] {
					scope, present := evaluated[target]
					if !present || !scope.limited {
						delete(result.scopes[call.callee], target)
						calleeChanged = true
						continue
					}
					merged := ps6080CPUUnionValues(prior, scope.values)
					if ps6080CPUValuesCoverDomain(merged, domains[call.scopes[target].enum]) {
						delete(result.scopes[call.callee], target)
						calleeChanged = true
						continue
					}
					if len(merged) != len(prior) {
						result.scopes[call.callee][target] = merged
						calleeChanged = true
					}
				}
			}
			if calleeChanged {
				enqueue(call.callee)
			}
		}
	}
	return result
}

func ps6080CPUReachCallValues(
	callScope *ps6080CPUCallScope,
	source map[string]token.Pos,
	domain []*types.Const,
) (map[string]token.Pos, bool) {
	if callScope == nil || callScope.universal || len(domain) == 0 {
		return nil, false
	}
	values := ps6080CPUUnionValues(nil, callScope.fixed)
	if callScope.source != nil {
		dynamic := source
		if dynamic == nil {
			dynamic = ps6080CPUFullDomain(domain)
		}
		if callScope.allowed != nil {
			dynamic = ps6080CPUIntersectValues(dynamic, callScope.allowed)
		}
		values = ps6080CPUUnionValues(values, dynamic)
	}
	return values, !ps6080CPUValuesCoverDomain(values, domain)
}

func ps6080CPUValuesCoverDomain(values map[string]token.Pos, domain []*types.Const) bool {
	if len(domain) == 0 {
		return false
	}
	for _, constant := range domain {
		if _, covered := values[constant.Val().ExactString()]; !covered {
			return false
		}
	}
	return true
}

func ps6080FunctionAtNode(
	pass *analysis.Pass,
	function *ps6080Function,
	node ast.Node,
	parents map[ast.Node]ast.Node,
) *ps6080Function {
	for current := node; current != nil; current = parents[current] {
		literal, ok := current.(*ast.FuncLit)
		if !ok {
			continue
		}
		result := *function
		result.signature, _ = pass.TypesInfo.TypeOf(literal).(*types.Signature)
		result.body = literal.Body
		return &result
	}
	return function
}

func ps6080ResolvedSiteSubject(
	pass *analysis.Pass,
	function *ps6080Function,
	subject types.Object,
	node ast.Node,
	enum *types.TypeName,
	parents map[ast.Node]ast.Node,
) types.Object {
	seen := make(map[types.Object]bool)
	for subject != nil && !seen[subject] {
		seen[subject] = true
		variable, ok := subject.(*types.Var)
		if !ok {
			break
		}
		literal := ps6080ContainingLiteral(node, parents)
		if literal != nil && !(literal.Pos() <= variable.Pos() && variable.Pos() < literal.End()) {
			break
		}
		initializer := ps6080StableLocalInitializer(pass, function, variable, node, parents)
		if initializer == nil {
			break
		}
		resolved, unique := ps6080EnumSubject(pass, initializer, enum)
		if !unique || resolved == subject {
			break
		}
		if ps6080ObjectMayChangeBetween(
			pass, ps6080FunctionBody(function), resolved, initializer.End(), node.Pos(),
		) {
			break
		}
		subject = resolved
	}
	return subject
}

func ps6080FunctionSites(
	pass *analysis.Pass,
	function *ps6080Function,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
) []*ps6080Site {
	body := ps6080FunctionBody(function)
	parents := ps6071Parents(body)
	graph := cfg.New(body, ps6080CallMayReturn(pass))
	invoked := ps6080InvokedFunctionLiteralResult(pass, body)
	invokedLiterals := invoked.literals
	literalGraphs := ps6080InvokedLiteralGraphs(pass, invokedLiterals)
	var result []*ps6080Site
	ast.Inspect(body, func(node ast.Node) bool {
		if literal, nested := node.(*ast.FuncLit); nested {
			return invokedLiterals[literal]
		}
		nodeGraph := ps6080GraphForNode(graph, literalGraphs, parents, node)
		if node != nil && !ps6080NodeReachable(pass, nodeGraph, parents, node) {
			return true
		}
		nodeFunction := ps6080FunctionAtNode(pass, function, node, parents)
		if statement, ok := node.(ast.Stmt); ok && ps6080PrecededByEscapingBranch(
			statement, parents, ps6080FunctionBody(nodeFunction),
		) {
			return false
		}
		switch value := node.(type) {
		case *ast.SwitchStmt:
			groups := make(map[*types.TypeName]*ps6080ConstantGroup)
			explicitGroups := make(map[*types.TypeName]*ps6080ConstantGroup)
			explicitSubjects := make(map[*types.TypeName]types.Object)
			ambiguousExplicitSubjects := make(map[*types.TypeName]bool)
			dispatchMutatedEnums := make(map[*types.TypeName]bool)
			defaultSupported := false
			hasDefault := false
			var defaultClause *ast.CaseClause
			defaultResolvedEnums := make(map[*types.TypeName]bool)
			for _, statement := range value.Body.List {
				clause, ok := statement.(*ast.CaseClause)
				if !ok {
					continue
				}
				supported := ps6080SwitchClauseSupported(
					pass, nodeFunction, nodeGraph, value, clause, parents,
				)
				if len(clause.List) == 0 {
					hasDefault = true
					defaultSupported = supported
					defaultClause = clause
					continue
				}
				clauseGroups := make(map[*types.TypeName]*ps6080ConstantGroup)
				allExpressionsSupported := true
				for _, expression := range clause.List {
					expressionSupported := supported
					if value.Tag != nil {
						assumptions := ps6080SwitchExpressionAssumptions(pass, value, expression)
						expressionSupported = ps6080CFGClauseExpressionSupports(
							pass, nodeFunction, nodeGraph, clause, parents,
							assumptions, expression,
						)
						for _, constant := range assumptions {
							enum, ok := ps6080EnumType(constant.Type())
							if ok && ps6080SwitchDispatchMutatesAssumptions(
								pass, nodeFunction, clause, expression, parents, assumptions,
							) {
								dispatchMutatedEnums[enum] = true
							}
						}
					}
					if value.Tag == nil {
						parsed, _ := ps6080GuardExpression(pass, expression, constantEnums)
						resolvedPerValue := false
						allMatchedSupported := true
						for enum, guard := range parsed {
							subject, unique := ps6080EnumSubject(pass, expression, enum)
							if !unique || len(domains[enum]) == 0 {
								continue
							}
							resolvedPerValue = true
							if !ambiguousExplicitSubjects[enum] {
								if previous := explicitSubjects[enum]; previous != nil && previous != subject {
									delete(explicitGroups, enum)
									delete(explicitSubjects, enum)
									ambiguousExplicitSubjects[enum] = true
								} else {
									explicitSubjects[enum] = subject
									if explicitGroups[enum] == nil {
										explicitGroups[enum] = &ps6080ConstantGroup{
											included: make(map[*types.Const]token.Pos),
										}
									}
								}
							}
							for _, constant := range domains[enum] {
								assumptions := map[types.Object]*types.Const{subject: constant}
								if ps6080SwitchDispatchMutatesAssumptions(
									pass, nodeFunction, clause, expression, parents, assumptions,
								) {
									dispatchMutatedEnums[enum] = true
								}
								if truth, known := ps6080ExpressionTruth(pass, expression, assumptions); known && truth &&
									!ambiguousExplicitSubjects[enum] {
									explicitGroups[enum].included[constant] = expression.Pos()
								}
								if !ps6080SiteSupports(&ps6080Site{
									constants: guard.included, excluded: guard.excluded, open: guard.open,
								}, constant) {
									continue
								}
								constantSupported := ps6080CFGClauseExpressionSupports(
									pass, nodeFunction, nodeGraph, clause, parents,
									assumptions, expression,
								)
								allMatchedSupported = allMatchedSupported && constantSupported
								if !constantSupported {
									continue
								}
								if clauseGroups[enum] == nil {
									clauseGroups[enum] = &ps6080ConstantGroup{
										included: make(map[*types.Const]token.Pos),
									}
								}
								clauseGroups[enum].included[constant] = expression.Pos()
							}
						}
						if resolvedPerValue {
							expressionSupported = allMatchedSupported
						} else if expressionSupported {
							ps6080GuardConstants(pass, expression, clauseGroups, constantEnums, true)
						} else {
							ps6080GuardRejections(pass, expression, clauseGroups, constantEnums)
						}
					} else {
						ps6080ExpressionConstants(pass, expression, explicitGroups, constantEnums, true)
						ps6080ExpressionConstants(
							pass, expression, clauseGroups, constantEnums, expressionSupported,
						)
					}
					allExpressionsSupported = allExpressionsSupported && expressionSupported
				}
				supported = allExpressionsSupported
				clauseSubjects := make(map[*types.TypeName]types.Object)
				for enum := range clauseGroups {
					if dispatchMutatedEnums[enum] {
						continue
					}
					if subject, unique := ps6080SwitchClauseSubject(pass, value, clause, enum); unique {
						clauseSubjects[enum] = subject
						if value.Tag != nil {
							explicitSubjects[enum] = subject
						}
					}
				}
				if supported {
					ps6080NarrowSwitchClauseGroups(
						pass, nodeFunction, ps6080SwitchClauseTerminalStatements(value, clause), clauseGroups,
						clauseSubjects, constantEnums,
					)
				}
				ps6080MergeGuardGroups(groups, clauseGroups, token.LOR)
			}
			if defaultSupported && defaultClause != nil {
				for enum, explicit := range explicitGroups {
					subject := explicitSubjects[enum]
					if value.Tag != nil {
						var unique bool
						subject, unique = ps6080EnumSubject(pass, value.Tag, enum)
						if !unique {
							continue
						}
					}
					if subject == nil || len(domains[enum]) == 0 {
						continue
					}
					defaultResolvedEnums[enum] = true
					for _, constant := range domains[enum] {
						if ps6080SiteSupports(&ps6080Site{
							constants: explicit.included, excluded: explicit.excluded, open: explicit.open,
						}, constant) {
							continue
						}
						assumptions := map[types.Object]*types.Const{subject: constant}
						if ps6080SwitchDispatchMutatesAssumptions(
							pass, nodeFunction, defaultClause, nil, parents, assumptions,
						) {
							dispatchMutatedEnums[enum] = true
						}
						if !ps6080CFGClauseSupports(
							pass, nodeFunction, nodeGraph, defaultClause, parents,
							assumptions,
						) {
							continue
						}
						if groups[enum] == nil {
							groups[enum] = &ps6080ConstantGroup{included: make(map[*types.Const]token.Pos)}
						}
						groups[enum].included[constant] = defaultClause.Pos()
					}
				}
			}
			if defaultSupported && defaultClause != nil {
				predicates, unconditional, resolved := ps6080SwitchClausePredicates(
					pass, nodeFunction, ps6080SwitchClauseTerminalStatements(value, defaultClause), groups,
					explicitSubjects, constantEnums,
				)
				if resolved && !unconditional && len(predicates) > 0 {
					for enum, predicate := range predicates {
						if defaultResolvedEnums[enum] {
							continue
						}
						if explicit := explicitGroups[enum]; explicit != nil {
							predicates[enum] = ps6080IntersectGroups(predicate, ps6080ComplementGroup(explicit))
						}
						ps6080MergeGuardGroups(
							groups, map[*types.TypeName]*ps6080ConstantGroup{enum: predicates[enum]}, token.LOR,
						)
						defaultResolvedEnums[enum] = true
					}
				}
			}
			if !hasDefault && ps6080StatementsAfterSwitchSupport(pass, nodeFunction, value, parents) {
				defaultSupported = true
			}
			for enum, group := range groups {
				if value.Tag != nil && !ps6080DispatchSubject(pass, value.Tag, enum) {
					delete(groups, enum)
					continue
				}
				if defaultSupported && !defaultResolvedEnums[enum] {
					if explicit := explicitGroups[enum]; explicit != nil {
						groups[enum] = ps6080UnionGroups(group, ps6080ComplementGroup(explicit))
					} else {
						group.open = true
					}
				}
			}
			start := len(result)
			result = ps6080AppendGroupedSites(result, nodeFunction, "switch", value.Switch, value.End(), groups)
			for _, site := range result[start:] {
				var subject types.Object
				if value.Tag != nil {
					subject, _ = ps6080EnumSubject(pass, value.Tag, site.enum)
				} else {
					subject = explicitSubjects[site.enum]
				}
				site.subject = ps6080ResolvedSiteSubject(
					pass, nodeFunction, subject, value, site.enum, parents,
				)
				site.scope = ps6080EnclosingGuardConstants(pass, value, site.enum, parents, constantEnums)
				if group := ps6080FlowAlternativeGroup(pass, nodeFunction, site.enum, value, parents); group != "" {
					site.group = group
				}
			}
		case *ast.IfStmt:
			if parent, nested := parents[value].(*ast.IfStmt); nested && parent.Else == value {
				return true
			}
			groups := make(map[*types.TypeName]*ps6080ConstantGroup)
			conditions := make(map[*types.TypeName]*ps6080ConstantGroup)
			terminalConditions := make(map[*types.TypeName]*ps6080ConstantGroup)
			for current := value; current != nil; {
				next, _ := current.Else.(*ast.IfStmt)
				if ps6080BranchesEscapeBlock(
					current.Body, parents, ps6080FunctionBody(nodeFunction),
				) {
					current = next
					continue
				}
				_, continuationSupported, continuationTerminates :=
					ps6080IfContinuationOutcomes(
						pass, nodeFunction, value, current.Cond, current.Body.List, parents,
					)
				ps6080GuardConstants(pass, current.Cond, conditions, constantEnums, true)
				bodyTerminates := ps6080StatementsTerminate(pass, current.Body.List)
				bodyMayFallThrough, bodyFallthroughResolved :=
					ps6080StatementsMayFallThrough(pass, current.Body.List)
				if bodyTerminates || bodyFallthroughResolved && !bodyMayFallThrough {
					ps6080GuardConstants(pass, current.Cond, terminalConditions, constantEnums, true)
				}
				_, supported := ps6080StatementOutcomes(pass, nodeFunction, current.Body.List)
				pathTerminates := bodyTerminates || bodyFallthroughResolved && !bodyMayFallThrough
				if !bodyTerminates && bodyFallthroughResolved && bodyMayFallThrough {
					supported = supported || continuationSupported
					pathTerminates = continuationTerminates
				}
				if !supported && pathTerminates {
					ps6080GuardRejections(pass, current.Cond, groups, constantEnums)
				} else if supported {
					ps6080GuardConstants(pass, current.Cond, groups, constantEnums, true)
				}
				current = next
			}
			if ps6080IfFallbackAlwaysSupports(pass, nodeFunction, value, parents) {
				for enum := range conditions {
					terminal := terminalConditions[enum]
					if terminal == nil {
						terminal = ps6080BooleanGroup(false)
					}
					ps6080MergeGuardGroups(groups, map[*types.TypeName]*ps6080ConstantGroup{
						enum: ps6080ComplementGroup(terminal),
					}, token.LOR)
				}
			}
			start := len(result)
			result = ps6080AppendGroupedSites(result, nodeFunction, "guard", value.If, value.End(), groups)
			for _, site := range result[start:] {
				subject, _ := ps6080EnumSubject(pass, value.Cond, site.enum)
				site.subject = ps6080ResolvedSiteSubject(
					pass, nodeFunction, subject, value, site.enum, parents,
				)
				site.group = ps6080IfAlternativeGroup(pass, nodeFunction, site.enum, value, parents)
			}
		case *ast.CompositeLit:
			nodeBody := ps6080BodyForNode(body, invokedLiterals, parents, value)
			if !ps6080MapLiteralDispatched(pass, value, parents, nodeBody, invokedLiterals) {
				return true
			}
			typeOf := pass.TypesInfo.TypeOf(value)
			if typeOf == nil {
				return true
			}
			mapping, ok := types.Unalias(typeOf).Underlying().(*types.Map)
			if !ok {
				return true
			}
			callable := ps6080CallableType(mapping.Elem())
			groups := make(map[*types.TypeName]*ps6080ConstantGroup)
			for _, element := range value.Elts {
				if keyed, ok := element.(*ast.KeyValueExpr); ok {
					ps6080ExpressionConstants(
						pass, keyed.Key, groups, constantEnums,
						ps6080MapValueSupported(pass, keyed.Value, mapping.Elem(), false),
					)
				}
			}
			for enum := range groups {
				if !ps6080MapKeyCompatible(mapping.Key(), enum, groups[enum]) {
					delete(groups, enum)
				}
			}
			start := len(result)
			result = ps6080AppendGroupedSites(result, function, "map table", value.Pos(), value.End(), groups)
			for _, site := range result[start:] {
				site.mapTable = true
				site.callable = callable
			}
		case *ast.ReturnStmt:
			start := len(result)
			result = ps6080AppendGuardSites(pass, result, nodeFunction, "return guard", value.Return, value.End(), value.Results, constantEnums)
			if clause, switchClause := parents[value].(*ast.CaseClause); switchClause {
				body, _ := parents[clause].(*ast.BlockStmt)
				switchStmt, _ := parents[body].(*ast.SwitchStmt)
				added := slices.DeleteFunc(result[start:], func(site *ps6080Site) bool {
					if ps6080SwitchClauseDispatchesEnum(
						pass, switchStmt, clause, value.Results, site.enum,
					) {
						return true
					}
					var subject types.Object
					for _, expression := range value.Results {
						candidate, unique := ps6080EnumSubject(pass, expression, site.enum)
						if !unique {
							continue
						}
						if subject != nil && subject != candidate {
							return false
						}
						subject = candidate
					}
					if subject == nil {
						return false
					}
					objects := map[types.Object]bool{subject: true}
					for equivalent := range ps6080CPUEquivalentSubjects(
						pass, nodeFunction, subject, clause, site.enum, parents,
					) {
						objects[equivalent] = true
					}
					return ps6080SwitchDispatchMutatesObjects(
						pass, nodeFunction, clause, nil, parents, objects,
					)
				})
				result = append(result[:start], added...)
			}
			for _, site := range result[start:] {
				for _, expression := range value.Results {
					subject, unique := ps6080EnumSubject(pass, expression, site.enum)
					if unique {
						site.subject = ps6080ResolvedSiteSubject(
							pass, nodeFunction, subject, value, site.enum, parents,
						)
						break
					}
				}
				site.group = ps6080EnclosingElseAlternativeGroup(
					pass, nodeFunction, site.enum, value, parents,
				)
				if site.group == "" {
					site.group = ps6080FlowAlternativeGroup(pass, nodeFunction, site.enum, value, parents)
				}
				if site.group == "" {
					site.group = ps6080LiteralAlternativeGroup("return alternatives", value, parents)
				}
			}
		}
		return true
	})
	ps6080GroupShapeIfAlternatives(pass, result, body)
	ps6080AssignSingleParameterSiteSubjects(function, result)
	ps6080AssignLiteralSiteScopes(
		pass, function, result, parents, invoked, constantEnums, domains,
	)
	return result
}

func ps6080AssignLiteralSiteScopes(
	pass *analysis.Pass,
	function *ps6080Function,
	sites []*ps6080Site,
	parents map[ast.Node]ast.Node,
	invoked *ps6080InvokedLiteralResult,
	constantEnums map[*types.Const][]*types.TypeName,
	domains map[*types.TypeName][]*types.Const,
) {
	for _, site := range sites {
		if site.subject == nil || site.function.body == function.body {
			continue
		}
		literal := ps6080ContainingLiteral(site.function.body, parents)
		if literal == nil {
			continue
		}
		site.literalScope = ps6080CPUInvocationScope(
			pass, function, literal, site.subject, site.enum, parents, invoked,
			constantEnums, domains, make(map[*ast.FuncLit]bool),
		)
		aliases := ps6080ReturnedScopeObjects(
			pass, function, site.subject, site.function.body, site.enum, parents,
		)
		for current := ast.Node(site.function.body); current != nil; current = parents[current] {
			returnedLiteral, nested := current.(*ast.FuncLit)
			if !nested {
				continue
			}
			returnedScopes := function.returnedLiteralScopes[returnedLiteral]
			for _, alias := range aliases {
				site.literalScope = ps6080MergeCPUArgumentScopes(
					site.literalScope, returnedScopes[alias],
				)
			}
		}
	}
}

func ps6080AssignSingleParameterSiteSubjects(
	function *ps6080Function,
	sites []*ps6080Site,
) {
	if function.signature == nil {
		return
	}
	for _, site := range sites {
		if site.subject != nil || site.function.signature != function.signature {
			continue
		}
		var subject types.Object
		ambiguous := false
		consider := func(variable *types.Var) {
			if variable == nil {
				return
			}
			enum, enumType := ps6080EnumType(variable.Type())
			if !enumType || enum != site.enum {
				return
			}
			if subject != nil && subject != variable {
				ambiguous = true
				return
			}
			subject = variable
		}
		consider(function.signature.Recv())
		for index := range function.signature.Params().Len() {
			consider(function.signature.Params().At(index))
		}
		if !ambiguous {
			site.subject = subject
		}
	}
}

func ps6080AppendGuardSites(
	pass *analysis.Pass,
	result []*ps6080Site,
	function *ps6080Function,
	kind string,
	position token.Pos,
	end token.Pos,
	expressions []ast.Expr,
	constantEnums map[*types.Const][]*types.TypeName,
) []*ps6080Site {
	dominantFailure := false
	for index, expression := range expressions {
		if ps6080NilResultSupports(function, index) &&
			ps6080ZeroOrFailureExpression(pass, expression, true) {
			dominantFailure = true
			break
		}
	}
	if dominantFailure {
		groups := make(map[*types.TypeName]*ps6080ConstantGroup)
		for _, expression := range expressions {
			parsed, _ := ps6080GuardExpression(pass, expression, constantEnums)
			for enum := range parsed {
				groups[enum] = ps6080BooleanGroup(false)
			}
		}
		return ps6080AppendGroupedSites(result, function, kind, position, end, groups)
	}
	groups := make(map[*types.TypeName]*ps6080ConstantGroup)
	for index, expression := range expressions {
		parsed, _ := ps6080GuardExpression(pass, expression, constantEnums)
		if len(parsed) > 0 {
			ps6080MergeGuardGroups(groups, parsed, token.LOR)
			continue
		}
		if !ps6080ZeroOrFailureExpression(pass, expression, ps6080NilResultSupports(function, index)) {
			return result
		}
	}
	return ps6080AppendGroupedSites(result, function, kind, position, end, groups)
}

func ps6080GuardConstants(
	pass *analysis.Pass,
	expression ast.Expr,
	groups map[*types.TypeName]*ps6080ConstantGroup,
	constantEnums map[*types.Const][]*types.TypeName,
	supported bool,
) {
	parsed, _ := ps6080GuardExpression(pass, expression, constantEnums)
	if !supported {
		for enum, group := range parsed {
			parsed[enum] = ps6080ComplementGroup(group)
		}
	}
	ps6080MergeGuardGroups(groups, parsed, token.LOR)
}

func ps6080ExplicitGuardSubject(
	pass *analysis.Pass,
	expression ast.Expr,
	enum *types.TypeName,
) (types.Object, bool) {
	subject, identity := ps6080EnumSubject(pass, expression, enum)
	if !identity {
		return nil, false
	}
	safe := true
	ast.Inspect(expression, func(node ast.Node) bool {
		if !safe {
			return false
		}
		if _, call := node.(*ast.CallExpr); call {
			safe = false
			return false
		}
		identifier, identifierNode := node.(*ast.Ident)
		if !identifierNode {
			return true
		}
		variable, variableNode := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
		if !variableNode {
			return true
		}
		variableEnum, _ := ps6080EnumType(variable.Type())
		if variableEnum != enum || subject != variable {
			safe = false
			return false
		}
		return true
	})
	return subject, safe
}

func ps6080EnumSubject(
	pass *analysis.Pass,
	expression ast.Expr,
	enum *types.TypeName,
) (types.Object, bool) {
	expression = ps2110Unparen(expression)
	if identifier, direct := expression.(*ast.Ident); direct {
		variable, variableNode := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
		if !variableNode {
			return nil, false
		}
		variableEnum, _ := ps6080EnumType(variable.Type())
		return variable, variableEnum == enum
	}
	switch value := expression.(type) {
	case *ast.CallExpr:
		if len(value.Args) == 1 && pass.TypesInfo.Types[value.Fun].IsType() {
			convertedEnum, converted := ps6080EnumType(pass.TypesInfo.TypeOf(value))
			if converted && convertedEnum == enum {
				return ps6080EnumSubject(pass, value.Args[0], enum)
			}
		}
	case *ast.UnaryExpr:
		if value.Op == token.NOT {
			return ps6080EnumSubject(pass, value.X, enum)
		}
	case *ast.BinaryExpr:
		if value.Op != token.LAND && value.Op != token.LOR && value.Op != token.EQL && value.Op != token.NEQ {
			return nil, false
		}
		left, leftIdentity := ps6080EnumSubject(pass, value.X, enum)
		right, rightIdentity := ps6080EnumSubject(pass, value.Y, enum)
		if ps6080ExpressionDispatchesEnum(pass, value.X, enum) && !leftIdentity ||
			ps6080ExpressionDispatchesEnum(pass, value.Y, enum) && !rightIdentity {
			return nil, false
		}
		switch {
		case leftIdentity && rightIdentity && left != right:
			return nil, false
		case leftIdentity:
			return left, true
		case rightIdentity:
			return right, true
		}
	}
	return nil, false
}

func ps6080GuardRejections(
	pass *analysis.Pass,
	expression ast.Expr,
	groups map[*types.TypeName]*ps6080ConstantGroup,
	constantEnums map[*types.Const][]*types.TypeName,
) {
	parsed, _ := ps6080GuardExpression(pass, expression, constantEnums)
	for enum, rejected := range parsed {
		group := groups[enum]
		if group == nil {
			group = ps6080BooleanGroup(false)
			groups[enum] = group
		}
		if rejected.open {
			// An open rejected set does not provide a finite positive support
			// proof. Retain the layer, but keep it closed and conservative.
			continue
		}
		ps6080CopyConstants(group.excluded, rejected.included)
	}
}

func ps6080GuardExpression(
	pass *analysis.Pass,
	expression ast.Expr,
	constantEnums map[*types.Const][]*types.TypeName,
) (map[*types.TypeName]*ps6080ConstantGroup, *bool) {
	expression = ps2110Unparen(expression)
	if value := pass.TypesInfo.Types[expression].Value; value != nil && value.Kind() == constant.Bool {
		truth := constant.BoolVal(value)
		return nil, &truth
	}
	switch value := expression.(type) {
	case *ast.UnaryExpr:
		if value.Op == token.NOT {
			groups, truth := ps6080GuardExpression(pass, value.X, constantEnums)
			if truth != nil {
				inverted := !*truth
				return nil, &inverted
			}
			for enum, group := range groups {
				groups[enum] = ps6080ComplementGroup(group)
			}
			return groups, nil
		}
	case *ast.BinaryExpr:
		switch value.Op {
		case token.LAND, token.LOR:
			left, leftTruth := ps6080GuardExpression(pass, value.X, constantEnums)
			right, rightTruth := ps6080GuardExpression(pass, value.Y, constantEnums)
			return ps6080CombineGuardGroups(left, leftTruth, right, rightTruth, value.Op)
		case token.EQL, token.NEQ:
			groups := make(map[*types.TypeName]*ps6080ConstantGroup)
			ps6080ComparedConstant(pass, value.X, value.Y, groups, constantEnums, true)
			ps6080ComparedConstant(pass, value.Y, value.X, groups, constantEnums, true)
			if value.Op == token.NEQ {
				for enum, group := range groups {
					groups[enum] = ps6080ComplementGroup(group)
				}
			}
			return groups, nil
		}
	}
	return nil, nil
}

func ps6080CombineGuardGroups(
	left map[*types.TypeName]*ps6080ConstantGroup,
	leftTruth *bool,
	right map[*types.TypeName]*ps6080ConstantGroup,
	rightTruth *bool,
	operator token.Token,
) (map[*types.TypeName]*ps6080ConstantGroup, *bool) {
	if leftTruth != nil && rightTruth != nil {
		truth := *leftTruth && *rightTruth
		if operator == token.LOR {
			truth = *leftTruth || *rightTruth
		}
		return nil, &truth
	}
	result := make(map[*types.TypeName]*ps6080ConstantGroup, len(left)+len(right))
	for enum := range left {
		result[enum] = nil
	}
	for enum := range right {
		result[enum] = nil
	}
	for enum := range result {
		leftGroup := ps6080GuardOperand(left[enum], leftTruth, operator)
		rightGroup := ps6080GuardOperand(right[enum], rightTruth, operator)
		if operator == token.LAND {
			result[enum] = ps6080IntersectGroups(leftGroup, rightGroup)
		} else {
			result[enum] = ps6080UnionGroups(leftGroup, rightGroup)
		}
	}
	return result, nil
}

func ps6080GuardOperand(
	group *ps6080ConstantGroup,
	truth *bool,
	operator token.Token,
) *ps6080ConstantGroup {
	if group != nil {
		return group
	}
	if truth != nil {
		return ps6080BooleanGroup(*truth)
	}
	// A predicate unrelated to this enum is a shape/backend restriction for
	// conjunction and contributes no values to a disjunction.
	return ps6080BooleanGroup(operator == token.LAND)
}

func ps6080BooleanGroup(truth bool) *ps6080ConstantGroup {
	return &ps6080ConstantGroup{
		included: make(map[*types.Const]token.Pos),
		excluded: make(map[*types.Const]token.Pos),
		open:     truth,
	}
}

func ps6080ComplementGroup(group *ps6080ConstantGroup) *ps6080ConstantGroup {
	result := ps6080BooleanGroup(!group.open)
	if group.open {
		for constant, position := range group.excluded {
			result.included[constant] = position
		}
	} else {
		result.open = true
		for constant, position := range group.included {
			result.excluded[constant] = position
		}
	}
	return result
}

func ps6080UnionGroups(left, right *ps6080ConstantGroup) *ps6080ConstantGroup {
	result := ps6080BooleanGroup(left.open || right.open)
	switch {
	case !left.open && !right.open:
		ps6080CopyConstants(result.included, left.included)
		ps6080CopyConstants(result.included, right.included)
	case left.open && right.open:
		for constant, position := range left.excluded {
			if other, exists := right.excluded[constant]; exists {
				result.excluded[constant] = min(position, other)
			}
		}
	case left.open:
		ps6080CopyConstants(result.excluded, left.excluded)
		for constant := range right.included {
			delete(result.excluded, constant)
		}
	case right.open:
		ps6080CopyConstants(result.excluded, right.excluded)
		for constant := range left.included {
			delete(result.excluded, constant)
		}
	}
	return result
}

func ps6080IntersectGroups(left, right *ps6080ConstantGroup) *ps6080ConstantGroup {
	result := ps6080BooleanGroup(left.open && right.open)
	switch {
	case left.open && right.open:
		ps6080CopyConstants(result.excluded, left.excluded)
		ps6080CopyConstants(result.excluded, right.excluded)
	case !left.open && !right.open:
		for constant, position := range left.included {
			if other, exists := right.included[constant]; exists {
				result.included[constant] = min(position, other)
			}
		}
	case left.open:
		ps6080CopyConstants(result.included, right.included)
		for constant := range left.excluded {
			delete(result.included, constant)
		}
	case right.open:
		ps6080CopyConstants(result.included, left.included)
		for constant := range right.excluded {
			delete(result.included, constant)
		}
	}
	return result
}

func ps6080MergeGuardGroups(
	destination map[*types.TypeName]*ps6080ConstantGroup,
	source map[*types.TypeName]*ps6080ConstantGroup,
	operator token.Token,
) {
	for enum, group := range source {
		if destination[enum] == nil {
			destination[enum] = group
			continue
		}
		if operator == token.LAND {
			destination[enum] = ps6080IntersectGroups(destination[enum], group)
		} else {
			destination[enum] = ps6080UnionGroups(destination[enum], group)
		}
	}
}

func ps6080CopyConstants(destination, source map[*types.Const]token.Pos) {
	for constant, position := range source {
		if old, exists := destination[constant]; !exists || position < old {
			destination[constant] = position
		}
	}
}

func ps6080UnionScopes(
	left map[*types.Const]token.Pos,
	right map[*types.Const]token.Pos,
) map[*types.Const]token.Pos {
	result := make(map[*types.Const]token.Pos)
	if len(left) == 0 || len(right) == 0 {
		return result
	}
	ps6080CopyConstants(result, left)
	ps6080CopyConstants(result, right)
	return result
}

func ps6080StatementsSupport(pass *analysis.Pass, function *ps6080Function, statements []ast.Stmt) bool {
	_, supported := ps6080StatementOutcomes(pass, function, statements)
	return supported
}

func ps6080StatementOutcomes(
	pass *analysis.Pass,
	function *ps6080Function,
	statements []ast.Stmt,
) (rejected, supported bool) {
	if len(statements) == 0 {
		return false, false
	}
	block := &ast.BlockStmt{Lbrace: statements[0].Pos() - 1, List: statements, Rbrace: statements[len(statements)-1].End()}
	parents := ps6071Parents(block)
	graph := cfg.New(block, ps6080CallMayReturn(pass))
	for _, statement := range statements {
		ast.Inspect(statement, func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			if node != nil && !ps6080NodeReachable(pass, graph, parents, node) {
				return true
			}
			switch value := node.(type) {
			case *ast.ReturnStmt:
				returnRejected := ps6080NakedReturnRejected(pass, function, value)
				if len(value.Results) > 0 {
					returnRejected = ps6080ExplicitReturnRejected(pass, function, value)
				}
				if returnRejected {
					rejected = true
				} else {
					supported = true
				}
				return false
			case *ast.ExprStmt:
				call, ok := ps2110Unparen(value.X).(*ast.CallExpr)
				if ok {
					if ps6080BuiltinPanic(pass, call) {
						rejected = true
					}
				}
			}
			return true
		})
	}
	return rejected, supported
}

func ps6080ExplicitReturnRejected(
	pass *analysis.Pass,
	function *ps6080Function,
	returned *ast.ReturnStmt,
) bool {
	states, resolved := ps6080ResolvedReturnStates(pass, function, returned)
	if !resolved {
		return false
	}
	return ps6080ReturnStatesRejected(pass, function, states)
}

func ps6080ExplicitReturnStates(
	pass *analysis.Pass,
	signature *types.Signature,
	returned *ast.ReturnStmt,
) ([]ps6080NamedResultState, bool) {
	if signature == nil || returned == nil || len(returned.Results) == 0 {
		return nil, false
	}
	resultCount := signature.Results().Len()
	states := make([]ps6080NamedResultState, resultCount)
	if len(returned.Results) == resultCount {
		for index, expression := range returned.Results {
			states[index] = ps6080NamedResultState{
				expression: expression, resultIndex: -1, known: true,
			}
		}
		return states, true
	}
	if len(returned.Results) != 1 {
		return nil, false
	}
	tuple, tupleResult := pass.TypesInfo.TypeOf(returned.Results[0]).(*types.Tuple)
	if !tupleResult || tuple.Len() != resultCount {
		return nil, false
	}
	for index := range resultCount {
		states[index] = ps6080NamedResultState{
			expression: returned.Results[0], resultIndex: index, known: true,
		}
	}
	return states, true
}

func ps6080ResolvedReturnStates(
	pass *analysis.Pass,
	function *ps6080Function,
	returned *ast.ReturnStmt,
) ([]ps6080NamedResultState, bool) {
	return ps6080ResolvedReturnStatesWithIndirect(
		pass, function, returned, ps6080NamedResultsMayChangeIndirectlyCached(pass, function),
	)
}

func ps6080ResolvedReturnStatesWithIndirect(
	pass *analysis.Pass,
	function *ps6080Function,
	returned *ast.ReturnStmt,
	indirect bool,
) ([]ps6080NamedResultState, bool) {
	signature := ps6080FunctionSignature(function)
	if signature == nil || returned == nil {
		return nil, false
	}
	if len(returned.Results) == 0 {
		if indirect {
			return nil, false
		}
		return ps6080NamedReturnState(pass, function, returned)
	}
	states, expanded := ps6080ExplicitReturnStates(pass, signature, returned)
	if !expanded || indirect {
		return states, expanded
	}
	named := make(map[types.Object]int, signature.Results().Len())
	for resultIndex := range signature.Results().Len() {
		result := signature.Results().At(resultIndex)
		if result.Name() != "" {
			named[result] = resultIndex
		}
	}
	needsNamedState := false
	for _, state := range states {
		if state.resultIndex >= 0 {
			continue
		}
		identifier, direct := ps2110Unparen(state.expression).(*ast.Ident)
		if !direct {
			continue
		}
		if _, found := named[pass.TypesInfo.ObjectOf(identifier)]; found {
			needsNamedState = true
			break
		}
	}
	if !needsNamedState {
		return states, true
	}
	namedState, resolved := ps6080NamedReturnState(pass, function, returned)
	if !resolved {
		return states, true
	}
	for resultIndex, state := range states {
		if state.resultIndex >= 0 {
			continue
		}
		identifier, direct := ps2110Unparen(state.expression).(*ast.Ident)
		if !direct {
			continue
		}
		if namedIndex, found := named[pass.TypesInfo.ObjectOf(identifier)]; found {
			states[resultIndex] = namedState[namedIndex]
		}
	}
	return states, true
}

func ps6080SwitchClauseSupported(
	pass *analysis.Pass,
	function *ps6080Function,
	graph *cfg.CFG,
	switchStmt *ast.SwitchStmt,
	clause *ast.CaseClause,
	parents map[ast.Node]ast.Node,
) bool {
	assumptions := ps6080SwitchClauseAssumptions(pass, switchStmt, clause)
	return ps6080CFGClauseSupports(pass, function, graph, clause, parents, assumptions)
}

func ps6080SwitchClauseAssumptions(
	pass *analysis.Pass,
	switchStmt *ast.SwitchStmt,
	clause *ast.CaseClause,
) map[types.Object]*types.Const {
	if switchStmt == nil || switchStmt.Tag == nil || clause == nil || len(clause.List) != 1 {
		return nil
	}
	return ps6080SwitchExpressionAssumptions(pass, switchStmt, clause.List[0])
}

func ps6080SwitchExpressionAssumptions(
	pass *analysis.Pass,
	switchStmt *ast.SwitchStmt,
	expression ast.Expr,
) map[types.Object]*types.Const {
	if switchStmt == nil || switchStmt.Tag == nil || expression == nil {
		return nil
	}
	constant := ps6080AliasConstant(pass, expression)
	if constant == nil {
		return nil
	}
	enum, ok := ps6080EnumType(constant.Type())
	if !ok {
		return nil
	}
	subject, unique := ps6080EnumSubject(pass, switchStmt.Tag, enum)
	if !unique {
		return nil
	}
	return map[types.Object]*types.Const{subject: constant}
}

func ps6080ExpressionTruth(
	pass *analysis.Pass,
	expression ast.Expr,
	assumptions map[types.Object]*types.Const,
) (bool, bool) {
	expression = ps2110Unparen(expression)
	if value := pass.TypesInfo.Types[expression].Value; value != nil && value.Kind() == constant.Bool {
		return constant.BoolVal(value), true
	}
	switch value := expression.(type) {
	case *ast.UnaryExpr:
		if value.Op != token.NOT {
			return false, false
		}
		truth, known := ps6080ExpressionTruth(pass, value.X, assumptions)
		return !truth, known
	case *ast.BinaryExpr:
		switch value.Op {
		case token.LAND, token.LOR:
			left, leftKnown := ps6080ExpressionTruth(pass, value.X, assumptions)
			if leftKnown && (value.Op == token.LAND && !left || value.Op == token.LOR && left) {
				return left, true
			}
			right, rightKnown := ps6080ExpressionTruth(pass, value.Y, assumptions)
			if leftKnown && rightKnown {
				if value.Op == token.LAND {
					return left && right, true
				}
				return left || right, true
			}
			return false, false
		case token.EQL, token.NEQ:
			left, leftKnown := ps6080AssumedConstant(pass, value.X, assumptions)
			right, rightKnown := ps6080AssumedConstant(pass, value.Y, assumptions)
			if !leftKnown || !rightKnown {
				return false, false
			}
			equal := constant.Compare(left.Val(), token.EQL, right.Val())
			if value.Op == token.NEQ {
				equal = !equal
			}
			return equal, true
		}
	}
	return false, false
}

func ps6080AssumedConstant(
	pass *analysis.Pass,
	expression ast.Expr,
	assumptions map[types.Object]*types.Const,
) (*types.Const, bool) {
	expression = ps2110Unparen(expression)
	if constant := ps6080AliasConstant(pass, expression); constant != nil {
		return constant, true
	}
	var object types.Object
	switch value := expression.(type) {
	case *ast.Ident:
		object = pass.TypesInfo.ObjectOf(value)
	case *ast.SelectorExpr:
		object = pass.TypesInfo.ObjectOf(value.Sel)
	case *ast.CallExpr:
		if len(value.Args) == 1 && pass.TypesInfo.Types[value.Fun].IsType() {
			return ps6080AssumedConstant(pass, value.Args[0], assumptions)
		}
	}
	constant := assumptions[object]
	return constant, constant != nil
}

func ps6080CFGClauseSupports(
	pass *analysis.Pass,
	function *ps6080Function,
	graph *cfg.CFG,
	clause *ast.CaseClause,
	parents map[ast.Node]ast.Node,
	assumptions map[types.Object]*types.Const,
) bool {
	return ps6080CFGClauseExpressionSupports(
		pass, function, graph, clause, parents, assumptions, nil,
	)
}

func ps6080CFGClauseExpressionSupports(
	pass *analysis.Pass,
	function *ps6080Function,
	graph *cfg.CFG,
	clause *ast.CaseClause,
	parents map[ast.Node]ast.Node,
	assumptions map[types.Object]*types.Const,
	matched ast.Expr,
) bool {
	if graph == nil || clause == nil {
		return true
	}
	assumptions = ps6080ExpandClauseAssumptions(
		pass, function, clause, parents, assumptions,
	)
	if ps6080SwitchDispatchMutatesAssumptions(
		pass, function, clause, matched, parents, assumptions,
	) {
		assumptions = nil
	}
	var entry *cfg.Block
	for _, block := range graph.Blocks {
		if block.Kind == cfg.KindSwitchCaseBody && block.Stmt == clause {
			entry = block
			break
		}
	}
	if entry == nil {
		return true
	}
	type state struct {
		block  *cfg.Block
		scoped bool
	}
	initial := state{block: entry, scoped: len(assumptions) > 0}
	seen := map[state]bool{initial: true}
	queue := []state{initial}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.block == nil || !current.block.Live {
			continue
		}
		active := assumptions
		if !current.scoped || ps6080CFGBlockMutatesAssumptions(
			pass, function, current.block, parents, assumptions,
		) {
			active = nil
			current.scoped = false
		}
		if returned := current.block.Return(); returned != nil {
			if ps6080CFGReturnSupports(pass, function, returned, active) {
				return true
			}
			continue
		}
		for _, successor := range ps6080FeasibleSuccessorsWithAssumptions(
			pass, parents, current.block, active,
		) {
			next := state{block: successor, scoped: current.scoped}
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return false
}

func ps6080SwitchDispatchMutatesAssumptions(
	pass *analysis.Pass,
	function *ps6080Function,
	clause *ast.CaseClause,
	matched ast.Expr,
	parents map[ast.Node]ast.Node,
	assumptions map[types.Object]*types.Const,
) bool {
	if clause == nil || len(assumptions) == 0 {
		return false
	}
	assumptions = ps6080ExpandClauseAssumptions(
		pass, function, clause, parents, assumptions,
	)
	objects := make(map[types.Object]bool, len(assumptions))
	for object := range assumptions {
		objects[object] = true
	}
	return ps6080SwitchDispatchMutatesObjects(
		pass, function, clause, matched, parents, objects,
	)
}

func ps6080SwitchDispatchMutatesObjects(
	pass *analysis.Pass,
	function *ps6080Function,
	clause *ast.CaseClause,
	matched ast.Expr,
	parents map[ast.Node]ast.Node,
	objects map[types.Object]bool,
) bool {
	if clause == nil || len(objects) == 0 {
		return false
	}
	body, _ := parents[clause].(*ast.BlockStmt)
	switchStmt, _ := parents[body].(*ast.SwitchStmt)
	if switchStmt == nil {
		return false
	}
	mutation := ps6080MutationAnalysis{
		pass: pass, objects: objects,
		cache:    make(map[ast.Node]ps6080MutationSummary),
		visiting: make(map[ast.Node]bool),
	}
	defaultTarget := len(clause.List) == 0
	for _, statement := range switchStmt.Body.List {
		candidate, ok := statement.(*ast.CaseClause)
		if !ok || len(candidate.List) == 0 {
			continue
		}
		for _, expression := range candidate.List {
			if mutation.nodeSummary(function, expression, parents) != ps6080MutationSafe {
				return true
			}
			if candidate == clause && matched != nil && expression == matched {
				return false
			}
		}
		if candidate == clause && !defaultTarget {
			return false
		}
	}
	return false
}

func ps6080ExpandClauseAssumptions(
	pass *analysis.Pass,
	function *ps6080Function,
	clause *ast.CaseClause,
	parents map[ast.Node]ast.Node,
	assumptions map[types.Object]*types.Const,
) map[types.Object]*types.Const {
	if len(assumptions) == 0 {
		return assumptions
	}
	result := make(map[types.Object]*types.Const, len(assumptions))
	for subject, constant := range assumptions {
		result[subject] = constant
		enum, ok := ps6080EnumType(constant.Type())
		if !ok {
			continue
		}
		for equivalent := range ps6080CPUEquivalentSubjects(
			pass, function, subject, clause, enum, parents,
		) {
			result[equivalent] = constant
		}
	}
	return result
}

func ps6080CFGBlockMutatesAssumptions(
	pass *analysis.Pass,
	function *ps6080Function,
	block *cfg.Block,
	parents map[ast.Node]ast.Node,
	assumptions map[types.Object]*types.Const,
) bool {
	if block == nil || len(assumptions) == 0 {
		return false
	}
	objects := make(map[types.Object]bool, len(assumptions))
	for object := range assumptions {
		objects[object] = true
	}
	mutation := ps6080MutationAnalysis{
		pass: pass, objects: objects,
		cache:    make(map[ast.Node]ps6080MutationSummary),
		visiting: make(map[ast.Node]bool),
	}
	for _, node := range block.Nodes {
		if mutation.nodeSummary(function, node, parents) != ps6080MutationSafe {
			return true
		}
	}
	return false
}

type ps6080MutationSummary uint8

const (
	ps6080MutationSafe ps6080MutationSummary = iota
	ps6080MutationUnknown
	ps6080MutationWrites
)

type ps6080MutationAnalysis struct {
	pass     *analysis.Pass
	objects  map[types.Object]bool
	cache    map[ast.Node]ps6080MutationSummary
	visiting map[ast.Node]bool
}

func (mutation *ps6080MutationAnalysis) nodeSummary(
	function *ps6080Function,
	node ast.Node,
	parents map[ast.Node]ast.Node,
) ps6080MutationSummary {
	if node == nil || len(mutation.objects) == 0 {
		return ps6080MutationSafe
	}
	if ps6080NodeMayMutateObjectsOutsideLiterals(
		mutation.pass, node, mutation.objects,
	) || ps6080NodeIndirectlyMutatesObjectsOutsideLiterals(
		mutation.pass, node, mutation.objects,
	) {
		return ps6080MutationWrites
	}
	summary := ps6080MutationSafe
	ast.Inspect(node, func(candidate ast.Node) bool {
		if candidate == nil || summary == ps6080MutationWrites {
			return false
		}
		if _, nested := candidate.(*ast.FuncLit); nested {
			return false
		}
		call, ok := candidate.(*ast.CallExpr)
		if !ok {
			return true
		}
		callSummary := mutation.callSummary(function, call, parents)
		if callSummary > summary {
			summary = callSummary
		}
		return summary != ps6080MutationWrites
	})
	return summary
}

func (mutation *ps6080MutationAnalysis) callSummary(
	function *ps6080Function,
	call *ast.CallExpr,
	parents map[ast.Node]ast.Node,
) ps6080MutationSummary {
	if call == nil {
		return ps6080MutationUnknown
	}
	if identifier, builtin := ps2110Unparen(call.Fun).(*ast.Ident); builtin {
		if _, ok := mutation.pass.TypesInfo.ObjectOf(identifier).(*types.Builtin); ok {
			return ps6080MutationSafe
		}
	}
	if target, named := ps6080StaticNamedCallee(
		mutation.pass, function, call, parents,
	); named {
		return mutation.namedCallSummary(function, call, target, parents)
	}
	return mutation.callableSummary(
		function, call.Fun, call, parents, make(map[types.Object]bool),
	)
}

func (mutation *ps6080MutationAnalysis) callableSummary(
	function *ps6080Function,
	expression ast.Expr,
	query ast.Node,
	parents map[ast.Node]ast.Node,
	seen map[types.Object]bool,
) ps6080MutationSummary {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.FuncLit:
		literalFunction := &ps6080Function{
			body: value.Body,
		}
		literalFunction.signature, _ = mutation.pass.TypesInfo.TypeOf(value).(*types.Signature)
		return mutation.bodySummary(value, literalFunction, value.Body, parents)
	case *ast.CallExpr:
		if len(value.Args) == 1 && mutation.pass.TypesInfo.Types[value.Fun].IsType() {
			return mutation.callableSummary(function, value.Args[0], query, parents, seen)
		}
	}
	identifier, direct := expression.(*ast.Ident)
	if !direct {
		return ps6080MutationUnknown
	}
	variable, local := mutation.pass.TypesInfo.ObjectOf(identifier).(*types.Var)
	if !local || seen[variable] {
		return ps6080MutationUnknown
	}
	seen[variable] = true
	initializer := ps6080StableLocalInitializer(
		mutation.pass, function, variable, query, parents,
	)
	if initializer == nil {
		return ps6080MutationUnknown
	}
	return mutation.callableSummary(function, initializer, query, parents, seen)
}

func (mutation *ps6080MutationAnalysis) namedCallSummary(
	caller *ps6080Function,
	call *ast.CallExpr,
	target ps6080NamedFunctionTarget,
	parents map[ast.Node]ast.Node,
) ps6080MutationSummary {
	if target.function == nil {
		return ps6080MutationUnknown
	}
	calleeObjects, aliasesPassed := mutation.namedCalleeObjects(
		caller, call, target, parents,
	)
	if target.function.Pkg() != mutation.pass.Pkg {
		if aliasesPassed {
			return ps6080MutationUnknown
		}
		return ps6080MutationSafe
	}
	declaration := ps6080NamedFunctionDeclaration(mutation.pass, target.function)
	if declaration == nil || declaration.Body == nil {
		return ps6080MutationUnknown
	}
	calleeFunction := &ps6080Function{
		declaration: declaration,
		object:      target.function,
		body:        declaration.Body,
	}
	calleeFunction.signature, _ = target.function.Type().(*types.Signature)
	child := ps6080MutationAnalysis{
		pass: mutation.pass, objects: calleeObjects,
		cache:    make(map[ast.Node]ps6080MutationSummary),
		visiting: mutation.visiting,
	}
	return child.bodySummary(
		declaration, calleeFunction, declaration.Body, ps6071Parents(declaration.Body),
	)
}

func (mutation *ps6080MutationAnalysis) namedCalleeObjects(
	caller *ps6080Function,
	call *ast.CallExpr,
	target ps6080NamedFunctionTarget,
	parents map[ast.Node]ast.Node,
) (map[types.Object]bool, bool) {
	result := make(map[types.Object]bool)
	for object := range mutation.objects {
		variable, variableObject := object.(*types.Var)
		if object.Pkg() != nil && object.Parent() == object.Pkg().Scope() ||
			variableObject && variable.IsField() {
			result[object] = true
		}
	}
	signature, _ := target.function.Type().(*types.Signature)
	if signature == nil {
		return result, false
	}
	aliasesPassed := false
	bind := func(parameter types.Object, expression ast.Expr) {
		if parameter == nil || expression == nil || mutation.expressionAliasSummary(
			caller, expression, call, parents, make(map[types.Object]bool),
		) == ps6080AliasDisjoint {
			return
		}
		result[parameter] = true
		aliasesPassed = true
	}
	var formals []types.Object
	if receiver := signature.Recv(); receiver != nil {
		switch {
		case target.receiver != nil:
			bind(receiver, target.receiver)
		case target.methodExpression:
			formals = append(formals, receiver)
		}
	}
	parameters := signature.Params()
	for index := range parameters.Len() {
		formals = append(formals, parameters.At(index))
	}
	formalAt := func(index int) types.Object {
		if index < len(formals) {
			return formals[index]
		}
		if signature.Variadic() && len(formals) > 0 {
			return formals[len(formals)-1]
		}
		return nil
	}
	if len(call.Args) == 1 {
		if tuple, tupleResult := mutation.pass.TypesInfo.TypeOf(call.Args[0]).(*types.Tuple); tupleResult {
			for index := range tuple.Len() {
				parameter := formalAt(index)
				if parameter == nil || !ps6080TypeMayCarryReference(
					tuple.At(index).Type(), make(map[types.Type]bool),
				) {
					continue
				}
				result[parameter] = true
				aliasesPassed = true
			}
			return result, aliasesPassed
		}
	}
	for index, argument := range call.Args {
		bind(formalAt(index), argument)
	}
	return result, aliasesPassed
}

type ps6080AliasSummary uint8

const (
	ps6080AliasDisjoint ps6080AliasSummary = iota
	ps6080AliasUnknown
	ps6080AliasMatches
)

func (mutation *ps6080MutationAnalysis) expressionAliasSummary(
	function *ps6080Function,
	expression ast.Expr,
	query ast.Node,
	parents map[ast.Node]ast.Node,
	seen map[types.Object]bool,
) ps6080AliasSummary {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			if ps6080ExpressionReferencesObjects(mutation.pass, value.X, mutation.objects) {
				return ps6080AliasMatches
			}
			return ps6080AliasDisjoint
		}
	case *ast.CallExpr:
		if len(value.Args) == 1 && mutation.pass.TypesInfo.Types[value.Fun].IsType() {
			return mutation.expressionAliasSummary(function, value.Args[0], query, parents, seen)
		}
	case *ast.Ident:
		object := mutation.pass.TypesInfo.ObjectOf(value)
		if mutation.objects[object] {
			typeOf := mutation.pass.TypesInfo.TypeOf(value)
			if typeOf == nil {
				return ps6080AliasUnknown
			}
			if ps6080TypeMayCarryReference(typeOf, make(map[types.Type]bool)) {
				return ps6080AliasMatches
			}
			return ps6080AliasDisjoint
		}
		variable, local := object.(*types.Var)
		if !local || seen[variable] {
			return ps6080ExpressionAliasFallback(mutation.pass, expression)
		}
		seen[variable] = true
		initializer := ps6080StableLocalInitializer(
			mutation.pass, function, variable, query, parents,
		)
		if initializer == nil {
			return ps6080ExpressionAliasFallback(mutation.pass, expression)
		}
		return mutation.expressionAliasSummary(function, initializer, query, parents, seen)
	}
	return ps6080ExpressionAliasFallback(mutation.pass, expression)
}

func ps6080ExpressionAliasFallback(pass *analysis.Pass, expression ast.Expr) ps6080AliasSummary {
	if ps6080TypeMayCarryReference(pass.TypesInfo.TypeOf(expression), make(map[types.Type]bool)) {
		return ps6080AliasUnknown
	}
	return ps6080AliasDisjoint
}

func ps6080TypeMayCarryReference(typeOf types.Type, visiting map[types.Type]bool) bool {
	if typeOf == nil || visiting[typeOf] {
		return false
	}
	visiting[typeOf] = true
	defer delete(visiting, typeOf)
	switch value := types.Unalias(typeOf).(type) {
	case *types.Pointer, *types.Signature, *types.Interface, *types.Slice,
		*types.Map, *types.Chan, *types.TypeParam:
		return true
	case *types.Named:
		return ps6080TypeMayCarryReference(value.Underlying(), visiting)
	case *types.Array:
		return ps6080TypeMayCarryReference(value.Elem(), visiting)
	case *types.Struct:
		for index := range value.NumFields() {
			if ps6080TypeMayCarryReference(value.Field(index).Type(), visiting) {
				return true
			}
		}
	case *types.Basic:
		return value.Kind() == types.UnsafePointer || value.Kind() == types.Uintptr
	}
	return false
}

func ps6080ExpressionReferencesObjects(
	pass *analysis.Pass,
	expression ast.Expr,
	objects map[types.Object]bool,
) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if node == nil || found {
			return false
		}
		switch value := node.(type) {
		case *ast.Ident:
			found = objects[pass.TypesInfo.ObjectOf(value)]
		case *ast.SelectorExpr:
			found = objects[pass.TypesInfo.ObjectOf(value.Sel)]
		}
		return !found
	})
	return found
}

func ps6080NodeIndirectlyMutatesObjectsOutsideLiterals(
	pass *analysis.Pass,
	node ast.Node,
	objects map[types.Object]bool,
) bool {
	mutates := false
	check := func(expression ast.Expr) {
		if expression == nil || mutates {
			return
		}
		if _, direct := ps2110Unparen(expression).(*ast.Ident); direct {
			return
		}
		if selector, selected := ps2110Unparen(expression).(*ast.SelectorExpr); selected &&
			objects[pass.TypesInfo.ObjectOf(selector.Sel)] {
			mutates = true
			return
		}
		mutates = objects[ps6080ExpressionRootObject(pass, expression)]
	}
	ast.Inspect(node, func(candidate ast.Node) bool {
		if candidate == nil || mutates {
			return false
		}
		if _, nested := candidate.(*ast.FuncLit); nested {
			return false
		}
		switch value := candidate.(type) {
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				check(left)
			}
		case *ast.IncDecStmt:
			check(value.X)
		case *ast.RangeStmt:
			check(value.Key)
			check(value.Value)
		}
		return !mutates
	})
	return mutates
}

func (mutation *ps6080MutationAnalysis) bodySummary(
	identity ast.Node,
	function *ps6080Function,
	body *ast.BlockStmt,
	parents map[ast.Node]ast.Node,
) ps6080MutationSummary {
	if identity == nil || body == nil {
		return ps6080MutationUnknown
	}
	if summary, ok := mutation.cache[identity]; ok {
		return summary
	}
	if mutation.visiting[identity] {
		return ps6080MutationUnknown
	}
	mutation.visiting[identity] = true
	summary := mutation.nodeSummary(function, body, parents)
	delete(mutation.visiting, identity)
	mutation.cache[identity] = summary
	return summary
}

func ps6080FeasibleSuccessorsWithAssumptions(
	pass *analysis.Pass,
	parents map[ast.Node]ast.Node,
	block *cfg.Block,
	assumptions map[types.Object]*types.Const,
) []*cfg.Block {
	if block == nil {
		return nil
	}
	feasible := ps6080FeasibleSuccessors(pass, parents, block)
	if len(feasible) != 2 || len(block.Succs) != 2 || len(block.Nodes) == 0 {
		return feasible
	}
	condition, ok := block.Nodes[len(block.Nodes)-1].(ast.Expr)
	if !ok {
		return feasible
	}
	if truth, known := ps6080ExpressionTruth(pass, condition, assumptions); known {
		index := 1
		if truth {
			index = 0
		}
		return block.Succs[index : index+1]
	}
	clause, inSwitch := parents[condition].(*ast.CaseClause)
	if !inSwitch {
		return feasible
	}
	switchBody, _ := parents[clause].(*ast.BlockStmt)
	switchStmt, _ := parents[switchBody].(*ast.SwitchStmt)
	if switchStmt == nil || switchStmt.Tag == nil {
		return feasible
	}
	tag, tagKnown := ps6080AssumedConstant(pass, switchStmt.Tag, assumptions)
	candidate := ps6080AliasConstant(pass, condition)
	if !tagKnown || candidate == nil {
		return feasible
	}
	index := 1
	if constant.Compare(tag.Val(), token.EQL, candidate.Val()) {
		index = 0
	}
	return block.Succs[index : index+1]
}

func ps6080CFGReturnSupports(
	pass *analysis.Pass,
	function *ps6080Function,
	returned *ast.ReturnStmt,
	assumptions map[types.Object]*types.Const,
) bool {
	signature := ps6080FunctionSignature(function)
	if signature == nil {
		return false
	}
	if len(returned.Results) == 0 && signature.Results().Len() == 0 {
		return true
	}
	if !ps6080ReturnAlwaysSupports(pass, function, returned, assumptions) {
		return false
	}
	return true
}

func ps6080ReturnAlwaysSupports(
	pass *analysis.Pass,
	function *ps6080Function,
	returned *ast.ReturnStmt,
	assumptions map[types.Object]*types.Const,
) bool {
	signature := ps6080FunctionSignature(function)
	if signature == nil {
		return false
	}
	results := returned.Results
	if len(results) == 0 {
		if ps6080NakedReturnRejected(pass, function, returned) {
			return false
		}
		if ps6080NamedResultsMayChangeIndirectlyCached(pass, function) {
			return true
		}
		var resolved bool
		results, resolved = ps6080NakedReturnResults(pass, function, returned)
		if !resolved {
			return true
		}
	} else if ps6080ExplicitReturnRejected(pass, function, returned) {
		return false
	}
	allFailure := true
	for index, expression := range results {
		if index >= signature.Results().Len() {
			return false
		}
		basic, ok := types.Unalias(signature.Results().At(index).Type()).Underlying().(*types.Basic)
		if ok && basic.Info()&types.IsBoolean != 0 {
			if truth, known := ps6080ExpressionTruth(pass, expression, assumptions); known {
				if truth {
					return true
				}
				continue
			}
		}
		if !ps6080ZeroOrFailureExpression(pass, expression, ps6080NilResultSupports(function, index)) {
			allFailure = false
		}
	}
	return !allFailure
}

func ps6080StatementsAfterSwitchSupport(
	pass *analysis.Pass,
	function *ps6080Function,
	switchStmt *ast.SwitchStmt,
	parents map[ast.Node]ast.Node,
) bool {
	block, ok := parents[switchStmt].(*ast.BlockStmt)
	if !ok {
		return false
	}
	index := slices.Index(block.List, ast.Stmt(switchStmt))
	if index < 0 {
		return false
	}
	if index+1 >= len(block.List) {
		signature := ps6080FunctionSignature(function)
		return signature != nil && signature.Results().Len() == 0 && block == ps6080FunctionBody(function)
	}
	return ps6080StatementsSupport(pass, function, block.List[index+1:])
}

func ps6080IfFallbackAlwaysSupports(
	pass *analysis.Pass,
	function *ps6080Function,
	conditional *ast.IfStmt,
	parents map[ast.Node]ast.Node,
) bool {
	current := conditional
	for {
		next, nested := current.Else.(*ast.IfStmt)
		if !nested {
			break
		}
		current = next
	}
	if fallback, ok := current.Else.(*ast.BlockStmt); ok {
		return len(fallback.List) == 1 && ps6080StatementAlwaysSupports(pass, function, fallback.List[0])
	}
	block, ok := parents[conditional].(*ast.BlockStmt)
	if !ok {
		return false
	}
	index := slices.Index(block.List, ast.Stmt(conditional))
	return index >= 0 && index+2 == len(block.List) &&
		ps6080StatementAlwaysSupports(pass, function, block.List[index+1])
}

func ps6080IfContinuationOutcomes(
	pass *analysis.Pass,
	function *ps6080Function,
	conditional *ast.IfStmt,
	condition ast.Expr,
	guardBody []ast.Stmt,
	parents map[ast.Node]ast.Node,
) (rejected, supported, terminates bool) {
	continuation, reachesBodyEnd := ps6080IfContinuationStatements(function, conditional, parents)
	mayFallThrough, fallthroughResolved := true, true
	if len(continuation) > 0 {
		if correlated, matched := ps6080CorrelatedContinuationOutcomes(
			pass, function, condition, guardBody, continuation,
		); matched {
			return correlated.rejected, correlated.supported, true
		}
		rejected, supported = ps6080StatementOutcomes(pass, function, continuation)
		mayFallThrough, fallthroughResolved = ps6080StatementsMayFallThrough(pass, continuation)
		terminates = fallthroughResolved && !mayFallThrough
	}
	signature := ps6080FunctionSignature(function)
	if fallthroughResolved && mayFallThrough && reachesBodyEnd && signature != nil && signature.Results().Len() == 0 {
		supported = true
		terminates = true
	}
	return rejected, supported, terminates
}

func ps6080IfContinuationStatements(
	function *ps6080Function,
	conditional *ast.IfStmt,
	parents map[ast.Node]ast.Node,
) ([]ast.Stmt, bool) {
	body := ps6080FunctionBody(function)
	var continuation []ast.Stmt
	var current ast.Stmt = conditional
	for {
		block, ok := parents[current].(*ast.BlockStmt)
		if !ok {
			return continuation, false
		}
		index := slices.Index(block.List, current)
		if index < 0 {
			return continuation, false
		}
		continuation = append(continuation, block.List[index+1:]...)
		if block == body {
			return continuation, true
		}
		if _, bare := parents[block].(*ast.BlockStmt); !bare {
			return continuation, false
		}
		current = block
	}
}

type ps6080Outcomes struct {
	rejected  bool
	supported bool
}

func ps6080CorrelatedContinuationOutcomes(
	pass *analysis.Pass,
	function *ps6080Function,
	condition ast.Expr,
	guardBody []ast.Stmt,
	continuation []ast.Stmt,
) (ps6080Outcomes, bool) {
	if !ps6080CorrelationStable(pass, function, condition, guardBody) {
		return ps6080Outcomes{}, false
	}
	conditional, ok := continuation[0].(*ast.IfStmt)
	for ok && conditional.Init == nil {
		switch {
		case ps6080EquivalentExpressions(pass, condition, conditional.Cond):
			if !ps6080StatementsTerminate(pass, conditional.Body.List) {
				return ps6080Outcomes{}, false
			}
			rejected, supported := ps6080StatementOutcomes(pass, function, conditional.Body.List)
			return ps6080Outcomes{rejected: rejected, supported: supported}, true
		case ps6080MutuallyExclusiveEqualities(pass, condition, conditional.Cond):
			conditional, ok = conditional.Else.(*ast.IfStmt)
		default:
			return ps6080Outcomes{}, false
		}
	}
	return ps6080Outcomes{}, false
}

func ps6080EquivalentExpressions(pass *analysis.Pass, left, right ast.Expr) bool {
	left = ps2110Unparen(left)
	right = ps2110Unparen(right)
	if types.ExprString(left) != types.ExprString(right) {
		return false
	}
	objects := func(expression ast.Expr) []types.Object {
		var result []types.Object
		ast.Inspect(expression, func(node ast.Node) bool {
			if identifier, ok := node.(*ast.Ident); ok {
				result = append(result, pass.TypesInfo.ObjectOf(identifier))
			}
			return true
		})
		return result
	}
	return slices.Equal(objects(left), objects(right))
}

func ps6080CorrelatedGuardMustAssignBefore(
	pass *analysis.Pass,
	function *ps6080Function,
	object types.Object,
	query ast.Node,
	parents map[ast.Node]ast.Node,
) bool {
	var guarded *ast.IfStmt
	for current := query; current != nil; current = parents[current] {
		conditional, ok := current.(*ast.IfStmt)
		if ok && conditional.Init == nil && ps6080NodeWithin(query, conditional.Body) {
			guarded = conditional
			break
		}
	}
	block, ok := parents[guarded].(*ast.BlockStmt)
	if !ok {
		return false
	}
	guardedIndex := slices.Index(block.List, ast.Stmt(guarded))
	if guardedIndex <= 0 {
		return false
	}
	conditionObjects := make(map[types.Object]bool)
	stable := true
	ast.Inspect(guarded.Cond, func(node ast.Node) bool {
		if node == nil || !stable {
			return false
		}
		switch value := node.(type) {
		case *ast.CallExpr, *ast.IndexExpr, *ast.IndexListExpr, *ast.SliceExpr,
			*ast.StarExpr, *ast.TypeAssertExpr, *ast.FuncLit:
			stable = false
		case *ast.UnaryExpr:
			stable = value.Op != token.AND && value.Op != token.ARROW
		case *ast.SelectorExpr:
			_, stable = pass.TypesInfo.ObjectOf(value.Sel).(*types.Const)
		case *ast.Ident:
			if variable, ok := pass.TypesInfo.ObjectOf(value).(*types.Var); ok {
				conditionObjects[variable] = true
			}
		}
		return stable
	})
	if !stable || len(conditionObjects) == 0 {
		return false
	}
	for condition := range conditionObjects {
		for _, file := range pass.Files {
			if ps6080ObjectAddressTaken(pass, file, condition) {
				return false
			}
		}
	}
	for index := guardedIndex - 1; index >= 0; index-- {
		prior, priorGuard := block.List[index].(*ast.IfStmt)
		if !priorGuard || prior.Init != nil ||
			!ps6080EquivalentExpressions(pass, prior.Cond, guarded.Cond) {
			continue
		}
		between := &ast.BlockStmt{List: block.List[index:guardedIndex]}
		if ps6080NodeMayMutateObjectsOutsideLiterals(pass, between, conditionObjects) ||
			ps6080CallsLiteralWritingObjects(pass, between, conditionObjects) {
			return false
		}
		assignments := ps6080NonIdentityFunctionAssignments(
			pass, object, ps6080FunctionAssignments(pass, prior.Body, object),
		)
		positions := make([]token.Pos, 0, len(assignments))
		for _, assignment := range assignments {
			positions = append(positions, assignment.node.Pos())
		}
		return len(positions) > 0 && ps6080CFGMustExecuteAnyAssignment(
			pass, cfg.New(prior.Body, ps6080CallMayReturn(pass)), parents, positions,
		)
	}
	return false
}

func ps6080NodeMayMutateObjectsOutsideLiterals(
	pass *analysis.Pass,
	node ast.Node,
	objects map[types.Object]bool,
) bool {
	unsafe := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		if candidate == nil || unsafe {
			return false
		}
		if _, literal := candidate.(*ast.FuncLit); literal {
			return false
		}
		switch value := candidate.(type) {
		case *ast.AssignStmt:
			unsafe = ps6080AssignmentTouchesObjects(pass, value, objects)
		case *ast.IncDecStmt, *ast.RangeStmt:
			if statement, ok := candidate.(ast.Stmt); ok {
				unsafe = ps6080StatementsWriteObjects(pass, []ast.Stmt{statement}, objects)
			}
		case *ast.UnaryExpr:
			identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
			unsafe = value.Op == token.AND && direct && objects[pass.TypesInfo.ObjectOf(identifier)]
		case *ast.SelectorExpr:
			unsafe = ps6080PointerMethodOnObjects(pass, value, objects)
		}
		return !unsafe
	})
	return unsafe
}

func ps6080CallsLiteralWritingObjects(
	pass *analysis.Pass,
	root ast.Node,
	objects map[types.Object]bool,
) bool {
	mutates := false
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil || mutates {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if literal, direct := ps2110Unparen(call.Fun).(*ast.FuncLit); direct {
			mutates = ps6080StatementsWriteObjects(pass, literal.Body.List, objects)
			return !mutates
		}
		identifier, direct := ps2110Unparen(call.Fun).(*ast.Ident)
		if !direct {
			return true
		}
		callable := pass.TypesInfo.ObjectOf(identifier)
		mutates = ps6080CallableInitializerWritesObjects(
			pass, callable, objects, make(map[types.Object]bool),
		)
		return !mutates
	})
	return mutates
}

func ps6080CallableInitializerWritesObjects(
	pass *analysis.Pass,
	callable types.Object,
	objects map[types.Object]bool,
	visiting map[types.Object]bool,
) bool {
	if callable == nil || visiting[callable] {
		return false
	}
	visiting[callable] = true
	defer delete(visiting, callable)
	writes := false
	var check func(ast.Expr)
	check = func(expression ast.Expr) {
		if writes {
			return
		}
		switch value := ps2110Unparen(expression).(type) {
		case *ast.FuncLit:
			writes = ps6080StatementsWriteObjects(pass, value.Body.List, objects)
		case *ast.Ident:
			writes = ps6080CallableInitializerWritesObjects(
				pass, pass.TypesInfo.ObjectOf(value), objects, visiting,
			)
		case *ast.CallExpr:
			if len(value.Args) == 1 && pass.TypesInfo.Types[value.Fun].IsType() {
				check(value.Args[0])
			}
		}
	}
	for _, file := range pass.Files {
		ast.Inspect(file, func(candidate ast.Node) bool {
			if candidate == nil || writes {
				return false
			}
			assignment, assignmentOK := candidate.(*ast.AssignStmt)
			if assignmentOK && len(assignment.Lhs) == len(assignment.Rhs) {
				for index, left := range assignment.Lhs {
					name, named := ps2110Unparen(left).(*ast.Ident)
					if named && pass.TypesInfo.ObjectOf(name) == callable {
						check(assignment.Rhs[index])
					}
				}
			}
			specification, valueSpec := candidate.(*ast.ValueSpec)
			if valueSpec && len(specification.Names) == len(specification.Values) {
				for index, name := range specification.Names {
					if pass.TypesInfo.ObjectOf(name) == callable {
						check(specification.Values[index])
					}
				}
			}
			return !writes
		})
	}
	return writes
}

func ps6080CorrelationStable(
	pass *analysis.Pass,
	function *ps6080Function,
	condition ast.Expr,
	guardBody []ast.Stmt,
) bool {
	variables := make(map[types.Object]bool)
	stable := true
	ast.Inspect(condition, func(node ast.Node) bool {
		if !stable {
			return false
		}
		switch value := node.(type) {
		case *ast.CallExpr, *ast.IndexExpr, *ast.IndexListExpr, *ast.SliceExpr,
			*ast.StarExpr, *ast.TypeAssertExpr, *ast.FuncLit:
			stable = false
			return false
		case *ast.UnaryExpr:
			if value.Op == token.AND || value.Op == token.ARROW || value.Op == token.MUL {
				stable = false
				return false
			}
		case *ast.SelectorExpr:
			if _, constant := pass.TypesInfo.ObjectOf(value.Sel).(*types.Const); !constant {
				stable = false
				return false
			}
		case *ast.Ident:
			variable, ok := pass.TypesInfo.ObjectOf(value).(*types.Var)
			if !ok {
				break
			}
			if variable.Pkg() != nil && variable.Parent() == variable.Pkg().Scope() {
				stable = false
				return false
			}
			if _, enum := ps6080EnumType(variable.Type()); !enum {
				stable = false
				return false
			}
			variables[variable] = true
		}
		return true
	})
	if !stable || len(variables) != 1 || ps6080StatementsWriteObjects(pass, guardBody, variables) {
		return false
	}
	body := ps6080FunctionBody(function)
	ast.Inspect(body, func(node ast.Node) bool {
		if !stable {
			return false
		}
		switch value := node.(type) {
		case *ast.UnaryExpr:
			identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
			if value.Op == token.AND && direct && variables[pass.TypesInfo.ObjectOf(identifier)] {
				stable = false
				return false
			}
		case *ast.SelectorExpr:
			if ps6080PointerMethodOnObjects(pass, value, variables) {
				stable = false
				return false
			}
		case *ast.FuncLit:
			ast.Inspect(value.Body, func(child ast.Node) bool {
				identifier, direct := child.(*ast.Ident)
				if direct && variables[pass.TypesInfo.ObjectOf(identifier)] {
					stable = false
					return false
				}
				return stable
			})
			return false
		}
		return true
	})
	return stable
}

func ps6080PointerMethodOnObjects(
	pass *analysis.Pass,
	selector *ast.SelectorExpr,
	objects map[types.Object]bool,
) bool {
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() != types.MethodVal {
		return false
	}
	identifier, direct := ps2110Unparen(selector.X).(*ast.Ident)
	if !direct || !objects[pass.TypesInfo.ObjectOf(identifier)] {
		return false
	}
	method, ok := selection.Obj().(*types.Func)
	if !ok {
		return false
	}
	signature, _ := method.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return false
	}
	_, pointer := types.Unalias(signature.Recv().Type()).(*types.Pointer)
	return pointer
}

func ps6080StatementsWriteObjects(
	pass *analysis.Pass,
	statements []ast.Stmt,
	objects map[types.Object]bool,
) bool {
	writes := false
	block := &ast.BlockStmt{List: statements}
	ast.Inspect(block, func(node ast.Node) bool {
		if writes {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			writes = ps6080AssignmentTouchesObjects(pass, value, objects)
		case *ast.IncDecStmt:
			identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
			writes = direct && objects[pass.TypesInfo.ObjectOf(identifier)]
		case *ast.RangeStmt:
			for _, expression := range []ast.Expr{value.Key, value.Value} {
				identifier, direct := ps2110Unparen(expression).(*ast.Ident)
				if direct && objects[pass.TypesInfo.ObjectOf(identifier)] {
					writes = true
					break
				}
			}
		}
		return !writes
	})
	return writes
}

func ps6080MutuallyExclusiveEqualities(pass *analysis.Pass, left, right ast.Expr) bool {
	leftSubject, leftConstant, leftOK := ps6080EnumEquality(pass, left)
	rightSubject, rightConstant, rightOK := ps6080EnumEquality(pass, right)
	return leftOK && rightOK && leftSubject == rightSubject &&
		constant.Compare(leftConstant.Val(), token.NEQ, rightConstant.Val())
}

func ps6080EnumEquality(pass *analysis.Pass, expression ast.Expr) (types.Object, *types.Const, bool) {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || binary.Op != token.EQL {
		return nil, nil, false
	}
	for _, pair := range [][2]ast.Expr{{binary.X, binary.Y}, {binary.Y, binary.X}} {
		constant := ps6080AliasConstant(pass, pair[1])
		if constant == nil {
			continue
		}
		enum, enumOK := ps6080EnumType(constant.Type())
		if !enumOK {
			continue
		}
		subject, unique := ps6080EnumSubject(pass, pair[0], enum)
		if unique {
			return subject, constant, true
		}
	}
	return nil, nil, false
}

func ps6080StatementsMayFallThrough(pass *analysis.Pass, statements []ast.Stmt) (bool, bool) {
	if len(statements) == 0 {
		return true, true
	}
	position := statements[len(statements)-1].End() + 1
	sentinelExpression := &ast.BasicLit{ValuePos: position, Kind: token.INT, Value: "0"}
	sentinel := &ast.ExprStmt{X: sentinelExpression}
	list := slices.Concat(statements, []ast.Stmt{sentinel})
	block := &ast.BlockStmt{Lbrace: statements[0].Pos() - 1, List: list, Rbrace: sentinel.End() + 1}
	parents := ps6071Parents(block)
	if ps6080BranchesEscapeBlock(block, parents, block) {
		return false, false
	}
	graph := cfg.New(block, ps6080CallMayReturn(pass))
	target := ps6079CFGBlockAt(graph, sentinelExpression.Pos())
	if target == nil || len(graph.Blocks) == 0 {
		return false, true
	}
	seen := map[*cfg.Block]bool{graph.Blocks[0]: true}
	queue := []*cfg.Block{graph.Blocks[0]}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == target {
			return true, true
		}
		for _, successor := range ps6080FeasibleSuccessors(pass, parents, current) {
			if !seen[successor] {
				seen[successor] = true
				queue = append(queue, successor)
			}
		}
	}
	return false, true
}

func ps6080StatementAlwaysSupports(pass *analysis.Pass, function *ps6080Function, statement ast.Stmt) bool {
	returned, ok := statement.(*ast.ReturnStmt)
	if !ok {
		return false
	}
	states, resolved := ps6080ResolvedReturnStates(pass, function, returned)
	if !resolved {
		return false
	}
	for index, state := range states {
		if !ps6080ResultStateAlwaysSupports(
			pass, state, ps6080NilResultSupports(function, index),
		) {
			return false
		}
	}
	return true
}

func ps6080NakedReturnRejected(
	pass *analysis.Pass,
	function *ps6080Function,
	returned *ast.ReturnStmt,
) bool {
	if ps6080NamedResultsMayChangeIndirectlyCached(pass, function) {
		return false
	}
	return ps6080NakedReturnAllFailure(pass, function, returned)
}

type ps6080NamedResultState struct {
	expression  ast.Expr
	resultIndex int
	known       bool
}

type ps6080ReturnFailureKey struct {
	function    *types.Func
	resultIndex int
	nilSupports bool
}

type ps6080ReturnFailureCache struct {
	declarationsOnce sync.Once
	declarations     map[*types.Func]*ast.FuncDecl
	returnability    ps6080ReturnabilityIndex
	summaries        sync.Map
	namedStates      sync.Map
	namedIndirect    sync.Map
	failures         sync.Map
	supports         sync.Map
}

type ps6080FunctionReturnSummary struct {
	once           sync.Once
	builds         int
	complete       bool
	allPathsReturn bool
	returns        [][]ps6080NamedResultState
}

type ps6080NamedReturnStateIndex struct {
	once   sync.Once
	builds int
	valid  bool
	states map[ast.Node][]ps6080NamedResultState
}

type ps6080ReturnabilityIndex struct {
	once             sync.Once
	builds           int
	mayReturn        map[*ast.BlockStmt]bool
	mustReturn       map[*ast.BlockStmt]bool
	mustPanic        map[*ast.BlockStmt]bool
	packageOnce      sync.Once
	packageBuilds    int
	packageCallables map[types.Object]ast.Expr
	owners           map[*ast.BlockStmt]*ast.BlockStmt
	recoveredReturns map[*ast.BlockStmt]map[ast.Node]bool
	contexts         sync.Map
	directPanics     sync.Map
}

func ps6080RecordRecoveredReturn(
	cache *ps6080ReturnFailureCache,
	body *ast.BlockStmt,
	node ast.Node,
) {
	if body == nil || node == nil {
		return
	}
	if cache.returnability.recoveredReturns[body] == nil {
		cache.returnability.recoveredReturns[body] = make(map[ast.Node]bool)
	}
	cache.returnability.recoveredReturns[body][node] = true
}

type ps6080ReturnabilityBodyContext struct {
	once             sync.Once
	builds           int
	mayEvaluations   atomic.Int64
	mustEvaluations  atomic.Int64
	panicEvaluations atomic.Int64
	recoverOnce      sync.Once
	mustRecoverPanic bool
	callablesOnce    sync.Once
	callableBuilds   atomic.Int64
	callables        map[types.Object]ast.Expr
	body             *ast.BlockStmt
	parents          map[ast.Node]ast.Node
	graph            *cfg.CFG
	sentinel         ast.Stmt
	sentinelEx       ast.Expr
}

type ps6080ResolvedCallable struct {
	function *types.Func
	literal  *ast.FuncLit
}

type ps6080ReturnabilityEffects struct {
	kills                  bool
	registersHardNonreturn bool
	registersPanic         bool
	registersRecover       bool
}

func ps6080ReturnFailureCacheFor(pass *analysis.Pass) *ps6080ReturnFailureCache {
	if cached, found := ps6080ReturnFailureCaches.Load(pass); found {
		return cached.(*ps6080ReturnFailureCache)
	}
	return &ps6080ReturnFailureCache{}
}

func ps6080FunctionDeclaration(
	pass *analysis.Pass,
	cache *ps6080ReturnFailureCache,
	function *types.Func,
) *ast.FuncDecl {
	cache.declarationsOnce.Do(func() {
		cache.declarations = make(map[*types.Func]*ast.FuncDecl)
		for _, file := range pass.Files {
			for _, declaration := range file.Decls {
				functionDeclaration, ok := declaration.(*ast.FuncDecl)
				if !ok || functionDeclaration.Body == nil {
					continue
				}
				object, _ := pass.TypesInfo.Defs[functionDeclaration.Name].(*types.Func)
				if object != nil {
					cache.declarations[object] = functionDeclaration
				}
			}
		}
	})
	return cache.declarations[function]
}

func ps6080ResultStateZeroOrFailure(
	pass *analysis.Pass,
	state ps6080NamedResultState,
	nilSupports bool,
) bool {
	if !state.known {
		return false
	}
	if state.expression == nil {
		return !nilSupports
	}
	if state.resultIndex < 0 {
		return ps6080ZeroOrFailureExpression(pass, state.expression, nilSupports)
	}
	return ps6080CallResultAlwaysFailure(
		pass, state.expression, state.resultIndex, nilSupports,
		ps6080ReturnFailureCacheFor(pass), make(map[ps6080ReturnFailureKey]bool),
	)
}

func ps6080ReturnabilityContext(
	body *ast.BlockStmt,
	cache *ps6080ReturnFailureCache,
) *ps6080ReturnabilityBodyContext {
	candidate := &ps6080ReturnabilityBodyContext{}
	value, _ := cache.returnability.contexts.LoadOrStore(body, candidate)
	context := value.(*ps6080ReturnabilityBodyContext)
	context.once.Do(func() {
		context.builds++
		context.body = body
		position := body.Rbrace + 1
		context.sentinelEx = &ast.BasicLit{ValuePos: position, Kind: token.INT, Value: "0"}
		context.sentinel = &ast.ExprStmt{X: context.sentinelEx}
		block := &ast.BlockStmt{
			Lbrace: body.Lbrace,
			List:   slices.Concat(body.List, []ast.Stmt{context.sentinel}),
			Rbrace: context.sentinel.End() + 1,
		}
		context.parents = ps6071Parents(block)
		context.graph = cfg.New(block, func(*ast.CallExpr) bool { return true })
	})
	return context
}

func ps6080KnownExternalCallDoesNotReturn(pass *analysis.Pass, call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	if ps6080BuiltinPanic(pass, call) || ps6080BuiltinCloseNil(pass, call) {
		return true
	}
	callee, _, known := typedCallee(pass, call.Fun)
	return known && callee.Pkg() != nil &&
		(callee.Pkg().Path() == "os" && callee.Name() == "Exit" ||
			callee.Pkg().Path() == "runtime" && callee.Name() == "Goexit")
}

func ps6080BuiltinCall(pass *analysis.Pass, call *ast.CallExpr) (*types.Builtin, bool) {
	var object types.Object
	switch function := ps2110Unparen(call.Fun).(type) {
	case *ast.Ident:
		object = pass.TypesInfo.ObjectOf(function)
	case *ast.SelectorExpr:
		object = pass.TypesInfo.ObjectOf(function.Sel)
	}
	builtin, found := object.(*types.Builtin)
	return builtin, found
}

func ps6080BuiltinCloseNil(pass *analysis.Pass, call *ast.CallExpr) bool {
	builtin, found := ps6080BuiltinCall(pass, call)
	return found && builtin.Name() == "close" && len(call.Args) == 1 &&
		ps6080DefinitelyNilChannel(pass, call.Args[0])
}

func ps6080BuiltinCallMayReturn(pass *analysis.Pass, call *ast.CallExpr) bool {
	builtin, found := ps6080BuiltinCall(pass, call)
	return found && builtin.Name() != "panic" && !ps6080BuiltinCloseNil(pass, call)
}

func ps6080BuiltinMakeMustReturn(pass *analysis.Pass, call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	typeOf := pass.TypesInfo.TypeOf(call)
	if typeOf == nil {
		return false
	}
	bounds := call.Args[1:]
	for _, bound := range bounds {
		value, known := ps6080ConstantInteger(pass, bound)
		if !known || value < 0 {
			return false
		}
	}
	switch types.Unalias(typeOf).Underlying().(type) {
	case *types.Slice:
		return len(bounds) == 1 || len(bounds) == 2 &&
			pass.TypesInfo.Types[bounds[0]].Value != nil &&
			pass.TypesInfo.Types[bounds[1]].Value != nil &&
			constant.Compare(
				pass.TypesInfo.Types[bounds[0]].Value, token.LEQ,
				pass.TypesInfo.Types[bounds[1]].Value,
			)
	case *types.Map:
		return len(bounds) <= 1
	case *types.Chan:
		return len(bounds) == 0 || len(bounds) == 1
	}
	return false
}

func ps6080InterfaceKeyOutcome(
	pass *analysis.Pass,
	mapExpression ast.Expr,
	key ast.Expr,
) (mustReturn, mustPanic bool) {
	typeOf := pass.TypesInfo.TypeOf(mapExpression)
	if typeOf == nil {
		return false, false
	}
	mapping, mapped := types.Unalias(typeOf).Underlying().(*types.Map)
	if !mapped {
		return false, false
	}
	if _, interfaceKey := types.Unalias(mapping.Key()).Underlying().(*types.Interface); !interfaceKey {
		return true, false
	}
	dynamic, nilValue, known := ps6080ExactInterfaceDynamicType(pass, key)
	if !known {
		dynamic = pass.TypesInfo.TypeOf(key)
		if dynamic != nil {
			_, interfaceType := types.Unalias(dynamic).Underlying().(*types.Interface)
			known = !interfaceType && !ps6080TypeHasFreeParameter(dynamic, nil)
			if known {
				dynamic = types.Default(dynamic)
			}
		}
	}
	if !known {
		return false, false
	}
	if nilValue {
		return true, false
	}
	if types.Comparable(dynamic) {
		return true, false
	}
	return false, true
}

func ps6080BuiltinDeleteOutcome(
	pass *analysis.Pass,
	call *ast.CallExpr,
) (mustReturn, mustPanic bool) {
	builtin, found := ps6080BuiltinCall(pass, call)
	if !found || builtin.Name() != "delete" || len(call.Args) != 2 {
		return false, false
	}
	return ps6080InterfaceKeyOutcome(pass, call.Args[0], call.Args[1])
}

func ps6080BuiltinCallMustReturn(pass *analysis.Pass, call *ast.CallExpr) bool {
	builtin, found := ps6080BuiltinCall(pass, call)
	if !found {
		return false
	}
	switch builtin.Name() {
	case "panic", "close":
		return false
	case "make":
		return ps6080BuiltinMakeMustReturn(pass, call)
	case "delete":
		mustReturn, _ := ps6080BuiltinDeleteOutcome(pass, call)
		return mustReturn
	default:
		return true
	}
}

func ps6080RecordImmutableCallable(
	pass *analysis.Pass,
	initializers map[types.Object]ast.Expr,
	invalid map[types.Object]bool,
	identifier *ast.Ident,
	expression ast.Expr,
	definition bool,
	packageScope bool,
) {
	if identifier == nil {
		return
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	variable, callable := object.(*types.Var)
	if !callable || !ps6080CallableType(variable.Type()) ||
		(variable.Parent() == pass.Pkg.Scope()) != packageScope {
		return
	}
	if !definition || expression == nil || invalid[object] || initializers[object] != nil ||
		packageScope && variable.Exported() {
		invalid[object] = true
		delete(initializers, object)
		return
	}
	initializers[object] = expression
}

func ps6080BuildImmutableCallables(
	pass *analysis.Pass,
	root ast.Node,
	packageScope bool,
) map[types.Object]ast.Expr {
	initializers := make(map[types.Object]ast.Expr)
	invalid := make(map[types.Object]bool)
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			return false
		}
		switch value := node.(type) {
		case *ast.ValueSpec:
			for index, name := range value.Names {
				var expression ast.Expr
				if len(value.Names) == len(value.Values) {
					expression = value.Values[index]
				}
				ps6080RecordImmutableCallable(
					pass, initializers, invalid, name, expression, true, packageScope,
				)
			}
		case *ast.AssignStmt:
			for index, left := range value.Lhs {
				identifier, direct := ps2110Unparen(left).(*ast.Ident)
				if !direct {
					continue
				}
				var expression ast.Expr
				if len(value.Lhs) == len(value.Rhs) {
					expression = value.Rhs[index]
				}
				object := pass.TypesInfo.ObjectOf(identifier)
				definition := value.Tok == token.DEFINE && pass.TypesInfo.Defs[identifier] == object
				ps6080RecordImmutableCallable(
					pass, initializers, invalid, identifier, expression, definition, packageScope,
				)
			}
		case *ast.RangeStmt:
			for _, expression := range []ast.Expr{value.Key, value.Value} {
				identifier, direct := ps2110Unparen(expression).(*ast.Ident)
				if direct {
					ps6080RecordImmutableCallable(
						pass, initializers, invalid, identifier, nil, false, packageScope,
					)
				}
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND {
				identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
				if direct {
					ps6080RecordImmutableCallable(
						pass, initializers, invalid, identifier, nil, false, packageScope,
					)
				}
			}
		case *ast.SelectorExpr:
			identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
			method, selected := pass.TypesInfo.ObjectOf(value.Sel).(*types.Func)
			if !direct || !selected {
				break
			}
			signature, _ := method.Type().(*types.Signature)
			if signature == nil || signature.Recv() == nil {
				break
			}
			if _, pointerReceiver := types.Unalias(signature.Recv().Type()).(*types.Pointer); pointerReceiver {
				ps6080RecordImmutableCallable(
					pass, initializers, invalid, identifier, nil, false, packageScope,
				)
			}
		}
		return true
	})
	for object := range invalid {
		delete(initializers, object)
	}
	return initializers
}

func ps6080ImmutableCallableInitializer(
	pass *analysis.Pass,
	context *ps6080ReturnabilityBodyContext,
	cache *ps6080ReturnFailureCache,
	variable *types.Var,
) ast.Expr {
	if variable == nil {
		return nil
	}
	if variable.Parent() == pass.Pkg.Scope() {
		cache.returnability.packageOnce.Do(func() {
			cache.returnability.packageBuilds++
			root := &ast.File{}
			for _, file := range pass.Files {
				root.Decls = append(root.Decls, file.Decls...)
			}
			cache.returnability.packageCallables = ps6080BuildImmutableCallables(
				pass, root, true,
			)
		})
		return cache.returnability.packageCallables[variable]
	}
	if context == nil {
		return nil
	}
	owner := context.body
	if indexed := cache.returnability.owners[context.body]; indexed != nil {
		owner = indexed
	}
	ownerContext := ps6080ReturnabilityContext(owner, cache)
	ownerContext.callablesOnce.Do(func() {
		ownerContext.callableBuilds.Add(1)
		ownerContext.callables = ps6080BuildImmutableCallables(pass, owner, false)
	})
	return ownerContext.callables[variable]
}

func ps6080ExactInterfaceMethod(
	pass *analysis.Pass,
	selector *ast.SelectorExpr,
) *types.Func {
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() != types.MethodVal {
		return nil
	}
	receiver := pass.TypesInfo.TypeOf(selector.X)
	if receiver == nil {
		return nil
	}
	if _, interfaceReceiver := types.Unalias(receiver).Underlying().(*types.Interface); !interfaceReceiver {
		return nil
	}
	dynamic, nilValue, known := ps6080ExactInterfaceDynamicType(pass, selector.X)
	if !known || nilValue || dynamic == nil {
		return nil
	}
	method := selection.Obj()
	if method == nil {
		return nil
	}
	resolved := types.NewMethodSet(dynamic).Lookup(method.Pkg(), method.Name())
	if resolved == nil {
		return nil
	}
	function, _ := resolved.Obj().(*types.Func)
	return function
}

func ps6080ResolveCallableExpression(
	pass *analysis.Pass,
	expression ast.Expr,
	context *ps6080ReturnabilityBodyContext,
	cache *ps6080ReturnFailureCache,
	seen map[types.Object]bool,
) ([]ps6080ResolvedCallable, bool) {
	if expression == nil {
		return nil, false
	}
	expression = ps2110Unparen(expression)
	if literal, direct := expression.(*ast.FuncLit); direct {
		return []ps6080ResolvedCallable{{literal: literal}}, true
	}
	if conversion, converted := expression.(*ast.CallExpr); converted && len(conversion.Args) == 1 &&
		pass.TypesInfo.Types[conversion.Fun].IsType() && ps6080CallableType(pass.TypesInfo.TypeOf(conversion)) {
		return ps6080ResolveCallableExpression(
			pass, conversion.Args[0], context, cache, seen,
		)
	}
	if selector, selected := expression.(*ast.SelectorExpr); selected {
		if method := ps6080ExactInterfaceMethod(pass, selector); method != nil {
			return []ps6080ResolvedCallable{{function: method}}, true
		}
	}
	if callee, _, direct := typedCallee(pass, expression); direct {
		return []ps6080ResolvedCallable{{function: callee}}, true
	}
	var object types.Object
	switch value := expression.(type) {
	case *ast.Ident:
		object = pass.TypesInfo.ObjectOf(value)
	case *ast.SelectorExpr:
		object = pass.TypesInfo.ObjectOf(value.Sel)
	}
	variable, variableCall := object.(*types.Var)
	if !variableCall || seen[object] {
		return nil, false
	}
	seen[object] = true
	defer delete(seen, object)
	initializer := ps6080ImmutableCallableInitializer(pass, context, cache, variable)
	if initializer == nil {
		return nil, false
	}
	return ps6080ResolveCallableExpression(pass, initializer, context, cache, seen)
}

func ps6080ResolvedCallableMayReturn(
	pass *analysis.Pass,
	target ps6080ResolvedCallable,
	cache *ps6080ReturnFailureCache,
	mayReturn map[*ast.BlockStmt]bool,
) bool {
	if target.literal != nil {
		return mayReturn[target.literal.Body]
	}
	callee := target.function
	if callee == nil || callee.Pkg() != pass.Pkg {
		return true
	}
	declaration := ps6080FunctionDeclaration(pass, cache, callee)
	return declaration == nil || mayReturn[declaration.Body]
}

func ps6080CallMayReturnWithFacts(
	pass *analysis.Pass,
	call *ast.CallExpr,
	context *ps6080ReturnabilityBodyContext,
	cache *ps6080ReturnFailureCache,
	mayReturn map[*ast.BlockStmt]bool,
	evaluating map[*ast.BlockStmt]bool,
) bool {
	if call == nil {
		return true
	}
	if ps6080KnownExternalCallDoesNotReturn(pass, call) {
		return false
	}
	if ps6080BuiltinCallMayReturn(pass, call) {
		return true
	}
	targets, complete := ps6080ResolveCallableExpression(
		pass, call.Fun, context, cache, make(map[types.Object]bool),
	)
	if !complete || len(targets) == 0 {
		return true
	}
	for _, target := range targets {
		if ps6080ResolvedCallableMayReturn(pass, target, cache, mayReturn) {
			return true
		}
	}
	return false
}

func ps6080ResolvedCallableMustReturn(
	pass *analysis.Pass,
	target ps6080ResolvedCallable,
	cache *ps6080ReturnFailureCache,
	mustReturn map[*ast.BlockStmt]bool,
) bool {
	if target.literal != nil {
		return mustReturn[target.literal.Body]
	}
	callee := target.function
	if callee == nil {
		return false
	}
	if callee.Pkg() != pass.Pkg {
		return callee.Pkg() != nil && callee.Pkg().Path() == "errors" && callee.Name() == "New"
	}
	declaration := ps6080FunctionDeclaration(pass, cache, callee)
	return declaration != nil && mustReturn[declaration.Body]
}

func ps6080CallMustReturnWithFacts(
	pass *analysis.Pass,
	call *ast.CallExpr,
	context *ps6080ReturnabilityBodyContext,
	cache *ps6080ReturnFailureCache,
	mustReturn map[*ast.BlockStmt]bool,
	evaluating map[*ast.BlockStmt]bool,
) bool {
	if call == nil {
		return true
	}
	if ps6080KnownExternalCallDoesNotReturn(pass, call) {
		return false
	}
	if ps6080BuiltinCallMustReturn(pass, call) {
		return true
	}
	targets, complete := ps6080ResolveCallableExpression(
		pass, call.Fun, context, cache, make(map[types.Object]bool),
	)
	if !complete || len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		if !ps6080ResolvedCallableMustReturn(pass, target, cache, mustReturn) {
			return false
		}
	}
	return true
}

func ps6080CallMustPanicWithFacts(
	pass *analysis.Pass,
	call *ast.CallExpr,
	context *ps6080ReturnabilityBodyContext,
	cache *ps6080ReturnFailureCache,
	mustPanic map[*ast.BlockStmt]bool,
) bool {
	if call == nil || len(call.Args) != 0 {
		return false
	}
	targets, complete := ps6080ResolveCallableExpression(
		pass, call.Fun, context, cache, make(map[types.Object]bool),
	)
	if !complete || len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		var body *ast.BlockStmt
		if target.literal != nil {
			body = target.literal.Body
		} else if target.function != nil && target.function.Pkg() == pass.Pkg {
			if declaration := ps6080FunctionDeclaration(pass, cache, target.function); declaration != nil {
				body = declaration.Body
			}
		}
		if body == nil || !mustPanic[body] {
			return false
		}
	}
	return true
}

func ps6080BodyMustRecoverPanic(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	cache *ps6080ReturnFailureCache,
) bool {
	if body == nil {
		return false
	}
	context := ps6080ReturnabilityContext(body, cache)
	context.recoverOnce.Do(func() {
		type recoverPath struct {
			block     *cfg.Block
			recovered bool
		}
		type visitState uint8
		const (
			visiting visitState = iota + 1
			accepted
			rejected
		)
		states := make(map[recoverPath]visitState)
		var visits func(recoverPath) bool
		visits = func(path recoverPath) bool {
			switch states[path] {
			case visiting, rejected:
				return false
			case accepted:
				return true
			}
			states[path] = visiting
			recovered := path.recovered
			for _, node := range path.block.Nodes {
				ast.Inspect(node, func(candidate ast.Node) bool {
					if candidate == nil {
						return false
					}
					if candidate != node {
						if _, nestedFunction := candidate.(*ast.FuncLit); nestedFunction {
							return false
						}
						if _, nestedStatement := candidate.(ast.Stmt); nestedStatement {
							return false
						}
					}
					call, called := candidate.(*ast.CallExpr)
					if called && ps6080NodeReachableWithLiveness(
						pass, context.parents, call, true, true,
					) && typedBuiltinName(pass, call.Fun, "recover") {
						recovered = true
					}
					return true
				})
				if _, returned := node.(*ast.ReturnStmt); returned ||
					node == context.sentinel || node == context.sentinelEx {
					if recovered {
						states[path] = accepted
						return true
					}
					states[path] = rejected
					return false
				}
			}
			successors := ps6080FeasibleSuccessors(pass, context.parents, path.block)
			if len(successors) == 0 {
				states[path] = rejected
				return false
			}
			for _, successor := range successors {
				if !successor.Live || !visits(recoverPath{block: successor, recovered: recovered}) {
					states[path] = rejected
					return false
				}
			}
			states[path] = accepted
			return true
		}
		context.mustRecoverPanic = context.graph != nil && len(context.graph.Blocks) > 0 &&
			context.graph.Blocks[0].Live && visits(recoverPath{block: context.graph.Blocks[0]})
	})
	return context.mustRecoverPanic
}

func ps6080CallMustRecoverPanic(
	pass *analysis.Pass,
	call *ast.CallExpr,
	context *ps6080ReturnabilityBodyContext,
	cache *ps6080ReturnFailureCache,
) bool {
	targets, complete := ps6080ResolveCallableExpression(
		pass, call.Fun, context, cache, make(map[types.Object]bool),
	)
	if !complete || len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		if target.literal == nil || !ps6080BodyMustRecoverPanic(pass, target.literal.Body, cache) {
			if target.function == nil || target.function.Pkg() != pass.Pkg {
				return false
			}
			declaration := ps6080FunctionDeclaration(pass, cache, target.function)
			if declaration == nil || !ps6080BodyMustRecoverPanic(pass, declaration.Body, cache) {
				return false
			}
		}
	}
	return true
}

func ps6080SelectCommunicationOperation(
	parents map[ast.Node]ast.Node,
	node ast.Node,
) bool {
	switch value := node.(type) {
	case *ast.SendStmt:
		clause, selected := parents[value].(*ast.CommClause)
		return selected && clause.Comm == value
	case *ast.UnaryExpr:
		if value.Op != token.ARROW {
			return false
		}
		current := ast.Node(value)
		for parent := parents[current]; parent != nil; parent = parents[current] {
			switch statement := parent.(type) {
			case *ast.ParenExpr:
				current = parent
				continue
			case *ast.ExprStmt:
				clause, selected := parents[statement].(*ast.CommClause)
				return selected && clause.Comm == statement
			case *ast.AssignStmt:
				clause, selected := parents[statement].(*ast.CommClause)
				return selected && clause.Comm == statement && len(statement.Rhs) == 1 &&
					ps2110Unparen(statement.Rhs[0]) == value
			default:
				return false
			}
		}
	}
	return false
}

func ps6080NodeChannelBlock(
	pass *analysis.Pass,
	node ast.Node,
	context *ps6080ReturnabilityBodyContext,
) (mayBlock, definitelyBlocks bool) {
	ast.Inspect(node, func(candidate ast.Node) bool {
		if candidate == nil || definitelyBlocks {
			return false
		}
		if candidate != node {
			if _, nestedFunction := candidate.(*ast.FuncLit); nestedFunction {
				return false
			}
			if _, nestedStatement := candidate.(ast.Stmt); nestedStatement {
				return false
			}
		}
		if !ps6080NodeReachableWithLiveness(pass, context.parents, candidate, true, true) {
			return false
		}
		var channel ast.Expr
		switch value := candidate.(type) {
		case *ast.SendStmt:
			if !ps6080SelectCommunicationOperation(context.parents, value) {
				channel = value.Chan
			}
		case *ast.UnaryExpr:
			if value.Op == token.ARROW &&
				!ps6080SelectCommunicationOperation(context.parents, value) {
				channel = value.X
			}
		}
		if channel == nil {
			return true
		}
		mayBlock = true
		definitelyBlocks = ps6080DefinitelyNilChannel(pass, channel)
		return !definitelyBlocks
	})
	return mayBlock, definitelyBlocks
}

func ps6080SelectMayBlock(
	parents map[ast.Node]ast.Node,
	successors []*cfg.Block,
) bool {
	for _, successor := range successors {
		if successor.Kind != cfg.KindSelectCaseBody {
			continue
		}
		clause, _ := successor.Stmt.(*ast.CommClause)
		selectBody, _ := parents[clause].(*ast.BlockStmt)
		selection, _ := parents[selectBody].(*ast.SelectStmt)
		if selection == nil {
			continue
		}
		for _, statement := range selection.Body.List {
			candidate, _ := statement.(*ast.CommClause)
			if candidate != nil && candidate.Comm == nil {
				return false
			}
		}
		return true
	}
	return false
}

func ps6080DefinitelyNilTypedExpression(pass *analysis.Pass, expression ast.Expr) bool {
	for expression != nil {
		expression = ps2110Unparen(expression)
		if ps6080NilExpression(pass, expression) {
			return true
		}
		conversion, converted := expression.(*ast.CallExpr)
		if !converted || len(conversion.Args) != 1 || conversion.Ellipsis.IsValid() ||
			!pass.TypesInfo.Types[conversion.Fun].IsType() {
			return false
		}
		target := pass.TypesInfo.TypeOf(conversion)
		if target != nil {
			if _, interfaceTarget := types.Unalias(target).Underlying().(*types.Interface); interfaceTarget {
				return ps6080NilExpression(pass, conversion.Args[0])
			}
		}
		expression = conversion.Args[0]
	}
	return false
}

func ps6080DefinitelyNonNilPointerExpression(pass *analysis.Pass, expression ast.Expr) bool {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.UnaryExpr:
		return value.Op == token.AND
	case *ast.CallExpr:
		if typedBuiltinName(pass, value.Fun, "new") {
			return true
		}
		if len(value.Args) == 1 && pass.TypesInfo.Types[value.Fun].IsType() {
			target := pass.TypesInfo.TypeOf(value)
			if target != nil {
				if _, pointer := types.Unalias(target).Underlying().(*types.Pointer); pointer {
					return ps6080DefinitelyNonNilPointerExpression(pass, value.Args[0])
				}
			}
		}
	}
	return false
}

func ps6080CompositeLength(pass *analysis.Pass, literal *ast.CompositeLit) (int64, bool) {
	next := int64(0)
	length := int64(0)
	for _, element := range literal.Elts {
		if keyed, ok := element.(*ast.KeyValueExpr); ok {
			key := pass.TypesInfo.Types[keyed.Key].Value
			if key == nil || key.Kind() != constant.Int {
				return 0, false
			}
			index, exact := constant.Int64Val(key)
			if !exact || index < 0 {
				return 0, false
			}
			next = index
		}
		next++
		length = max(length, next)
	}
	return length, true
}

func ps6080KnownContainerSizes(
	pass *analysis.Pass,
	expression ast.Expr,
) (length int64, lengthKnown bool, capacity int64, capacityKnown bool) {
	expression = ps2110Unparen(expression)
	typeOf := pass.TypesInfo.TypeOf(expression)
	if typeOf == nil {
		return 0, false, 0, false
	}
	underlying := types.Unalias(typeOf).Underlying()
	if pointer, ok := underlying.(*types.Pointer); ok {
		if array, arrayPointer := types.Unalias(pointer.Elem()).Underlying().(*types.Array); arrayPointer {
			return array.Len(), true, array.Len(), true
		}
	}
	if array, ok := underlying.(*types.Array); ok {
		return array.Len(), true, array.Len(), true
	}
	if basic, ok := underlying.(*types.Basic); ok && basic.Info()&types.IsString != 0 {
		value := pass.TypesInfo.Types[expression].Value
		if value != nil && value.Kind() == constant.String {
			length := int64(len(constant.StringVal(value)))
			return length, true, length, true
		}
		return 0, false, 0, false
	}
	if _, slice := underlying.(*types.Slice); !slice {
		return 0, false, 0, false
	}
	if ps6080DefinitelyNilTypedExpression(pass, expression) {
		return 0, true, 0, true
	}
	if literal, composite := expression.(*ast.CompositeLit); composite {
		length, known := ps6080CompositeLength(pass, literal)
		return length, known, length, known
	}
	call, called := expression.(*ast.CallExpr)
	if !called || !typedBuiltinName(pass, call.Fun, "make") || len(call.Args) < 2 {
		return 0, false, 0, false
	}
	lengthValue := pass.TypesInfo.Types[call.Args[1]].Value
	if lengthValue == nil || lengthValue.Kind() != constant.Int {
		return 0, false, 0, false
	}
	length, lengthKnown = constant.Int64Val(lengthValue)
	if !lengthKnown || length < 0 {
		return 0, false, 0, false
	}
	capacity = length
	capacityKnown = true
	if len(call.Args) > 2 {
		capacityValue := pass.TypesInfo.Types[call.Args[2]].Value
		if capacityValue == nil || capacityValue.Kind() != constant.Int {
			capacityKnown = false
		} else {
			capacity, capacityKnown = constant.Int64Val(capacityValue)
			capacityKnown = capacityKnown && capacity >= 0
		}
	}
	return length, true, capacity, capacityKnown
}

func ps6080ConstantInteger(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	if expression == nil {
		return 0, false
	}
	value := pass.TypesInfo.Types[expression].Value
	if value == nil || value.Kind() != constant.Int {
		return 0, false
	}
	integer, exact := constant.Int64Val(value)
	return integer, exact
}

func ps6080SliceBoundsMustPanic(pass *analysis.Pass, expression *ast.SliceExpr) bool {
	length, lengthKnown, capacity, capacityKnown := ps6080KnownContainerSizes(pass, expression.X)
	low, lowKnown := int64(0), true
	if expression.Low != nil {
		low, lowKnown = ps6080ConstantInteger(pass, expression.Low)
	}
	high, highKnown := length, lengthKnown
	if expression.High != nil {
		high, highKnown = ps6080ConstantInteger(pass, expression.High)
	}
	maximum, maximumKnown := capacity, capacityKnown
	if expression.Max != nil {
		maximum, maximumKnown = ps6080ConstantInteger(pass, expression.Max)
	}
	for _, bound := range []struct {
		value int64
		known bool
	}{{low, lowKnown}, {high, highKnown}, {maximum, maximumKnown}} {
		if bound.known && bound.value < 0 {
			return true
		}
	}
	if lowKnown && highKnown && low > high {
		return true
	}
	if expression.Max != nil && highKnown && maximumKnown && high > maximum {
		return true
	}
	if expression.Max != nil && lowKnown && maximumKnown && low > maximum {
		return true
	}
	if capacityKnown {
		if lowKnown && low > capacity || expression.High != nil && highKnown && high > capacity ||
			expression.Max != nil && maximumKnown && maximum > capacity {
			return true
		}
	}
	return false
}

func ps6080SliceToArrayMustPanic(pass *analysis.Pass, call *ast.CallExpr) bool {
	if len(call.Args) != 1 || call.Ellipsis.IsValid() || !pass.TypesInfo.Types[call.Fun].IsType() {
		return false
	}
	from := pass.TypesInfo.TypeOf(call.Args[0])
	to := pass.TypesInfo.TypeOf(call)
	if from == nil || to == nil {
		return false
	}
	if _, slice := types.Unalias(from).Underlying().(*types.Slice); !slice {
		return false
	}
	target := types.Unalias(to)
	if pointer, pointerTarget := target.Underlying().(*types.Pointer); pointerTarget {
		target = types.Unalias(pointer.Elem())
	}
	array, arrayTarget := target.Underlying().(*types.Array)
	if !arrayTarget {
		return false
	}
	length, known, _, _ := ps6080KnownContainerSizes(pass, call.Args[0])
	return known && length < array.Len()
}

func ps6080CommaOKTypeAssertion(
	parents map[ast.Node]ast.Node,
	assertion *ast.TypeAssertExpr,
) bool {
	current := ast.Node(assertion)
	for {
		parent, parenthesized := parents[current].(*ast.ParenExpr)
		if !parenthesized {
			break
		}
		current = parent
	}
	switch parent := parents[current].(type) {
	case *ast.AssignStmt:
		return len(parent.Lhs) == 2 && len(parent.Rhs) == 1
	case *ast.ValueSpec:
		return len(parent.Names) == 2 && len(parent.Values) == 1
	}
	return false
}

func ps6080TypeAssertionMustPanic(
	pass *analysis.Pass,
	parents map[ast.Node]ast.Node,
	assertion *ast.TypeAssertExpr,
) bool {
	if assertion.Type == nil || ps6080CommaOKTypeAssertion(parents, assertion) {
		return false
	}
	dynamic, nilValue, known := ps6080ExactInterfaceDynamicType(pass, assertion.X)
	if !known {
		return false
	}
	if nilValue {
		return true
	}
	target := pass.TypesInfo.TypeOf(assertion.Type)
	if target == nil || ps6080TypeHasFreeParameter(target, nil) {
		return false
	}
	if _, interfaceTarget := types.Unalias(target).Underlying().(*types.Interface); interfaceTarget {
		return !types.AssignableTo(dynamic, target)
	}
	return !types.Identical(types.Unalias(dynamic), types.Unalias(target))
}

func ps6080InterfaceComparisonOutcome(
	pass *analysis.Pass,
	expression *ast.BinaryExpr,
) (relevant, mustReturn, mustPanic bool) {
	if expression.Op != token.EQL && expression.Op != token.NEQ {
		return false, false, false
	}
	for _, operand := range []ast.Expr{expression.X, expression.Y} {
		typeOf := pass.TypesInfo.TypeOf(operand)
		if typeOf == nil {
			return false, false, false
		}
		if _, interfaceType := types.Unalias(typeOf).Underlying().(*types.Interface); !interfaceType {
			return false, false, false
		}
	}
	left, leftNil, leftKnown := ps6080ExactInterfaceDynamicType(pass, expression.X)
	right, rightNil, rightKnown := ps6080ExactInterfaceDynamicType(pass, expression.Y)
	if !leftKnown || !rightKnown {
		if leftKnown && (leftNil || types.Comparable(left)) ||
			rightKnown && (rightNil || types.Comparable(right)) {
			return true, true, false
		}
		return true, false, false
	}
	if leftNil || rightNil || !types.Identical(left, right) || types.Comparable(left) {
		return true, true, false
	}
	return true, false, true
}

func ps6080InterfaceComparisonMustPanic(pass *analysis.Pass, expression *ast.BinaryExpr) bool {
	_, _, mustPanic := ps6080InterfaceComparisonOutcome(pass, expression)
	return mustPanic
}

func ps6080ShiftMustReturn(pass *analysis.Pass, expression *ast.BinaryExpr) bool {
	if expression.Op != token.SHL && expression.Op != token.SHR {
		return true
	}
	if shift, known := ps6080ConstantInteger(pass, expression.Y); known {
		return shift >= 0
	}
	typeOf := pass.TypesInfo.TypeOf(expression.Y)
	if typeOf == nil {
		return false
	}
	basic, basicType := types.Unalias(typeOf).Underlying().(*types.Basic)
	return basicType && basic.Info()&types.IsUnsigned != 0
}

func ps6080DirectExpressionMustPanic(
	pass *analysis.Pass,
	expression ast.Expr,
	parents map[ast.Node]ast.Node,
	cache *ps6080ReturnFailureCache,
) bool {
	if expression == nil {
		return false
	}
	expression = ps2110Unparen(expression)
	if cached, found := cache.returnability.directPanics.Load(expression); found {
		return cached.(bool)
	}
	panics := false
	recurse := func(candidate ast.Expr) bool {
		return ps6080DirectExpressionMustPanic(pass, candidate, parents, cache)
	}
	switch value := expression.(type) {
	case *ast.SelectorExpr:
		path := ps6080SelectorEmbeddedPathOutcome(pass, value)
		panics = recurse(value.X) || ps6080SelectorRootMustPanic(pass, value) || path.mustPanic
	case *ast.IndexExpr:
		panics = recurse(value.X) || recurse(value.Index) ||
			ps6080IndexRootMustPanic(pass, value) ||
			ps6080NilMapWriteMustPanic(pass, parents, value)
	case *ast.IndexListExpr:
		panics = recurse(value.X)
		for _, index := range value.Indices {
			panics = panics || recurse(index)
		}
	case *ast.StarExpr:
		panics = recurse(value.X) || ps6080DefinitelyNilTypedExpression(pass, value.X)
	case *ast.CallExpr:
		if pass.TypesInfo.Types[value].Value == nil {
			panics = recurse(value.Fun)
			for _, argument := range value.Args {
				panics = panics || recurse(argument)
			}
			panics = panics || ps6080SliceToArrayMustPanic(pass, value)
			_, deletePanics := ps6080BuiltinDeleteOutcome(pass, value)
			panics = panics || deletePanics
		}
	case *ast.UnaryExpr:
		panics = recurse(value.X)
	case *ast.BinaryExpr:
		panics = recurse(value.X)
		if !panics && (value.Op != token.LAND && value.Op != token.LOR ||
			pass.TypesInfo.Types[value.X].Value == nil ||
			value.Op == token.LAND && constant.BoolVal(pass.TypesInfo.Types[value.X].Value) ||
			value.Op == token.LOR && !constant.BoolVal(pass.TypesInfo.Types[value.X].Value)) {
			panics = recurse(value.Y)
		}
		if !panics {
			panics = ps6080InterfaceComparisonMustPanic(pass, value)
		}
		if !panics && (value.Op == token.QUO || value.Op == token.REM) {
			divisor := pass.TypesInfo.Types[value.Y].Value
			panics = divisor != nil && divisor.Kind() == constant.Int && constant.Sign(divisor) == 0
		}
		if !panics && (value.Op == token.SHL || value.Op == token.SHR) {
			shift, known := ps6080ConstantInteger(pass, value.Y)
			panics = known && shift < 0
		}
	case *ast.SliceExpr:
		panics = recurse(value.X) || recurse(value.Low) || recurse(value.High) || recurse(value.Max) ||
			ps6080SliceRootMustPanic(pass, value)
	case *ast.TypeAssertExpr:
		panics = recurse(value.X) || ps6080TypeAssertionMustPanic(pass, parents, value)
	case *ast.CompositeLit:
		for _, element := range value.Elts {
			switch item := element.(type) {
			case *ast.KeyValueExpr:
				panics = panics || recurse(item.Key) || recurse(item.Value)
				if !panics {
					_, keyPanics := ps6080InterfaceKeyOutcome(pass, value, item.Key)
					panics = keyPanics
				}
			case ast.Expr:
				panics = panics || recurse(item)
			}
		}
	}
	cache.returnability.directPanics.Store(expression, panics)
	return panics
}

func ps6080NodeExpressionMustPanic(
	pass *analysis.Pass,
	node ast.Node,
	context *ps6080ReturnabilityBodyContext,
	cache *ps6080ReturnFailureCache,
) bool {
	panics := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		if candidate == nil || panics {
			return false
		}
		if candidate != node {
			if _, nestedFunction := candidate.(*ast.FuncLit); nestedFunction {
				return false
			}
			if _, nestedStatement := candidate.(ast.Stmt); nestedStatement {
				return false
			}
		}
		expression, found := candidate.(ast.Expr)
		if !found || !ps6080NodeReachableWithLiveness(
			pass, context.parents, expression, true, true,
		) {
			return true
		}
		panics = ps6080DirectExpressionMustPanic(pass, expression, context.parents, cache)
		return !panics
	})
	return panics
}

type ps6080ExpressionOutcome struct {
	mustReturn bool
	mustPanic  bool
}

func ps6080ExpressionSequenceOutcome(
	pass *analysis.Pass,
	expressions []ast.Expr,
	context *ps6080ReturnabilityBodyContext,
	cache *ps6080ReturnFailureCache,
	mustReturn map[*ast.BlockStmt]bool,
	mustPanic map[*ast.BlockStmt]bool,
) ps6080ExpressionOutcome {
	for _, expression := range expressions {
		outcome := ps6080OrderedExpressionOutcome(
			pass, expression, context, cache, mustReturn, mustPanic,
		)
		if outcome.mustPanic {
			return outcome
		}
		if !outcome.mustReturn {
			return ps6080ExpressionOutcome{}
		}
	}
	return ps6080ExpressionOutcome{mustReturn: true}
}

func ps6080SelectorRootMustPanic(pass *analysis.Pass, selector *ast.SelectorExpr) bool {
	if !ps6080DefinitelyNilTypedExpression(pass, selector.X) {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil {
		return false
	}
	switch selection.Kind() {
	case types.FieldVal:
		return true
	case types.MethodVal:
		method, _ := selection.Obj().(*types.Func)
		if method == nil {
			return false
		}
		signature, _ := method.Type().(*types.Signature)
		if signature == nil || signature.Recv() == nil {
			return false
		}
		_, pointerReceiver := types.Unalias(signature.Recv().Type()).Underlying().(*types.Pointer)
		return !pointerReceiver
	}
	return false
}

func ps6080SelectionPointerReceiver(selection *types.Selection) bool {
	if selection == nil || selection.Kind() != types.MethodVal {
		return false
	}
	method, _ := selection.Obj().(*types.Func)
	if method == nil {
		return false
	}
	signature, _ := method.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return false
	}
	_, pointerReceiver := types.Unalias(signature.Recv().Type()).Underlying().(*types.Pointer)
	return pointerReceiver
}

func ps6080CompositeFieldValue(
	pass *analysis.Pass,
	expression ast.Expr,
	field *types.Var,
	fieldIndex int,
) (ast.Expr, bool, bool) {
	expression = ps2110Unparen(expression)
	if address, addressed := expression.(*ast.UnaryExpr); addressed && address.Op == token.AND {
		expression = ps2110Unparen(address.X)
	}
	literal, composite := expression.(*ast.CompositeLit)
	if !composite {
		return nil, false, false
	}
	unkeyedIndex := 0
	for _, element := range literal.Elts {
		if keyed, isKeyed := element.(*ast.KeyValueExpr); isKeyed {
			identifier, named := ps2110Unparen(keyed.Key).(*ast.Ident)
			if named && pass.TypesInfo.ObjectOf(identifier) == field {
				return keyed.Value, false, true
			}
			continue
		}
		if unkeyedIndex == fieldIndex {
			return element, false, true
		}
		unkeyedIndex++
	}
	return nil, true, true
}

func ps6080SelectorEmbeddedPathOutcome(
	pass *analysis.Pass,
	selector *ast.SelectorExpr,
) ps6080ExpressionOutcome {
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || len(selection.Index()) <= 1 {
		return ps6080ExpressionOutcome{mustReturn: true}
	}
	fieldCount := len(selection.Index()) - 1
	currentType := selection.Recv()
	currentExpression := selector.X
	currentZero := false
	pointerReceiver := ps6080SelectionPointerReceiver(selection)
	for pathIndex, fieldIndex := range selection.Index()[:fieldCount] {
		underlying := types.Unalias(currentType).Underlying()
		if pointer, pointerType := underlying.(*types.Pointer); pointerType {
			if currentZero || ps6080DefinitelyNilTypedExpression(pass, currentExpression) {
				return ps6080ExpressionOutcome{mustPanic: true}
			}
			if !ps6080DefinitelyNonNilPointerExpression(pass, currentExpression) {
				return ps6080ExpressionOutcome{}
			}
			underlying = types.Unalias(pointer.Elem()).Underlying()
		}
		structure, structured := underlying.(*types.Struct)
		if !structured || fieldIndex < 0 || fieldIndex >= structure.NumFields() {
			return ps6080ExpressionOutcome{}
		}
		field := structure.Field(fieldIndex)
		fieldExpression, zero, known := ps6080CompositeFieldValue(
			pass, currentExpression, field, fieldIndex,
		)
		if currentZero {
			zero, known = true, true
		}
		fieldType := field.Type()
		if _, pointerField := types.Unalias(fieldType).Underlying().(*types.Pointer); pointerField {
			finalReceiver := pathIndex == fieldCount-1 && pointerReceiver
			nilField := zero || known && ps6080DefinitelyNilTypedExpression(pass, fieldExpression)
			if nilField {
				if finalReceiver {
					return ps6080ExpressionOutcome{mustReturn: true}
				}
				return ps6080ExpressionOutcome{mustPanic: true}
			}
			if !known || !ps6080DefinitelyNonNilPointerExpression(pass, fieldExpression) {
				if finalReceiver {
					return ps6080ExpressionOutcome{mustReturn: true}
				}
				return ps6080ExpressionOutcome{}
			}
		}
		currentType = fieldType
		currentExpression = fieldExpression
		currentZero = zero
	}
	return ps6080ExpressionOutcome{mustReturn: true}
}

func ps6080IndexRootMustPanic(pass *analysis.Pass, expression *ast.IndexExpr) bool {
	typeOf := pass.TypesInfo.TypeOf(expression.X)
	if typeOf == nil {
		return false
	}
	underlying := types.Unalias(typeOf).Underlying()
	if _, mapping := underlying.(*types.Map); mapping {
		_, mustPanic := ps6080InterfaceKeyOutcome(pass, expression.X, expression.Index)
		return mustPanic
	}
	if pointer, pointerType := underlying.(*types.Pointer); pointerType {
		if _, arrayPointer := types.Unalias(pointer.Elem()).Underlying().(*types.Array); arrayPointer && ps6080DefinitelyNilTypedExpression(pass, expression.X) {
			return true
		}
		underlying = types.Unalias(pointer.Elem()).Underlying()
	}
	switch underlying.(type) {
	case *types.Array, *types.Slice, *types.Basic:
		length, known, _, _ := ps6080KnownContainerSizes(pass, expression.X)
		index, indexKnown := ps6080ConstantInteger(pass, expression.Index)
		return known && (length == 0 || indexKnown && (index < 0 || index >= length))
	}
	return false
}

func ps6080IndexRootMustReturn(pass *analysis.Pass, expression *ast.IndexExpr) bool {
	typeOf := pass.TypesInfo.TypeOf(expression.X)
	if typeOf == nil {
		return false
	}
	underlying := types.Unalias(typeOf).Underlying()
	if _, mapping := underlying.(*types.Map); mapping {
		mustReturn, _ := ps6080InterfaceKeyOutcome(pass, expression.X, expression.Index)
		return mustReturn
	}
	if pointer, pointerType := underlying.(*types.Pointer); pointerType {
		if !ps6080DefinitelyNonNilPointerExpression(pass, expression.X) {
			return false
		}
		underlying = types.Unalias(pointer.Elem()).Underlying()
	}
	switch underlying.(type) {
	case *types.Array, *types.Slice, *types.Basic:
		length, known, _, _ := ps6080KnownContainerSizes(pass, expression.X)
		index, indexKnown := ps6080ConstantInteger(pass, expression.Index)
		return known && indexKnown && index >= 0 && index < length
	}
	return false
}

func ps6080MapIndexWritten(
	parents map[ast.Node]ast.Node,
	expression *ast.IndexExpr,
) bool {
	current := ast.Node(expression)
	for {
		parent, parenthesized := parents[current].(*ast.ParenExpr)
		if !parenthesized {
			break
		}
		current = parent
	}
	switch parent := parents[current].(type) {
	case *ast.AssignStmt:
		for _, destination := range parent.Lhs {
			if ps2110Unparen(destination) == expression {
				return true
			}
		}
	case *ast.IncDecStmt:
		return ps2110Unparen(parent.X) == expression
	}
	return false
}

func ps6080NilMapWriteMustPanic(
	pass *analysis.Pass,
	parents map[ast.Node]ast.Node,
	expression *ast.IndexExpr,
) bool {
	typeOf := pass.TypesInfo.TypeOf(expression.X)
	if typeOf == nil {
		return false
	}
	_, mapping := types.Unalias(typeOf).Underlying().(*types.Map)
	return mapping && ps6080MapIndexWritten(parents, expression) &&
		ps6080DefinitelyNilTypedExpression(pass, expression.X)
}

func ps6080DefinitelyNonNilMapExpression(pass *analysis.Pass, expression ast.Expr) bool {
	expression = ps2110Unparen(expression)
	typeOf := pass.TypesInfo.TypeOf(expression)
	if typeOf == nil {
		return false
	}
	if _, mapping := types.Unalias(typeOf).Underlying().(*types.Map); !mapping {
		return false
	}
	switch value := expression.(type) {
	case *ast.CompositeLit:
		return true
	case *ast.CallExpr:
		if typedBuiltinName(pass, value.Fun, "make") {
			return ps6080BuiltinMakeMustReturn(pass, value)
		}
		if len(value.Args) == 1 && pass.TypesInfo.Types[value.Fun].IsType() {
			return ps6080DefinitelyNonNilMapExpression(pass, value.Args[0])
		}
	}
	return false
}

func ps6080SliceRootMustPanic(pass *analysis.Pass, expression *ast.SliceExpr) bool {
	if ps6080SliceBoundsMustPanic(pass, expression) {
		return true
	}
	typeOf := pass.TypesInfo.TypeOf(expression.X)
	if typeOf == nil {
		return false
	}
	pointer, pointerType := types.Unalias(typeOf).Underlying().(*types.Pointer)
	if !pointerType {
		return false
	}
	_, arrayPointer := types.Unalias(pointer.Elem()).Underlying().(*types.Array)
	return arrayPointer && ps6080DefinitelyNilTypedExpression(pass, expression.X)
}

func ps6080SliceRootMustReturn(pass *analysis.Pass, expression *ast.SliceExpr) bool {
	if ps6080SliceRootMustPanic(pass, expression) {
		return false
	}
	typeOf := pass.TypesInfo.TypeOf(expression.X)
	if typeOf != nil {
		if _, pointer := types.Unalias(typeOf).Underlying().(*types.Pointer); pointer &&
			!ps6080DefinitelyNonNilPointerExpression(pass, expression.X) {
			return false
		}
	}
	length, lengthKnown, capacity, capacityKnown := ps6080KnownContainerSizes(pass, expression.X)
	if !lengthKnown || !capacityKnown {
		return false
	}
	low, lowKnown := int64(0), true
	if expression.Low != nil {
		low, lowKnown = ps6080ConstantInteger(pass, expression.Low)
	}
	high, highKnown := length, true
	if expression.High != nil {
		high, highKnown = ps6080ConstantInteger(pass, expression.High)
	}
	maximum, maximumKnown := capacity, true
	if expression.Max != nil {
		maximum, maximumKnown = ps6080ConstantInteger(pass, expression.Max)
	}
	if !lowKnown || !highKnown || !maximumKnown || low < 0 || high < low || high > capacity {
		return false
	}
	if expression.Max != nil && (maximum < high || maximum > capacity) {
		return false
	}
	return true
}

func ps6080OrderedExpressionOutcome(
	pass *analysis.Pass,
	expression ast.Expr,
	context *ps6080ReturnabilityBodyContext,
	cache *ps6080ReturnFailureCache,
	mustReturn map[*ast.BlockStmt]bool,
	mustPanic map[*ast.BlockStmt]bool,
) ps6080ExpressionOutcome {
	if expression == nil {
		return ps6080ExpressionOutcome{mustReturn: true}
	}
	expression = ps2110Unparen(expression)
	sequence := func(expressions ...ast.Expr) ps6080ExpressionOutcome {
		return ps6080ExpressionSequenceOutcome(
			pass, expressions, context, cache, mustReturn, mustPanic,
		)
	}
	switch value := expression.(type) {
	case *ast.BasicLit, *ast.Ident, *ast.FuncLit:
		return ps6080ExpressionOutcome{mustReturn: true}
	case *ast.SelectorExpr:
		if outcome := sequence(value.X); !outcome.mustReturn {
			return outcome
		}
		if ps6080SelectorRootMustPanic(pass, value) {
			return ps6080ExpressionOutcome{mustPanic: true}
		}
		if path := ps6080SelectorEmbeddedPathOutcome(pass, value); !path.mustReturn {
			return path
		}
		selection := pass.TypesInfo.Selections[value]
		if selection != nil && selection.Kind() == types.MethodVal {
			method, _ := selection.Obj().(*types.Func)
			if method != nil {
				signature, _ := method.Type().(*types.Signature)
				if signature != nil && signature.Recv() != nil {
					if _, pointerReceiver := types.Unalias(
						signature.Recv().Type(),
					).Underlying().(*types.Pointer); pointerReceiver {
						return ps6080ExpressionOutcome{mustReturn: true}
					}
				}
			}
		}
		typeOf := pass.TypesInfo.TypeOf(value.X)
		if typeOf != nil {
			if _, pointer := types.Unalias(typeOf).Underlying().(*types.Pointer); pointer {
				return ps6080ExpressionOutcome{}
			}
		}
		return ps6080ExpressionOutcome{mustReturn: true}
	case *ast.IndexExpr:
		if outcome := sequence(value.X, value.Index); !outcome.mustReturn {
			return outcome
		}
		if ps6080IndexRootMustPanic(pass, value) {
			return ps6080ExpressionOutcome{mustPanic: true}
		}
		return ps6080ExpressionOutcome{mustReturn: ps6080IndexRootMustReturn(pass, value)}
	case *ast.IndexListExpr:
		return sequence(value.X)
	case *ast.StarExpr:
		if outcome := sequence(value.X); !outcome.mustReturn {
			return outcome
		}
		if ps6080DefinitelyNilTypedExpression(pass, value.X) {
			return ps6080ExpressionOutcome{mustPanic: true}
		}
		return ps6080ExpressionOutcome{}
	case *ast.CallExpr:
		if pass.TypesInfo.Types[value].Value != nil {
			return ps6080ExpressionOutcome{mustReturn: true}
		}
		if !pass.TypesInfo.Types[value.Fun].IsType() {
			if outcome := sequence(value.Fun); !outcome.mustReturn {
				return outcome
			}
		}
		if outcome := sequence(value.Args...); !outcome.mustReturn {
			return outcome
		}
		if pass.TypesInfo.Types[value.Fun].IsType() {
			if ps6080SliceToArrayMustPanic(pass, value) {
				return ps6080ExpressionOutcome{mustPanic: true}
			}
			return ps6080ExpressionOutcome{mustReturn: true}
		}
		if _, deletePanics := ps6080BuiltinDeleteOutcome(pass, value); deletePanics {
			return ps6080ExpressionOutcome{mustPanic: true}
		}
		if ps6080BuiltinPanic(pass, value) || ps6080BuiltinCloseNil(pass, value) ||
			ps6080CallMustPanicWithFacts(pass, value, context, cache, mustPanic) {
			return ps6080ExpressionOutcome{mustPanic: true}
		}
		return ps6080ExpressionOutcome{mustReturn: ps6080CallMustReturnWithFacts(
			pass, value, context, cache, mustReturn, make(map[*ast.BlockStmt]bool),
		)}
	case *ast.UnaryExpr:
		if outcome := sequence(value.X); !outcome.mustReturn {
			return outcome
		}
		if value.Op == token.ARROW {
			if ps6080SelectCommunicationOperation(context.parents, value) {
				return ps6080ExpressionOutcome{mustReturn: true}
			}
			return ps6080ExpressionOutcome{}
		}
		return ps6080ExpressionOutcome{mustReturn: true}
	case *ast.BinaryExpr:
		left := ps6080OrderedExpressionOutcome(
			pass, value.X, context, cache, mustReturn, mustPanic,
		)
		if !left.mustReturn {
			return left
		}
		if value.Op == token.LAND || value.Op == token.LOR {
			leftValue := pass.TypesInfo.Types[value.X].Value
			if leftValue != nil {
				evaluatesRight := value.Op == token.LAND && constant.BoolVal(leftValue) ||
					value.Op == token.LOR && !constant.BoolVal(leftValue)
				if !evaluatesRight {
					return ps6080ExpressionOutcome{mustReturn: true}
				}
				return ps6080OrderedExpressionOutcome(
					pass, value.Y, context, cache, mustReturn, mustPanic,
				)
			}
			right := ps6080OrderedExpressionOutcome(
				pass, value.Y, context, cache, mustReturn, mustPanic,
			)
			return ps6080ExpressionOutcome{mustReturn: right.mustReturn}
		}
		if outcome := sequence(value.Y); !outcome.mustReturn {
			return outcome
		}
		if ps6080InterfaceComparisonMustPanic(pass, value) {
			return ps6080ExpressionOutcome{mustPanic: true}
		}
		if relevant, comparisonReturns, _ := ps6080InterfaceComparisonOutcome(pass, value); relevant && !comparisonReturns {
			return ps6080ExpressionOutcome{}
		}
		if value.Op == token.QUO || value.Op == token.REM {
			divisor := pass.TypesInfo.Types[value.Y].Value
			if divisor == nil {
				return ps6080ExpressionOutcome{}
			}
			if divisor.Kind() == constant.Int && constant.Sign(divisor) == 0 {
				return ps6080ExpressionOutcome{mustPanic: true}
			}
		}
		if !ps6080ShiftMustReturn(pass, value) {
			shift, known := ps6080ConstantInteger(pass, value.Y)
			if known && shift < 0 {
				return ps6080ExpressionOutcome{mustPanic: true}
			}
			return ps6080ExpressionOutcome{}
		}
		return ps6080ExpressionOutcome{mustReturn: true}
	case *ast.SliceExpr:
		if outcome := sequence(value.X, value.Low, value.High, value.Max); !outcome.mustReturn {
			return outcome
		}
		if ps6080SliceRootMustPanic(pass, value) {
			return ps6080ExpressionOutcome{mustPanic: true}
		}
		return ps6080ExpressionOutcome{mustReturn: ps6080SliceRootMustReturn(pass, value)}
	case *ast.TypeAssertExpr:
		if outcome := sequence(value.X); !outcome.mustReturn {
			return outcome
		}
		if ps6080TypeAssertionMustPanic(pass, context.parents, value) {
			return ps6080ExpressionOutcome{mustPanic: true}
		}
		if value.Type == nil || ps6080CommaOKTypeAssertion(context.parents, value) {
			return ps6080ExpressionOutcome{mustReturn: true}
		}
		_, _, known := ps6080ExactInterfaceDynamicType(pass, value.X)
		return ps6080ExpressionOutcome{mustReturn: known}
	case *ast.CompositeLit:
		typeOf := pass.TypesInfo.TypeOf(value)
		var mapping bool
		if typeOf != nil {
			_, mapping = types.Unalias(typeOf).Underlying().(*types.Map)
		}
		for _, element := range value.Elts {
			if keyed, keyValue := element.(*ast.KeyValueExpr); keyValue {
				if outcome := sequence(keyed.Key, keyed.Value); !outcome.mustReturn {
					return outcome
				}
				if mapping {
					keyReturns, keyPanics := ps6080InterfaceKeyOutcome(pass, value, keyed.Key)
					if keyPanics {
						return ps6080ExpressionOutcome{mustPanic: true}
					}
					if !keyReturns {
						return ps6080ExpressionOutcome{}
					}
				}
			} else {
				if outcome := sequence(element); !outcome.mustReturn {
					return outcome
				}
			}
		}
		return ps6080ExpressionOutcome{mustReturn: true}
	}
	return ps6080ExpressionOutcome{mustReturn: true}
}

func ps6080AssignmentDestinationOperandsOutcome(
	pass *analysis.Pass,
	destination ast.Expr,
	context *ps6080ReturnabilityBodyContext,
	cache *ps6080ReturnFailureCache,
	mustReturn map[*ast.BlockStmt]bool,
	mustPanic map[*ast.BlockStmt]bool,
) ps6080ExpressionOutcome {
	destination = ps2110Unparen(destination)
	var operands []ast.Expr
	switch value := destination.(type) {
	case *ast.IndexExpr:
		operands = []ast.Expr{value.X, value.Index}
	case *ast.SelectorExpr:
		if outcome := ps6080ExpressionSequenceOutcome(
			pass, []ast.Expr{value.X}, context, cache, mustReturn, mustPanic,
		); !outcome.mustReturn {
			return outcome
		}
		if ps6080SelectorRootMustPanic(pass, value) {
			return ps6080ExpressionOutcome{mustPanic: true}
		}
		if path := ps6080SelectorEmbeddedPathOutcome(pass, value); !path.mustReturn {
			return path
		}
		typeOf := pass.TypesInfo.TypeOf(value.X)
		if typeOf != nil {
			if _, pointer := types.Unalias(typeOf).Underlying().(*types.Pointer); pointer {
				return ps6080ExpressionOutcome{
					mustReturn: ps6080DefinitelyNonNilPointerExpression(pass, value.X),
				}
			}
		}
		return ps6080ExpressionOutcome{mustReturn: true}
	case *ast.StarExpr:
		operands = []ast.Expr{value.X}
	default:
		return ps6080ExpressionOutcome{mustReturn: true}
	}
	return ps6080ExpressionSequenceOutcome(
		pass, operands, context, cache, mustReturn, mustPanic,
	)
}

func ps6080AssignmentDestinationRootOutcome(
	pass *analysis.Pass,
	destination ast.Expr,
	parents map[ast.Node]ast.Node,
) ps6080ExpressionOutcome {
	destination = ps2110Unparen(destination)
	switch value := destination.(type) {
	case *ast.IndexExpr:
		if ps6080IndexRootMustPanic(pass, value) ||
			ps6080NilMapWriteMustPanic(pass, parents, value) {
			return ps6080ExpressionOutcome{mustPanic: true}
		}
		typeOf := pass.TypesInfo.TypeOf(value.X)
		if typeOf != nil {
			if _, mapping := types.Unalias(typeOf).Underlying().(*types.Map); mapping {
				keyReturns, _ := ps6080InterfaceKeyOutcome(pass, value.X, value.Index)
				return ps6080ExpressionOutcome{mustReturn: keyReturns &&
					ps6080DefinitelyNonNilMapExpression(pass, value.X)}
			}
		}
		return ps6080ExpressionOutcome{mustReturn: ps6080IndexRootMustReturn(pass, value)}
	case *ast.SelectorExpr:
		if ps6080SelectorRootMustPanic(pass, value) {
			return ps6080ExpressionOutcome{mustPanic: true}
		}
		if path := ps6080SelectorEmbeddedPathOutcome(pass, value); !path.mustReturn {
			return path
		}
		typeOf := pass.TypesInfo.TypeOf(value.X)
		if typeOf != nil {
			if _, pointer := types.Unalias(typeOf).Underlying().(*types.Pointer); pointer {
				return ps6080ExpressionOutcome{
					mustReturn: ps6080DefinitelyNonNilPointerExpression(pass, value.X),
				}
			}
		}
	case *ast.StarExpr:
		if ps6080DefinitelyNilTypedExpression(pass, value.X) {
			return ps6080ExpressionOutcome{mustPanic: true}
		}
		return ps6080ExpressionOutcome{}
	}
	return ps6080ExpressionOutcome{mustReturn: true}
}

func ps6080CompoundAssignmentOutcome(
	pass *analysis.Pass,
	assignment *ast.AssignStmt,
) ps6080ExpressionOutcome {
	switch assignment.Tok {
	case token.QUO_ASSIGN, token.REM_ASSIGN:
		if len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return ps6080ExpressionOutcome{}
		}
		typeOf := pass.TypesInfo.TypeOf(assignment.Lhs[0])
		if typeOf == nil {
			return ps6080ExpressionOutcome{}
		}
		basic, basicType := types.Unalias(typeOf).Underlying().(*types.Basic)
		if !basicType || basic.Info()&types.IsInteger == 0 {
			return ps6080ExpressionOutcome{mustReturn: true}
		}
		divisor := pass.TypesInfo.Types[assignment.Rhs[0]].Value
		if divisor == nil || divisor.Kind() != constant.Int {
			return ps6080ExpressionOutcome{}
		}
		if constant.Sign(divisor) == 0 {
			return ps6080ExpressionOutcome{mustPanic: true}
		}
		return ps6080ExpressionOutcome{mustReturn: true}
	case token.SHL_ASSIGN, token.SHR_ASSIGN:
		if len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return ps6080ExpressionOutcome{}
		}
		if shift, known := ps6080ConstantInteger(pass, assignment.Rhs[0]); known {
			if shift < 0 {
				return ps6080ExpressionOutcome{mustPanic: true}
			}
			return ps6080ExpressionOutcome{mustReturn: true}
		}
		typeOf := pass.TypesInfo.TypeOf(assignment.Rhs[0])
		if typeOf == nil {
			return ps6080ExpressionOutcome{}
		}
		basic, basicType := types.Unalias(typeOf).Underlying().(*types.Basic)
		return ps6080ExpressionOutcome{mustReturn: basicType && basic.Info()&types.IsUnsigned != 0}
	default:
		return ps6080ExpressionOutcome{mustReturn: true}
	}
}

func ps6080NodeOrderedExpressionOutcome(
	pass *analysis.Pass,
	node ast.Node,
	context *ps6080ReturnabilityBodyContext,
	cache *ps6080ReturnFailureCache,
	mustReturn map[*ast.BlockStmt]bool,
	mustPanic map[*ast.BlockStmt]bool,
) ps6080ExpressionOutcome {
	sequence := func(expressions []ast.Expr) ps6080ExpressionOutcome {
		return ps6080ExpressionSequenceOutcome(
			pass, expressions, context, cache, mustReturn, mustPanic,
		)
	}
	switch value := node.(type) {
	case ast.Expr:
		return sequence([]ast.Expr{value})
	case *ast.ExprStmt:
		return sequence([]ast.Expr{value.X})
	case *ast.AssignStmt:
		for _, destination := range value.Lhs {
			outcome := ps6080AssignmentDestinationOperandsOutcome(
				pass, destination, context, cache, mustReturn, mustPanic,
			)
			if !outcome.mustReturn {
				return outcome
			}
		}
		if outcome := sequence(value.Rhs); !outcome.mustReturn {
			return outcome
		}
		if outcome := ps6080CompoundAssignmentOutcome(pass, value); !outcome.mustReturn {
			return outcome
		}
		for _, destination := range value.Lhs {
			outcome := ps6080AssignmentDestinationRootOutcome(
				pass, destination, context.parents,
			)
			if !outcome.mustReturn {
				return outcome
			}
		}
		return ps6080ExpressionOutcome{mustReturn: true}
	case *ast.ReturnStmt:
		return sequence(value.Results)
	case *ast.GoStmt:
		expressions := []ast.Expr{value.Call.Fun}
		expressions = append(expressions, value.Call.Args...)
		return sequence(expressions)
	case *ast.DeferStmt:
		expressions := []ast.Expr{value.Call.Fun}
		expressions = append(expressions, value.Call.Args...)
		return sequence(expressions)
	case *ast.SendStmt:
		return sequence([]ast.Expr{value.Chan, value.Value})
	case *ast.IncDecStmt:
		if outcome := ps6080AssignmentDestinationOperandsOutcome(
			pass, value.X, context, cache, mustReturn, mustPanic,
		); !outcome.mustReturn {
			return outcome
		}
		return ps6080AssignmentDestinationRootOutcome(pass, value.X, context.parents)
	case *ast.DeclStmt:
		declaration, _ := value.Decl.(*ast.GenDecl)
		var expressions []ast.Expr
		if declaration != nil {
			for _, specification := range declaration.Specs {
				values, _ := specification.(*ast.ValueSpec)
				if values != nil {
					expressions = append(expressions, values.Values...)
				}
			}
		}
		return sequence(expressions)
	}
	return ps6080ExpressionOutcome{mustReturn: true}
}

func ps6080NodeReturnabilityEffects(
	pass *analysis.Pass,
	node ast.Node,
	context *ps6080ReturnabilityBodyContext,
	cache *ps6080ReturnFailureCache,
	callReturns func(*ast.CallExpr) bool,
	callPanics func(*ast.CallExpr) bool,
	evaluating map[*ast.BlockStmt]bool,
) ps6080ReturnabilityEffects {
	var effects ps6080ReturnabilityEffects
	ast.Inspect(node, func(candidate ast.Node) bool {
		if candidate == nil || effects.kills {
			return false
		}
		if candidate != node {
			if _, nestedFunction := candidate.(*ast.FuncLit); nestedFunction {
				return false
			}
			if _, nestedStatement := candidate.(ast.Stmt); nestedStatement {
				return false
			}
		}
		call, called := candidate.(*ast.CallExpr)
		if !called || !ps6080NodeReachableWithLiveness(
			pass, context.parents, call, true, true,
		) {
			return true
		}
		if pass.TypesInfo.Types[call.Fun].IsType() {
			return true
		}
		parent := context.parents[call]
		if statement, asynchronous := parent.(*ast.GoStmt); asynchronous && statement.Call == call {
			return true
		}
		if statement, deferred := parent.(*ast.DeferStmt); deferred && statement.Call == call {
			if callReturns(call) {
				effects.registersRecover = ps6080CallMustRecoverPanic(
					pass, call, context, cache,
				)
			} else if ps6080BuiltinPanic(pass, call) || ps6080BuiltinCloseNil(pass, call) ||
				callPanics != nil && callPanics(call) {
				effects.registersPanic = true
			} else {
				effects.registersHardNonreturn = true
			}
			return true
		}
		if !callReturns(call) {
			effects.kills = true
		}
		return !effects.kills
	})
	return effects
}

func ps6080BodyMayReturnWithFacts(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	cache *ps6080ReturnFailureCache,
	mayReturn map[*ast.BlockStmt]bool,
	evaluating map[*ast.BlockStmt]bool,
) bool {
	if body == nil {
		return true
	}
	if evaluating[body] {
		return false
	}
	evaluating[body] = true
	defer delete(evaluating, body)
	context := ps6080ReturnabilityContext(body, cache)
	context.mayEvaluations.Add(1)
	graph := context.graph
	if graph == nil || len(graph.Blocks) == 0 || !graph.Blocks[0].Live {
		return false
	}
	type pathState struct {
		block    *cfg.Block
		recovers bool
	}
	first := pathState{block: graph.Blocks[0]}
	seen := map[pathState]bool{first: true}
	queue := []pathState{first}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		blocked := false
		recovers := current.recovers
		for _, node := range current.block.Nodes {
			if ps6080NodeExpressionMustPanic(pass, node, context, cache) {
				if recovers {
					ps6080RecordRecoveredReturn(cache, body, node)
					return true
				}
				blocked = true
				break
			}
			_, definitelyBlocks := ps6080NodeChannelBlock(pass, node, context)
			if definitelyBlocks {
				blocked = true
				break
			}
			effects := ps6080NodeReturnabilityEffects(
				pass, node, context, cache,
				func(call *ast.CallExpr) bool {
					return ps6080CallMayReturnWithFacts(
						pass, call, context, cache, mayReturn, evaluating,
					)
				},
				func(call *ast.CallExpr) bool {
					return ps6080CallMustPanicWithFacts(
						pass, call, context, cache, cache.returnability.mustPanic,
					)
				},
				evaluating,
			)
			if effects.kills || effects.registersHardNonreturn ||
				effects.registersPanic && !recovers {
				blocked = true
				break
			}
			recovers = recovers || effects.registersRecover
			if _, returned := node.(*ast.ReturnStmt); returned ||
				node == context.sentinel || node == context.sentinelEx {
				return true
			}
		}
		if blocked {
			continue
		}
		for _, successor := range ps6080FeasibleSuccessors(pass, context.parents, current.block) {
			next := pathState{block: successor, recovers: recovers}
			if successor.Live && !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return false
}

func ps6080BodyMustReturnWithFacts(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	cache *ps6080ReturnFailureCache,
	mustReturn map[*ast.BlockStmt]bool,
	evaluating map[*ast.BlockStmt]bool,
) bool {
	if body == nil {
		return true
	}
	if evaluating[body] {
		return false
	}
	evaluating[body] = true
	defer delete(evaluating, body)
	context := ps6080ReturnabilityContext(body, cache)
	context.mustEvaluations.Add(1)
	graph := context.graph
	if graph == nil || len(graph.Blocks) == 0 || !graph.Blocks[0].Live {
		return false
	}
	type pathState struct {
		block    *cfg.Block
		recovers bool
	}
	type visitState uint8
	const (
		visiting visitState = iota + 1
		accepted
		rejected
	)
	states := make(map[pathState]visitState)
	var visits func(pathState) bool
	visits = func(current pathState) bool {
		switch states[current] {
		case visiting, rejected:
			return false
		case accepted:
			return true
		}
		states[current] = visiting
		recovers := current.recovers
		for _, node := range current.block.Nodes {
			outcome := ps6080NodeOrderedExpressionOutcome(
				pass, node, context, cache, mustReturn, cache.returnability.mustPanic,
			)
			if outcome.mustPanic {
				if recovers {
					ps6080RecordRecoveredReturn(cache, body, node)
					states[current] = accepted
					return true
				}
				states[current] = rejected
				return false
			}
			if !outcome.mustReturn {
				states[current] = rejected
				return false
			}
			if mayBlock, _ := ps6080NodeChannelBlock(pass, node, context); mayBlock {
				states[current] = rejected
				return false
			}
			effects := ps6080NodeReturnabilityEffects(
				pass, node, context, cache,
				func(call *ast.CallExpr) bool {
					return ps6080CallMustReturnWithFacts(
						pass, call, context, cache, mustReturn, evaluating,
					)
				},
				func(call *ast.CallExpr) bool {
					return ps6080CallMustPanicWithFacts(
						pass, call, context, cache, cache.returnability.mustPanic,
					)
				},
				evaluating,
			)
			if effects.kills || effects.registersHardNonreturn ||
				effects.registersPanic && !recovers {
				states[current] = rejected
				return false
			}
			recovers = recovers || effects.registersRecover
			if _, returned := node.(*ast.ReturnStmt); returned ||
				node == context.sentinel || node == context.sentinelEx {
				states[current] = accepted
				return true
			}
		}
		successors := ps6080FeasibleSuccessors(pass, context.parents, current.block)
		if len(successors) == 0 || ps6080SelectMayBlock(context.parents, successors) {
			states[current] = rejected
			return false
		}
		for _, successor := range successors {
			if !successor.Live || !visits(pathState{block: successor, recovers: recovers}) {
				states[current] = rejected
				return false
			}
		}
		states[current] = accepted
		return true
	}
	return visits(pathState{block: graph.Blocks[0]})
}

func ps6080BodyMustPanicWithFacts(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	cache *ps6080ReturnFailureCache,
	mustPanic map[*ast.BlockStmt]bool,
	mustReturn map[*ast.BlockStmt]bool,
) bool {
	if body == nil {
		return false
	}
	context := ps6080ReturnabilityContext(body, cache)
	context.panicEvaluations.Add(1)
	graph := context.graph
	if graph == nil || len(graph.Blocks) == 0 || !graph.Blocks[0].Live {
		return false
	}
	type visitState uint8
	const (
		visiting visitState = iota + 1
		accepted
		rejected
	)
	states := make(map[*cfg.Block]visitState, len(graph.Blocks))
	var visits func(*cfg.Block) bool
	visits = func(block *cfg.Block) bool {
		switch states[block] {
		case visiting, rejected:
			return false
		case accepted:
			return true
		}
		states[block] = visiting
		for _, node := range block.Nodes {
			if _, deferred := node.(*ast.DeferStmt); deferred {
				states[block] = rejected
				return false
			}
			outcome := ps6080NodeOrderedExpressionOutcome(
				pass, node, context, cache, mustReturn, mustPanic,
			)
			if outcome.mustPanic {
				states[block] = accepted
				return true
			}
			if !outcome.mustReturn {
				states[block] = rejected
				return false
			}
			if mayBlock, _ := ps6080NodeChannelBlock(pass, node, context); mayBlock {
				states[block] = rejected
				return false
			}
			if _, returned := node.(*ast.ReturnStmt); returned ||
				node == context.sentinel || node == context.sentinelEx {
				states[block] = rejected
				return false
			}
		}
		successors := ps6080FeasibleSuccessors(pass, context.parents, block)
		if len(successors) == 0 || ps6080SelectMayBlock(context.parents, successors) {
			states[block] = rejected
			return false
		}
		for _, successor := range successors {
			if !successor.Live || !visits(successor) {
				states[block] = rejected
				return false
			}
		}
		states[block] = accepted
		return true
	}
	return visits(graph.Blocks[0])
}

func ps6080ReturnabilityDependencies(
	pass *analysis.Pass,
	cache *ps6080ReturnFailureCache,
) (map[*ast.BlockStmt]bool, map[*ast.BlockStmt]map[*ast.BlockStmt]bool) {
	nodes := make(map[*ast.BlockStmt]bool, len(cache.declarations))
	reverse := make(map[*ast.BlockStmt]map[*ast.BlockStmt]bool)
	scanned := make(map[*ast.BlockStmt]bool)
	cache.returnability.owners = make(map[*ast.BlockStmt]*ast.BlockStmt)
	indexOwner := func(body *ast.BlockStmt) {
		if body == nil || cache.returnability.owners[body] != nil {
			return
		}
		cache.returnability.owners[body] = body
		ast.Inspect(body, func(node ast.Node) bool {
			if literal, nested := node.(*ast.FuncLit); nested {
				cache.returnability.owners[literal.Body] = body
			}
			return true
		})
	}
	for _, declaration := range cache.declarations {
		indexOwner(declaration.Body)
	}
	var scan func(*ast.BlockStmt)
	scan = func(body *ast.BlockStmt) {
		if body == nil || scanned[body] {
			return
		}
		scanned[body] = true
		nodes[body] = true
		context := ps6080ReturnabilityContext(body, cache)
		ast.Inspect(body, func(node ast.Node) bool {
			if node == nil {
				return false
			}
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			call, called := node.(*ast.CallExpr)
			if !called {
				return true
			}
			targets, complete := ps6080ResolveCallableExpression(
				pass, call.Fun, context, cache, make(map[types.Object]bool),
			)
			if !complete {
				return true
			}
			for _, target := range targets {
				var targetBody *ast.BlockStmt
				if target.literal != nil {
					targetBody = target.literal.Body
				} else if target.function != nil && target.function.Pkg() == pass.Pkg {
					if declaration := cache.declarations[target.function]; declaration != nil {
						targetBody = declaration.Body
					}
				}
				if targetBody == nil {
					continue
				}
				if target.literal != nil {
					indexOwner(targetBody)
				}
				nodes[targetBody] = true
				if reverse[targetBody] == nil {
					reverse[targetBody] = make(map[*ast.BlockStmt]bool)
				}
				reverse[targetBody][body] = true
				if target.literal != nil {
					scan(targetBody)
				}
			}
			return true
		})
	}
	for _, declaration := range cache.declarations {
		scan(declaration.Body)
	}
	return nodes, reverse
}

func ps6080BuildReturnabilityFacts(
	nodes map[*ast.BlockStmt]bool,
	reverse map[*ast.BlockStmt]map[*ast.BlockStmt]bool,
	facts map[*ast.BlockStmt]bool,
	evaluate func(*ast.BlockStmt) bool,
) {
	forward := make(map[*ast.BlockStmt]map[*ast.BlockStmt]bool, len(nodes))
	for callee, callers := range reverse {
		for caller := range callers {
			if forward[caller] == nil {
				forward[caller] = make(map[*ast.BlockStmt]bool)
			}
			forward[caller][callee] = true
		}
	}
	index := 0
	indices := make(map[*ast.BlockStmt]int, len(nodes))
	lowlink := make(map[*ast.BlockStmt]int, len(nodes))
	onStack := make(map[*ast.BlockStmt]bool, len(nodes))
	var stack []*ast.BlockStmt
	var components [][]*ast.BlockStmt
	var connect func(*ast.BlockStmt)
	connect = func(body *ast.BlockStmt) {
		index++
		indices[body] = index
		lowlink[body] = index
		stack = append(stack, body)
		onStack[body] = true
		for dependency := range forward[body] {
			if indices[dependency] == 0 {
				connect(dependency)
				lowlink[body] = min(lowlink[body], lowlink[dependency])
			} else if onStack[dependency] {
				lowlink[body] = min(lowlink[body], indices[dependency])
			}
		}
		if lowlink[body] != indices[body] {
			return
		}
		var component []*ast.BlockStmt
		for len(stack) > 0 {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[last] = false
			component = append(component, last)
			if last == body {
				break
			}
		}
		components = append(components, component)
	}
	for body := range nodes {
		if indices[body] == 0 {
			connect(body)
		}
	}
	componentOf := make(map[*ast.BlockStmt]int, len(nodes))
	for componentIndex, component := range components {
		for _, body := range component {
			componentOf[body] = componentIndex
		}
	}
	type componentEdge struct {
		callee int
		caller int
	}
	outgoing := make([][]int, len(components))
	edges := make(map[componentEdge]bool)
	indegree := make([]int, len(components))
	for callee, callers := range reverse {
		calleeComponent := componentOf[callee]
		for caller := range callers {
			callerComponent := componentOf[caller]
			if calleeComponent == callerComponent {
				continue
			}
			edge := componentEdge{callee: calleeComponent, caller: callerComponent}
			if !edges[edge] {
				edges[edge] = true
				outgoing[calleeComponent] = append(outgoing[calleeComponent], callerComponent)
				indegree[callerComponent]++
			}
		}
	}
	var componentQueue []int
	for componentIndex := range components {
		if indegree[componentIndex] == 0 {
			componentQueue = append(componentQueue, componentIndex)
		}
	}
	for len(componentQueue) > 0 {
		componentIndex := componentQueue[0]
		componentQueue = componentQueue[1:]
		component := components[componentIndex]
		queue := slices.Clone(component)
		queued := make(map[*ast.BlockStmt]bool, len(component))
		for _, body := range component {
			queued[body] = true
		}
		for len(queue) > 0 {
			body := queue[0]
			queue = queue[1:]
			queued[body] = false
			if facts[body] || !evaluate(body) {
				continue
			}
			facts[body] = true
			for caller := range reverse[body] {
				if componentOf[caller] == componentIndex && !facts[caller] && !queued[caller] {
					queued[caller] = true
					queue = append(queue, caller)
				}
			}
		}
		for _, callerComponent := range outgoing[componentIndex] {
			indegree[callerComponent]--
			if indegree[callerComponent] == 0 {
				componentQueue = append(componentQueue, callerComponent)
			}
		}
	}
}

func ps6080BuildJointReturnabilityFacts(
	nodes map[*ast.BlockStmt]bool,
	reverse map[*ast.BlockStmt]map[*ast.BlockStmt]bool,
	mustReturn map[*ast.BlockStmt]bool,
	mustPanic map[*ast.BlockStmt]bool,
	evaluateReturn func(*ast.BlockStmt) bool,
	evaluatePanic func(*ast.BlockStmt) bool,
) {
	queue := make([]*ast.BlockStmt, 0, len(nodes))
	queued := make(map[*ast.BlockStmt]bool, len(nodes))
	enqueue := func(body *ast.BlockStmt) {
		if body != nil && !queued[body] && (!mustReturn[body] || !mustPanic[body]) {
			queued[body] = true
			queue = append(queue, body)
		}
	}
	// The first two fixed-point passes already prove every return fact that is
	// independent of panic facts, followed by every panic fact enabled by those
	// returns. A newly proved panic can only affect its callers, so seed the
	// joint worklist at those dependency edges instead of evaluating every body
	// a second time. Further return/panic discoveries propagate the same way.
	for callee, panics := range mustPanic {
		if !panics {
			continue
		}
		for caller := range reverse[callee] {
			enqueue(caller)
		}
	}
	for len(queue) > 0 {
		body := queue[0]
		queue = queue[1:]
		queued[body] = false
		changed := false
		if !mustReturn[body] && evaluateReturn(body) {
			mustReturn[body] = true
			changed = true
		}
		if !mustPanic[body] && evaluatePanic(body) {
			mustPanic[body] = true
			changed = true
		}
		if !changed {
			continue
		}
		for caller := range reverse[body] {
			enqueue(caller)
		}
	}
}

func ps6080BuildReturnabilityIndex(pass *analysis.Pass, cache *ps6080ReturnFailureCache) {
	cache.returnability.once.Do(func() {
		cache.returnability.builds++
		ps6080FunctionDeclaration(pass, cache, nil)
		nodes, reverse := ps6080ReturnabilityDependencies(pass, cache)
		cache.returnability.mayReturn = make(map[*ast.BlockStmt]bool, len(nodes))
		cache.returnability.mustReturn = make(map[*ast.BlockStmt]bool, len(nodes))
		cache.returnability.mustPanic = make(map[*ast.BlockStmt]bool, len(nodes))
		cache.returnability.recoveredReturns = make(map[*ast.BlockStmt]map[ast.Node]bool)
		ps6080BuildReturnabilityFacts(
			nodes, reverse, cache.returnability.mustReturn,
			func(body *ast.BlockStmt) bool {
				return ps6080BodyMustReturnWithFacts(
					pass, body, cache, cache.returnability.mustReturn,
					make(map[*ast.BlockStmt]bool),
				)
			},
		)
		ps6080BuildReturnabilityFacts(
			nodes, reverse, cache.returnability.mustPanic,
			func(body *ast.BlockStmt) bool {
				return ps6080BodyMustPanicWithFacts(
					pass, body, cache, cache.returnability.mustPanic,
					cache.returnability.mustReturn,
				)
			},
		)
		ps6080BuildJointReturnabilityFacts(
			nodes, reverse,
			cache.returnability.mustReturn, cache.returnability.mustPanic,
			func(body *ast.BlockStmt) bool {
				return ps6080BodyMustReturnWithFacts(
					pass, body, cache, cache.returnability.mustReturn,
					make(map[*ast.BlockStmt]bool),
				)
			},
			func(body *ast.BlockStmt) bool {
				return ps6080BodyMustPanicWithFacts(
					pass, body, cache, cache.returnability.mustPanic,
					cache.returnability.mustReturn,
				)
			},
		)
		ps6080BuildReturnabilityFacts(
			nodes, reverse, cache.returnability.mayReturn,
			func(body *ast.BlockStmt) bool {
				return ps6080BodyMayReturnWithFacts(
					pass, body, cache, cache.returnability.mayReturn,
					make(map[*ast.BlockStmt]bool),
				)
			},
		)
	})
}

func ps6080ResultStateAlwaysSupports(
	pass *analysis.Pass,
	state ps6080NamedResultState,
	nilSupports bool,
) bool {
	if !state.known {
		return false
	}
	if state.expression == nil {
		return nilSupports
	}
	if state.resultIndex < 0 {
		expression := ps2110Unparen(state.expression)
		if ps6080NilExpression(pass, expression) {
			return nilSupports
		}
		return pass.TypesInfo.Types[expression].Value != nil &&
			!ps6080ZeroOrFailureExpression(pass, expression, nilSupports)
	}
	return ps6080CallResultAlwaysSupports(
		pass, state.expression, state.resultIndex, nilSupports,
		ps6080ReturnFailureCacheFor(pass), make(map[ps6080ReturnFailureKey]bool),
	)
}

func ps6080RecoveredAssignmentCommittedPrefix(
	pass *analysis.Pass,
	assignment *ast.AssignStmt,
	context *ps6080ReturnabilityBodyContext,
	cache *ps6080ReturnFailureCache,
) (int, bool) {
	mustReturn := cache.returnability.mustReturn
	mustPanic := cache.returnability.mustPanic
	for _, destination := range assignment.Lhs {
		outcome := ps6080AssignmentDestinationOperandsOutcome(
			pass, destination, context, cache, mustReturn, mustPanic,
		)
		if outcome.mustPanic {
			return 0, true
		}
		if !outcome.mustReturn {
			return 0, false
		}
	}
	if outcome := ps6080ExpressionSequenceOutcome(
		pass, assignment.Rhs, context, cache, mustReturn, mustPanic,
	); outcome.mustPanic {
		return 0, true
	} else if !outcome.mustReturn {
		return 0, false
	}
	if outcome := ps6080CompoundAssignmentOutcome(pass, assignment); outcome.mustPanic {
		return 0, true
	} else if !outcome.mustReturn {
		return 0, false
	}
	for index, destination := range assignment.Lhs {
		outcome := ps6080AssignmentDestinationRootOutcome(
			pass, destination, context.parents,
		)
		if outcome.mustPanic {
			return index, true
		}
		if !outcome.mustReturn {
			return 0, false
		}
	}
	return 0, false
}

func ps6080InvalidateRecoveredAssignmentPrefix(
	pass *analysis.Pass,
	function *ps6080Function,
	assignment *ast.AssignStmt,
	committed int,
	states []ps6080NamedResultState,
) {
	signature := ps6080FunctionSignature(function)
	if signature == nil {
		clear(states)
		return
	}
	results := make(map[types.Object]int, signature.Results().Len())
	for index := range signature.Results().Len() {
		results[signature.Results().At(index)] = index
	}
	committed = min(committed, len(assignment.Lhs))
	for _, destination := range assignment.Lhs[:committed] {
		identifier, direct := ps2110Unparen(destination).(*ast.Ident)
		if !direct {
			continue
		}
		if resultIndex, result := results[pass.TypesInfo.ObjectOf(identifier)]; result {
			states[resultIndex] = ps6080NamedResultState{}
		}
	}
}

func ps6080FunctionReturnSummaryFor(
	pass *analysis.Pass,
	cache *ps6080ReturnFailureCache,
	functionObject *types.Func,
) *ps6080FunctionReturnSummary {
	candidate := &ps6080FunctionReturnSummary{}
	value, _ := cache.summaries.LoadOrStore(functionObject, candidate)
	summary := value.(*ps6080FunctionReturnSummary)
	summary.once.Do(func() {
		summary.builds++
		declaration := ps6080FunctionDeclaration(pass, cache, functionObject)
		signature, _ := functionObject.Type().(*types.Signature)
		if declaration == nil || signature == nil {
			return
		}
		function := &ps6080Function{
			declaration: declaration,
			object:      functionObject,
			signature:   signature,
			body:        declaration.Body,
		}
		parents := ps6071Parents(declaration.Body)
		graph := cfg.New(declaration.Body, ps6080CallMayReturn(pass))
		ps6080BuildReturnabilityIndex(pass, cache)
		summary.allPathsReturn = cache.returnability.mustReturn[declaration.Body]
		indirect := ps6080NamedResultsMayChangeIndirectlyCached(pass, function)
		summary.complete = true
		ast.Inspect(declaration.Body, func(node ast.Node) bool {
			if !summary.complete {
				return false
			}
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			returned, returnedNode := node.(*ast.ReturnStmt)
			if !returnedNode || !ps6080NodeReachable(pass, graph, parents, returned) ||
				!ps6080StaticallyReachable(pass, parents, returned) {
				return true
			}
			states, resolved := ps6080ResolvedReturnStatesWithIndirect(
				pass, function, returned, indirect,
			)
			if !resolved || len(states) != signature.Results().Len() {
				summary.complete = false
				return false
			}
			summary.returns = append(summary.returns, states)
			return false
		})
		for recovered := range cache.returnability.recoveredReturns[declaration.Body] {
			if !summary.complete {
				break
			}
			if indirect {
				summary.complete = false
				break
			}
			states, resolved := ps6080NamedResultStateAt(pass, function, recovered)
			if !resolved || len(states) != signature.Results().Len() {
				summary.complete = false
				break
			}
			if assignment, assigned := recovered.(*ast.AssignStmt); assigned {
				context := ps6080ReturnabilityContext(declaration.Body, cache)
				committed, located := ps6080RecoveredAssignmentCommittedPrefix(
					pass, assignment, context, cache,
				)
				if !located {
					clear(states)
				} else {
					ps6080InvalidateRecoveredAssignmentPrefix(
						pass, function, assignment, committed, states,
					)
				}
			}
			summary.returns = append(summary.returns, states)
		}
		if len(summary.returns) == 0 {
			summary.complete = false
		}
	})
	return summary
}

func ps6080CallResultAlwaysFailure(
	pass *analysis.Pass,
	expression ast.Expr,
	resultIndex int,
	nilSupports bool,
	cache *ps6080ReturnFailureCache,
	visiting map[ps6080ReturnFailureKey]bool,
) bool {
	call, called := ps2110Unparen(expression).(*ast.CallExpr)
	if !called {
		return false
	}
	callee, _, known := typedCallee(pass, call.Fun)
	if !known || callee.Pkg() != pass.Pkg {
		return false
	}
	key := ps6080ReturnFailureKey{
		function: callee, resultIndex: resultIndex, nilSupports: nilSupports,
	}
	if cached, found := cache.failures.Load(key); found {
		return cached.(bool)
	}
	if visiting[key] {
		return false
	}
	signature, _ := callee.Type().(*types.Signature)
	if signature == nil || resultIndex < 0 ||
		resultIndex >= signature.Results().Len() {
		return false
	}
	visiting[key] = true
	defer delete(visiting, key)
	summary := ps6080FunctionReturnSummaryFor(pass, cache, callee)
	allFailure := summary.complete
	for _, states := range summary.returns {
		if !allFailure || !states[resultIndex].known {
			allFailure = false
			break
		}
		state := states[resultIndex]
		if state.expression == nil {
			allFailure = !nilSupports
		} else if state.resultIndex < 0 {
			allFailure = ps6080ZeroOrFailureExpression(
				pass, state.expression, nilSupports,
			)
		} else {
			allFailure = ps6080CallResultAlwaysFailure(
				pass, state.expression, state.resultIndex, nilSupports, cache, visiting,
			)
		}
	}
	cache.failures.Store(key, allFailure)
	return allFailure
}

func ps6080CallResultAlwaysSupports(
	pass *analysis.Pass,
	expression ast.Expr,
	resultIndex int,
	nilSupports bool,
	cache *ps6080ReturnFailureCache,
	visiting map[ps6080ReturnFailureKey]bool,
) bool {
	call, called := ps2110Unparen(expression).(*ast.CallExpr)
	if !called {
		return false
	}
	callee, _, known := typedCallee(pass, call.Fun)
	if !known || callee.Pkg() != pass.Pkg {
		return false
	}
	key := ps6080ReturnFailureKey{
		function: callee, resultIndex: resultIndex, nilSupports: nilSupports,
	}
	if cached, found := cache.supports.Load(key); found {
		return cached.(bool)
	}
	if visiting[key] {
		return false
	}
	signature, _ := callee.Type().(*types.Signature)
	if signature == nil || resultIndex < 0 || resultIndex >= signature.Results().Len() {
		return false
	}
	visiting[key] = true
	defer delete(visiting, key)
	summary := ps6080FunctionReturnSummaryFor(pass, cache, callee)
	allSupport := summary.complete && summary.allPathsReturn
	for _, states := range summary.returns {
		if !allSupport || !states[resultIndex].known {
			allSupport = false
			break
		}
		state := states[resultIndex]
		if state.expression == nil {
			allSupport = nilSupports
		} else if state.resultIndex < 0 {
			expression := ps2110Unparen(state.expression)
			if ps6080NilExpression(pass, expression) {
				allSupport = nilSupports
			} else {
				allSupport = pass.TypesInfo.Types[expression].Value != nil &&
					!ps6080ZeroOrFailureExpression(pass, expression, nilSupports)
			}
		} else {
			allSupport = ps6080CallResultAlwaysSupports(
				pass, state.expression, state.resultIndex, nilSupports, cache, visiting,
			)
		}
	}
	cache.supports.Store(key, allSupport)
	return allSupport
}

func ps6080NamedReturnState(
	pass *analysis.Pass,
	function *ps6080Function,
	returned *ast.ReturnStmt,
) ([]ps6080NamedResultState, bool) {
	return ps6080NamedResultStateAt(pass, function, returned)
}

func ps6080NamedResultStateAt(
	pass *analysis.Pass,
	function *ps6080Function,
	node ast.Node,
) ([]ps6080NamedResultState, bool) {
	body := ps6080FunctionBody(function)
	if body == nil || node == nil {
		return nil, false
	}
	cache := ps6080ReturnFailureCacheFor(pass)
	candidate := &ps6080NamedReturnStateIndex{}
	value, _ := cache.namedStates.LoadOrStore(body, candidate)
	index := value.(*ps6080NamedReturnStateIndex)
	index.once.Do(func() {
		index.builds++
		index.states, index.valid = ps6080BuildNamedReturnStateIndex(pass, function)
	})
	state, found := index.states[node]
	if !index.valid || !found {
		return nil, false
	}
	return slices.Clone(state), true
}

func ps6080BuildNamedReturnStateIndex(
	pass *analysis.Pass,
	function *ps6080Function,
) (map[ast.Node][]ps6080NamedResultState, bool) {
	signature := ps6080FunctionSignature(function)
	body := ps6080FunctionBody(function)
	if signature == nil || body == nil {
		return nil, false
	}
	objects := make(map[types.Object]int, signature.Results().Len())
	initial := make([]ps6080NamedResultState, signature.Results().Len())
	for index := range signature.Results().Len() {
		result := signature.Results().At(index)
		if result.Name() == "" {
			return nil, false
		}
		objects[result] = index
		initial[index].known = true
		initial[index].resultIndex = -1
	}
	parents := ps6071Parents(body)

	clone := func(source []ps6080NamedResultState) []ps6080NamedResultState {
		return slices.Clone(source)
	}
	merge := func(destination, source []ps6080NamedResultState) bool {
		changed := false
		for index := range destination {
			if destination[index].known &&
				(!source[index].known || destination[index].expression != source[index].expression ||
					destination[index].resultIndex != source[index].resultIndex) {
				destination[index] = ps6080NamedResultState{}
				changed = true
			}
		}
		return changed
	}
	touches := func(expression ast.Expr) (int, bool) {
		index := -1
		ast.Inspect(expression, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			if resultIndex, result := objects[pass.TypesInfo.ObjectOf(identifier)]; result {
				index = resultIndex
				return false
			}
			return true
		})
		return index, index >= 0
	}
	transfer := func(state []ps6080NamedResultState, node ast.Node) []ps6080NamedResultState {
		if expression, ok := node.(ast.Expr); ok {
			if ranged, rangeExpression := parents[expression].(*ast.RangeStmt); rangeExpression &&
				(ranged.Key == expression || ranged.Value == expression) &&
				!ps6080StaticallyEmptyRange(pass, ranged) {
				if resultIndex, result := touches(expression); result {
					state[resultIndex] = ps6080NamedResultState{}
				}
			}
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			tupleAssignment := false
			if len(value.Rhs) == 1 {
				tuple, tupleResult := pass.TypesInfo.TypeOf(value.Rhs[0]).(*types.Tuple)
				tupleAssignment = tupleResult && tuple.Len() == len(value.Lhs)
			}
			for leftIndex, left := range value.Lhs {
				identifier, direct := ps2110Unparen(left).(*ast.Ident)
				if direct {
					if resultIndex, result := objects[pass.TypesInfo.ObjectOf(identifier)]; result {
						if (value.Tok == token.ASSIGN || value.Tok == token.DEFINE) &&
							len(value.Lhs) == len(value.Rhs) {
							state[resultIndex] = ps6080NamedResultState{
								expression: value.Rhs[leftIndex], resultIndex: -1, known: true,
							}
						} else if (value.Tok == token.ASSIGN || value.Tok == token.DEFINE) &&
							tupleAssignment {
							state[resultIndex] = ps6080NamedResultState{
								expression: value.Rhs[0], resultIndex: leftIndex, known: true,
							}
						} else {
							state[resultIndex] = ps6080NamedResultState{}
						}
					}
					continue
				}
				if resultIndex, result := touches(left); result {
					state[resultIndex] = ps6080NamedResultState{}
				}
			}
		case *ast.IncDecStmt:
			if resultIndex, result := touches(value.X); result {
				state[resultIndex] = ps6080NamedResultState{}
			}
		}
		return state
	}

	graph := cfg.New(body, ps6080CallMayReturn(pass))
	if len(graph.Blocks) == 0 || !graph.Blocks[0].Live {
		return nil, false
	}
	states := map[*cfg.Block][]ps6080NamedResultState{graph.Blocks[0]: clone(initial)}
	pending := []*cfg.Block{graph.Blocks[0]}
	inQueue := map[*cfg.Block]bool{graph.Blocks[0]: true}
	for len(pending) > 0 {
		block := pending[0]
		pending = pending[1:]
		inQueue[block] = false
		state := clone(states[block])
		for _, node := range block.Nodes {
			state = transfer(state, node)
		}
		for _, successor := range ps6080FeasibleSuccessors(pass, parents, block) {
			if !successor.Live {
				continue
			}
			if states[successor] == nil {
				states[successor] = clone(state)
				if !inQueue[successor] {
					inQueue[successor] = true
					pending = append(pending, successor)
				}
			} else if merge(states[successor], state) && !inQueue[successor] {
				inQueue[successor] = true
				pending = append(pending, successor)
			}
		}
	}
	result := make(map[ast.Node][]ps6080NamedResultState)
	for _, block := range graph.Blocks {
		if states[block] == nil {
			continue
		}
		state := clone(states[block])
		for _, node := range block.Nodes {
			result[node] = clone(state)
			state = transfer(state, node)
		}
	}
	return result, true
}

func ps6080NakedReturnAllFailure(
	pass *analysis.Pass,
	function *ps6080Function,
	returned *ast.ReturnStmt,
) bool {
	signature := ps6080FunctionSignature(function)
	if signature == nil || signature.Results().Len() == 0 {
		return false
	}
	state, resolved := ps6080NamedReturnState(pass, function, returned)
	if !resolved {
		return false
	}
	return ps6080ReturnStatesRejected(pass, function, state)
}

func ps6080ReturnStatesRejected(
	pass *analysis.Pass,
	function *ps6080Function,
	state []ps6080NamedResultState,
) bool {
	for index := range state {
		if state[index].known && state[index].expression != nil &&
			ps6080NilResultSupports(function, index) &&
			ps6080ResultStateZeroOrFailure(pass, state[index], true) {
			return true
		}
	}
	for index := range state {
		if !state[index].known {
			return false
		}
		expression := state[index].expression
		if expression == nil {
			if ps6080NilResultSupports(function, index) {
				return false
			}
			continue
		}
		if !ps6080ResultStateZeroOrFailure(
			pass, state[index], ps6080NilResultSupports(function, index),
		) {
			return false
		}
	}
	return true
}

func ps6080AssignmentTouchesObjects(
	pass *analysis.Pass,
	assignment *ast.AssignStmt,
	objects map[types.Object]bool,
) bool {
	for _, left := range assignment.Lhs {
		identifier, direct := ps2110Unparen(left).(*ast.Ident)
		if direct && objects[pass.TypesInfo.ObjectOf(identifier)] {
			return true
		}
	}
	return false
}

func ps6080NakedReturnResults(
	pass *analysis.Pass,
	function *ps6080Function,
	returned *ast.ReturnStmt,
) ([]ast.Expr, bool) {
	signature := ps6080FunctionSignature(function)
	if signature == nil {
		return nil, false
	}
	if signature.Results().Len() == 0 {
		return []ast.Expr{}, true
	}
	state, resolved := ps6080NamedReturnState(pass, function, returned)
	if !resolved {
		return nil, false
	}
	results := make([]ast.Expr, signature.Results().Len())
	for index := range signature.Results().Len() {
		result := signature.Results().At(index)
		if result.Name() == "" || !state[index].known || state[index].expression == nil {
			return nil, false
		}
		results[index] = state[index].expression
	}
	return results, true
}

func ps6080NarrowSwitchClauseGroups(
	pass *analysis.Pass,
	function *ps6080Function,
	statements []ast.Stmt,
	groups map[*types.TypeName]*ps6080ConstantGroup,
	subjects map[*types.TypeName]types.Object,
	constantEnums map[*types.Const][]*types.TypeName,
) {
	predicates, unconditional, resolved := ps6080SwitchClausePredicates(
		pass, function, statements, groups, subjects, constantEnums,
	)
	if !resolved || unconditional {
		return
	}
	for enum, group := range groups {
		if predicate := predicates[enum]; predicate != nil {
			groups[enum] = ps6080IntersectGroups(group, predicate)
		}
	}
}

func ps6080SwitchClausePredicates(
	pass *analysis.Pass,
	function *ps6080Function,
	statements []ast.Stmt,
	targets map[*types.TypeName]*ps6080ConstantGroup,
	targetSubjects map[*types.TypeName]types.Object,
	constantEnums map[*types.Const][]*types.TypeName,
) (map[*types.TypeName]*ps6080ConstantGroup, bool, bool) {
	if len(statements) == 0 || ps6080StatementsHaveBranch(statements) {
		return nil, false, false
	}
	returned, ok := ps6080SingleTerminalReturn(statements)
	if !ok {
		return nil, false, false
	}
	results := returned.Results
	if len(results) == 0 {
		if ps6080NamedResultsMayChangeIndirectlyCached(pass, function) {
			return nil, false, false
		}
		for _, statement := range statements[:len(statements)-1] {
			assignment, straightLine := statement.(*ast.AssignStmt)
			if !straightLine || len(assignment.Lhs) != len(assignment.Rhs) ||
				ps6080AssignmentMayChangeNamedResult(pass, function, assignment) {
				return nil, false, false
			}
		}
		var resolved bool
		results, resolved = ps6080NakedReturnResults(pass, function, returned)
		if !resolved {
			return nil, false, false
		}
	}
	predicates := make(map[*types.TypeName]*ps6080ConstantGroup)
	for index, result := range results {
		typeOf := pass.TypesInfo.TypeOf(result)
		if typeOf != nil {
			if basic, boolean := types.Unalias(typeOf).Underlying().(*types.Basic); boolean && basic.Info()&types.IsBoolean != 0 {
				parsed, _ := ps6080GuardExpression(pass, result, constantEnums)
				relevant := make(map[*types.TypeName]*ps6080ConstantGroup)
				targeted := false
				for enum, group := range parsed {
					if targets[enum] == nil {
						continue
					}
					targeted = true
					subject, unique := ps6080EnumSubject(pass, result, enum)
					if unique && targetSubjects[enum] == subject {
						relevant[enum] = group
					}
				}
				if len(relevant) > 0 {
					ps6080MergeGuardGroups(predicates, relevant, token.LOR)
					continue
				}
				if targeted {
					return nil, false, false
				}
			}
		}
		if !ps6080ZeroOrFailureExpression(pass, result, ps6080NilResultSupports(function, index)) {
			return nil, true, true
		}
	}
	return predicates, false, true
}

func ps6080SwitchClauseTerminalStatements(switchStmt *ast.SwitchStmt, clause *ast.CaseClause) []ast.Stmt {
	index := slices.Index(switchStmt.Body.List, ast.Stmt(clause))
	seen := make(map[*ast.CaseClause]bool)
	for index >= 0 && index < len(switchStmt.Body.List) {
		current, ok := switchStmt.Body.List[index].(*ast.CaseClause)
		if !ok || seen[current] {
			return nil
		}
		seen[current] = true
		if len(current.Body) > 0 {
			if branch, ok := current.Body[len(current.Body)-1].(*ast.BranchStmt); ok && branch.Tok == token.FALLTHROUGH {
				if len(current.Body) != 1 {
					return nil
				}
				index++
				continue
			}
		}
		return current.Body
	}
	return nil
}

func ps6080SwitchClauseSubject(
	pass *analysis.Pass,
	switchStmt *ast.SwitchStmt,
	clause *ast.CaseClause,
	enum *types.TypeName,
) (types.Object, bool) {
	if switchStmt.Tag != nil {
		return ps6080EnumSubject(pass, switchStmt.Tag, enum)
	}
	var subject types.Object
	for _, expression := range clause.List {
		if !ps6080ExpressionDispatchesEnum(pass, expression, enum) {
			continue
		}
		current, unique := ps6080EnumSubject(pass, expression, enum)
		if !unique || subject != nil && subject != current {
			return nil, false
		}
		subject = current
	}
	return subject, subject != nil
}

func ps6080SwitchClauseDispatchesEnum(
	pass *analysis.Pass,
	switchStmt *ast.SwitchStmt,
	clause *ast.CaseClause,
	results []ast.Expr,
	enum *types.TypeName,
) bool {
	if switchStmt == nil {
		return false
	}
	var resultSubject types.Object
	for _, result := range results {
		if !ps6080ExpressionDispatchesEnum(pass, result, enum) {
			continue
		}
		subject, unique := ps6080EnumSubject(pass, result, enum)
		if !unique || resultSubject != nil && resultSubject != subject {
			return false
		}
		resultSubject = subject
	}
	if resultSubject == nil {
		return false
	}
	if switchStmt.Tag != nil {
		subject, unique := ps6080EnumSubject(pass, switchStmt.Tag, enum)
		return unique && subject == resultSubject
	}
	if clause != nil && len(clause.List) > 0 {
		return ps6080ExpressionsStrictlyDispatchSubject(pass, clause.List, enum, resultSubject)
	}
	found := false
	for _, statement := range switchStmt.Body.List {
		current, ok := statement.(*ast.CaseClause)
		if !ok || len(current.List) == 0 {
			continue
		}
		for _, expression := range current.List {
			subject, safe := ps6080ExplicitGuardSubject(pass, expression, enum)
			if !safe {
				continue
			}
			if subject != resultSubject {
				return false
			}
			found = true
		}
	}
	return found
}

func ps6080ExpressionsStrictlyDispatchSubject(
	pass *analysis.Pass,
	expressions []ast.Expr,
	enum *types.TypeName,
	subject types.Object,
) bool {
	if len(expressions) == 0 {
		return false
	}
	for _, expression := range expressions {
		current, safe := ps6080ExplicitGuardSubject(pass, expression, enum)
		if !safe || current != subject {
			return false
		}
	}
	return true
}

func ps6080StatementsHaveBranch(statements []ast.Stmt) bool {
	found := false
	for _, statement := range statements {
		ast.Inspect(statement, func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			if _, branch := node.(*ast.BranchStmt); branch {
				found = true
				return false
			}
			return !found
		})
	}
	return found
}

func ps6080BranchesEscapeBlock(
	block *ast.BlockStmt,
	parents map[ast.Node]ast.Node,
	body *ast.BlockStmt,
) bool {
	if block == nil || body == nil {
		return true
	}
	return ps6080NodeHasEscapingBranch(block, parents, ps6080Labels(body))
}

func ps6080PrecededByEscapingBranch(
	statement ast.Stmt,
	parents map[ast.Node]ast.Node,
	body *ast.BlockStmt,
) bool {
	block, ok := parents[statement].(*ast.BlockStmt)
	if !ok || body == nil {
		return false
	}
	index := slices.Index(block.List, statement)
	if index <= 0 {
		return false
	}
	labels := ps6080Labels(body)
	for _, prior := range block.List[:index] {
		if ps6080NodeHasEscapingBranch(prior, parents, labels) {
			return true
		}
	}
	return false
}

func ps6080Labels(body *ast.BlockStmt) map[string]*ast.LabeledStmt {
	labels := make(map[string]*ast.LabeledStmt)
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if labeled, ok := node.(*ast.LabeledStmt); ok {
			labels[labeled.Label.Name] = labeled
		}
		return true
	})
	return labels
}

func ps6080NodeHasEscapingBranch(
	region ast.Node,
	parents map[ast.Node]ast.Node,
	labels map[string]*ast.LabeledStmt,
) bool {
	escapes := false
	ast.Inspect(region, func(node ast.Node) bool {
		if escapes {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		branch, ok := node.(*ast.BranchStmt)
		if !ok {
			return true
		}
		target := ps6080BranchTarget(branch, parents, labels)
		if target == nil || !ps6080NodeWithin(target, region) {
			escapes = true
			return false
		}
		return true
	})
	return escapes
}

func ps6080BranchTarget(
	branch *ast.BranchStmt,
	parents map[ast.Node]ast.Node,
	labels map[string]*ast.LabeledStmt,
) ast.Node {
	if branch.Label != nil {
		target, ok := labels[branch.Label.Name]
		if !ok {
			return nil
		}
		return target
	}
	for current := parents[branch]; current != nil; current = parents[current] {
		switch branch.Tok {
		case token.BREAK:
			switch current.(type) {
			case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
				return current
			}
		case token.CONTINUE:
			switch current.(type) {
			case *ast.ForStmt, *ast.RangeStmt:
				return current
			}
		case token.FALLTHROUGH:
			if _, clause := current.(*ast.CaseClause); clause {
				return current
			}
		case token.GOTO:
			return nil
		}
	}
	return nil
}

func ps6080NodeWithin(node, region ast.Node) bool {
	return node != nil && region != nil && region.Pos() <= node.Pos() && node.End() <= region.End()
}

func ps6080AssignmentMayChangeNamedResult(
	pass *analysis.Pass,
	function *ps6080Function,
	assignment *ast.AssignStmt,
) bool {
	signature := ps6080FunctionSignature(function)
	if signature == nil {
		return true
	}
	results := make(map[types.Object]bool, signature.Results().Len())
	for index := range signature.Results().Len() {
		results[signature.Results().At(index)] = true
	}
	if assignment.Tok != token.ASSIGN && assignment.Tok != token.DEFINE &&
		ps6080AssignmentTouchesObjects(pass, assignment, results) {
		return true
	}
	for _, expression := range assignment.Lhs {
		if _, direct := ps2110Unparen(expression).(*ast.Ident); !direct {
			return true
		}
	}
	unsafe := false
	for _, expression := range assignment.Rhs {
		ast.Inspect(expression, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.CallExpr:
				unsafe = true
				return false
			case *ast.UnaryExpr:
				identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
				if value.Op == token.AND && direct && results[pass.TypesInfo.ObjectOf(identifier)] {
					unsafe = true
					return false
				}
			}
			return !unsafe
		})
	}
	return unsafe
}

func ps6080NamedResultsMayChangeIndirectlyCached(
	pass *analysis.Pass,
	function *ps6080Function,
) bool {
	body := ps6080FunctionBody(function)
	if body == nil {
		return true
	}
	cache := ps6080ReturnFailureCacheFor(pass)
	if cached, found := cache.namedIndirect.Load(body); found {
		return cached.(bool)
	}
	result := ps6080NamedResultsMayChangeIndirectly(pass, function)
	actual, _ := cache.namedIndirect.LoadOrStore(body, result)
	return actual.(bool)
}

func ps6080NamedResultsMayChangeIndirectly(
	pass *analysis.Pass,
	function *ps6080Function,
) bool {
	signature := ps6080FunctionSignature(function)
	if signature == nil {
		return true
	}
	results := make(map[types.Object]bool, signature.Results().Len())
	for index := range signature.Results().Len() {
		results[signature.Results().At(index)] = true
	}
	unsafe := false
	ast.Inspect(ps6080FunctionBody(function), func(node ast.Node) bool {
		if unsafe {
			return false
		}
		switch value := node.(type) {
		case *ast.UnaryExpr:
			identifier, direct := ps2110Unparen(value.X).(*ast.Ident)
			if value.Op == token.AND && direct && results[pass.TypesInfo.ObjectOf(identifier)] {
				unsafe = true
				return false
			}
		case *ast.FuncLit:
			ast.Inspect(value.Body, func(child ast.Node) bool {
				identifier, direct := child.(*ast.Ident)
				if direct && results[pass.TypesInfo.ObjectOf(identifier)] {
					unsafe = true
					return false
				}
				return !unsafe
			})
			return false
		}
		return true
	})
	return unsafe
}

func ps6080SingleTerminalReturn(statements []ast.Stmt) (*ast.ReturnStmt, bool) {
	returned, ok := statements[len(statements)-1].(*ast.ReturnStmt)
	if !ok {
		return nil, false
	}
	count := 0
	for _, statement := range statements {
		ast.Inspect(statement, func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			if _, found := node.(*ast.ReturnStmt); found {
				count++
			}
			return true
		})
	}
	return returned, count == 1
}

func ps6080NilResultSupports(function *ps6080Function, index int) bool {
	if function == nil {
		return false
	}
	signature := ps6080FunctionSignature(function)
	if signature == nil || index < 0 || index >= signature.Results().Len() {
		return false
	}
	result := signature.Results().At(index).Type()
	errorObject := types.Universe.Lookup("error")
	return errorObject != nil && types.Identical(result, errorObject.Type())
}

func ps6080ZeroOrFailureExpression(pass *analysis.Pass, expression ast.Expr, nilSupports bool) bool {
	return ps6080ZeroOrFailureExpressionSeen(pass, expression, nilSupports, make(map[types.Object]bool))
}

func ps6080ZeroOrFailureExpressionSeen(
	pass *analysis.Pass,
	expression ast.Expr,
	nilSupports bool,
	seen map[types.Object]bool,
) bool {
	expression = ps2110Unparen(expression)
	if value := pass.TypesInfo.Types[expression].Value; value != nil {
		switch value.Kind() {
		case constant.Bool:
			return !constant.BoolVal(value)
		case constant.Int, constant.Float, constant.Complex:
			return value.ExactString() == "0"
		case constant.String:
			return constant.StringVal(value) == ""
		}
	}
	switch value := expression.(type) {
	case *ast.Ident:
		if ps6080NilExpression(pass, value) {
			return !nilSupports
		}
		object, ok := pass.TypesInfo.ObjectOf(value).(*types.Var)
		if ok && ps6080StandardErrorSentinel(pass, object) {
			return true
		}
		if !nilSupports || !ok || seen[object] || object.Pkg() == nil ||
			object.Parent() == object.Pkg().Scope() && object.Exported() {
			return false
		}
		initializer, unique := ps6080VariableInitializer(pass, object)
		if !unique {
			return false
		}
		seen[object] = true
		return ps6080ZeroOrFailureExpressionSeen(pass, initializer, nilSupports, seen)
	case *ast.SelectorExpr:
		return ps6080StandardErrorSentinel(pass, pass.TypesInfo.ObjectOf(value.Sel))
	case *ast.CallExpr:
		callee, _, known := typedCallee(pass, value.Fun)
		if !known || callee.Pkg() == nil {
			return false
		}
		return callee.Pkg().Path() == "errors" && callee.Name() == "New" ||
			callee.Pkg().Path() == "fmt" && (callee.Name() == "Errorf" || callee.Name() == "Error")
	}
	return false
}

func ps6080StandardErrorSentinel(pass *analysis.Pass, object types.Object) bool {
	variable, ok := object.(*types.Var)
	if !ok || variable.Pkg() == nil || variable.Parent() != variable.Pkg().Scope() {
		return false
	}
	errorObject := types.Universe.Lookup("error")
	if errorObject == nil || !types.AssignableTo(variable.Type(), errorObject.Type()) {
		return false
	}
	known := false
	switch variable.Pkg().Path() {
	case "errors":
		known = variable.Name() == "ErrUnsupported"
	case "context":
		known = variable.Name() == "Canceled" || variable.Name() == "DeadlineExceeded"
	case "io":
		known = variable.Name() == "EOF" || strings.HasPrefix(variable.Name(), "Err")
	case "io/fs", "os", "net":
		known = strings.HasPrefix(variable.Name(), "Err")
	}
	return known && !ps6080VariableWrittenOrExposed(pass, variable)
}

type ps6080VariableInitializerResult struct {
	expression       ast.Expr
	stable           bool
	writtenOrExposed bool
}

type ps6080VariableInitializerCache struct {
	once    sync.Once
	results map[*types.Var]ps6080VariableInitializerResult
}

func ps6080VariableInitializerResultFor(
	pass *analysis.Pass,
	variable *types.Var,
) ps6080VariableInitializerResult {
	if cache, found := ps6080VariableInitCaches.Load(pass); found {
		initializers := cache.(*ps6080VariableInitializerCache)
		initializers.once.Do(func() {
			initializers.results = ps6080VariableInitializers(pass)
		})
		return initializers.results[variable]
	}
	return ps6080VariableInitializers(pass)[variable]
}

func ps6080VariableInitializer(pass *analysis.Pass, variable *types.Var) (ast.Expr, bool) {
	result := ps6080VariableInitializerResultFor(pass, variable)
	return result.expression, result.stable
}

func ps6080VariableWrittenOrExposed(pass *analysis.Pass, variable *types.Var) bool {
	return ps6080VariableInitializerResultFor(pass, variable).writtenOrExposed
}

func ps6080DirectVariable(pass *analysis.Pass, expression ast.Expr) (*types.Var, *ast.Ident) {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		variable, _ := pass.TypesInfo.ObjectOf(value).(*types.Var)
		return variable, value
	case *ast.SelectorExpr:
		variable, _ := pass.TypesInfo.ObjectOf(value.Sel).(*types.Var)
		return variable, nil
	default:
		return nil, nil
	}
}

type ps6080NodeLiveness struct {
	found bool
	live  bool
}

type ps6080BodyLiveness struct {
	nodes map[ast.Node]ps6080NodeLiveness
}

func ps6080BuildBodyLiveness(
	pass *analysis.Pass,
	body *ast.BlockStmt,
	parents map[ast.Node]ast.Node,
) *ps6080BodyLiveness {
	index := &ps6080BodyLiveness{nodes: make(map[ast.Node]ps6080NodeLiveness)}
	record := func(node ast.Node, live bool) {
		if node == nil {
			return
		}
		current := index.nodes[node]
		current.found = true
		current.live = current.live || live
		index.nodes[node] = current
	}
	graph := cfg.New(body, ps6080CallMayReturn(pass))
	for _, block := range graph.Blocks {
		for _, cfgNode := range block.Nodes {
			ast.Inspect(cfgNode, func(node ast.Node) bool {
				record(node, block.Live)
				_, literal := node.(*ast.FuncLit)
				return !literal
			})
			for current := cfgNode; current != nil && current != body; current = parents[current] {
				record(current, block.Live)
			}
		}
	}
	return index
}

func ps6080FunctionValueWrapper(
	pass *analysis.Pass,
	expression ast.Expr,
	parents map[ast.Node]ast.Node,
) ast.Node {
	var current ast.Node = expression
	for {
		switch parent := parents[current].(type) {
		case *ast.ParenExpr:
			current = parent
		case *ast.CallExpr:
			if len(parent.Args) != 1 || parent.Args[0] != current ||
				!pass.TypesInfo.Types[parent.Fun].IsType() {
				return current
			}
			current = parent
		default:
			return current
		}
	}
}

func ps6080FunctionValueIdentifier(pass *analysis.Pass, expression ast.Expr) *ast.Ident {
	for {
		switch value := expression.(type) {
		case *ast.ParenExpr:
			expression = value.X
		case *ast.CallExpr:
			if len(value.Args) != 1 || !pass.TypesInfo.Types[value.Fun].IsType() {
				return nil
			}
			expression = value.Args[0]
		case *ast.Ident:
			return value
		default:
			return nil
		}
	}
}

func ps6080FunctionValueComparedWithNil(
	pass *analysis.Pass,
	expression ast.Expr,
	parents map[ast.Node]ast.Node,
) bool {
	current := ps6080FunctionValueWrapper(pass, expression, parents)
	comparison, ok := parents[current].(*ast.BinaryExpr)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return false
	}
	if comparison.X == current {
		return ps6080NilExpression(pass, comparison.Y)
	}
	return comparison.Y == current && ps6080NilExpression(pass, comparison.X)
}

func ps6080BlankDiscardedExpression(
	pass *analysis.Pass,
	expression ast.Expr,
	parents map[ast.Node]ast.Node,
) bool {
	var current ast.Node = expression
	for {
		switch parent := parents[current].(type) {
		case *ast.ParenExpr:
			current = parent
		case *ast.CallExpr:
			if len(parent.Args) != 1 || parent.Args[0] != current ||
				!pass.TypesInfo.Types[parent.Fun].IsType() {
				return false
			}
			current = parent
		case *ast.KeyValueExpr:
			if parent.Value != current {
				return false
			}
			current = parent
		case *ast.CompositeLit:
			if parent.Type == current {
				return false
			}
			current = parent
		case *ast.UnaryExpr:
			if parent.Op != token.AND || parent.X != current {
				return false
			}
			current = parent
		case *ast.AssignStmt:
			if len(parent.Lhs) != len(parent.Rhs) {
				return false
			}
			for index, right := range parent.Rhs {
				if right != current {
					continue
				}
				identifier, direct := ps2110Unparen(parent.Lhs[index]).(*ast.Ident)
				return direct && identifier.Name == "_"
			}
			return false
		case *ast.ValueSpec:
			if len(parent.Names) != len(parent.Values) {
				return false
			}
			for index, value := range parent.Values {
				if value == current {
					return parent.Names[index].Name == "_"
				}
			}
			return false
		default:
			return false
		}
	}
}

func ps6080FunctionLiteralBinding(
	pass *analysis.Pass,
	literal *ast.FuncLit,
	parents map[ast.Node]ast.Node,
) (types.Object, *ast.Ident, bool) {
	current := ps6080FunctionValueWrapper(pass, literal, parents)
	switch parent := parents[current].(type) {
	case *ast.AssignStmt:
		if len(parent.Lhs) != len(parent.Rhs) {
			return nil, nil, false
		}
		for index, right := range parent.Rhs {
			if right != current {
				continue
			}
			identifier, direct := ps2110Unparen(parent.Lhs[index]).(*ast.Ident)
			if !direct || identifier.Name == "_" {
				return nil, nil, false
			}
			object := pass.TypesInfo.ObjectOf(identifier)
			return object, identifier, object != nil && object.Pkg() == pass.Pkg &&
				object.Parent() != object.Pkg().Scope()
		}
	case *ast.ValueSpec:
		if len(parent.Names) != len(parent.Values) {
			return nil, nil, false
		}
		for index, value := range parent.Values {
			if value != current || parent.Names[index].Name == "_" {
				continue
			}
			identifier := parent.Names[index]
			object := pass.TypesInfo.Defs[identifier]
			return object, identifier, object != nil && object.Pkg() == pass.Pkg &&
				object.Parent() != object.Pkg().Scope()
		}
	}
	return nil, nil, false
}

func ps6080WriteOnlyIdentifier(identifier *ast.Ident, parents map[ast.Node]ast.Node) bool {
	switch parent := parents[identifier].(type) {
	case *ast.AssignStmt:
		for _, expression := range parent.Lhs {
			if ps2110Unparen(expression) == identifier {
				return true
			}
		}
	case *ast.RangeStmt:
		return ps2110Unparen(parent.Key) == identifier || ps2110Unparen(parent.Value) == identifier
	case *ast.ValueSpec:
		return slices.Contains(parent.Names, identifier)
	case *ast.Field:
		return slices.Contains(parent.Names, identifier)
	}
	return false
}

func ps6080LocallyReachable(
	pass *analysis.Pass,
	node ast.Node,
	parents map[ast.Node]ast.Node,
	bodyIndex func(*ast.BlockStmt) *ps6080BodyLiveness,
) bool {
	if !ps6080StaticallyReachable(pass, parents, node) {
		return false
	}
	for current := parents[node]; current != nil; current = parents[current] {
		var body *ast.BlockStmt
		switch scope := current.(type) {
		case *ast.FuncLit:
			body = scope.Body
		case *ast.FuncDecl:
			body = scope.Body
		}
		if body == nil {
			continue
		}
		indexed := bodyIndex(body).nodes[node]
		return ps6080NodeReachableWithLiveness(
			pass, parents, node, indexed.found, indexed.live,
		)
	}
	return true
}

func ps6080FunctionLiteralExecutions(
	pass *analysis.Pass,
	file *ast.File,
	parents map[ast.Node]ast.Node,
	bodyIndex func(*ast.BlockStmt) *ps6080BodyLiveness,
) map[*ast.FuncLit]bool {
	uses := make(map[types.Object][]*ast.Ident)
	namedResults := make(map[types.Object]bool)
	var literals []*ast.FuncLit
	ast.Inspect(file, func(node ast.Node) bool {
		if identifier, ok := node.(*ast.Ident); ok {
			if object := pass.TypesInfo.ObjectOf(identifier); object != nil {
				uses[object] = append(uses[object], identifier)
			}
		}
		if literal, ok := node.(*ast.FuncLit); ok {
			literals = append(literals, literal)
		}
		if functionType, ok := node.(*ast.FuncType); ok && functionType.Results != nil {
			for _, field := range functionType.Results.List {
				for _, name := range field.Names {
					if object := pass.TypesInfo.ObjectOf(name); object != nil {
						namedResults[object] = true
					}
				}
			}
		}
		return true
	})
	invocations := make(map[*ast.BlockStmt]map[*ast.FuncLit]bool)
	rootBody := func(literal *ast.FuncLit) *ast.BlockStmt {
		for current := ast.Node(literal); current != nil; current = parents[current] {
			if declaration, ok := current.(*ast.FuncDecl); ok {
				return declaration.Body
			}
		}
		return nil
	}
	type literalBinding struct {
		object types.Object
		safe   bool
	}
	type aliasUse struct {
		destination types.Object
		safe        bool
	}
	bindings := make(map[*ast.FuncLit]literalBinding, len(literals))
	boundObjects := make(map[types.Object]bool)
	for _, literal := range literals {
		if ps6080BlankDiscardedExpression(pass, literal, parents) {
			continue
		}
		object, _, safe := ps6080FunctionLiteralBinding(pass, literal, parents)
		bindings[literal] = literalBinding{object: object, safe: safe && !namedResults[object]}
		if object != nil {
			boundObjects[object] = true
		}
	}
	safeAliasDestination := func(object types.Object) bool {
		return object != nil && object.Pkg() == pass.Pkg &&
			object.Parent() != object.Pkg().Scope() && !namedResults[object]
	}
	aliasUses := make(map[*ast.Ident]aliasUse)
	aliasDestinations := make(map[types.Object][]types.Object)
	recordAlias := func(destination, source ast.Expr) {
		sourceIdentifier := ps6080FunctionValueIdentifier(pass, source)
		if sourceIdentifier == nil {
			return
		}
		destinationIdentifier, direct := ps2110Unparen(destination).(*ast.Ident)
		if !direct || destinationIdentifier.Name == "_" {
			return
		}
		sourceObject := pass.TypesInfo.ObjectOf(sourceIdentifier)
		if sourceObject == nil {
			return
		}
		destinationObject := pass.TypesInfo.ObjectOf(destinationIdentifier)
		safe := safeAliasDestination(destinationObject)
		aliasUses[sourceIdentifier] = aliasUse{destination: destinationObject, safe: safe}
		if safe {
			aliasDestinations[sourceObject] = append(
				aliasDestinations[sourceObject], destinationObject,
			)
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) == len(value.Rhs) {
				for index := range value.Lhs {
					recordAlias(value.Lhs[index], value.Rhs[index])
				}
			}
		case *ast.ValueSpec:
			if len(value.Names) == len(value.Values) {
				for index := range value.Names {
					recordAlias(value.Names[index], value.Values[index])
				}
			}
		}
		return true
	})
	relevantObjects := make(map[types.Object]bool, len(boundObjects))
	relevantQueue := make([]types.Object, 0, len(boundObjects))
	for object := range boundObjects {
		relevantObjects[object] = true
		relevantQueue = append(relevantQueue, object)
	}
	for len(relevantQueue) > 0 {
		object := relevantQueue[0]
		relevantQueue = relevantQueue[1:]
		for _, destination := range aliasDestinations[object] {
			if relevantObjects[destination] {
				continue
			}
			relevantObjects[destination] = true
			relevantQueue = append(relevantQueue, destination)
		}
	}
	result := make(map[*ast.FuncLit]bool, len(literals))
	activeObjects := make(map[types.Object]bool, len(relevantObjects))
	type aliasDependency struct {
		source            types.Object
		destinationActive bool
		containerActive   bool
		activated         bool
	}
	literalDependents := make(map[*ast.FuncLit][]*ast.FuncLit)
	objectDependentsByLiteral := make(map[*ast.FuncLit][]types.Object)
	literalDependentsByObject := make(map[types.Object][]*ast.FuncLit, len(literals))
	aliasesByDestination := make(map[types.Object][]*aliasDependency)
	aliasesByContainer := make(map[*ast.FuncLit][]*aliasDependency)
	var literalQueue []*ast.FuncLit
	var objectQueue []types.Object
	activateLiteral := func(literal *ast.FuncLit) {
		if literal == nil || result[literal] {
			return
		}
		result[literal] = true
		literalQueue = append(literalQueue, literal)
	}
	activateObject := func(object types.Object) {
		if object == nil || activeObjects[object] {
			return
		}
		activeObjects[object] = true
		objectQueue = append(objectQueue, object)
	}
	literalDependsOn := func(literal, container *ast.FuncLit) {
		if container == nil {
			activateLiteral(literal)
			return
		}
		literalDependents[container] = append(literalDependents[container], literal)
	}
	objectDependsOn := func(object types.Object, container *ast.FuncLit) {
		if container == nil {
			activateObject(object)
			return
		}
		objectDependentsByLiteral[container] = append(
			objectDependentsByLiteral[container], object,
		)
	}
	for object := range relevantObjects {
		for _, use := range uses[object] {
			if ps6080BlankDiscardedExpression(pass, use, parents) ||
				ps6080FunctionValueComparedWithNil(pass, use, parents) ||
				ps6080WriteOnlyIdentifier(use, parents) ||
				!ps6080LocallyReachable(pass, use, parents, bodyIndex) {
				continue
			}
			if alias, copied := aliasUses[use]; copied {
				if alias.safe {
					container := ps6080ContainingLiteral(use, parents)
					dependency := &aliasDependency{
						source: object, containerActive: container == nil,
					}
					aliasesByDestination[alias.destination] = append(
						aliasesByDestination[alias.destination], dependency,
					)
					if container != nil {
						aliasesByContainer[container] = append(
							aliasesByContainer[container], dependency,
						)
					}
				} else {
					objectDependsOn(object, ps6080ContainingLiteral(use, parents))
				}
				continue
			}
			objectDependsOn(object, ps6080ContainingLiteral(use, parents))
		}
	}
	for _, literal := range literals {
		binding, tracked := bindings[literal]
		if !tracked || !ps6080LocallyReachable(pass, literal, parents, bodyIndex) {
			continue
		}
		body := rootBody(literal)
		if body != nil {
			invoked, known := invocations[body]
			if !known {
				invoked = ps6080InvokedFunctionLiterals(pass, body)
				invocations[body] = invoked
			}
			if invoked[literal] {
				activateLiteral(literal)
			}
		}
		if !binding.safe {
			literalDependsOn(literal, ps6080ContainingLiteral(parents[literal], parents))
			continue
		}
		literalDependentsByObject[binding.object] = append(
			literalDependentsByObject[binding.object], literal,
		)
	}
	for len(literalQueue) > 0 || len(objectQueue) > 0 {
		if len(literalQueue) > 0 {
			container := literalQueue[0]
			literalQueue = literalQueue[1:]
			for _, literal := range literalDependents[container] {
				activateLiteral(literal)
			}
			for _, object := range objectDependentsByLiteral[container] {
				activateObject(object)
			}
			for _, dependency := range aliasesByContainer[container] {
				dependency.containerActive = true
				if dependency.destinationActive && !dependency.activated {
					dependency.activated = true
					activateObject(dependency.source)
				}
			}
			continue
		}
		object := objectQueue[0]
		objectQueue = objectQueue[1:]
		for _, literal := range literalDependentsByObject[object] {
			activateLiteral(literal)
		}
		for _, dependency := range aliasesByDestination[object] {
			dependency.destinationActive = true
			if dependency.containerActive && !dependency.activated {
				dependency.activated = true
				activateObject(dependency.source)
			}
		}
	}
	return result
}

func ps6080VariableInitializers(pass *analysis.Pass) map[*types.Var]ps6080VariableInitializerResult {
	type state struct {
		initializer  ast.Expr
		definitions  int
		writes       int
		addressTaken bool
	}
	states := make(map[*types.Var]*state)
	stateFor := func(variable *types.Var) *state {
		if variable == nil {
			return nil
		}
		if states[variable] == nil {
			states[variable] = &state{}
		}
		return states[variable]
	}
	for _, file := range pass.Files {
		parents := ps6071Parents(file)
		bodyIndexes := make(map[*ast.BlockStmt]*ps6080BodyLiveness)
		bodyIndex := func(body *ast.BlockStmt) *ps6080BodyLiveness {
			if index := bodyIndexes[body]; index != nil {
				return index
			}
			index := ps6080BuildBodyLiveness(pass, body, parents)
			bodyIndexes[body] = index
			return index
		}
		literalExecutions := ps6080FunctionLiteralExecutions(pass, file, parents, bodyIndex)
		reachable := func(node ast.Node) bool {
			query := node
			scope := node
			for {
				var body *ast.BlockStmt
				var literal *ast.FuncLit
				for current := scope; current != nil; current = parents[current] {
					switch value := current.(type) {
					case *ast.FuncLit:
						body = value.Body
						literal = value
					case *ast.FuncDecl:
						body = value.Body
					}
					if body != nil {
						break
					}
				}
				if body == nil {
					return ps6080NodeReachableWithLiveness(pass, parents, query, true, true)
				}
				indexed := bodyIndex(body).nodes[query]
				if !ps6080StaticallyReachable(pass, parents, query) ||
					!ps6080NodeReachableWithLiveness(pass, parents, query, indexed.found, indexed.live) {
					return false
				}
				if literal == nil {
					return true
				}
				if !literalExecutions[literal] {
					return false
				}
				query = literal
				scope = parents[literal]
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.AssignStmt:
				if !reachable(value) {
					break
				}
				for index, left := range value.Lhs {
					variable, identifier := ps6080DirectVariable(pass, left)
					if variable == nil {
						continue
					}
					current := stateFor(variable)
					current.writes++
					if value.Tok == token.DEFINE && identifier != nil &&
						pass.TypesInfo.Defs[identifier] == variable {
						current.definitions++
						if len(value.Lhs) == len(value.Rhs) {
							current.initializer = value.Rhs[index]
						}
					}
				}
			case *ast.IncDecStmt:
				if variable, _ := ps6080DirectVariable(pass, value.X); variable != nil && reachable(value) {
					stateFor(variable).writes++
				}
			case *ast.ValueSpec:
				if !reachable(value) {
					break
				}
				for index, name := range value.Names {
					variable, ok := pass.TypesInfo.Defs[name].(*types.Var)
					if !ok {
						continue
					}
					current := stateFor(variable)
					current.definitions++
					current.writes++
					if len(value.Names) == len(value.Values) {
						current.initializer = value.Values[index]
					}
				}
			case *ast.RangeStmt:
				if !reachable(value) || ps6080StaticallyEmptyRange(pass, value) {
					break
				}
				for _, expression := range []ast.Expr{value.Key, value.Value} {
					variable, identifier := ps6080DirectVariable(pass, expression)
					if variable == nil {
						continue
					}
					current := stateFor(variable)
					current.writes++
					if value.Tok == token.DEFINE && identifier != nil &&
						pass.TypesInfo.Defs[identifier] == variable {
						current.definitions++
					}
				}
			case *ast.UnaryExpr:
				if value.Op != token.AND || !reachable(value) {
					break
				}
				if variable, _ := ps6080DirectVariable(pass, value.X); variable != nil {
					stateFor(variable).addressTaken = true
				}
			}
			return true
		})
	}
	result := make(map[*types.Var]ps6080VariableInitializerResult, len(states))
	for variable, current := range states {
		result[variable] = ps6080VariableInitializerResult{
			expression:       current.initializer,
			writtenOrExposed: current.writes != 0 || current.addressTaken,
			stable: current.definitions == 1 && current.writes == 1 &&
				current.initializer != nil && !current.addressTaken,
		}
	}
	return result
}

func ps6080MapValueSupported(
	pass *analysis.Pass,
	expression ast.Expr,
	element types.Type,
	packageInitializer bool,
) bool {
	return ps6080MapValueSupportedSeen(
		pass, expression, element, packageInitializer, make(map[*types.Var]bool),
	)
}

func ps6080MapValueSupportedSeen(
	pass *analysis.Pass,
	expression ast.Expr,
	element types.Type,
	packageInitializer bool,
	seen map[*types.Var]bool,
) bool {
	expression = ps2110Unparen(expression)
	if ps6080NilExpression(pass, expression) {
		return false
	}
	if ps6080CallableType(element) {
		switch value := expression.(type) {
		case *ast.FuncLit:
			return true
		case *ast.Ident:
			object := pass.TypesInfo.ObjectOf(value)
			if _, known := object.(*types.Func); known {
				return true
			}
			variable, known := object.(*types.Var)
			if !known || seen[variable] || !ps6080CallableType(variable.Type()) ||
				!packageInitializer && variable.Parent() == pass.Pkg.Scope() && variable.Exported() {
				return false
			}
			initializer, stable := ps6080VariableInitializer(pass, variable)
			if !stable {
				return false
			}
			seen[variable] = true
			return ps6080MapValueSupportedSeen(pass, initializer, element, packageInitializer, seen)
		case *ast.SelectorExpr:
			_, _, known := typedCallee(pass, value)
			return known
		case *ast.IndexExpr:
			_, _, known := typedCallee(pass, value)
			return known
		case *ast.IndexListExpr:
			_, _, known := typedCallee(pass, value)
			return known
		case *ast.CallExpr:
			if len(value.Args) == 1 && pass.TypesInfo.Types[value.Fun].IsType() {
				return ps6080MapValueSupportedSeen(
					pass, value.Args[0], element, packageInitializer, seen,
				)
			}
		}
		return false
	}
	if value := pass.TypesInfo.Types[expression].Value; value != nil {
		switch value.Kind() {
		case constant.Bool:
			return constant.BoolVal(value)
		case constant.Int, constant.Float, constant.Complex:
			return value.ExactString() != "0"
		case constant.String:
			return constant.StringVal(value) != ""
		}
	}
	return true
}

func ps6080MapKeyCompatible(key types.Type, enum *types.TypeName, group *ps6080ConstantGroup) bool {
	key = types.Unalias(key)
	if named, ok := key.(*types.Named); ok {
		if named.Obj() == enum {
			return true
		}
		for _, constants := range []map[*types.Const]token.Pos{group.included, group.excluded} {
			for constant := range constants {
				if types.Identical(types.Unalias(constant.Type()), key) {
					return true
				}
			}
		}
		return false
	}
	basic, ok := key.Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsInteger != 0
}

func ps6080CallMayReturn(pass *analysis.Pass) func(*ast.CallExpr) bool {
	return func(call *ast.CallExpr) bool { return !ps6080BuiltinPanic(pass, call) }
}

func ps6080InvokedLiteralGraphs(
	pass *analysis.Pass,
	literals map[*ast.FuncLit]bool,
) map[*ast.FuncLit]*cfg.CFG {
	result := make(map[*ast.FuncLit]*cfg.CFG, len(literals))
	for literal := range literals {
		result[literal] = cfg.New(literal.Body, ps6080CallMayReturn(pass))
	}
	return result
}

func ps6080GraphForNode(
	outer *cfg.CFG,
	literals map[*ast.FuncLit]*cfg.CFG,
	parents map[ast.Node]ast.Node,
	node ast.Node,
) *cfg.CFG {
	for current := node; current != nil; current = parents[current] {
		if literal, ok := current.(*ast.FuncLit); ok {
			if graph := literals[literal]; graph != nil {
				return graph
			}
			return nil
		}
	}
	return outer
}

func ps6080BodyForNode(
	outer *ast.BlockStmt,
	literals map[*ast.FuncLit]bool,
	parents map[ast.Node]ast.Node,
	node ast.Node,
) *ast.BlockStmt {
	for current := node; current != nil; current = parents[current] {
		if literal, ok := current.(*ast.FuncLit); ok {
			if literals[literal] {
				return literal.Body
			}
			break
		}
	}
	return outer
}

func ps6080EnclosingControlRegion(node ast.Node, parents map[ast.Node]ast.Node) ast.Node {
	for current := node; current != nil; current = parents[current] {
		if ps6080ControlRegion(current, parents) {
			return current
		}
	}
	return nil
}

func ps6080ControlRegion(node ast.Node, parents map[ast.Node]ast.Node) bool {
	switch value := node.(type) {
	case *ast.CaseClause, *ast.CommClause:
		return true
	case *ast.BlockStmt:
		_, bareBlock := parents[value].(*ast.BlockStmt)
		_, bareCase := parents[value].(*ast.CaseClause)
		_, bareComm := parents[value].(*ast.CommClause)
		return !bareBlock && !bareCase && !bareComm
	}
	return false
}

func ps6080MutuallyExclusiveControlRegions(
	binding ast.Node,
	use ast.Node,
	parents map[ast.Node]ast.Node,
) bool {
	if bindingBlock, ok := binding.(*ast.BlockStmt); ok {
		if conditional, branch := ps6080IfBranch(bindingBlock, parents); conditional != nil {
			for node := use; node != nil; node = parents[node] {
				useBlock, block := node.(*ast.BlockStmt)
				if !block {
					continue
				}
				if useConditional, useBranch := ps6080IfBranch(useBlock, parents); useConditional == conditional {
					return branch != useBranch
				}
			}
		}
	}
	var current ast.Node
	for node := use; node != nil; node = parents[node] {
		switch node.(type) {
		case *ast.CaseClause, *ast.CommClause:
			current = node
		}
		if current != nil {
			break
		}
	}
	if current == nil || binding == current || parents[binding] != parents[current] {
		return false
	}
	switch from := binding.(type) {
	case *ast.CommClause:
		_, ok := current.(*ast.CommClause)
		return ok
	case *ast.CaseClause:
		to, ok := current.(*ast.CaseClause)
		if !ok {
			return false
		}
		block, ok := parents[from].(*ast.BlockStmt)
		return ok && !ps6080CaseFallsThroughTo(from, to, block)
	}
	return false
}

func ps6080IfBranch(block *ast.BlockStmt, parents map[ast.Node]ast.Node) (*ast.IfStmt, bool) {
	conditional, ok := parents[block].(*ast.IfStmt)
	if !ok {
		return nil, false
	}
	if conditional.Body == block {
		return conditional, true
	}
	if conditional.Else == block {
		return conditional, false
	}
	return nil, false
}

func ps6080IfAlternative(block *ast.BlockStmt, parents map[ast.Node]ast.Node) (ast.Node, ast.Node) {
	conditional, _ := ps6080IfBranch(block, parents)
	if conditional == nil || conditional.Else == nil {
		return nil, nil
	}
	root := conditional
	for {
		parent, nested := parents[root].(*ast.IfStmt)
		if !nested || parent.Else != root {
			break
		}
		root = parent
	}
	if len(ps6080IfAlternativeLeaves(root)) == 0 {
		return nil, nil
	}
	return root, block
}

func ps6080IfAlternativeLeaves(root *ast.IfStmt) map[ast.Node]bool {
	result := make(map[ast.Node]bool)
	for current := root; current != nil; {
		result[current.Body] = true
		switch alternative := current.Else.(type) {
		case *ast.IfStmt:
			current = alternative
		case *ast.BlockStmt:
			result[alternative] = true
			return result
		default:
			return nil
		}
	}
	return nil
}

func ps6080SwitchHasDefault(statement *ast.SwitchStmt) bool {
	for _, candidate := range statement.Body.List {
		if clause, ok := candidate.(*ast.CaseClause); ok && len(clause.List) == 0 {
			return true
		}
	}
	return false
}

func ps6080AlternativeCovered(alternative ast.Node, covered map[ast.Node]bool) bool {
	switch statement := alternative.(type) {
	case *ast.IfStmt:
		leaves := ps6080IfAlternativeLeaves(statement)
		if len(leaves) == 0 {
			return false
		}
		for leaf := range leaves {
			if !covered[leaf] {
				return false
			}
		}
		return true
	case *ast.SwitchStmt:
		if !ps6080SwitchHasDefault(statement) {
			return false
		}
		nextCovered := false
		for index := len(statement.Body.List) - 1; index >= 0; index-- {
			clause, ok := statement.Body.List[index].(*ast.CaseClause)
			if !ok {
				return false
			}
			currentCovered := covered[clause]
			if !currentCovered && len(clause.Body) > 0 {
				branch, fallsThrough := clause.Body[len(clause.Body)-1].(*ast.BranchStmt)
				currentCovered = fallsThrough && branch.Tok == token.FALLTHROUGH && nextCovered
			}
			if !currentCovered {
				return false
			}
			nextCovered = currentCovered
		}
		return true
	}
	return false
}

func ps6080SwitchEntryNode(statement *ast.SwitchStmt) ast.Node {
	if statement.Tag != nil {
		return statement.Tag
	}
	for _, candidate := range statement.Body.List {
		clause, ok := candidate.(*ast.CaseClause)
		if ok && len(clause.List) > 0 {
			return clause.List[0]
		}
	}
	return statement
}

func ps6080CaseFallsThroughTo(from, to *ast.CaseClause, block *ast.BlockStmt) bool {
	fromIndex := slices.Index(block.List, ast.Stmt(from))
	toIndex := slices.Index(block.List, ast.Stmt(to))
	if fromIndex < 0 || fromIndex >= toIndex {
		return false
	}
	for index := fromIndex; index < toIndex; index++ {
		clause, ok := block.List[index].(*ast.CaseClause)
		if !ok || len(clause.Body) == 0 {
			return false
		}
		branch, ok := clause.Body[len(clause.Body)-1].(*ast.BranchStmt)
		if !ok || branch.Tok != token.FALLTHROUGH {
			return false
		}
	}
	return true
}

func ps6080BuiltinPanic(pass *analysis.Pass, call *ast.CallExpr) bool {
	identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	return ok && pass.TypesInfo.ObjectOf(identifier) == types.Universe.Lookup("panic")
}

func ps6080ExpressionEvaluated(pass *analysis.Pass, parent *ast.BinaryExpr, child ast.Expr) bool {
	if child == parent.X {
		return true
	}
	left := pass.TypesInfo.Types[parent.X].Value
	if left == nil || left.Kind() != constant.Bool {
		return true
	}
	truth := constant.BoolVal(left)
	if parent.Op == token.LAND {
		return truth
	}
	return !truth
}

func ps6080CallArgumentEvaluated(pass *analysis.Pass, call *ast.CallExpr) bool {
	var object types.Object
	switch function := ps2110Unparen(call.Fun).(type) {
	case *ast.Ident:
		object = pass.TypesInfo.ObjectOf(function)
	case *ast.SelectorExpr:
		object = pass.TypesInfo.ObjectOf(function.Sel)
	}
	if object != nil && object.Pkg() != nil && object.Pkg().Path() == "unsafe" {
		switch object.Name() {
		case "Sizeof", "Alignof", "Offsetof":
			return false
		}
	}
	if len(call.Args) == 1 &&
		(typedBuiltinName(pass, call.Fun, "len") || typedBuiltinName(pass, call.Fun, "cap")) {
		return pass.TypesInfo.Types[call].Value == nil
	}
	return true
}

func ps6080DispatchSubject(pass *analysis.Pass, expression ast.Expr, enum *types.TypeName) bool {
	if expression == nil {
		return false
	}
	typeOf := types.Unalias(pass.TypesInfo.TypeOf(expression))
	switch value := typeOf.(type) {
	case *types.Named:
		if value.Obj() != enum {
			return false
		}
	case *types.TypeParam:
		// A type parameter constrained to an integer enum is resolved at the
		// directly reached instantiation. The compared constant still provides
		// the enum identity.
	case *types.Basic:
		if value.Info()&types.IsInteger == 0 {
			return false
		}
	default:
		return false
	}
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if _, variable := pass.TypesInfo.ObjectOf(identifier).(*types.Var); variable {
			found = true
			return false
		}
		return true
	})
	if found {
		return true
	}
	switch ps2110Unparen(expression).(type) {
	case *ast.CallExpr, *ast.IndexExpr:
		return pass.TypesInfo.Types[expression].Value == nil
	}
	return false
}

func ps6080NodeReachable(
	pass *analysis.Pass,
	graph *cfg.CFG,
	parents map[ast.Node]ast.Node,
	node ast.Node,
) bool {
	if node == nil {
		return false
	}
	found, live := ps6080CFGNodeLive(graph, node)
	return ps6080NodeReachableWithLiveness(pass, parents, node, found, live)
}

func ps6080NodeReachableWithLiveness(
	pass *analysis.Pass,
	parents map[ast.Node]ast.Node,
	node ast.Node,
	found bool,
	live bool,
) bool {
	if node == nil {
		return false
	}
	if found && !live {
		return false
	} else if !found {
		return false
	}
	position := node.Pos()
	for current := node; current != nil; current = parents[current] {
		if _, boundary := current.(*ast.FuncLit); boundary && current != node {
			break
		}
		if binary, ok := parents[current].(*ast.BinaryExpr); ok && binary.Y == current &&
			(binary.Op == token.LAND || binary.Op == token.LOR) && !ps6080ExpressionEvaluated(pass, binary, binary.Y) {
			return false
		}
		if call, ok := parents[current].(*ast.CallExpr); ok && !ps6080CallArgumentEvaluated(pass, call) {
			for _, argument := range call.Args {
				if argument == current {
					return false
				}
			}
		}
		if loop, ok := parents[current].(*ast.ForStmt); ok && loop.Cond != nil {
			value := pass.TypesInfo.Types[loop.Cond].Value
			if value != nil && value.Kind() == constant.Bool && !constant.BoolVal(value) {
				inBody := loop.Body.Pos() <= position && position < loop.Body.End()
				inPost := loop.Post != nil && loop.Post.Pos() <= position && position < loop.Post.End()
				if inBody || inPost {
					return false
				}
			}
		}
		conditional, ok := parents[current].(*ast.IfStmt)
		if !ok {
			continue
		}
		value := pass.TypesInfo.Types[conditional.Cond].Value
		if value == nil || value.Kind() != constant.Bool {
			continue
		}
		truth := constant.BoolVal(value)
		inBody := conditional.Body.Pos() <= position && position < conditional.Body.End()
		inElse := conditional.Else != nil && conditional.Else.Pos() <= position && position < conditional.Else.End()
		if inBody && !truth || inElse && truth {
			return false
		}
	}
	return true
}

func ps6080CFGNodeLive(graph *cfg.CFG, node ast.Node) (found, live bool) {
	if graph == nil || node == nil {
		return false, false
	}
	if block := ps6079CFGBlockAt(graph, node.Pos()); block != nil {
		return true, block.Live
	}
	for _, block := range graph.Blocks {
		for _, cfgNode := range block.Nodes {
			if node.Pos() <= cfgNode.Pos() && cfgNode.End() <= node.End() {
				found = true
				live = live || block.Live
			}
		}
	}
	return found, live
}

func ps6080MapLiteralDispatched(
	pass *analysis.Pass,
	literal *ast.CompositeLit,
	parents map[ast.Node]ast.Node,
	body *ast.BlockStmt,
	invokedLiterals map[*ast.FuncLit]bool,
) bool {
	graphs := make(map[*ast.BlockStmt]*cfg.CFG)
	reachable := func(node ast.Node) bool {
		nodeBody := ps6080BodyForNode(body, invokedLiterals, parents, node)
		graph := graphs[nodeBody]
		if graph == nil {
			graph = cfg.New(nodeBody, ps6080CallMayReturn(pass))
			graphs[nodeBody] = graph
		}
		return ps6080NodeReachable(pass, graph, parents, node)
	}
	var expression ast.Expr = literal
	for {
		switch parent := parents[expression].(type) {
		case *ast.ParenExpr:
			if parent.X != expression {
				break
			}
			expression = parent
			continue
		case *ast.CallExpr:
			if len(parent.Args) != 1 || parent.Args[0] != expression ||
				!pass.TypesInfo.Types[parent.Fun].IsType() {
				break
			}
			expression = parent
			continue
		}
		break
	}
	if index, ok := parents[expression].(*ast.IndexExpr); ok && index.X == expression {
		return reachable(index) && ps6080DynamicDispatchIndex(pass, index.Index)
	}
	var object types.Object
	var binding ast.Node
	var tableContainer types.Object
	var tableAggregate ast.Expr
	var tableField types.Object
	switch parent := parents[expression].(type) {
	case *ast.AssignStmt:
		binding = parent
		if len(parent.Lhs) == len(parent.Rhs) {
			for index, right := range parent.Rhs {
				if right == expression {
					if identifier, ok := ps2110Unparen(parent.Lhs[index]).(*ast.Ident); ok {
						object = pass.TypesInfo.ObjectOf(identifier)
					}
				}
			}
		}
	case *ast.ValueSpec:
		binding = parent
		if len(parent.Names) == len(parent.Values) {
			for index, right := range parent.Values {
				if right == expression {
					object = pass.TypesInfo.Defs[parent.Names[index]]
				}
			}
		}
	}
	if keyed, ok := parents[expression].(*ast.KeyValueExpr); ok && keyed.Value == expression {
		if aggregate, aggregateLiteral := parents[keyed].(*ast.CompositeLit); aggregateLiteral {
			if identifier, named := ps2110Unparen(keyed.Key).(*ast.Ident); named {
				tableField = pass.TypesInfo.ObjectOf(identifier)
			}
			tableAggregate = aggregate
			var aggregateExpression ast.Expr = aggregate
			for {
				switch parent := parents[aggregateExpression].(type) {
				case *ast.ParenExpr:
					aggregateExpression = parent
					continue
				case *ast.CallExpr:
					if len(parent.Args) == 1 && parent.Args[0] == aggregateExpression &&
						pass.TypesInfo.Types[parent.Fun].IsType() {
						aggregateExpression = parent
						continue
					}
				}
				break
			}
			switch parent := parents[aggregateExpression].(type) {
			case *ast.AssignStmt:
				if len(parent.Lhs) == len(parent.Rhs) {
					for index, right := range parent.Rhs {
						if right == aggregateExpression {
							if identifier, direct := ps2110Unparen(parent.Lhs[index]).(*ast.Ident); direct {
								tableContainer = pass.TypesInfo.ObjectOf(identifier)
								binding = parent
							}
						}
					}
				}
			case *ast.ValueSpec:
				if len(parent.Names) == len(parent.Values) {
					for index, right := range parent.Values {
						if right == aggregateExpression {
							tableContainer = pass.TypesInfo.Defs[parent.Names[index]]
							binding = parent
						}
					}
				}
			}
		}
	}
	aliases := make(map[types.Object]bool)
	initialAliases := make(map[types.Object]bool)
	if object != nil {
		aliases = ps6080MapAliases(pass, object, body)
		initialAliases[object] = true
	}
	context := ps6080NewMapAliasContext(pass, object, aliases, initialAliases, body)
	context.tablePosition = literal.Pos()
	context.tableBinding = binding
	context.tableLiteral = expression
	context.tableContainer = tableContainer
	context.tableAggregate = tableAggregate
	context.tableField = tableField
	namedSafeMaps := ps6080NamedCallbackSafeMapParameters(pass)
	namedCallbacks := ps6080NamedCallbackParameters(pass)
	dispatched := false
	mutated := false
	ast.Inspect(body, func(node ast.Node) bool {
		if function, ok := node.(*ast.FuncLit); ok && !invokedLiterals[function] {
			return false
		}
		if assignment, ok := node.(*ast.AssignStmt); ok && reachable(assignment) {
			for index, left := range assignment.Lhs {
				if len(assignment.Lhs) == len(assignment.Rhs) {
					if identifier, direct := ps2110Unparen(left).(*ast.Ident); direct {
						object := pass.TypesInfo.ObjectOf(identifier)
						if ps6080MapAliasIdentity(pass, assignment.Rhs[index], object) ||
							object == context.table && context.active(assignment.Rhs[index], assignment.Pos()) {
							continue
						}
					}
				}
				if ps6080MapMutationTarget(left, assignment.Pos(), context) {
					mutated = true
				}
			}
			for index, right := range assignment.Rhs {
				if assignment == context.tableBinding && context.tableAggregate != nil &&
					context.within(context.tableAggregate, right) {
					continue
				}
				if function, ok := ps2110Unparen(right).(*ast.FuncLit); ok && invokedLiterals[function] {
					continue
				}
				var destination ast.Expr
				if len(assignment.Lhs) == len(assignment.Rhs) {
					destination = assignment.Lhs[index]
				}
				if ps6080MapAliasEscapes(right, assignment.Pos(), destination, context) {
					mutated = true
				}
			}
		}
		if specification, ok := node.(*ast.ValueSpec); ok && reachable(specification) {
			for index, right := range specification.Values {
				if specification == context.tableBinding && context.tableAggregate != nil &&
					context.within(context.tableAggregate, right) {
					continue
				}
				if function, ok := ps2110Unparen(right).(*ast.FuncLit); ok && invokedLiterals[function] {
					continue
				}
				var destination ast.Expr
				if len(specification.Names) == len(specification.Values) {
					destination = specification.Names[index]
				}
				if ps6080MapAliasEscapes(right, specification.Pos(), destination, context) {
					mutated = true
				}
			}
		}
		if ranged, ok := node.(*ast.RangeStmt); ok && reachable(ranged) &&
			!ps6080StaticallyEmptyRange(pass, ranged) {
			var element ast.Expr
			if composite, direct := ps2110Unparen(ranged.X).(*ast.CompositeLit); direct &&
				len(composite.Elts) == 1 {
				element = composite.Elts[0]
			}
			for _, target := range []ast.Expr{ranged.Key, ranged.Value} {
				if target == ranged.Value && element != nil {
					if identifier, direct := ps2110Unparen(target).(*ast.Ident); direct &&
						pass.TypesInfo.ObjectOf(identifier) == context.table &&
						context.active(element, ranged.Pos()) {
						continue
					}
				}
				if target != nil && ps6080MapMutationTarget(target, ranged.Pos(), context) {
					mutated = true
				}
			}
		}
		if call, ok := node.(*ast.CallExpr); ok && reachable(call) && !context.literalCalls[call] {
			var safeArguments ps6080IndexSet
			if callee, _, direct := typedCallee(pass, call.Fun); direct {
				safeArguments = ps6080NamedCallSafeMapArguments(
					call, namedSafeMaps[callee], namedCallbacks[callee],
					context.callbackArguments[call],
				)
			}
			if ps6080MapMutationCallWithSafeArguments(call, context, safeArguments) {
				mutated = true
			}
		}
		if returned, ok := node.(*ast.ReturnStmt); ok && reachable(returned) {
			for _, result := range returned.Results {
				if ps6080MapAliasEscapes(result, returned.Pos(), nil, context) {
					mutated = true
				}
			}
		}
		if send, ok := node.(*ast.SendStmt); ok && reachable(send) &&
			ps6080MapAliasEscapes(send.Value, send.Pos(), nil, context) {
			mutated = true
		}
		index, ok := node.(*ast.IndexExpr)
		if !ok {
			return true
		}
		if context.active(index.X, index.Pos()) &&
			reachable(index) && ps6080DynamicDispatchIndex(pass, index.Index) {
			dispatched = true
			return false
		}
		return true
	})
	return dispatched && !mutated
}

func ps6080DynamicDispatchIndex(pass *analysis.Pass, expression ast.Expr) bool {
	if expression == nil || pass.TypesInfo.Types[expression].Value != nil {
		return false
	}
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.Ident:
			if _, variable := pass.TypesInfo.ObjectOf(value).(*types.Var); variable {
				found = true
				return false
			}
		case *ast.CallExpr:
			found = true
			return false
		}
		return true
	})
	return found
}

func ps6080GuardContextKey(pass *analysis.Pass, conditional *ast.IfStmt, enum *types.TypeName) string {
	if !ps6080StatementsTerminate(pass, conditional.Body.List) {
		return ""
	}
	seen := make(map[string]bool)
	ast.Inspect(conditional.Cond, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		variable, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
		if !ok {
			return true
		}
		variableEnum, _ := ps6080EnumType(variable.Type())
		if variableEnum != enum {
			seen[variable.Name()] = true
		}
		return true
	})
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	slices.Sort(result)
	if len(result) == 0 {
		return "enum alternatives"
	}
	return strings.Join(result, ",")
}

func ps6080LiteralAlternativeGroup(group string, node ast.Node, parents map[ast.Node]ast.Node) string {
	if group == "" {
		return ""
	}
	for current := node; current != nil; current = parents[current] {
		if literal, ok := current.(*ast.FuncLit); ok {
			return group + "@" + strconv.Itoa(int(literal.Pos()))
		}
	}
	return group
}

func ps6080IfAlternativeGroup(
	pass *analysis.Pass,
	function *ps6080Function,
	enum *types.TypeName,
	conditional *ast.IfStmt,
	parents map[ast.Node]ast.Node,
) string {
	if group := ps6080FlowAlternativeGroup(pass, function, enum, conditional, parents); group != "" {
		return group
	}
	return ps6080LiteralAlternativeGroup(
		ps6080GuardContextKey(pass, conditional, enum), conditional, parents,
	)
}

func ps6080GroupShapeIfAlternatives(
	pass *analysis.Pass,
	sites []*ps6080Site,
	root ast.Node,
) {
	var conditionals []*ast.IfStmt
	var literals []*ast.FuncLit
	ast.Inspect(root, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.IfStmt:
			if value.Else != nil {
				conditionals = append(conditionals, value)
			}
		case *ast.FuncLit:
			literals = append(literals, value)
		}
		return true
	})
	for conditionalIndex := len(conditionals) - 1; conditionalIndex >= 0; conditionalIndex-- {
		conditional := conditionals[conditionalIndex]
		enums := make(map[*types.TypeName]bool, len(sites))
		for _, site := range sites {
			enums[site.enum] = true
		}
		for enum := range enums {
			if ps6080ExpressionDispatchesEnum(pass, conditional.Cond, enum) {
				continue
			}
			literal := ps6080EnclosingFunctionLiteral(literals, conditional.Pos(), conditional.End())
			thenSites, thenGroups := ps6080LogicalSitesWithin(
				sites, enum, conditional.Body, literal, literals,
			)
			elseSites, elseGroups := ps6080LogicalSitesWithin(
				sites, enum, conditional.Else, literal, literals,
			)
			if thenGroups != 1 || elseGroups != 1 {
				continue
			}
			group := "shape alternatives@" + strconv.Itoa(int(conditional.Pos()))
			for _, site := range append(thenSites, elseSites...) {
				site.group = group
			}
		}
	}
}

func ps6080LogicalSitesWithin(
	sites []*ps6080Site,
	enum *types.TypeName,
	region ast.Node,
	literal *ast.FuncLit,
	literals []*ast.FuncLit,
) ([]*ps6080Site, int) {
	var result []*ps6080Site
	groups := make(map[string]bool, len(sites))
	for _, site := range sites {
		if site.enum != enum || site.position < region.Pos() || region.End() < site.end ||
			ps6080EnclosingFunctionLiteral(literals, site.position, site.end) != literal {
			continue
		}
		result = append(result, site)
		group := site.group
		if group == "" {
			group = site.kind + "@" + strconv.Itoa(int(site.position)) + ":" + strconv.Itoa(int(site.end))
		}
		groups[group] = true
	}
	return result, len(groups)
}

func ps6080EnclosingFunctionLiteral(
	literals []*ast.FuncLit,
	position token.Pos,
	end token.Pos,
) *ast.FuncLit {
	var result *ast.FuncLit
	for _, literal := range literals {
		if literal.Pos() <= position && end <= literal.End() &&
			(result == nil || result.Pos() < literal.Pos()) {
			result = literal
		}
	}
	return result
}

func ps6080EnclosingElseAlternativeGroup(
	pass *analysis.Pass,
	function *ps6080Function,
	enum *types.TypeName,
	node ast.Node,
	parents map[ast.Node]ast.Node,
) string {
	for current := node; current != nil; current = parents[current] {
		if _, literal := current.(*ast.FuncLit); literal {
			return ""
		}
		block, blockNode := current.(*ast.BlockStmt)
		if !blockNode {
			continue
		}
		conditional, elseBlock := parents[block].(*ast.IfStmt)
		if !elseBlock || conditional.Else != block {
			continue
		}
		for {
			outer, elseIf := parents[conditional].(*ast.IfStmt)
			if !elseIf || outer.Else != conditional {
				break
			}
			conditional = outer
		}
		return ps6080IfAlternativeGroup(pass, function, enum, conditional, parents)
	}
	return ""
}

func ps6080FlowAlternativeGroup(
	pass *analysis.Pass,
	function *ps6080Function,
	enum *types.TypeName,
	node ast.Stmt,
	parents map[ast.Node]ast.Node,
) string {
	block, ok := parents[node].(*ast.BlockStmt)
	if !ok || len(block.List) < 2 {
		return ""
	}
	index := slices.Index(block.List, node)
	last := len(block.List) - 1
	if index < 0 {
		return ""
	}
	if _, returned := block.List[last].(*ast.ReturnStmt); !returned {
		return ""
	}
	start := last
	for start > 0 && ps6080AlternativeStatementSupports(pass, function, enum, block.List[start-1]) {
		start--
	}
	if start == last || index < start || index > last {
		return ""
	}
	return "flow alternatives@" + strconv.Itoa(int(block.List[start].Pos()))
}

func ps6080AlternativeStatementSupports(
	pass *analysis.Pass,
	function *ps6080Function,
	enum *types.TypeName,
	statement ast.Stmt,
) bool {
	switch value := statement.(type) {
	case *ast.IfStmt:
		return value.Else == nil && ps6080ExpressionDispatchesEnum(pass, value.Cond, enum) &&
			ps6080StatementsTerminate(pass, value.Body.List) &&
			ps6080StatementsSupport(pass, function, value.Body.List)
	case *ast.SwitchStmt:
		if value.Tag != nil && !ps6080DispatchSubject(pass, value.Tag, enum) {
			return false
		}
		supported := false
		for _, statement := range value.Body.List {
			clause, ok := statement.(*ast.CaseClause)
			if !ok {
				continue
			}
			if len(clause.List) == 0 {
				return false
			}
			if value.Tag == nil && !slices.ContainsFunc(clause.List, func(expression ast.Expr) bool {
				return ps6080ExpressionDispatchesEnum(pass, expression, enum)
			}) {
				continue
			}
			if ps6080StatementsTerminate(pass, clause.Body) && ps6080StatementsSupport(pass, function, clause.Body) {
				supported = true
			}
		}
		return supported
	}
	return false
}

func ps6080ExpressionDispatchesEnum(pass *analysis.Pass, expression ast.Expr, enum *types.TypeName) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		variable, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
		if !ok {
			return true
		}
		variableEnum, _ := ps6080EnumType(variable.Type())
		if variableEnum == enum {
			found = true
			return false
		}
		return true
	})
	return found
}

func ps6080StatementsTerminate(pass *analysis.Pass, statements []ast.Stmt) bool {
	if len(statements) == 0 {
		return false
	}
	switch last := statements[len(statements)-1].(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.ExprStmt:
		call, ok := ps2110Unparen(last.X).(*ast.CallExpr)
		if !ok {
			return false
		}
		return ps6080BuiltinPanic(pass, call)
	case *ast.BlockStmt:
		return ps6080StatementsTerminate(pass, last.List)
	case *ast.IfStmt:
		return last.Else != nil && ps6080StatementsTerminate(pass, last.Body.List) &&
			ps6080StatementsTerminate(pass, []ast.Stmt{last.Else})
	case *ast.SwitchStmt:
		return ps6080SwitchClausesTerminate(pass, last.Body, true)
	case *ast.TypeSwitchStmt:
		return ps6080SwitchClausesTerminate(pass, last.Body, false)
	}
	return false
}

func ps6080SwitchClausesTerminate(pass *analysis.Pass, body *ast.BlockStmt, allowFallthrough bool) bool {
	if body == nil || len(body.List) == 0 {
		return false
	}
	hasDefault := false
	nextTerminates := false
	for index := len(body.List) - 1; index >= 0; index-- {
		statement := body.List[index]
		clause, ok := statement.(*ast.CaseClause)
		if !ok {
			return false
		}
		hasDefault = hasDefault || len(clause.List) == 0
		terminates := ps6080StatementsTerminate(pass, clause.Body)
		if len(clause.Body) > 0 {
			if branch, ok := clause.Body[len(clause.Body)-1].(*ast.BranchStmt); ok && branch.Tok == token.FALLTHROUGH {
				terminates = allowFallthrough && nextTerminates &&
					!ps6080ClausePrefixMayEscape(clause.Body[:len(clause.Body)-1])
			} else if terminates && ps6080ClausePrefixMayEscape(clause.Body[:len(clause.Body)-1]) {
				terminates = false
			}
		}
		if !terminates {
			return false
		}
		nextTerminates = true
	}
	return hasDefault
}

func ps6080ClausePrefixMayEscape(statements []ast.Stmt) bool {
	block := &ast.BlockStmt{List: statements}
	parents := ps6071Parents(block)
	labels := make(map[string]*ast.LabeledStmt)
	ast.Inspect(block, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if statement, ok := node.(*ast.LabeledStmt); ok {
			labels[statement.Label.Name] = statement
		}
		return true
	})
	escapes := false
	ast.Inspect(block, func(node ast.Node) bool {
		if escapes {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		branch, ok := node.(*ast.BranchStmt)
		if !ok || branch.Tok != token.BREAK && branch.Tok != token.GOTO && branch.Tok != token.CONTINUE {
			return true
		}
		if branch.Tok == token.GOTO || !ps6080BranchTargetWithinClausePrefix(branch, parents, labels) {
			escapes = true
			return false
		}
		return true
	})
	return escapes
}

func ps6080BranchTargetWithinClausePrefix(
	branch *ast.BranchStmt,
	parents map[ast.Node]ast.Node,
	labels map[string]*ast.LabeledStmt,
) bool {
	if branch.Label != nil {
		target := labels[branch.Label.Name]
		if target == nil {
			return false
		}
		contained := false
		for current := ast.Node(branch); current != nil; current = parents[current] {
			if current == target {
				contained = true
				break
			}
		}
		if !contained {
			return false
		}
		switch target.Stmt.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			return true
		case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			return branch.Tok == token.BREAK
		}
		return false
	}
	for current := parents[branch]; current != nil; current = parents[current] {
		switch current.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			return true
		case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			if branch.Tok == token.BREAK {
				return true
			}
		}
	}
	return false
}

func ps6080EnclosingGuardConstants(
	pass *analysis.Pass,
	node ast.Node,
	enum *types.TypeName,
	parents map[ast.Node]ast.Node,
	constantEnums map[*types.Const][]*types.TypeName,
) map[*types.Const]token.Pos {
	for current := node; current != nil; current = parents[current] {
		if _, boundary := current.(*ast.FuncLit); boundary {
			break
		}
		conditional, ok := parents[current].(*ast.IfStmt)
		if !ok || !(conditional.Body.Pos() <= node.Pos() && node.End() <= conditional.Body.End()) {
			continue
		}
		groups := make(map[*types.TypeName]*ps6080ConstantGroup)
		ps6080GuardConstants(pass, conditional.Cond, groups, constantEnums, true)
		if group := groups[enum]; group != nil {
			return group.included
		}
		return nil
	}
	return nil
}

func ps6080CallableType(typeOf types.Type) bool {
	if typeOf == nil {
		return false
	}
	_, ok := types.Unalias(typeOf).Underlying().(*types.Signature)
	return ok
}

func ps6080CanonicalizeSites(sites []*ps6080Site) {
	canonical := make(map[*types.TypeName]map[string]*types.Const)
	for _, site := range sites {
		if canonical[site.enum] == nil {
			canonical[site.enum] = make(map[string]*types.Const)
		}
		for _, constants := range []map[*types.Const]token.Pos{site.constants, site.excluded, site.scope} {
			for constant := range constants {
				key := constant.Val().ExactString()
				current := canonical[site.enum][key]
				if current == nil || ps6080EarlierConstant(site.enum, constant, current) {
					canonical[site.enum][key] = constant
				}
			}
		}
	}
	for _, site := range sites {
		normalize := func(constants map[*types.Const]token.Pos) map[*types.Const]token.Pos {
			result := make(map[*types.Const]token.Pos, len(constants))
			for constant, position := range constants {
				representative := canonical[site.enum][constant.Val().ExactString()]
				if prior, exists := result[representative]; !exists || position < prior {
					result[representative] = position
				}
			}
			return result
		}
		site.constants = normalize(site.constants)
		site.excluded = normalize(site.excluded)
		site.scope = normalize(site.scope)
	}
}

func ps6080EarlierConstant(enum *types.TypeName, left, right *types.Const) bool {
	leftDirect := ps6080DirectEnum(left) == enum
	rightDirect := ps6080DirectEnum(right) == enum
	if leftDirect != rightDirect {
		return leftDirect
	}
	if left.Pos().IsValid() != right.Pos().IsValid() {
		return left.Pos().IsValid()
	}
	if left.Pos() != right.Pos() {
		return left.Pos() < right.Pos()
	}
	return left.Name() < right.Name()
}

func ps6080AppendGroupedSites(
	result []*ps6080Site,
	function *ps6080Function,
	kind string,
	position token.Pos,
	end token.Pos,
	groups map[*types.TypeName]*ps6080ConstantGroup,
) []*ps6080Site {
	for enum, group := range groups {
		result = append(result, &ps6080Site{
			function: function, kind: kind, validated: function.validated,
			position: position, end: end,
			enum: enum, constants: group.included, excluded: group.excluded, open: group.open,
		})
	}
	return result
}

func ps6080ExpressionConstants(
	pass *analysis.Pass,
	expression ast.Expr,
	groups map[*types.TypeName]*ps6080ConstantGroup,
	constantEnums map[*types.Const][]*types.TypeName,
	supported bool,
) {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		ps6080AddConstant(pass, value, groups, constantEnums, supported)
	case *ast.SelectorExpr:
		ps6080AddConstant(pass, value.Sel, groups, constantEnums, supported)
	case *ast.CallExpr:
		ps6080DirectConstant(pass, value, groups, constantEnums, supported)
	case *ast.UnaryExpr:
		if value.Op == token.NOT {
			ps6080ExpressionConstants(pass, value.X, groups, constantEnums, supported)
		}
	case *ast.BinaryExpr:
		switch value.Op {
		case token.LAND, token.LOR:
			ps6080ExpressionConstants(pass, value.X, groups, constantEnums, supported)
			ps6080ExpressionConstants(pass, value.Y, groups, constantEnums, supported)
		case token.EQL, token.NEQ:
			included := supported && value.Op == token.EQL
			ps6080DirectConstant(pass, value.X, groups, constantEnums, included)
			ps6080DirectConstant(pass, value.Y, groups, constantEnums, included)
		}
	}
}

func ps6080DirectConstant(
	pass *analysis.Pass,
	expression ast.Expr,
	groups map[*types.TypeName]*ps6080ConstantGroup,
	constantEnums map[*types.Const][]*types.TypeName,
	supported bool,
) {
	constant := ps6080AliasConstant(pass, expression)
	if constant == nil {
		return
	}
	ps6080AddConstantObject(constant, expression.Pos(), groups, constantEnums, supported)
}

func ps6080ComparedConstant(
	pass *analysis.Pass,
	expression ast.Expr,
	subject ast.Expr,
	groups map[*types.TypeName]*ps6080ConstantGroup,
	constantEnums map[*types.Const][]*types.TypeName,
	supported bool,
) {
	constant := ps6080AliasConstant(pass, expression)
	if constant == nil {
		return
	}
	for _, enum := range constantEnums[constant] {
		if ps6080DispatchSubject(pass, subject, enum) {
			ps6080AddConstantObject(constant, expression.Pos(), groups, constantEnums, supported)
			return
		}
	}
}

func ps6080AddConstant(
	pass *analysis.Pass,
	identifier *ast.Ident,
	groups map[*types.TypeName]*ps6080ConstantGroup,
	constantEnums map[*types.Const][]*types.TypeName,
	supported bool,
) {
	constant, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Const)
	if !ok {
		return
	}
	ps6080AddConstantObject(constant, identifier.Pos(), groups, constantEnums, supported)
}

func ps6080AddConstantObject(
	constant *types.Const,
	position token.Pos,
	groups map[*types.TypeName]*ps6080ConstantGroup,
	constantEnums map[*types.Const][]*types.TypeName,
	supported bool,
) {
	for _, enum := range constantEnums[constant] {
		if groups[enum] == nil {
			groups[enum] = &ps6080ConstantGroup{
				included: make(map[*types.Const]token.Pos),
				excluded: make(map[*types.Const]token.Pos),
			}
		}
		if supported {
			groups[enum].included[constant] = position
		} else {
			groups[enum].excluded[constant] = position
		}
	}
}

func ps6080EnumType(typeOf types.Type) (*types.TypeName, bool) {
	if typeOf == nil {
		return nil, false
	}
	named, ok := types.Unalias(typeOf).(*types.Named)
	if !ok || named.Obj() == nil {
		return nil, false
	}
	basic, ok := named.Underlying().(*types.Basic)
	return named.Obj(), ok && basic.Info()&types.IsInteger != 0
}

func ps6080DirectEnum(constant *types.Const) *types.TypeName {
	if constant == nil {
		return nil
	}
	enum, _ := ps6080EnumType(constant.Type())
	return enum
}

type ps6080ConstantRelation struct {
	left  *types.Const
	right *types.Const
}

// ps6080ConstantEnums connects an internal storage ID to a public enum only
// when the source declares an exact constant alias between them. Numeric value
// equality alone is deliberately insufficient: unrelated enum families often
// reuse small iota values. This covers the owner issue's public
// `IQ2_XXS QuantType = tIQ2_XXS` declaration while keeping unrelated integer
// protocols isolated.
func ps6080ConstantEnums(pass *analysis.Pass) map[*types.Const][]*types.TypeName {
	result := make(map[*types.Const][]*types.TypeName)
	var relations []ps6080ConstantRelation
	add := func(constant *types.Const, enum *types.TypeName) bool {
		if constant == nil || enum == nil || slices.Contains(result[constant], enum) {
			return false
		}
		result[constant] = append(result[constant], enum)
		return true
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			for _, specification := range general.Specs {
				value, ok := specification.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for index, name := range value.Names {
					constant, ok := pass.TypesInfo.Defs[name].(*types.Const)
					if !ok {
						continue
					}
					add(constant, ps6080DirectEnum(constant))
					if len(value.Values) != len(value.Names) {
						continue
					}
					alias := ps6080AliasConstant(pass, value.Values[index])
					if alias != nil && alias != constant && alias.Val().ExactString() == constant.Val().ExactString() {
						relations = append(relations, ps6080ConstantRelation{left: constant, right: alias})
					}
				}
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, relation := range relations {
			for _, enum := range result[relation.left] {
				changed = add(relation.right, enum) || changed
			}
			for _, enum := range result[relation.right] {
				changed = add(relation.left, enum) || changed
			}
		}
	}
	return result
}

func ps6080AliasConstant(pass *analysis.Pass, expression ast.Expr) *types.Const {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		constant, _ := pass.TypesInfo.ObjectOf(value).(*types.Const)
		if constant != nil && constant.Pkg() == nil {
			return nil
		}
		return constant
	case *ast.SelectorExpr:
		constant, _ := pass.TypesInfo.ObjectOf(value.Sel).(*types.Const)
		if constant != nil && constant.Pkg() == nil {
			return nil
		}
		return constant
	case *ast.CallExpr:
		if len(value.Args) == 1 && pass.TypesInfo.Types[value.Fun].IsType() {
			return ps6080AliasConstant(pass, value.Args[0])
		}
	}
	return nil
}

func ps6080Eligible(
	sites []*ps6080Site,
	storageReach map[*types.Func]bool,
	decodeReach map[*types.Func]bool,
	constantEnums map[*types.Const][]*types.TypeName,
) map[*types.TypeName]map[*types.Const]ps6080Evidence {
	result := make(map[*types.TypeName]map[*types.Const]ps6080Evidence)
	domains := ps6080EnumDomains(constantEnums)
	for _, site := range sites {
		storage, decode := ps6080EvidenceRoles(site.function, storageReach, decodeReach)
		if !storage && !decode {
			continue
		}
		if result[site.enum] == nil {
			result[site.enum] = make(map[*types.Const]ps6080Evidence)
		}
		constants := site.constants
		if site.open {
			constants = make(map[*types.Const]token.Pos, len(domains[site.enum]))
			for _, constant := range domains[site.enum] {
				if ps6080SiteSupports(site, constant) {
					constants[constant] = constant.Pos()
				}
			}
		}
		for constant := range constants {
			evidence := result[site.enum][constant]
			if storage {
				evidence.storage = append(evidence.storage, site)
			}
			if decode {
				evidence.decode = append(evidence.decode, site)
			}
			result[site.enum][constant] = evidence
		}
	}
	for enum, constants := range result {
		for constant, evidence := range constants {
			if !ps6080IndependentEvidence(evidence) {
				delete(constants, constant)
			}
		}
		if len(constants) == 0 {
			delete(result, enum)
		}
	}
	return result
}

func ps6080EnumDomains(
	constantEnums map[*types.Const][]*types.TypeName,
) map[*types.TypeName][]*types.Const {
	byValue := make(map[*types.TypeName]map[string]*types.Const)
	for constant, enums := range constantEnums {
		for _, enum := range enums {
			if byValue[enum] == nil {
				byValue[enum] = make(map[string]*types.Const)
			}
			value := constant.Val().ExactString()
			prior := byValue[enum][value]
			if prior == nil || ps6080EarlierConstant(enum, constant, prior) {
				byValue[enum][value] = constant
			}
		}
	}
	result := make(map[*types.TypeName][]*types.Const, len(byValue))
	for enum, constants := range byValue {
		for _, constant := range constants {
			result[enum] = append(result[enum], constant)
		}
		slices.SortFunc(result[enum], func(left, right *types.Const) int {
			return cmp.Compare(left.Pos(), right.Pos())
		})
	}
	return result
}

func ps6080IndependentEvidence(evidence ps6080Evidence) bool {
	for _, storage := range evidence.storage {
		for _, decode := range evidence.decode {
			if storage.position != decode.position || storage.end != decode.end {
				return true
			}
		}
	}
	return false
}

func ps6080EvidenceRoles(
	function *ps6080Function,
	storageReach map[*types.Func]bool,
	decodeReach map[*types.Func]bool,
) (storage, decode bool) {
	directStorage := function.roles&ps6080StorageRole != 0
	directDecode := function.roles&ps6080DecodeRole != 0 && decodeReach[function.object]
	switch {
	case directStorage && !directDecode:
		return true, false
	case directDecode && !directStorage:
		return false, true
	case directStorage && directDecode:
		return false, false
	default:
		return storageReach[function.object] && !decodeReach[function.object],
			decodeReach[function.object] && !storageReach[function.object]
	}
}

func ps6080RoleSites(sites []*ps6080Site, enum *types.TypeName, reachable map[*types.Func]bool) []*ps6080Site {
	type siteKey struct {
		function *types.Func
		position token.Pos
		end      token.Pos
		kind     string
		group    string
		subject  types.Object
	}
	seen := make(map[siteKey]*ps6080Site, len(sites))
	var result []*ps6080Site
	for _, site := range sites {
		if site.enum != enum || !reachable[site.function.object] {
			continue
		}
		key := siteKey{
			function: site.function.object, position: site.position, end: site.end,
			kind: site.kind, group: site.group,
			subject: site.subject,
		}
		if (site.kind == "guard" || site.kind == "return guard") && site.group != "" {
			key.position = token.NoPos
			key.end = token.NoPos
			key.kind = "grouped alternatives"
		}
		if strings.HasPrefix(site.group, "flow alternatives@") {
			key.position = token.NoPos
			key.end = token.NoPos
			key.kind = "flow alternatives"
		}
		if strings.HasPrefix(site.group, "shape alternatives@") {
			key.position = token.NoPos
			key.end = token.NoPos
			key.kind = "shape alternatives"
		}
		if site.mapTable {
			key.function = nil
			key.subject = nil
		}
		if prior := seen[key]; prior != nil {
			prior.validated = prior.validated && site.validated
			merged := ps6080UnionGroups(
				&ps6080ConstantGroup{included: prior.constants, excluded: prior.excluded, open: prior.open},
				&ps6080ConstantGroup{included: site.constants, excluded: site.excluded, open: site.open},
			)
			prior.constants = merged.included
			prior.excluded = merged.excluded
			prior.open = merged.open
			prior.scope = ps6080UnionScopes(prior.scope, site.scope)
			for _, reference := range site.references {
				if !slices.Contains(prior.references, reference) {
					prior.references = append(prior.references, reference)
				}
			}
			if prior.referenceRoutes == nil {
				prior.referenceRoutes = make(map[*types.Func][]ps6080ReferenceRoute)
			}
			for reference, routes := range site.referenceRoutes {
				prior.referenceRoutes[reference] = append(prior.referenceRoutes[reference], routes...)
			}
			if site.position < prior.position {
				prior.position = site.position
			}
			if site.end > prior.end {
				prior.end = site.end
			}
			continue
		}
		copy := *site
		copy.constants = make(map[*types.Const]token.Pos, len(site.constants))
		for constant, position := range site.constants {
			copy.constants[constant] = position
		}
		copy.excluded = make(map[*types.Const]token.Pos, len(site.excluded))
		for constant, position := range site.excluded {
			copy.excluded[constant] = position
		}
		copy.scope = make(map[*types.Const]token.Pos, len(site.scope))
		ps6080CopyConstants(copy.scope, site.scope)
		copy.references = slices.Clone(site.references)
		copy.referenceRoutes = make(map[*types.Func][]ps6080ReferenceRoute, len(site.referenceRoutes))
		for reference, routes := range site.referenceRoutes {
			copy.referenceRoutes[reference] = slices.Clone(routes)
		}
		seen[key] = &copy
		result = append(result, &copy)
	}
	slices.SortFunc(result, func(left, right *ps6080Site) int {
		return cmp.Compare(left.position, right.position)
	})
	return result
}

func ps6080ScopedRoleSites(
	sites []*ps6080Site,
	enum *types.TypeName,
	reachable ps6080CPUReachability,
	domains map[*types.TypeName][]*types.Const,
) []*ps6080Site {
	result := ps6080RoleSites(sites, enum, reachable.functions)
	scoped := make([]*ps6080Site, 0, len(result))
	for _, site := range result {
		var values map[string]token.Pos
		limited := false
		if site.literalScope != nil {
			values, limited = ps6080CPUReachCallValues(
				site.literalScope,
				reachable.scopes[site.function.object][site.literalScope.source],
				domains[site.literalScope.enum],
			)
		} else if len(site.references) > 0 {
			limited = true
			for _, reference := range site.references {
				for _, route := range site.referenceRoutes[reference] {
					var referenceValues map[string]token.Pos
					var referenceLimited bool
					if route.literalScope != nil {
						referenceValues, referenceLimited = ps6080CPUReachCallValues(
							route.literalScope,
							reachable.scopes[reference][route.literalScope.source],
							domains[route.literalScope.enum],
						)
					} else {
						referenceValues, referenceLimited = reachable.scopes[reference][route.subject]
					}
					if route.subject == nil && route.literalScope == nil || !referenceLimited {
						limited = false
						break
					}
					values = ps6080CPUUnionValues(values, referenceValues)
				}
				if !limited {
					break
				}
			}
		} else if site.subject != nil {
			values, limited = reachable.scopes[site.function.object][site.subject]
		}
		if !limited {
			scoped = append(scoped, site)
			continue
		}
		if len(values) == 0 {
			continue
		}
		reachScope := make(map[*types.Const]token.Pos, len(values))
		for _, constant := range domains[enum] {
			value := constant.Val().ExactString()
			if position, admitted := values[value]; admitted {
				reachScope[constant] = position
			}
		}
		if len(reachScope) == 0 {
			continue
		}
		site.reachScope = reachScope
		scoped = append(scoped, site)
	}
	return scoped
}

func ps6080BackendEvidence(
	constant *types.Const,
	sites []*ps6080Site,
	owners []ps6080BackendOwnerReach,
	domains map[*types.TypeName][]*types.Const,
) []string {
	seen := make(map[string]bool)
	var result []string
	for _, site := range sites {
		if !ps6080SiteSupports(site, constant) || !ps6080SiteApplies(constant, site, sites) {
			continue
		}
		references := site.references
		if len(references) == 0 {
			references = []*types.Func{site.function.object}
		}
		for _, reference := range references {
			routes := site.referenceRoutes[reference]
			if len(routes) == 0 {
				routes = []ps6080ReferenceRoute{{
					subject: site.subject, literalScope: site.literalScope,
				}}
			}
			for _, owner := range owners {
				if !owner.reach.functions[reference] {
					continue
				}
				admitted := false
				for _, route := range routes {
					var values map[string]token.Pos
					var limited bool
					if route.literalScope != nil {
						values, limited = ps6080CPUReachCallValues(
							route.literalScope,
							owner.reach.scopes[reference][route.literalScope.source],
							domains[route.literalScope.enum],
						)
					} else {
						values, limited = owner.reach.scopes[reference][route.subject]
					}
					if route.subject == nil && route.literalScope == nil || !limited {
						admitted = true
						break
					}
					if _, supported := values[constant.Val().ExactString()]; supported {
						admitted = true
						break
					}
				}
				if !admitted {
					continue
				}
				if !seen[owner.name] {
					seen[owner.name] = true
					result = append(result, owner.name)
				}
			}
		}
	}
	slices.Sort(result)
	return result
}

func ps6080SuppressedConstants(pass *analysis.Pass) map[*types.Const]bool {
	result := make(map[*types.Const]bool)
	lines := make(map[string]bool)
	for _, file := range pass.Files {
		for _, group := range file.Comments {
			text := strings.ToLower(ps6080CommentText(group))
			if !ps6080ValidatedText(text) {
				continue
			}
			position := pass.Fset.Position(group.Pos())
			lines[position.Filename+":"+strconv.Itoa(position.Line)] = true
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			generalValidated := ps6080ValidatedComments(general.Doc)
			for _, specification := range general.Specs {
				value, ok := specification.(*ast.ValueSpec)
				if !ok {
					continue
				}
				validated := generalValidated || ps6080ValidatedComments(value.Doc) || ps6080ValidatedComments(value.Comment)
				for _, identifier := range value.Names {
					constant, ok := pass.TypesInfo.Defs[identifier].(*types.Const)
					position := pass.Fset.Position(identifier.Pos())
					sameLine := lines[position.Filename+":"+strconv.Itoa(position.Line)]
					if ok && (validated || sameLine) {
						result[constant] = true
					}
				}
			}
		}
	}
	return result
}

func ps6080ValidatedComments(groups ...*ast.CommentGroup) bool {
	for _, group := range groups {
		if group != nil && ps6080ValidatedText(strings.ToLower(ps6080CommentText(group))) {
			return true
		}
	}
	return false
}

func ps6080CoverageValidatedComments(groups ...*ast.CommentGroup) bool {
	for _, group := range groups {
		if group != nil && ps6080HasDirective(ps6080CommentText(group), "perfscan:quant-matmul-coverage-validated") {
			return true
		}
	}
	return false
}

func ps6080ValidatedText(text string) bool {
	return ps6080HasDirective(text, "perfscan:quant-matmul-read-only") ||
		ps6080HasDirective(text, "perfscan:quant-matmul-coverage-validated")
}

func ps6080HasDirective(text, directive string) bool {
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.Trim(line, "/*"))
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.EqualFold(fields[0], directive) {
			return true
		}
	}
	return false
}

func ps6080CommentText(groups ...*ast.CommentGroup) string {
	var result strings.Builder
	capacity := 0
	for _, group := range groups {
		if group != nil {
			for _, comment := range group.List {
				capacity += 1 + len(comment.Text)
			}
		}
	}
	result.Grow(capacity)
	for _, group := range groups {
		if group != nil {
			for _, comment := range group.List {
				result.WriteByte('\n')
				result.WriteString(comment.Text)
			}
		}
	}
	return result.String()
}

func ps6080SiteLabel(pass *analysis.Pass, site *ps6080Site) string {
	position := pass.Fset.Position(site.position)
	return site.function.name + " " + site.kind + " (" + filepath.Base(position.Filename) + ":" + strconv.Itoa(position.Line) + ")"
}

func ps6080UniqueFunctions(sites []*ps6080Site) []string {
	seen := make(map[string]bool)
	var result []string
	for _, site := range sites {
		if !seen[site.function.name] {
			seen[site.function.name] = true
			result = append(result, site.function.name)
		}
	}
	slices.Sort(result)
	return result
}

func ps6080Report(pass *analysis.Pass, finding *ps6080Finding) {
	missing := make([]string, len(finding.missing))
	related := make([]analysis.RelatedInformation, 0, len(finding.missing)+2)
	for index, site := range finding.missing {
		missing[index] = ps6080SiteLabel(pass, site)
		related = append(related, analysis.RelatedInformation{
			Pos: site.position, End: site.end,
			Message: "CPU matmul dispatch layer omits " + finding.constant.Name(),
		})
	}
	for _, sites := range [][]*ps6080Site{finding.evidence.storage, finding.evidence.decode} {
		if len(sites) > 0 {
			site := sites[0]
			related = append(related, analysis.RelatedInformation{
				Pos:     ps6080SiteEvidencePosition(site, finding.constant),
				Message: "eligibility evidence in " + ps6080SiteLabel(pass, site),
			})
		}
	}
	present := ps6080UniqueFunctions(finding.present)
	if len(present) == 0 {
		present = []string{"<none>"}
	}
	backend := ""
	if len(finding.backend) > 0 {
		backend = "; other backend matmul evidence mentions it in " + strings.Join(finding.backend, ", ")
	}
	consequence := "has incomplete fast/general route coverage"
	if len(finding.present) == 0 {
		consequence = "can still reach an unsupported-type fallback on CPU"
	}
	pass.Report(analysis.Diagnostic{
		Pos: finding.constant.Pos(), End: finding.constant.Pos() + token.Pos(len(finding.constant.Name())),
		Message: "quant variant " + finding.constant.Name() + " (" + finding.enum.Name() + ") has storage coverage in " +
			strings.Join(ps6080UniqueFunctions(finding.evidence.storage), ", ") + " and portable decode coverage in " +
			strings.Join(ps6080UniqueFunctions(finding.evidence.decode), ", ") + " but is absent from " +
			strconv.Itoa(len(finding.missing)) + " of " + strconv.Itoa(finding.total) + " reachable CPU matmul dispatch layers: " +
			strings.Join(missing, ", ") + "; layers already mentioning it: " + strings.Join(present, ", ") + backend +
			"; it " + consequence + ". Reconcile the typed variant across M=1 and all-M support/helper/dequant layers, or add a semantics- and benchmark-backed narrow suppression (advisory, no automatic fix)",
		Related: related,
	})
}

func ps6080SiteEvidencePosition(site *ps6080Site, constant *types.Const) token.Pos {
	if site == nil {
		if constant != nil {
			return constant.Pos()
		}
		return token.NoPos
	}
	if constant != nil {
		value := constant.Val().ExactString()
		for candidate, position := range site.constants {
			if candidate.Val().ExactString() == value && position.IsValid() {
				return position
			}
		}
	}
	if site.position.IsValid() {
		return site.position
	}
	if constant != nil {
		return constant.Pos()
	}
	return token.NoPos
}
