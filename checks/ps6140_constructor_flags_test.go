package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6140ConstructorFlagSnapshot(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body  string
		known, flag bool
	}{
		{"fresh-zero", "", true, false},
		{"explicit-true", "d.moe=true", true, true},
		{"explicit-false", "d.moe=false", true, false},
		{"ordered-helper", "set(d)", true, true},
		{"ordered-source-closure", "fn:=func(){d.moe=true};fn()", true, true},
		{"different-owner", "other:=common();other.moe=true", true, false},
		{"conditional-true", "if choose{d.moe=true}", false, false},
		{"unknown-value", "d.moe=choose", false, false},
		{"conflicting-stores", "d.moe=true;d.moe=false", false, false},
		{"opaque-owner", "opaque(d)", false, false},
		{"opaque-field", "opaqueBool(&d.moe)", false, false},
		{"source-field-pointer", "setBool(&d.moe)", false, false},
		{"deferred", "defer set(d)", false, false},
		{"asynchronous", "go set(d)", false, false},
		{"whole-owner", "*d=owner{}", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pkg := ps6125TestSSA(t, `package extent
type owner struct{moe bool}
func common()*owner{return &owner{}}
func set(d *owner){d.moe=true}
func setBool(p *bool){*p=true}
func opaque(*owner)
func opaqueBool(*bool)
func build(choose bool)*owner{d:=common();`+test.body+`;return d}
`)
			ownerType := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			context := ps6125NewSSAContext(pkg.Func("build"), nil, nil, 1024)
			owner := ps6140TestConstructorOwner(t, context, ownerType)
			field := ps6136FieldVar(ownerType, "moe")
			proof := ps6140ConstructorFlags(context, owner, ownerType, map[*types.Var]bool{field: true}, 16384)
			if (proof != nil) != test.known {
				t.Fatalf("known=%v want=%v", proof != nil, test.known)
			}
			if proof != nil && (proof.values[field] != test.flag || proof.owner != owner || proof.constructor != context) {
				t.Fatal("wrong flag/constructor/allocation association")
			}
			if ps6140ConstructorFlags(context, owner, ownerType, map[*types.Var]bool{field: true}, 0) != nil {
				t.Fatal("exhausted snapshot budget accepted")
			}
			if ps6140ConstructorFlags(context, owner, ownerType, map[*types.Var]bool{field: false}, 16384) != nil {
				t.Fatal("disabled field masqueraded as a known zero snapshot")
			}
		})
	}
}

func TestPS6140AcyclicOwnerInvocation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body string
		want       bool
	}{
		{"once", "point()", true},
		{"conditional-once", "if choose{point()}", true},
		{"repeated", "for i:=0;i<n;i++{point()}", false},
		{"break-after-call", "for {point();break}", true},
		{"before-loop", "point();for i:=0;i<n;i++{sink(i)}", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pkg := ps6125TestSSA(t, "package extent;func point(){};func sink(int){};func use(n int,choose bool){"+test.body+"}")
			context := ps6125NewSSAContext(pkg.Func("use"), nil, nil, 1024)
			var site *ssa.Call
			for _, call := range ps6125ContextCalls(context) {
				if call.Call.StaticCallee() == pkg.Func("point") {
					if site != nil {
						t.Fatal("ambiguous actual point invocation")
					}
					site = call
				}
			}
			if site == nil {
				t.Fatal("actual point invocation missing")
			}
			budget := 1024
			if got := ps6140AcyclicInstruction(context, site, &budget); got != test.want {
				t.Fatalf("single execution=%v want=%v", got, test.want)
			}
			budget = 0
			if ps6140AcyclicInstruction(context, site, &budget) {
				t.Fatal("exhausted invocation budget accepted")
			}
			budget = 1024
			if ps6140AcyclicInstruction(ps6125NewSSAContext(pkg.Func("point"), nil, nil, 1024), site, &budget) {
				t.Fatal("foreign invocation accepted")
			}
		})
	}
}

func ps6140TestConstructorOwner(t *testing.T, context *ps6125SSAContext, owner *types.Named) ps6125SSAReference {
	t.Helper()
	for _, block := range context.flow.function.Blocks {
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if !ok || len(returned.Results) == 0 {
				continue
			}
			if constant, ok := returned.Results[0].(*ssa.Const); ok && constant.IsNil() {
				continue
			}
			if root := ps6136OwnerRoot(context, returned.Results[0], owner); root.value != nil {
				return root
			}
		}
	}
	t.Fatal("actual fresh constructor result prerequisite missing")
	return ps6125SSAReference{}
}

func TestPS6140AuthenticConstructorFlags(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name                    string
		postNorm, sandwich, moe bool
	}{
		{"newDecoder", false, false, false},
		{"newOLMo2Decoder", true, false, false},
		{"newGemma2Decoder", false, true, false},
		{"newMixtralDecoder", false, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6140CompileLoadedMetal(t, "before")
			if err != nil {
				t.Fatal(err)
			}
			pkg := ps6136FixtureSSA(fixture)
			ownerType := pkg.Pkg.Scope().Lookup("Decoder").Type().(*types.Named)
			context := ps6125NewSSAContext(pkg.Func(test.name), nil, nil, 16384)
			if context == nil {
				t.Fatal("actual architecture constructor absent")
			}
			owner := ps6140TestConstructorOwner(t, context, ownerType)
			wanted := map[*types.Var]bool{ps6136FieldVar(ownerType, "postNorm"): test.postNorm, ps6136FieldVar(ownerType, "sandwich"): test.sandwich, ps6136FieldVar(ownerType, "moe"): test.moe}
			fields := make(map[*types.Var]bool)
			for field := range wanted {
				fields[field] = true
			}
			proof := ps6140ConstructorFlags(context, owner, ownerType, fields, 65536)
			if proof == nil {
				ps6140ConstructorFlagsCheck(context, owner, ownerType, fields, 65536, func(stage string) { t.Log(stage) })
				t.Fatal("actual architecture flag snapshot rejected")
			}
			for field, want := range wanted {
				if proof.values[field] != want {
					t.Fatalf("%s=%v want=%v", field.Name(), proof.values[field], want)
				}
			}
		})
	}
}
