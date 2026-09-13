package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6136AuthenticCompleteConstructorInventory(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		t.Run(revision, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6136CompileOwners(t, revision, true, true)
			if err != nil {
				t.Fatal(err)
			}
			pkg := ps6136FixtureSSA(fixture)
			for _, test := range []struct {
				owner string
				count int
			}{{"GPTDecoder", 2}, {"Decoder", 28}} {
				owner := pkg.Pkg.Scope().Lookup(test.owner).Type().(*types.Named)
				inventory := ps6136ConstructorInventory(pkg, owner)
				if len(inventory) != test.count {
					t.Fatalf("%s full source constructor inventory=%d want %d", test.owner, len(inventory), test.count)
				}
			}
			owner := pkg.Pkg.Scope().Lookup("Decoder").Type().(*types.Named)
			profile, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, "ProfileMetalStep")
			if profile == nil || pkg.Prog.FuncValue(profile.(*types.Func)) == nil || len(pkg.Prog.FuncValue(profile.(*types.Func)).Blocks) == 0 {
				t.Fatal("actual one-row profiling API body missing from complete owner replay")
			}
		})
	}
}

func TestPS6136ConstructorInventoryBoundaries(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, extra string
		valid       bool
	}{
		{"fresh_only", "", true},
		{"borrowed", `func borrowed(d *owner)*owner{return d}`, false},
		{"global", `var d=&owner{};func existing()*owner{return d}`, false},
		{"opaque", `func opaque()*owner;func unknown()*owner{return opaque()}`, false},
		{"two_instances", `var flag bool;func ambiguous()*owner{if flag{return &owner{}};return &owner{}}`, false},
		{"wrapper_borrowed", `type wrapper struct{d *owner};func(w wrapper)existing()*owner{return w.d}`, false},
		{"anonymous_borrowed", `func getters(d *owner)func()*owner{return func()*owner{return d}}`, false},
		{"alternate_result_role", `func reversed(d *owner)(error,*owner){return nil,d}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pkg := ps6125TestSSA(t, `package extent;type owner struct{width int};func fresh()*owner{return &owner{}};`+test.extra)
			owner := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			if got := ps6136ConstructorInventory(pkg, owner) != nil; got != test.valid {
				t.Fatalf("closed constructor inventory=%v want %v", got, test.valid)
			}
		})
	}
}

func TestPS6136AuthenticFreshGeometryWrites(t *testing.T) {
	t.Parallel()
	fixture, err := ps6136CompileOwners(t, "before", true, true)
	if err != nil {
		t.Fatal(err)
	}
	pkg := ps6136FixtureSSA(fixture)
	for _, name := range []string{"GPTDecoder", "Decoder"} {
		owner := pkg.Pkg.Scope().Lookup(name).Type().(*types.Named)
		fields := make(map[*types.Var]bool)
		for _, name := range []string{"v", "maxLen", "logits", "ops"} {
			field, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, name)
			fields[field.(*types.Var)] = true
		}
		writes := ps6136FreshConstructorWrites(pkg, owner, fields)
		if writes == nil || len(writes.positions) == 0 {
			t.Fatal("actual constructor geometry/workspace writes lack closed fresh ownership contexts")
		}
		pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
		initializers := ps6136ConstructorWriteNodes(pass, pkg, owner, fields, writes)
		if initializers == nil {
			t.Fatal("actual constructor initialization targets or source caller closure missing")
		}
		for field := range fields {
			if !ps6136FieldEffects(pass, field, initializers) {
				t.Fatalf("actual %s.%s has unclassified direct field effects", name, field.Name())
			}
		}
	}
}
