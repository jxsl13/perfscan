package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"
)

// pkgRefCount counts identifiers in file that resolve to the named
// package import — the guard fixes use to avoid orphaning an import by
// rewriting a file's last reference to it.
func pkgRefCount(pass *analysis.Pass, file *ast.File, path string) int {
	n := 0
	ast.Inspect(file, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if pn, ok := pass.TypesInfo.Uses[id].(*types.PkgName); ok && pn.Imported().Path() == path {
			n++
		}
		return true
	})
	return n
}

// identObject returns the object introduced or referenced by id. Assignment
// helpers use this when the same syntax can either define a short-declaration
// local or assign to an existing local.
func identObject(pass *analysis.Pass, id *ast.Ident) types.Object {
	if obj := pass.TypesInfo.Defs[id]; obj != nil {
		return obj
	}
	return pass.TypesInfo.Uses[id]
}

// tokenSpan is a half-open source interval removed by a suggested fix.
type tokenSpan struct {
	start token.Pos
	end   token.Pos
}

func (s tokenSpan) contains(pos token.Pos) bool {
	return pos >= s.start && pos < s.end
}

// fixDeletedCallScaffolding builds a deletion-only rewrite for nested typed
// package-call scaffolding. It centralizes the three non-local safety checks
// shared by multi-call rules: comments must survive, the last package
// qualifier may only disappear with its import, and deleted local-constant
// uses must not orphan a declaration.
func fixDeletedCallScaffolding(pass *analysis.Pass, file *ast.File, pkgPath, message string, spans ...tokenSpan) (analysis.SuggestedFix, bool) {
	return fixDeletedCallScaffoldingPaths(pass, file, []string{pkgPath}, message, spans...)
}

// fixDeletedCallScaffoldingPaths is the multi-package form used when one
// observer call sheds clone layers from different standard-library packages.
func fixDeletedCallScaffoldingPaths(pass *analysis.Pass, file *ast.File, pkgPaths []string, message string, spans ...tokenSpan) (analysis.SuggestedFix, bool) {
	if len(spans) == 0 {
		return analysis.SuggestedFix{}, false
	}
	edits := make([]analysis.TextEdit, 0, len(spans))
	for _, span := range spans {
		edits = append(edits, analysis.TextEdit{Pos: span.start, End: span.end})
	}
	return fixReplacedCallScaffoldingPaths(pass, file, pkgPaths, message, edits...)
}

// fixReplacedCallScaffoldingPaths is the replacement-capable core of the
// shared call-chain editor. TextEdits may replace wrapper punctuation while
// retaining the inner operand byte-for-byte; import and local-declaration
// safety are computed from the removed source spans exactly as for deletions.
// This lets a terminal rule collapse several independently useful rewrites to
// their fixed point in one pass without duplicating comment/import liveness.
func fixReplacedCallScaffoldingPaths(pass *analysis.Pass, file *ast.File, pkgPaths []string, message string, edits ...analysis.TextEdit) (analysis.SuggestedFix, bool) {
	if len(edits) == 0 {
		return analysis.SuggestedFix{}, false
	}
	spans := make([]tokenSpan, 0, len(edits))
	for _, edit := range edits {
		if edit.Pos > edit.End || ps2111CommentIn(file, edit.Pos, edit.End) {
			return analysis.SuggestedFix{}, false
		}
		spans = append(spans, tokenSpan{start: edit.Pos, end: edit.End})
	}

	allowedImports := make(map[string]bool, len(pkgPaths))
	seenPaths := make(map[string]bool, len(pkgPaths))
	var importsToDrop []string
	for _, pkgPath := range pkgPaths {
		if pkgPath == "" || seenPaths[pkgPath] {
			continue
		}
		seenPaths[pkgPath] = true
		removedRefs := packageRefsInSpans(pass, file, pkgPath, spans...)
		if removedRefs == 0 || removedRefs != pkgRefCount(pass, file, pkgPath) {
			continue
		}
		importsToDrop = append(importsToDrop, pkgPath)
		allowedImports[pkgPath] = true
	}
	if len(importsToDrop) > 0 {
		if ps2110ImportsC(file) {
			return analysis.SuggestedFix{}, false
		}
		importEdits, ok := dropImportsEdits(file, importsToDrop)
		if !ok {
			return analysis.SuggestedFix{}, false
		}
		for _, edit := range importEdits {
			if ps2111CommentIn(file, edit.Pos, edit.End) {
				return analysis.SuggestedFix{}, false
			}
		}
		edits = append(edits, importEdits...)
	}
	if len(allowedImports) == 0 {
		allowedImports = nil
	}
	if !deletionsKeepRequiredUsesAllowingImports(pass, file, allowedImports, spans...) {
		return analysis.SuggestedFix{}, false
	}
	return analysis.SuggestedFix{Message: message, TextEdits: edits}, true
}

// dropImportsEdits removes several import paths using exact ImportSpec spans.
// Exact spans stay disjoint even when independent diagnostics delete adjacent
// imports; format.Source normalizes the leftover whitespace. Attached comments
// make the edit unsafe and are rejected.
func dropImportsEdits(file *ast.File, paths []string) ([]analysis.TextEdit, bool) {
	wanted := make(map[string]bool, len(paths))
	for _, path := range paths {
		wanted[strconv.Quote(path)] = true
	}
	found := make(map[string]bool, len(paths))
	var edits []analysis.TextEdit
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			continue
		}
		removed := make([]bool, len(gen.Specs))
		removedCount := 0
		for i, spec := range gen.Specs {
			imp, ok := spec.(*ast.ImportSpec)
			if !ok || imp.Path == nil || !wanted[imp.Path.Value] {
				continue
			}
			if imp.Doc != nil || imp.Comment != nil {
				return nil, false
			}
			removed[i] = true
			removedCount++
			found[imp.Path.Value] = true
		}
		if removedCount == 0 {
			continue
		}
		if removedCount == len(gen.Specs) {
			edits = append(edits, analysis.TextEdit{Pos: gen.Pos(), End: gen.End()})
			continue
		}
		for index, remove := range removed {
			if !remove {
				continue
			}
			end := gen.Rparen
			if index+1 < len(gen.Specs) {
				end = gen.Specs[index+1].Pos()
			}
			edits = append(edits, analysis.TextEdit{Pos: gen.Specs[index].Pos(), End: end})
		}
	}
	return edits, len(found) == len(wanted)
}

func packageRefsInSpans(pass *analysis.Pass, file *ast.File, pkgPath string, spans ...tokenSpan) int {
	removed := 0
	ast.Inspect(file, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		inside := false
		for _, span := range spans {
			if span.contains(id.Pos()) {
				inside = true
				break
			}
		}
		if !inside {
			return true
		}
		if pkg, ok := pass.TypesInfo.Uses[id].(*types.PkgName); ok && pkg.Imported().Path() == pkgPath {
			removed++
		}
		return true
	})
	return removed
}

// deletionsKeepRequiredUses reports whether deleting spans leaves at least one
// use of every import qualifier and function-local constant referenced only by
// the deleted text. Removing the final such use would orphan an import or make
// a local declaration fail to compile. Package constants may remain unused.
func deletionsKeepRequiredUses(pass *analysis.Pass, file *ast.File, spans ...tokenSpan) bool {
	return deletionsKeepRequiredUsesAllowingImports(pass, file, nil, spans...)
}

// deletionsKeepRequiredUsesAllowingImports is the import-aware form used by a
// fix that supplies its own edit for each path in allowedImports.
func deletionsKeepRequiredUsesAllowingImports(pass *analysis.Pass, file *ast.File, allowedImports map[string]bool, spans ...tokenSpan) bool {
	deleted := func(pos token.Pos) bool {
		for _, span := range spans {
			if span.contains(pos) {
				return true
			}
		}
		return false
	}

	removedObjects := make(map[types.Object]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if !ok || !deleted(id.Pos()) {
			return true
		}
		obj := pass.TypesInfo.Uses[id]
		switch typed := obj.(type) {
		case *types.PkgName:
			if !allowedImports[typed.Imported().Path()] {
				removedObjects[obj] = true
			}
		case *types.Const:
			if typed.Pkg() == pass.Pkg && typed.Parent() != pass.Pkg.Scope() {
				removedObjects[obj] = true
			}
		}
		return true
	})
	if len(removedObjects) == 0 {
		return true
	}
	ast.Inspect(file, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if !ok || deleted(id.Pos()) {
			return true
		}
		delete(removedObjects, pass.TypesInfo.Uses[id])
		return len(removedObjects) != 0
	})
	return len(removedObjects) == 0
}

// deletionsKeepRequiredLocalVariables reports whether removing spans leaves a
// use of every function-local variable referenced only inside those spans.
// Parameters, receivers, named results, fields, and package variables may be
// unused and are exempt; ordinary var/short/range locals are not.
//
// Most call-chain edits retain their semantic operand and therefore need only
// deletionsKeepRequiredUses' import/constant guard. Constant-result rewrites
// can discard the operand itself and use this additional guard to avoid
// producing an otherwise-correct expression in a file that no longer compiles.
func deletionsKeepRequiredLocalVariables(pass *analysis.Pass, file *ast.File, spans ...tokenSpan) bool {
	deleted := func(pos token.Pos) bool {
		for _, span := range spans {
			if span.contains(pos) {
				return true
			}
		}

		return false
	}
	exempt := make(map[types.Object]bool)
	addFields := func(fields *ast.FieldList) {
		if fields == nil {
			return
		}
		for _, field := range fields.List {
			for _, name := range field.Names {
				if object := pass.TypesInfo.Defs[name]; object != nil {
					exempt[object] = true
				}
			}
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.FuncDecl:
			addFields(value.Recv)
			addFields(value.Type.Params)
			addFields(value.Type.Results)
		case *ast.FuncLit:
			addFields(value.Type.Params)
			addFields(value.Type.Results)
		}
		return true
	})

	removed := make(map[types.Object]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || !deleted(identifier.Pos()) {
			return true
		}
		variable, ok := pass.TypesInfo.Uses[identifier].(*types.Var)
		if !ok || variable.IsField() || variable.Pkg() != pass.Pkg || variable.Parent() == pass.Pkg.Scope() || exempt[variable] {
			return true
		}
		removed[variable] = true
		return true
	})
	if len(removed) == 0 {
		return true
	}
	ast.Inspect(file, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || deleted(identifier.Pos()) {
			return true
		}
		delete(removed, pass.TypesInfo.Uses[identifier])
		return len(removed) != 0
	})
	return len(removed) == 0
}

// dropImportEdit returns an edit removing path's import spec (alias included).
// A sole spec drops the entire import declaration; a grouped declaration drops
// the spec and one separating boundary. Callers must reject cgo files before
// applying this edit because their import layout is tied to the C preamble.
func dropImportEdit(file *ast.File, path string) (analysis.TextEdit, bool) {
	quoted := strconv.Quote(path)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			continue
		}
		for i, spec := range gen.Specs {
			imp, ok := spec.(*ast.ImportSpec)
			if !ok || imp.Path == nil || imp.Path.Value != quoted {
				continue
			}
			switch {
			case len(gen.Specs) == 1:
				return analysis.TextEdit{Pos: gen.Pos(), End: gen.End()}, true
			case i+1 < len(gen.Specs):
				return analysis.TextEdit{Pos: imp.Pos(), End: gen.Specs[i+1].Pos()}, true
			default:
				return analysis.TextEdit{Pos: gen.Specs[i-1].End(), End: imp.End()}, true
			}
		}
	}
	return analysis.TextEdit{}, false
}

// baseIdentName returns the root identifier name of x, x.f, x[i], (x), &x.
func baseIdentName(e ast.Expr) string {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			return x.Name
		case *ast.SelectorExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		case *ast.UnaryExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		default:
			return ""
		}
	}
}

// exprMentions reports whether e references the identifier name.
func exprMentions(e ast.Expr, name string) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

// exprEqual compares two expressions by their rendered text.
func exprEqual(a, b ast.Expr) bool {
	return exprTextRendered(a) == exprTextRendered(b)
}

// isSliceMake reports whether call is make([]T, ...).
func isSliceMake(call *ast.CallExpr) bool {
	if astCalleeName(call) != "make" || len(call.Args) == 0 {
		return false
	}
	at, ok := call.Args[0].(*ast.ArrayType)
	return ok && at.Len == nil
}

func astCalleeName(call *ast.CallExpr) string {
	if id, ok := call.Fun.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// enclosingMethodRecv returns the receiver identifier name and method name
// of the *ast.FuncDecl whose body lexically encloses target, when that
// FuncDecl is a method with a NAMED receiver. ok is false otherwise (free
// function, unnamed or blank receiver, or no enclosing declaration). Go
// never nests FuncDecls and closures are *ast.FuncLit, so at most one
// FuncDecl encloses target — a closure inside a method still resolves to
// that method (it closes over the receiver).
func enclosingMethodRecv(pass *analysis.Pass, target ast.Node) (recvName, methodName string, ok bool) {
	for _, f := range pass.Files {
		if target.Pos() < f.Pos() || target.End() > f.End() {
			continue
		}
		for _, d := range f.Decls {
			fn, isFn := d.(*ast.FuncDecl)
			if !isFn || fn.Body == nil ||
				fn.Body.Pos() > target.Pos() || target.End() > fn.Body.End() {
				continue
			}
			if fn.Recv == nil || len(fn.Recv.List) == 0 || len(fn.Recv.List[0].Names) == 0 {
				return "", "", false
			}
			name := fn.Recv.List[0].Names[0].Name
			if name == "_" {
				return "", "", false
			}
			return name, fn.Name.Name, true
		}
		return "", "", false
	}
	return "", "", false
}

// writeFixSelfDispatches reports whether rewriting a write whose writer
// expression is writer would dispatch back into the method that lexically
// encloses node — i.e. the "fix" would turn a correct delegation (like
// WriteString(s) { return d.Write([]byte(s)) }) into unbounded recursion.
// dispatch is the set of method names the rewritten call lands on (e.g.
// "WriteString", or "Write" for the fmt.Fprintf family). It triggers only
// when writer is a bare identifier naming the enclosing method's receiver
// and that method's name is in dispatch; a selector like d.buf or any other
// expression is a different object and is safe. When this reports true the
// finding must be suppressed entirely: the original code is the correct
// delegation and no valid rewrite exists.
func writeFixSelfDispatches(pass *analysis.Pass, node ast.Node, writer ast.Expr, dispatch ...string) bool {
	id, isIdent := writer.(*ast.Ident)
	if !isIdent {
		return false
	}
	recvName, methodName, ok := enclosingMethodRecv(pass, node)
	if !ok || id.Name != recvName {
		return false
	}
	for _, m := range dispatch {
		if methodName == m {
			return true
		}
	}
	return false
}

// isPointerMethod reports whether fn is a method with a pointer receiver.
func isPointerMethod(fn *ast.FuncDecl) bool {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return false
	}
	_, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
	return ok
}

// collectEscaping returns the local names that outlive a loop iteration:
// returned, stored by reference into a field/slot (recv.f = x, ring[i] = x,
// m[k] = x), or used as a value in a composite literal.
func collectEscaping(body *ast.BlockStmt) map[string]bool {
	escaping := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ReturnStmt:
			for _, r := range x.Results {
				if id, ok := r.(*ast.Ident); ok {
					escaping[id.Name] = true
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range x.Lhs {
				switch lhs.(type) {
				case *ast.SelectorExpr, *ast.IndexExpr, *ast.StarExpr:
					if i < len(x.Rhs) {
						if id, ok := x.Rhs[i].(*ast.Ident); ok {
							escaping[id.Name] = true
						}
					}
				}
			}
		case *ast.CompositeLit:
			for _, elt := range x.Elts {
				switch e := elt.(type) {
				case *ast.KeyValueExpr:
					if id, ok := e.Value.(*ast.Ident); ok {
						escaping[id.Name] = true
					}
				case *ast.Ident:
					escaping[e.Name] = true
				}
			}
		}
		return true
	})
	return escaping
}
