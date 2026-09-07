package checks

import (
	"cmp"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2145 implements owner issue #798. It recognizes a deliberately narrow
// package-local path wrapper -> streaming parser chain whose reader-derived
// heap payload remains live through returned subslices.
var PS2145 = register(&lint.Check{
	ID:       "PS2145",
	Category: "alloc",
	Slug:     "retained-file-payload-copied-by-stream-parser",
	Level:    lint.LevelAggressive,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a file-path wrapper heap-copies reader-derived payload retained by returned views",
		Text: `A path wrapper can preserve a useful streaming parser yet make a large
regular file take an avoidable detour through a file-sized Go heap buffer. The
cost is especially durable when the parser returns subslices of that buffer:
the complete allocation remains live for the result's lifetime even though a
regular-file path could expose the same immutable bytes through explicitly
owned read-only storage.

PS2145 implements a bounded package-local form of that ownership boundary. It
requires an os.Open file-path wrapper with the exact file deferred for Close,
an optional single bufio.NewReader adapter, and one package-local parser call
whose result is directly returned or bound and returned once. The parser must
fill a small fixed header from the same reader, derive
an unbounded Uint32/Uint64 size through the concrete encoding/binary
LittleEndian or BigEndian receiver, allocate and ReadFull the exact payload (or
use the exact LimitReader/ReadAll chain), and return a concrete struct containing
at least one non-full slice view of that payload. File, reader, header, size,
payload, alias, and result uses are closed. Every nested ReadFull destination
and endian input must preserve a statically valid complete source window, and
the complete wrapper/parser chain must remain reachable under bounded constant
control flow. Small bounded payloads, transformed or independently copied
output, ReaderAt paths, existing Close-owned results, unknown calls, mutations,
escapes, and ambiguous error/control flow stay silent. Proof work is capped at
one wrapper plus one parser, 1,024 AST nodes, and 64 relevant uses.

os.Open does not prove that a path names a regular file. The candidate remedy
is a separate conditional regular-file API, not a rewrite of the streaming
entry point: Stat the same opened handle, use a read-only mapping only for a
regular stable file, parse views without routing them back through the copying
parser, and return an owner whose Close keeps the mapping alive through the
last view use. Reuse the same unadvanced handle for the buffered fallback when
mapping is unsupported or fails. ReaderAt plus independently owned or lazy
range output may be preferable when a mapping contract does not fit.

There is NO automatic fix. Mapped views become invalid immediately after the
owner is closed, and concurrent file truncation can fault while they are in
use. Preserve short-read and parse errors, slice bounds, offsets, aliasing,
fallback order, file identity, and eager-copy contracts. Profile the real
caller and benchmark full payload consumption on every supported platform.`,
		Before: `func openRaw(path string) (*rawModel, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	defer f.Close()
	return parseRaw(bufio.NewReader(f))
}

func parseRaw(r io.Reader) (*rawModel, error) {
	var header [8]byte
	if _, err := io.ReadFull(r, header[:]); err != nil { return nil, err }
	size := binary.LittleEndian.Uint64(header[:])
	if size > uint64(^uint(0)>>1) { return nil, errTooLarge }
	payload := make([]byte, int(size))
	if _, err := io.ReadFull(r, payload); err != nil { return nil, err }
	return &rawModel{meta: payload[:16:16], weights: payload[16:]}, nil
}`,
		After: `// Keep parseRaw for streams. Add a separate regular-file entry point:

type mappedRaw struct {
	model *rawModel
	data  []byte
}

func (m *mappedRaw) Close() error { /* unmap data after the last view use */ }

func openRawOwned(path string) (*mappedRaw, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	defer f.Close()
	if info, err := f.Stat(); err == nil && info.Mode().IsRegular() {
		if data, err := mapReadOnly(f, info.Size()); err == nil {
			return parseMappedOwner(data) // views stay owned until Close
		}
	}
	return parseBufferedOwner(f) // same handle, portable fallback
}`,
		MeasuredWin: `Owner issue #798 measured GoAI's 638 MiB TinyLlama Q4_K_M
file on Apple M2 over ten fresh, order-alternated processes. A Close-owned
read-only mapping changed lazy raw open from 78.824 ms to 8.860 ms (8.90x), and
full consumption of every encoded tensor from 113.81 ms to 72.72 ms (1.57x).
Heap fell from 652.25 MiB/op to 15.07 MiB/op. The full-consumption result shows
that the gain was not merely deferred page faults; these remain attributed
project results, not a universal mapping guarantee.

Perfscan's independent Darwin/arm64 validation used the same full-consumption
boundary with a 16 MiB regular file on Apple M2 Pro and six fixed fresh-process
pairs. The read-only mapping won all six pairs: the pairwise median improved
6.79% (1.073x), while the separate medians moved from 21.668 ms to 20.212 ms.
Heap fell from 16,781,765 B/op to 536 B/op and allocations from 8 to 6. This is
a scoped platform result, not a guarantee for other files, callers, or hosts.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2145",
		Doc:  "file-path streaming parser heap-copies payload retained by returned subslices",
		Run:  runPS2145,
	},
})

const (
	ps2145WrapperNodeLimit = 256
	ps2145ParserNodeLimit  = 768
	ps2145CombinedLimit    = 1024
	ps2145UseLimit         = 64
	ps2145HeaderLimit      = 64 << 10
	ps2145SmallPayload     = 1 << 20
)

type ps2145Function struct {
	declaration *ast.FuncDecl
	signature   *types.Signature
	nodes       int
}

type ps2145Wrapper struct {
	file       types.Object
	open       *ast.CallExpr
	openError  types.Object
	openGuard  *ast.IfStmt
	fileArg    ast.Expr
	closeRoot  ast.Expr
	statRoots  []ast.Expr
	reader     types.Object
	parserCall *ast.CallExpr
	parser     *ps2145Function
	result     types.Object
	resultRoot ast.Expr
	parseError types.Object
	errorRoot  ast.Node
	parseRoot  ast.Node
}

type ps2145Header struct {
	object types.Object
	filled *ast.CallExpr
	buffer ast.Expr
	length int64
}

type ps2145Size struct {
	object    types.Object
	call      *ast.CallExpr
	input     ast.Expr
	bits      int
	alias     types.Object
	aliasRoot ast.Expr
	guards    []ps2145Guard
}

type ps2145Guard struct {
	object types.Object
	upper  uint64
	root   ast.Expr
	stmt   *ast.IfStmt
}

type ps2145Payload struct {
	object     types.Object
	definition ast.Node
	fill       *ast.CallExpr
	buffer     ast.Expr
	sizeUse    ast.Expr
	errorGuard *ast.IfStmt
	error      types.Object
}

func runPS2145(pass *analysis.Pass) (any, error) {
	functions := make(map[*types.Func]*ps2145Function)
	var ordered []*ps2145Function
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
			signature, ok := object.Type().(*types.Signature)
			if !ok {
				continue
			}
			fact := &ps2145Function{
				declaration: function,
				signature:   signature,
				nodes:       ps2145NodeCount(function.Body),
			}
			functions[object] = fact
			ordered = append(ordered, fact)
		}
	}

	slices.SortFunc(ordered, func(left, right *ps2145Function) int {
		return cmp.Compare(left.declaration.Pos(), right.declaration.Pos())
	})
	for _, function := range ordered {
		wrapper, ok := ps2145WrapperProof(pass, function, functions)
		if !ok {
			continue
		}
		if function.nodes+wrapper.parser.nodes > ps2145CombinedLimit || !ps2145ParserProof(pass, &wrapper) {
			continue
		}
		pass.Report(analysis.Diagnostic{
			Pos:     wrapper.parserCall.Pos(),
			End:     wrapper.parserCall.End(),
			Message: "os.Open file-path candidate is routed through a streaming parser that heap-copies reader-derived payload bytes retained by returned subslices; preserve the streaming API and evaluate a conditional regular-file specialization with an explicit Close-owned read-only mapping or ReaderAt representation plus same-handle buffered fallback (no auto-fix: regular-file identity, mapping lifetime, truncation risk, errors, and alias ownership require review)",
		})
	}
	return nil, nil
}

func ps2145WrapperProof(pass *analysis.Pass, function *ps2145Function, functions map[*types.Func]*ps2145Function) (ps2145Wrapper, bool) {
	if function.nodes > ps2145WrapperNodeLimit || function.signature.TypeParams().Len() != 0 || function.signature.Variadic() ||
		!ps2145ResultSignature(function.signature) || ps2145HasClose(function.signature.Results().At(0).Type()) ||
		ps2145WrapperForbiddenControl(function.declaration.Body) {
		return ps2145Wrapper{}, false
	}
	var wrapper ps2145Wrapper
	for _, statement := range function.declaration.Body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || len(assignment.Rhs) != 1 || len(assignment.Lhs) != 2 {
			continue
		}
		call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
		if !ok || !ps2145PackageFunction(pass, call, "os", "Open", 1) {
			continue
		}
		identifier, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
		errorID, errorOK := ps2110Unparen(assignment.Lhs[1]).(*ast.Ident)
		object := identObject(pass, identifier)
		errorObject := identObject(pass, errorID)
		if !ok || !errorOK || object == nil || errorObject == nil || wrapper.file != nil {
			return ps2145Wrapper{}, false
		}
		wrapper.file, wrapper.open, wrapper.openError = object, call, errorObject
		wrapper.openGuard = ps2145ErrorGuardAfter(pass, function.declaration.Body, errorObject, assignment.End())
	}
	ast.Inspect(function.declaration.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if ok {
			if root, isStat := ps2145FileStat(pass, call, wrapper.file); isStat {
				wrapper.statRoots = append(wrapper.statRoots, root)
			}
		}
		return true
	})
	if wrapper.file == nil || wrapper.openGuard == nil {
		return ps2145Wrapper{}, false
	}

	for _, statement := range function.declaration.Body.List {
		if deferred, ok := statement.(*ast.DeferStmt); ok {
			if !ps2145FileClose(pass, deferred.Call, wrapper.file) || wrapper.closeRoot != nil || deferred.Pos() <= wrapper.open.Pos() {
				return ps2145Wrapper{}, false
			}
			selector := ps2110Unparen(deferred.Call.Fun).(*ast.SelectorExpr)
			wrapper.closeRoot = selector.X
		}
		returned, ok := statement.(*ast.ReturnStmt)
		if ok && len(returned.Results) == 1 {
			call, callOK := ps2110Unparen(returned.Results[0]).(*ast.CallExpr)
			if callOK && call.Pos() > wrapper.open.Pos() {
				callee, signature, typed := typedCallee(pass, call.Fun)
				parser := functions[callee]
				if typed && signature.Recv() == nil && parser != nil && parser != function && len(call.Args) == 1 && !call.Ellipsis.IsValid() &&
					ps2145ResultSignature(signature) && !ps2145HasClose(signature.Results().At(0).Type()) &&
					types.Identical(signature.Results().At(0).Type(), function.signature.Results().At(0).Type()) {
					if wrapper.parserCall != nil {
						return ps2145Wrapper{}, false
					}
					wrapper.parserCall, wrapper.parser = call, parser
				}
			}
		}
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 2 || len(assignment.Rhs) != 1 {
			continue
		}
		call, callOK := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
		if !callOK || call.Pos() <= wrapper.open.Pos() || len(call.Args) != 1 || call.Ellipsis.IsValid() {
			continue
		}
		callee, signature, typed := typedCallee(pass, call.Fun)
		parser := functions[callee]
		if !typed || signature.Recv() != nil || parser == nil || parser == function || !ps2145ResultSignature(signature) ||
			ps2145HasClose(signature.Results().At(0).Type()) || !types.Identical(signature.Results().At(0).Type(), function.signature.Results().At(0).Type()) {
			continue
		}
		resultID, resultOK := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
		errorID, errorOK := ps2110Unparen(assignment.Lhs[1]).(*ast.Ident)
		resultObject, errorObject := identObject(pass, resultID), identObject(pass, errorID)
		if !resultOK || !errorOK || resultObject == nil || errorObject == nil || wrapper.parserCall != nil {
			return ps2145Wrapper{}, false
		}
		guard := ps2145ErrorGuardAfter(pass, function.declaration.Body, errorObject, assignment.End())
		resultRoot := ps2145ResultReturnAfter(pass, function.declaration.Body, resultObject, guard)
		if guard == nil || resultRoot == nil {
			continue
		}
		wrapper.parserCall, wrapper.parser = call, parser
		wrapper.result, wrapper.resultRoot, wrapper.parseError, wrapper.errorRoot, wrapper.parseRoot = resultObject, resultRoot, errorObject, guard, assignment
	}
	if wrapper.closeRoot == nil || wrapper.parserCall == nil || wrapper.openGuard.End() >= wrapper.parserCall.Pos() ||
		wrapper.closeRoot.Pos() >= wrapper.parserCall.Pos() || !ps2145ReachableBefore(pass, function.declaration.Body, wrapper.parserCall.Pos()) {
		return ps2145Wrapper{}, false
	}

	argument := ps2110Unparen(wrapper.parserCall.Args[0])
	if identifier, ok := argument.(*ast.Ident); ok && pass.TypesInfo.Uses[identifier] == wrapper.file {
		wrapper.fileArg = identifier
	} else if call, ok := argument.(*ast.CallExpr); ok {
		fileArgument, ok := ps2145BufioAdapter(pass, call, wrapper.file)
		if !ok {
			return ps2145Wrapper{}, false
		}
		wrapper.fileArg = fileArgument
	} else if identifier, ok := argument.(*ast.Ident); ok {
		reader := pass.TypesInfo.Uses[identifier]
		fileArgument, ok := ps2145LocalReader(pass, function.declaration.Body, reader, wrapper.file, wrapper.parserCall.Pos())
		if !ok {
			return ps2145Wrapper{}, false
		}
		wrapper.reader, wrapper.fileArg = reader, fileArgument
	} else {
		return ps2145Wrapper{}, false
	}

	fileRoots := []ast.Node{wrapper.fileArg, wrapper.closeRoot}
	for _, root := range wrapper.statRoots {
		fileRoots = append(fileRoots, root)
	}
	if !ps2145UsesOnly(pass, function.declaration.Body, wrapper.file, fileRoots...) {
		return ps2145Wrapper{}, false
	}
	if wrapper.reader != nil && !ps2145UsesOnly(pass, function.declaration.Body, wrapper.reader, wrapper.parserCall.Args[0]) {
		return ps2145Wrapper{}, false
	}
	if wrapper.result != nil && (!ps2145UsesOnly(pass, function.declaration.Body, wrapper.result, wrapper.resultRoot) ||
		!ps2145UsesOnly(pass, function.declaration.Body, wrapper.parseError, wrapper.openGuard, wrapper.parseRoot, wrapper.errorRoot, wrapper.resultRoot)) {
		return ps2145Wrapper{}, false
	}
	openErrorRoots := []ast.Node{wrapper.openGuard}
	if wrapper.parseError == wrapper.openError {
		openErrorRoots = append(openErrorRoots, wrapper.parseRoot, wrapper.errorRoot, wrapper.resultRoot)
	}
	if !ps2145UsesOnly(pass, function.declaration.Body, wrapper.openError, openErrorRoots...) {
		return ps2145Wrapper{}, false
	}
	return wrapper, true
}

func ps2145ResultReturnAfter(pass *analysis.Pass, body *ast.BlockStmt, object types.Object, guard *ast.IfStmt) ast.Expr {
	if guard == nil {
		return nil
	}
	for _, statement := range body.List {
		returned, ok := statement.(*ast.ReturnStmt)
		if !ok || returned.Pos() <= guard.End() || len(returned.Results) != 2 ||
			ps2145BaseObject(pass, returned.Results[0]) != object || !ps2145Nil(pass, returned.Results[1]) {
			continue
		}
		return returned.Results[0]
	}
	return nil
}

func ps2145ParserProof(pass *analysis.Pass, wrapper *ps2145Wrapper) bool {
	parser := wrapper.parser
	if parser.nodes > ps2145ParserNodeLimit || parser.signature.TypeParams().Len() != 0 || parser.signature.Variadic() ||
		parser.signature.Params().Len() != 1 || !ps2145ResultSignature(parser.signature) ||
		!types.Identical(parser.signature.Results().At(0).Type(), wrapper.parser.signature.Results().At(0).Type()) {
		return false
	}
	reader := parser.signature.Params().At(0)
	if !ps2145ReaderType(reader.Type()) || ps2145ForbiddenControl(parser.declaration.Body) {
		return false
	}
	header, ok := ps2145HeaderProof(pass, parser.declaration.Body, reader)
	if !ok {
		return false
	}
	size, ok := ps2145SizeProof(pass, parser.declaration.Body, header)
	if !ok || size.call.Pos() <= header.filled.End() {
		return false
	}
	payload, ok := ps2145PayloadProof(pass, parser.declaration.Body, reader, size)
	if !ok || payload.definition.Pos() <= size.call.End() {
		return false
	}
	result, aliasObject, aliasRoot, ok := ps2145ReturnedViews(pass, parser, payload)
	if !ok || result.Pos() <= payload.errorGuard.End() || !ps2145ReachableBefore(pass, parser.declaration.Body, result.Pos()) {
		return false
	}

	readerRoots := []ast.Node{header.filled.Args[0]}
	if ps2145PackageFunction(pass, payload.fill, "io", "ReadFull", 2) {
		readerRoots = append(readerRoots, payload.fill.Args[0])
	} else {
		limit := ps2110Unparen(payload.fill.Args[0]).(*ast.CallExpr)
		readerRoots = append(readerRoots, limit.Args[0])
	}
	if !ps2145UsesOnly(pass, parser.declaration.Body, reader, readerRoots...) ||
		!ps2145UsesOnly(pass, parser.declaration.Body, header.object, header.buffer, size.input) {
		return false
	}
	sizeRoots := []ast.Node{size.aliasRoot}
	for _, guard := range size.guards {
		if guard.object == size.object {
			sizeRoots = append(sizeRoots, guard.root)
		}
	}
	sizeRoots = append(sizeRoots, payload.sizeUse)
	if !ps2145UsesOnly(pass, parser.declaration.Body, size.object, sizeRoots...) {
		return false
	}
	if size.alias != nil {
		aliasRoots := []ast.Node{payload.sizeUse}
		for _, guard := range size.guards {
			if guard.object == size.alias {
				aliasRoots = append(aliasRoots, guard.root)
			}
		}
		if !ps2145UsesOnly(pass, parser.declaration.Body, size.alias, aliasRoots...) {
			return false
		}
	}
	payloadRoots := []ast.Node{payload.buffer, result.Results[0]}
	if aliasRoot != nil {
		payloadRoots = append(payloadRoots, aliasRoot)
	}
	if !ps2145UsesOnly(pass, parser.declaration.Body, payload.object, payloadRoots...) ||
		(aliasObject != nil && !ps2145UsesOnly(pass, parser.declaration.Body, aliasObject, result.Results[0])) {
		return false
	}
	if payload.error != nil && !ps2145UsesOnly(pass, parser.declaration.Body, payload.error, payload.definition, payload.errorGuard) {
		return false
	}
	return ps2145RelevantUses(pass, parser.declaration.Body, reader, header.object, size.object, size.alias, payload.object, aliasObject) <= ps2145UseLimit
}

func ps2145HeaderProof(pass *analysis.Pass, body *ast.BlockStmt, reader types.Object) (ps2145Header, bool) {
	var headers []ps2145Header
	for _, statement := range body.List {
		if declaration, ok := statement.(*ast.DeclStmt); ok {
			general, ok := declaration.Decl.(*ast.GenDecl)
			if !ok || general.Tok != token.VAR {
				continue
			}
			for _, specification := range general.Specs {
				value, ok := specification.(*ast.ValueSpec)
				if !ok || len(value.Names) != 1 || len(value.Values) != 0 {
					continue
				}
				object := pass.TypesInfo.Defs[value.Names[0]]
				if length, ok := ps2145FixedBytes(object); ok {
					headers = append(headers, ps2145Header{object: object, length: length})
				}
			}
		}
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		identifier, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
		if !ok {
			continue
		}
		object := identObject(pass, identifier)
		call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
		if !ok || object == nil || !ps2145MakeBytes(pass, call, nil) {
			continue
		}
		length, ok := ps2145Int64Constant(pass, call.Args[1])
		if ok && length >= 4 && length <= ps2145HeaderLimit {
			headers = append(headers, ps2145Header{object: object, length: length})
		}
	}
	for _, header := range headers {
		for _, statement := range body.List {
			condition, ok := statement.(*ast.IfStmt)
			if !ok {
				continue
			}
			call, buffer, ok := ps2145ReadFullGuard(pass, condition, reader, header.object)
			if ok {
				header.filled, header.buffer = call, buffer
				return header, true
			}
		}
	}
	return ps2145Header{}, false
}

func ps2145SizeProof(pass *analysis.Pass, body *ast.BlockStmt, header ps2145Header) (ps2145Size, bool) {
	var size ps2145Size
	for _, statement := range body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		identifier, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
		if !ok {
			continue
		}
		call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
		if !ok || !ps2145EndianSize(pass, call, header) {
			continue
		}
		object := identObject(pass, identifier)
		if object == nil {
			continue
		}
		basic, ok := types.Unalias(object.Type()).Underlying().(*types.Basic)
		if !ok || basic.Kind() != types.Uint32 && basic.Kind() != types.Uint64 || size.object != nil {
			return ps2145Size{}, false
		}
		size = ps2145Size{object: object, call: call, input: call.Args[0], bits: 32}
		if basic.Kind() == types.Uint64 {
			size.bits = 64
		}
	}
	if size.object == nil {
		return ps2145Size{}, false
	}
	for _, statement := range body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || assignment.Pos() <= size.call.End() || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 ||
			ps2145BaseObject(pass, assignment.Rhs[0]) != size.object {
			continue
		}
		identifier, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
		object := identObject(pass, identifier)
		if !ok || object == nil || !types.Identical(object.Type(), size.object.Type()) || size.alias != nil {
			return ps2145Size{}, false
		}
		size.alias, size.aliasRoot = object, assignment.Rhs[0]
	}
	for _, statement := range body.List {
		condition, ok := statement.(*ast.IfStmt)
		if !ok || condition.Pos() <= size.call.End() {
			continue
		}
		for _, object := range []types.Object{size.object, size.alias} {
			bound, root, ok := ps2145UpperGuard(pass, condition, object)
			if ok {
				size.guards = append(size.guards, ps2145Guard{object: object, upper: bound, root: root, stmt: condition})
				break
			}
		}
	}
	return size, true
}

func ps2145PayloadProof(pass *analysis.Pass, body *ast.BlockStmt, reader types.Object, size ps2145Size) (ps2145Payload, bool) {
	for _, statement := range body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Pos() <= size.call.End() || len(assignment.Rhs) != 1 {
			continue
		}
		if len(assignment.Lhs) == 1 {
			identifier, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
			if !ok {
				continue
			}
			object := identObject(pass, identifier)
			call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
			if !ok || object == nil || !ps2145MakeByteSlice(pass, call) {
				continue
			}
			sizeUse, ok := ps2145SizeExpression(pass, call.Args[1], size, assignment.Pos())
			if !ok || ps2145SmallAt(size, assignment.Pos()) {
				continue
			}
			for _, later := range body.List {
				condition, ok := later.(*ast.IfStmt)
				if !ok {
					continue
				}
				fill, buffer, ok := ps2145ReadFullGuard(pass, condition, reader, object)
				if ok && fill.Pos() > assignment.End() {
					return ps2145Payload{object: object, definition: assignment, fill: fill, buffer: buffer, sizeUse: sizeUse, errorGuard: condition}, true
				}
			}
		}
		if len(assignment.Lhs) == 2 {
			payloadID, payloadOK := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
			errorID, errorOK := ps2110Unparen(assignment.Lhs[1]).(*ast.Ident)
			payloadObject, errorObject := identObject(pass, payloadID), identObject(pass, errorID)
			call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
			if !ok {
				continue
			}
			_, sizeUse, ok := ps2145ReadAllLimit(pass, call, reader, size, assignment.Pos())
			if !payloadOK || !errorOK || !ok || payloadObject == nil || errorObject == nil || ps2145SmallAt(size, assignment.Pos()) {
				continue
			}
			guard := ps2145ErrorGuardAfter(pass, body, errorObject, assignment.End())
			if guard == nil {
				continue
			}
			return ps2145Payload{object: payloadObject, definition: assignment, fill: call, buffer: call, sizeUse: sizeUse, errorGuard: guard, error: errorObject}, true
		}
	}
	return ps2145Payload{}, false
}

func ps2145ReturnedViews(pass *analysis.Pass, parser *ps2145Function, payload ps2145Payload) (*ast.ReturnStmt, types.Object, ast.Expr, bool) {
	var alias types.Object
	var aliasRoot ast.Expr
	for _, statement := range parser.declaration.Body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Pos() <= payload.errorGuard.End() || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		identifier, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
		if !ok || !ps2145NonFullSlice(pass, assignment.Rhs[0], payload.object) || alias != nil {
			continue
		}
		alias, aliasRoot = identObject(pass, identifier), assignment.Rhs[0]
	}
	for _, statement := range parser.declaration.Body.List {
		returned, ok := statement.(*ast.ReturnStmt)
		if !ok || returned.Pos() <= payload.fill.End() || len(returned.Results) != 2 || !ps2145Nil(pass, returned.Results[1]) ||
			!types.Identical(pass.TypesInfo.TypeOf(returned.Results[0]), parser.signature.Results().At(0).Type()) {
			continue
		}
		if ps2145ViewResult(pass, returned.Results[0], payload.object, alias, aliasRoot != nil) {
			return returned, alias, aliasRoot, true
		}
	}
	return nil, nil, nil, false
}

func ps2145ViewResult(pass *analysis.Pass, expression ast.Expr, payload, alias types.Object, aliasNonFull bool) bool {
	root := ps2110Unparen(expression)
	if address, ok := root.(*ast.UnaryExpr); ok && address.Op == token.AND {
		root = ps2110Unparen(address.X)
	}
	literal, ok := root.(*ast.CompositeLit)
	if !ok || ps2145ContainsUnsafeCallOrClosure(pass, literal, payload, alias) {
		return false
	}
	structure, ok := types.Unalias(pass.TypesInfo.TypeOf(literal)).Underlying().(*types.Struct)
	if !ok {
		return false
	}
	nonFull, tracked := false, false
	valid := true
	for index, element := range literal.Elts {
		expression := element
		var field *types.Var
		if keyed, isKeyed := element.(*ast.KeyValueExpr); isKeyed {
			expression = keyed.Value
			if identifier, isIdentifier := ps2110Unparen(keyed.Key).(*ast.Ident); isIdentifier {
				field, _ = pass.TypesInfo.Uses[identifier].(*types.Var)
			}
		} else if index < structure.NumFields() {
			field = structure.Field(index)
		}
		if ps2145ExpressionMentions(pass, expression, payload, alias) {
			if field == nil || !ps2145ByteSlice(field.Type()) {
				valid = false
				continue
			}
			tracked = true
			switch value := ps2110Unparen(expression).(type) {
			case *ast.Ident:
				object := pass.TypesInfo.Uses[value]
				if object != payload && object != alias {
					valid = false
				} else if object == alias && aliasNonFull {
					nonFull = true
				}
			case *ast.SliceExpr:
				object := ps2145BaseObject(pass, value.X)
				if object != payload && object != alias {
					valid = false
				}
				if ps2145SliceRestricts(pass, value, object) {
					nonFull = true
				}
			default:
				valid = false
			}
		}
	}
	return valid && tracked && nonFull
}

func ps2145ReadFullGuard(pass *analysis.Pass, statement *ast.IfStmt, reader, buffer types.Object) (*ast.CallExpr, ast.Expr, bool) {
	if statement == nil || statement.Else != nil || !ps2145TerminalReturn(statement.Body) {
		return nil, nil, false
	}
	assignment, ok := statement.Init.(*ast.AssignStmt)
	if !ok || len(assignment.Rhs) != 1 || len(assignment.Lhs) != 2 {
		return nil, nil, false
	}
	call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
	if !ok || !ps2145PackageFunction(pass, call, "io", "ReadFull", 2) ||
		ps2145BaseObject(pass, call.Args[0]) != reader || !ps2145FullBuffer(pass, call.Args[1], buffer) {
		return nil, nil, false
	}
	errorID, ok := ps2110Unparen(assignment.Lhs[1]).(*ast.Ident)
	errorObject := identObject(pass, errorID)
	if !ok || errorObject == nil || !ps2145ErrorCondition(pass, statement.Cond, errorObject) {
		return nil, nil, false
	}
	return call, call.Args[1], true
}

func ps2145ReadAllLimit(pass *analysis.Pass, call *ast.CallExpr, reader types.Object, size ps2145Size, before token.Pos) (*ast.CallExpr, ast.Expr, bool) {
	if !ps2145PackageFunction(pass, call, "io", "ReadAll", 1) {
		return nil, nil, false
	}
	limit, ok := ps2110Unparen(call.Args[0]).(*ast.CallExpr)
	if !ok || !ps2145PackageFunction(pass, limit, "io", "LimitReader", 2) || ps2145BaseObject(pass, limit.Args[0]) != reader {
		return nil, nil, false
	}
	sizeUse, ok := ps2145SizeExpression(pass, limit.Args[1], size, before)
	conversion, converted := ps2110Unparen(sizeUse).(*ast.CallExpr)
	if !ok || !converted || !ps2145BuiltinType(pass, conversion.Fun, types.Int64) {
		return nil, nil, false
	}
	return limit, sizeUse, true
}

func ps2145ErrorGuardAfter(pass *analysis.Pass, body *ast.BlockStmt, object types.Object, after token.Pos) *ast.IfStmt {
	for _, statement := range body.List {
		condition, ok := statement.(*ast.IfStmt)
		if ok && condition.Pos() > after && condition.Init == nil && condition.Else == nil &&
			ps2145ErrorCondition(pass, condition.Cond, object) && ps2145TerminalReturn(condition.Body) {
			return condition
		}
	}
	return nil
}

func ps2145ErrorCondition(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || binary.Op != token.NEQ {
		return false
	}
	return ps2145BaseObject(pass, binary.X) == object && ps2145Nil(pass, binary.Y) ||
		ps2145BaseObject(pass, binary.Y) == object && ps2145Nil(pass, binary.X)
}

func ps2145UpperGuard(pass *analysis.Pass, statement *ast.IfStmt, object types.Object) (uint64, ast.Expr, bool) {
	if object == nil {
		return 0, nil, false
	}
	if statement.Init != nil || statement.Else != nil || !ps2145TerminalReturn(statement.Body) {
		return 0, nil, false
	}
	condition, ok := ps2110Unparen(statement.Cond).(*ast.BinaryExpr)
	if !ok {
		return 0, nil, false
	}
	var constantExpression ast.Expr
	strict := false
	switch {
	case ps2145BaseObject(pass, condition.X) == object && (condition.Op == token.GTR || condition.Op == token.GEQ):
		constantExpression, strict = condition.Y, condition.Op == token.GTR
	case ps2145BaseObject(pass, condition.Y) == object && (condition.Op == token.LSS || condition.Op == token.LEQ):
		constantExpression, strict = condition.X, condition.Op == token.LSS
	default:
		return 0, nil, false
	}
	bound, ok := ps2145Uint64Constant(pass, constantExpression)
	if !ok {
		return 0, nil, false
	}
	if !strict {
		if bound == 0 {
			return 0, nil, false
		}
		bound--
	}
	return bound, statement.Cond, true
}

func ps2145EndianSize(pass *analysis.Pass, call *ast.CallExpr, header ps2145Header) bool {
	if len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return false
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || signature.Recv() == nil || function.Pkg() == nil || function.Pkg().Path() != "encoding/binary" ||
		function.Name() != "Uint32" && function.Name() != "Uint64" {
		return false
	}
	width := int64(4)
	if function.Name() == "Uint64" {
		width = 8
	}
	if !ps2145EndianInput(pass, call.Args[0], header, width) {
		return false
	}
	method, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	receiver, ok := ps2110Unparen(method.X).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	object, ok := pass.TypesInfo.Uses[receiver.Sel].(*types.Var)
	return ok && object.Pkg() != nil && object.Pkg().Path() == "encoding/binary" &&
		(object.Name() == "LittleEndian" || object.Name() == "BigEndian")
}

func ps2145EndianInput(pass *analysis.Pass, expression ast.Expr, header ps2145Header, width int64) bool {
	low, high, ok := ps2145ConstantWindow(pass, expression, header.object, header.length)
	return ok && high-low >= width
}

func ps2145ConstantWindow(pass *analysis.Pass, expression ast.Expr, object types.Object, length int64) (int64, int64, bool) {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		if pass.TypesInfo.Uses[value] != object {
			return 0, 0, false
		}
		return 0, length, true
	case *ast.SliceExpr:
		if value.Max != nil {
			return 0, 0, false
		}
		baseLow, baseHigh, ok := ps2145ConstantWindow(pass, value.X, object, length)
		if !ok {
			return 0, 0, false
		}
		low, high := int64(0), baseHigh-baseLow
		if value.Low != nil {
			low, ok = ps2145Int64Constant(pass, value.Low)
			if !ok {
				return 0, 0, false
			}
		}
		if value.High != nil {
			high, ok = ps2145Int64Constant(pass, value.High)
			if !ok {
				return 0, 0, false
			}
		}
		if low < 0 || low > high || high > baseHigh-baseLow {
			return 0, 0, false
		}
		return baseLow + low, baseLow + high, true
	default:
		return 0, 0, false
	}
}

func ps2145LocalReader(pass *analysis.Pass, body *ast.BlockStmt, reader, file types.Object, before token.Pos) (ast.Expr, bool) {
	if reader == nil {
		return nil, false
	}
	for _, statement := range body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Pos() >= before || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		identifier, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
		if !ok || identObject(pass, identifier) != reader {
			continue
		}
		call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
		if !ok {
			return nil, false
		}
		fileArgument, ok := ps2145BufioAdapter(pass, call, file)
		return fileArgument, ok
	}
	return nil, false
}

func ps2145BufioAdapter(pass *analysis.Pass, call *ast.CallExpr, file types.Object) (ast.Expr, bool) {
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || signature.Recv() != nil || function.Pkg() == nil || function.Pkg().Path() != "bufio" ||
		(function.Name() != "NewReader" && function.Name() != "NewReaderSize") || len(call.Args) < 1 || len(call.Args) > 2 || call.Ellipsis.IsValid() ||
		ps2145BaseObject(pass, call.Args[0]) != file {
		return nil, false
	}
	if function.Name() == "NewReader" && len(call.Args) != 1 || function.Name() == "NewReaderSize" && len(call.Args) != 2 {
		return nil, false
	}
	return call.Args[0], true
}

func ps2145FileClose(pass *analysis.Pass, call *ast.CallExpr, file types.Object) bool {
	if call == nil || len(call.Args) != 0 {
		return false
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	selector, selected := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	return ok && selected && signature.Recv() != nil && function.Pkg() != nil && function.Pkg().Path() == "os" &&
		function.Name() == "Close" && ps2145BaseObject(pass, selector.X) == file
}

func ps2145FileStat(pass *analysis.Pass, call *ast.CallExpr, file types.Object) (ast.Expr, bool) {
	if call == nil || len(call.Args) != 0 {
		return nil, false
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	selector, selected := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || !selected || signature.Recv() == nil || function.Pkg() == nil || function.Pkg().Path() != "os" ||
		function.Name() != "Stat" || ps2145BaseObject(pass, selector.X) != file {
		return nil, false
	}
	return selector.X, true
}

func ps2145PackageFunction(pass *analysis.Pass, call *ast.CallExpr, path, name string, arity int) bool {
	if call == nil || len(call.Args) != arity || call.Ellipsis.IsValid() {
		return false
	}
	function, signature, ok := typedCallee(pass, call.Fun)
	return ok && signature.Recv() == nil && function.Pkg() != nil && function.Pkg().Path() == path && function.Name() == name
}

func ps2145MakeBytes(pass *analysis.Pass, call *ast.CallExpr, size types.Object) bool {
	if !ps2145MakeByteSlice(pass, call) {
		return false
	}
	if size == nil {
		_, ok := ps2145Int64Constant(pass, call.Args[1])
		return ok
	}
	return ps2145BaseObject(pass, call.Args[1]) == size
}

func ps2145MakeByteSlice(pass *analysis.Pass, call *ast.CallExpr) bool {
	if call == nil || len(call.Args) != 2 || call.Ellipsis.IsValid() || !ps2145ByteSlice(pass.TypesInfo.TypeOf(call)) {
		return false
	}
	identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := pass.TypesInfo.Uses[identifier].(*types.Builtin)
	return ok && builtin.Name() == "make"
}

func ps2145SizeExpression(pass *analysis.Pass, expression ast.Expr, size ps2145Size, before token.Pos) (ast.Expr, bool) {
	root := ps2110Unparen(expression)
	if object := ps2145BaseObject(pass, root); object == size.object || object != nil && object == size.alias {
		return expression, true
	}
	conversion, ok := root.(*ast.CallExpr)
	if !ok || len(conversion.Args) != 1 || conversion.Ellipsis.IsValid() {
		return nil, false
	}
	source := ps2145BaseObject(pass, conversion.Args[0])
	if source != size.object && (size.alias == nil || source != size.alias) {
		return nil, false
	}
	kind := types.Invalid
	switch {
	case ps2145BuiltinType(pass, conversion.Fun, types.Int):
		kind = types.Int
	case ps2145BuiltinType(pass, conversion.Fun, types.Int64):
		kind = types.Int64
	default:
		return nil, false
	}
	max := uint64(1<<63 - 1)
	if kind == types.Int {
		bytes := pass.TypesSizes.Sizeof(types.Typ[types.Int])
		if bytes <= 0 || bytes > 8 {
			return nil, false
		}
		bits := uint(bytes * 8)
		max = uint64(1)<<(bits-1) - 1
	}
	if size.bits == 32 && max >= uint64(1<<32-1) || ps2145UpperAt(size, before, max) {
		return expression, true
	}
	return nil, false
}

func ps2145UpperAt(size ps2145Size, before token.Pos, max uint64) bool {
	for _, guard := range size.guards {
		if guard.stmt.End() < before && guard.upper <= max {
			return true
		}
	}
	return false
}

func ps2145SmallAt(size ps2145Size, before token.Pos) bool {
	return ps2145UpperAt(size, before, ps2145SmallPayload)
}

func ps2145FixedBytes(object types.Object) (int64, bool) {
	if object == nil {
		return 0, false
	}
	array, ok := types.Unalias(object.Type()).Underlying().(*types.Array)
	if !ok || !ps2145Byte(array.Elem()) || array.Len() < 4 || array.Len() > ps2145HeaderLimit {
		return 0, false
	}
	return array.Len(), true
}

func ps2145ResultSignature(signature *types.Signature) bool {
	if signature == nil || signature.Results().Len() != 2 || !types.Identical(signature.Results().At(1).Type(), types.Universe.Lookup("error").Type()) {
		return false
	}
	typeOf := types.Unalias(signature.Results().At(0).Type())
	if pointer, ok := typeOf.(*types.Pointer); ok {
		typeOf = types.Unalias(pointer.Elem())
	}
	_, ok := typeOf.Underlying().(*types.Struct)
	return ok
}

func ps2145HasClose(typeOf types.Type) bool {
	typesToCheck := []types.Type{typeOf}
	if named, ok := types.Unalias(typeOf).(*types.Named); ok {
		typesToCheck = append(typesToCheck, types.NewPointer(named))
	}
	for _, candidate := range typesToCheck {
		selection := types.NewMethodSet(candidate).Lookup(nil, "Close")
		if selection == nil {
			continue
		}
		signature, ok := selection.Obj().Type().(*types.Signature)
		if ok && signature.Params().Len() == 0 && signature.Results().Len() == 1 &&
			types.Identical(signature.Results().At(0).Type(), types.Universe.Lookup("error").Type()) {
			return true
		}
	}
	return false
}

func ps2145ReaderType(typeOf types.Type) bool {
	typeOf = types.Unalias(typeOf)
	if named, ok := typeOf.(*types.Named); ok {
		object := named.Obj()
		return object.Pkg() != nil && object.Pkg().Path() == "io" && object.Name() == "Reader"
	}
	if pointer, ok := typeOf.(*types.Pointer); ok {
		if named, ok := types.Unalias(pointer.Elem()).(*types.Named); ok {
			object := named.Obj()
			return object.Pkg() != nil && object.Pkg().Path() == "bufio" && object.Name() == "Reader"
		}
	}
	return false
}

func ps2145ByteSlice(typeOf types.Type) bool {
	slice, ok := types.Unalias(typeOf).Underlying().(*types.Slice)
	return ok && ps2145Byte(slice.Elem())
}

func ps2145Byte(typeOf types.Type) bool {
	basic, ok := types.Unalias(typeOf).Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Uint8
}

func ps2145BaseObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return pass.TypesInfo.Uses[value]
	case *ast.SliceExpr:
		return ps2145BaseObject(pass, value.X)
	}
	return nil
}

func ps2145UsesOnly(pass *analysis.Pass, body *ast.BlockStmt, object types.Object, allowed ...ast.Node) bool {
	if object == nil {
		return false
	}
	valid := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[identifier] != object {
			return true
		}
		for _, root := range allowed {
			if root != nil && ps2145Within(identifier, root) {
				return true
			}
		}
		valid = false
		return false
	})
	return valid
}

func ps2145RelevantUses(pass *analysis.Pass, body *ast.BlockStmt, objects ...types.Object) int {
	set := make(map[types.Object]bool, len(objects))
	for _, object := range objects {
		if object != nil {
			set[object] = true
		}
	}
	uses := 0
	ast.Inspect(body, func(node ast.Node) bool {
		if identifier, ok := node.(*ast.Ident); ok && set[pass.TypesInfo.Uses[identifier]] {
			uses++
		}
		return uses <= ps2145UseLimit
	})
	return uses
}

func ps2145ForbiddenControl(body *ast.BlockStmt) bool {
	forbidden := false
	ast.Inspect(body, func(node ast.Node) bool {
		if forbidden {
			return false
		}
		switch node.(type) {
		case *ast.FuncLit, *ast.GoStmt, *ast.SendStmt, *ast.DeferStmt, *ast.BranchStmt,
			*ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			forbidden = true
			return false
		}
		return true
	})
	return forbidden
}

func ps2145WrapperForbiddenControl(body *ast.BlockStmt) bool {
	forbidden := false
	ast.Inspect(body, func(node ast.Node) bool {
		if forbidden {
			return false
		}
		switch node.(type) {
		case *ast.FuncLit, *ast.GoStmt, *ast.SendStmt, *ast.BranchStmt,
			*ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			forbidden = true
			return false
		}
		return true
	})
	return forbidden
}

func ps2145ReachableBefore(pass *analysis.Pass, body *ast.BlockStmt, before token.Pos) bool {
	return !ps2144PositionIn(before, ps2144Unreachable(pass, body))
}

func ps2145NodeCount(node ast.Node) int {
	count := 0
	ast.Inspect(node, func(node ast.Node) bool {
		if node != nil {
			count++
		}
		return count <= ps2145CombinedLimit
	})
	return count
}

func ps2145TerminalReturn(body *ast.BlockStmt) bool {
	if body == nil || len(body.List) == 0 {
		return false
	}
	_, ok := body.List[len(body.List)-1].(*ast.ReturnStmt)
	return ok
}

func ps2145NonFullSlice(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	slice, ok := ps2110Unparen(expression).(*ast.SliceExpr)
	return ok && ps2145BaseObject(pass, slice.X) == object && ps2145SliceRestricts(pass, slice, object)
}

func ps2145FullBuffer(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return pass.TypesInfo.Uses[value] == object
	case *ast.SliceExpr:
		return value.Low == nil && value.High == nil && value.Max == nil && ps2145FullBuffer(pass, value.X, object)
	default:
		return false
	}
}

func ps2145SliceRestricts(pass *analysis.Pass, slice *ast.SliceExpr, object types.Object) bool {
	if slice.Low != nil {
		if low, ok := ps2145Int64Constant(pass, slice.Low); !ok || low != 0 {
			return true
		}
	}
	if slice.High != nil && !ps2145FullExtent(pass, slice.High, object) {
		return true
	}
	return slice.Max != nil && !ps2145FullExtent(pass, slice.Max, object)
}

func ps2145FullExtent(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() || ps2145BaseObject(pass, call.Args[0]) != object {
		return false
	}
	identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := pass.TypesInfo.Uses[identifier].(*types.Builtin)
	return ok && (builtin.Name() == "len" || builtin.Name() == "cap")
}

func ps2145ContainsUnsafeCallOrClosure(pass *analysis.Pass, node ast.Node, payload, alias types.Object) bool {
	found := false
	ast.Inspect(node, func(node ast.Node) bool {
		if found {
			return false
		}
		switch value := node.(type) {
		case *ast.FuncLit:
			found = true
			return false
		case *ast.CallExpr:
			identifier, ok := ps2110Unparen(value.Fun).(*ast.Ident)
			builtin, builtinOK := pass.TypesInfo.Uses[identifier].(*types.Builtin)
			argument := types.Object(nil)
			if len(value.Args) == 1 {
				argument = ps2145BaseObject(pass, value.Args[0])
			}
			if !ok || !builtinOK || builtin.Name() != "len" && builtin.Name() != "cap" ||
				argument != payload && argument != alias {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func ps2145ExpressionMentions(pass *analysis.Pass, expression ast.Expr, objects ...types.Object) bool {
	set := make(map[types.Object]bool, len(objects))
	for _, object := range objects {
		if object != nil {
			set[object] = true
		}
	}
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if identifier, ok := node.(*ast.Ident); ok && set[pass.TypesInfo.Uses[identifier]] {
			found = true
			return false
		}
		return !found
	})
	return found
}

func ps2145Within(node, root ast.Node) bool {
	return node != nil && root != nil && node.Pos() >= root.Pos() && node.End() <= root.End()
}

func ps2145Nil(pass *analysis.Pass, expression ast.Expr) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && pass.TypesInfo.Uses[identifier] == types.Universe.Lookup("nil")
}

func ps2145BuiltinType(pass *analysis.Pass, expression ast.Expr, kind types.BasicKind) bool {
	if !pass.TypesInfo.Types[expression].IsType() {
		return false
	}
	basic, ok := types.Unalias(pass.TypesInfo.TypeOf(expression)).Underlying().(*types.Basic)
	return ok && basic.Kind() == kind
}

func ps2145Int64Constant(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	if value == nil || value.Kind() != constant.Int {
		return 0, false
	}
	return constant.Int64Val(value)
}

func ps2145Uint64Constant(pass *analysis.Pass, expression ast.Expr) (uint64, bool) {
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	if value == nil || value.Kind() != constant.Int || constant.Sign(value) < 0 {
		return 0, false
	}
	return constant.Uint64Val(value)
}
