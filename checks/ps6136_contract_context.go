package checks

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

type ps6136ContractContext struct {
	pass         *analysis.Pass
	packages     map[string]*types.Package
	declarations map[*types.Func]*ast.FuncDecl
}

func ps6136ContractsContext(pass *analysis.Pass) *ps6136ContractContext {
	context := &ps6136ContractContext{pass: pass, packages: make(map[string]*types.Package), declarations: make(map[*types.Func]*ast.FuncDecl)}
	var add func(*types.Package)
	add = func(pkg *types.Package) {
		if pkg == nil || context.packages[pkg.Path()] != nil {
			return
		}
		context.packages[pkg.Path()] = pkg
		for _, imported := range pkg.Imports() {
			add(imported)
		}
	}
	add(pass.Pkg)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok {
				if object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func); ok {
					context.declarations[object] = function
				}
			}
		}
	}
	return context
}

func (context *ps6136ContractContext) named(identity string) *types.Named {
	separator := strings.LastIndexByte(identity, '.')
	if separator < 1 {
		return nil
	}
	pkg := context.packages[identity[:separator]]
	if pkg == nil {
		return nil
	}
	object, ok := pkg.Scope().Lookup(identity[separator+1:]).(*types.TypeName)
	if !ok {
		return nil
	}
	named, ok := types.Unalias(object.Type()).(*types.Named)
	if !ok || named.TypeParams().Len() != 0 {
		return nil
	}
	return named
}

func (context *ps6136ContractContext) callable(identity string) *types.Func {
	separator := strings.LastIndexByte(identity, '.')
	if separator < 1 {
		return nil
	}
	if pkg := context.packages[identity[:separator]]; pkg != nil {
		function, _ := pkg.Scope().Lookup(identity[separator+1:]).(*types.Func)
		return function
	}
	named := context.named(identity[:separator])
	if named == nil {
		return nil
	}
	object, _, _ := types.LookupFieldOrMethod(named, true, context.pass.Pkg, identity[separator+1:])
	function, _ := object.(*types.Func)
	if function == nil || ps6090FunctionID(function) != identity {
		return nil
	}
	return function
}

func ps6136FieldVar(typ types.Type, name string) *types.Var {
	if pointer, ok := types.Unalias(typ).(*types.Pointer); ok {
		typ = pointer.Elem()
	}
	structure, ok := typ.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	for index := 0; index < structure.NumFields(); index++ {
		field := structure.Field(index)
		if field.Name() == name {
			return field
		}
	}
	return nil
}

// Build SSA lazily only after a valid contract resolves to this source package.
// Dependency packages provide typed API metadata, never fabricated source bodies.
func ps6136SourcePackage(pass *analysis.Pass) *ssa.Package {
	program := ssa.NewProgram(pass.Fset, ssa.SanityCheckFunctions)
	seen := make(map[*types.Package]bool)
	var add func(*types.Package)
	add = func(pkg *types.Package) {
		if seen[pkg] {
			return
		}
		seen[pkg] = true
		for _, imported := range pkg.Imports() {
			add(imported)
		}
		if pkg != pass.Pkg {
			program.CreatePackage(pkg, nil, nil, true)
		}
	}
	add(pass.Pkg)
	pkg := program.CreatePackage(pass.Pkg, pass.Files, pass.TypesInfo, true)
	pkg.Build()
	return pkg
}
