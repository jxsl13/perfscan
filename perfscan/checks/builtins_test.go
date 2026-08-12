package checks

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// TestBuiltinInScope pins the shared shadow guard: a name resolves to the
// builtin only when no local, package-level, or import-alias declaration of
// that name shadows it at the given position.
func TestBuiltinInScope(t *testing.T) {
	const src = `package p

import mycopy "strings"

func clear() {}

func local() {
	copy := 0
	_ = copy // pos LOCAL: copy shadowed by a local var
}

func fine() {
	_ = 0 // pos FINE: copy is the builtin here
}

var _ = mycopy.Count
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Scopes: map[ast.Node]*types.Scope{}}
	conf := types.Config{Importer: importer.Default()}
	pkg, err := conf.Check("p", fset, []*ast.File{f}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Pkg: pkg, TypesInfo: info}

	posOf := func(marker string) token.Pos {
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if contains(c.Text, marker) {
					return c.Pos()
				}
			}
		}
		t.Fatalf("marker %q not found", marker)
		return 0
	}

	// copy is shadowed by a local var inside local().
	if builtinInScope(pass, posOf("LOCAL"), "copy") {
		t.Error("copy should be shadowed by the local var at LOCAL")
	}
	// copy is the builtin inside fine().
	if !builtinInScope(pass, posOf("FINE"), "copy") {
		t.Error("copy should resolve to the builtin at FINE")
	}
	// clear is a package-level func across the whole file.
	if builtinInScope(pass, posOf("FINE"), "clear") {
		t.Error("clear should be shadowed by the package-level func")
	}
	// A real builtin never shadowed: len.
	if !builtinInScope(pass, posOf("FINE"), "len") {
		t.Error("len should resolve to the builtin at FINE")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
