package checks

import (
	"go/types"
	"testing"
)

func TestPS6140PublicConstructorScope(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, before, after string
		want                bool
	}{
		{"genuine", "", "", true},
		{"same-final-value", "", "d.moe=false", true},
		{"later-wrapper-flag", "", "d.moe=true", false},
		{"conditional-wrapper-flag", "", "if choose{d.moe=true}", false},
		{"earlier-wrapper-effect", "sink=1", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pass, pkg := ps6136CellTestPackage(t, ps6140ScenarioSource+`
var choose bool;var sink int
func New(n int)*owner{`+test.before+`;d:=dense(n);`+test.after+`;return d}
`)
			owner := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			public := ps6125NewSSAContext(pkg.Func("New"), nil, nil, 16384)
			var constructor *ps6125SSAContext
			for _, call := range ps6125ContextCalls(public) {
				if call.Call.StaticCallee() == pkg.Func("dense") {
					constructor = ps6136Call(public, call)
				}
			}
			if constructor == nil {
				t.Fatal("actual public/private call prerequisite missing")
			}
			root := ps6140TestConstructorOwner(t, constructor, owner)
			field := ps6136FieldVar(owner, "moe")
			snapshot := ps6140ConstructorFlags(constructor, root, owner, map[*types.Var]bool{field: true}, 65536)
			flags := ps6140ImmutableConstructorFlags(pass, pkg, snapshot, owner, 65536)
			if flags == nil || snapshot.values[field] {
				t.Fatal("genuine private false-flag class prerequisite missing")
			}
			scope := ps6140PublicConstructorScope(constructor, flags, owner, 65536)
			if (scope != nil) != test.want || scope != nil && scope != public {
				t.Fatalf("same flags at public publication=%v want=%v", scope != nil, test.want)
			}
			if ps6140PublicConstructorScope(constructor, flags, owner, 0) != nil || ps6140PublicConstructorScope(public, flags, owner, 65536) != nil {
				t.Fatal("missing budget or mismatched private snapshot admitted")
			}
		})
	}
}
