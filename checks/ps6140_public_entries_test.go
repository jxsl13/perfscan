package checks

import (
	"go/types"
	"testing"
)

func TestPS6140PublicWorkspaceUses(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, extra string
		want        bool
	}{
		{"private-unreachable-read", "", true},
		{"public-read", "func(d *owner)Extra(){d.scratch.b.touch()}", false},
		{"public-helper-read", "func Inspect(d *owner){d.private()}", false},
		{"public-helper-unused", "func Inspect(n int,d *owner){ignore(d.scratch.b)}", true},
		{"public-query", "func(d *owner)Query()bool{return d.moe}", true},
		{"public-helper-async", "func(d *owner)Extra(){go d.private()}", false},
		{"public-helper-deferred", "func(d *owner)Extra(){defer d.private()}", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pass, pkg := ps6136CellTestPackage(t, `package cells
type buffer interface{touch()};type slot struct{b buffer}
type api interface{apply(buffer)};type linear struct{};func(linear)apply(buffer){}
type block struct{projection api};type owner struct{blocks []block;moe bool;scratch *slot}
func dense(n int)*owner{d:=&owner{scratch:&slot{}};for i:=0;i<n;i++{b:=block{projection:linear{}};d.blocks=append(d.blocks,b)};return d}
func(d *owner)Run(){for _,b:=range d.blocks{b.projection.apply(d.scratch.b)}}
func(d *owner)private(){d.scratch.b.touch()};func ignore(buffer){}
`+test.extra)
			collection, flags, owner := ps6140TestScenarioProofs(t, pass, pkg, "dense")
			workspace := ps6136FieldVar(owner, "scratch")
			slot := ps6136FieldVar(workspace.Type(), "b")
			object, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, "Run")
			method := pkg.Prog.FuncValue(object.(*types.Func))
			positive := ps6140NewConstructorScenario(collection, flags, owner, method, 65536)
			if ps6140ScenarioWorkspaceUses(positive, workspace, slot, 0, 65536) == nil {
				t.Fatal("genuine selected Run unused-formal prerequisite missing")
			}
			proof := ps6140PublicWorkspaceUses(pkg, collection, flags, owner, workspace, slot, 0, 65536)
			if (proof != nil) != test.want {
				t.Fatalf("complete public source uses=%v want=%v", proof != nil, test.want)
			}
			for entry, uses := range proof {
				if entry.function.Name() == "Inspect" && test.want && (entry.parameter != 1 || uses.scenario.receiver != (ps6125SSAReference{context: uses.scenario.root, value: entry.function.Params[1]})) {
					t.Fatal("non-receiver owner role lost")
				}
				if uses.scenario.root.flow.function != entry.function || uses.workspace != workspace {
					t.Fatal("entry proof lost source association")
				}
			}
			// A missing source entry cannot create a scenario.
			if ps6140NewConstructorScenarioEntry(collection, flags, owner, nil, 0, 65536) != nil {
				t.Fatal("missing entry admitted")
			}
		})
	}
}

func TestPS6140PublicOwnerEntries(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, extra string
		want        int
	}{
		{"genuine", "", 2},
		{"non-receiver-role", "func Inspect(n int,d *owner){d.private()}", 3},
		{"two-owner-roles", "func Compare(a,b *owner){a.private();b.private()}", 4},
		{"value-receiver", "func(d owner)Copied(){}", 0},
		{"nested-input", "func Inspect(items []*owner){}", 0},
		{"bodyless-input", "func Inspect(d *owner)", 0},
		{"private-callable-global", "var exposed=(*owner).private", 0},
		{"boxed-private-callable", "var exposed any=(*owner).private", 0},
		{"returned-private-callable", "func Expose()func(*owner){return (*owner).private}", 0},
		{"interface-input", "func Inspect(x any){x.(*owner).private()}", 0},
		{"interface-owner-provider", "func Inspect(x interface{Owner()*owner}){x.Owner().private()}", 0},
		{"generic-input", "func Inspect[T any](x T){any(x).(*owner).private()}", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, pkg := ps6136CellTestPackage(t, `package cells
type owner struct{}
func(d *owner)Run(){d.private()};func(d *owner)Query()int{return 1}
func(d *owner)private(){}
func New()*owner{return &owner{}}
`+test.extra)
			owner := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			entries := ps6140PublicOwnerEntries(pkg, owner, 65536)
			if len(entries) != test.want {
				t.Fatalf("entries=%d want=%d", len(entries), test.want)
			}
			for _, entry := range entries {
				if !entry.function.Object().Exported() || entry.function.Name() == "private" || !types.Identical(entry.function.Params[entry.parameter].Type(), types.NewPointer(owner)) {
					t.Fatal("entry is not an exact public owner input role")
				}
			}
			if ps6140PublicOwnerEntries(pkg, owner, 1) != nil || ps6140PublicOwnerEntries(nil, owner, 65536) != nil {
				t.Fatal("missing package or exhausted inventory admitted")
			}
		})
	}
}
