package checks

import (
	"go/types"
	"testing"
)

func TestPS6140FlagSnapshotUnknownEffectsReview(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		known      bool
	}{
		{"freshPrerequisite", "", true},
		{"opaqueBoxedOwner", "opaqueAny(d)", false},
		{"opaqueCapturedOwner", "opaqueFunc(func(){d.moe=true})", false},
		{"opaqueWrappedOwner", "opaqueHolder(holder{d})", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pkg := ps6125TestSSA(t, `package extent
type owner struct{moe bool}
type holder struct{d *owner}
func opaqueAny(any)
func opaqueFunc(func())
func opaqueHolder(holder)
func common()*owner{return &owner{}}
func build()*owner{d:=common();`+tc.body+`;return d}`)
			ownerType := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			context := ps6125NewSSAContext(pkg.Func("build"), nil, nil, 1024)
			owner := ps6140TestConstructorOwner(t, context, ownerType)
			field := ps6136FieldVar(ownerType, "moe")
			proof := ps6140ConstructorFlags(context, owner, ownerType, map[*types.Var]bool{field: true}, 16384)
			if (proof != nil) != tc.known {
				t.Fatalf("known=%v want=%v", proof != nil, tc.known)
			}
		})
	}
}
