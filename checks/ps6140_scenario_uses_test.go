package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6140ScenarioWorkspaceUses(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, invocation, extra string
		want                    bool
	}{
		{"genuine", "d.scratch.b,d.other.b", "", true},
		{"source-unused-extra", "d.scratch.b,d.other.b", "ignore(d.scratch.b)", true},
		{"direct-read", "d.scratch.b,d.other.b", "d.scratch.b.touch()", false},
		{"opaque-read", "d.scratch.b,d.other.b", "opaque(d.scratch.b)", false},
		{"source-read", "d.scratch.b,d.other.b", "read(d.scratch.b)", false},
		{"duplicate-used-argument", "d.scratch.b,d.scratch.b", "", false},
		{"retained", "d.scratch.b,d.other.b", "saved=d.scratch.b", false},
		{"captured-owner", "d.scratch.b,d.other.b", "thunk=func(){d.scratch.b.touch()}", false},
		{"called-owner-unused", "d.scratch.b,d.other.b", "func(){ignore(d.scratch.b)}()", true},
		{"called-owner-read", "d.scratch.b,d.other.b", "func(){read(d.scratch.b)}()", false},
		{"forwarded-owner-unused", "d.scratch.b,d.other.b", "invoke(func(){ignore(d.scratch.b)})", true},
		{"forwarded-owner-read", "d.scratch.b,d.other.b", "invoke(func(){read(d.scratch.b)})", false},
		{"opaque-callback", "d.scratch.b,d.other.b", "opaqueCallback(func(){read(d.scratch.b)})", false},
		{"returned-callback", "d.scratch.b,d.other.b", "thunk=produce(d)", false},
		{"returned-private-callback", "d.scratch.b,d.other.b", "produceUnused(d)()", true},
		{"nested-owner-unused", "d.scratch.b,d.other.b", "fn:=func(){ignore(d.scratch.b)};func(){fn()}()", true},
		{"forwarded-nested-owner-unused", "d.scratch.b,d.other.b", "fn:=func(){ignore(d.scratch.b)};invoke(func(){fn()})", true},
		{"nested-owner-read", "d.scratch.b,d.other.b", "fn:=func(){read(d.scratch.b)};func(){fn()}()", false},
		{"nested-owner-retained", "d.scratch.b,d.other.b", "fn:=func(){read(d.scratch.b)};thunk=func(){fn()}", false},
		{"nested-callback-retained", "d.scratch.b,d.other.b", "fn:=func(){read(d.scratch.b)};func(){thunk=fn}()", false},
		{"deferred-owner", "d.scratch.b,d.other.b", "defer func(){read(d.scratch.b)}()", false},
		{"deferred-scalar-restoration", "d.scratch.b,d.other.b", "previous:=d.opts.enabled;defer func(){d.opts.enabled=previous}()", true},
		{"deferred-callable-restoration", "d.scratch.b,d.other.b", "previous:=d.opts.callback;defer func(){d.opts.callback=previous}()", true},
		{"deferred-restoration-plus-read", "d.scratch.b,d.other.b", "previous:=d.opts.enabled;defer func(){d.opts.enabled=previous;read(d.scratch.b)}()", false},
		{"deferred-callable-execution", "d.scratch.b,d.other.b", "previous:=d.opts.callback;defer func(){d.opts.callback=previous;previous()}()", false},
		{"deferred-slot-replacement", "d.scratch.b,d.other.b", "previous:=d.scratch;defer func(){d.scratch=previous}()", false},
		{"asynchronous-owner", "d.scratch.b,d.other.b", "go func(){read(d.scratch.b)}()", false},
		{"deferred-owner-method", "d.scratch.b,d.other.b", "defer d.observe()", false},
		{"asynchronous-owner-method", "d.scratch.b,d.other.b", "go d.observe()", false},
		{"deferred-owner-helper", "d.scratch.b,d.other.b", "defer observe(d)", false},
		{"asynchronous-owner-helper", "d.scratch.b,d.other.b", "go observe(d)", false},
		{"deferred-nested-owner", "d.scratch.b,d.other.b", "fn:=func(){read(d.scratch.b)};func(){defer fn()}()", false},
		{"captured-buffer", "d.scratch.b,d.other.b", "buf:=d.scratch.b;thunk=func(){buf.touch()}", false},
		{"deferred", "d.scratch.b,d.other.b", "defer read(d.scratch.b)", false},
		{"asynchronous", "d.scratch.b,d.other.b", "go read(d.scratch.b)", false},
		{"other-may-alias", "d.scratch.b,d.other.b", "read(other.scratch.b)", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pass, pkg := ps6136CellTestPackage(t, `package cells
type buffer interface{touch()};type slot struct{b buffer}
type api interface{apply(buffer,buffer)};type linear struct{}
func(linear)apply(ignored buffer,used buffer){used.touch()}
type settings struct{enabled bool;callback func()}
type block struct{projection api};type owner struct{blocks []block;moe bool;scratch,other *slot;opts settings}
type storage struct{};func(*storage)touch(){};func freshBuffer()buffer{return &storage{}}
func dense(n int)*owner{d:=&owner{scratch:&slot{b:freshBuffer()},other:&slot{b:freshBuffer()}};for i:=0;i<n;i++{b:=block{projection:linear{}};d.blocks=append(d.blocks,b)};return d}
func ignore(buffer){};func read(b buffer){b.touch()}
func invoke(fn func()){fn()};func produce(d *owner)func(){return func(){read(d.scratch.b)}}
func produceUnused(d *owner)func(){return func(){ignore(d.scratch.b)}}
func(d *owner)observe(){read(d.scratch.b)};func observe(d *owner){read(d.scratch.b)}
var saved buffer;var thunk func()
func(d *owner)Run(other *owner,opaque func(buffer),opaqueCallback func(func())){for _,b:=range d.blocks{b.projection.apply(`+test.invocation+`)};`+test.extra+`}
`)
			collection, flags, owner := ps6140TestScenarioProofs(t, pass, pkg, "dense")
			object, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, "Run")
			method := pkg.Prog.FuncValue(object.(*types.Func))
			workspace := ps6136FieldVar(owner, "scratch")
			slot := ps6136FieldVar(workspace.Type(), "b")
			// All controls retain the real projection and its unused formal;
			// the additional observation, not a missing positive, must reject.
			prerequisite := ps6140NewConstructorScenario(collection, flags, owner, method, 65536)
			leaves := 0
			if !prerequisite.walk(prerequisite.root, func(context *ps6125SSAContext, call *ssa.Call) bool {
				block := pkg.Pkg.Scope().Lookup("block").Type().(*types.Named)
				for field, concrete := range collection.projectors {
					if ps6140UnusedProjectionFormal(context, call, collection.appended, concrete, prerequisite.receiver, owner, block, collection.blocks, field, workspace, slot, 0, 256) != nil {
						leaves++
						return true
					}
				}
				return false
			}) || leaves != 1 {
				t.Fatal("actual unused projection prerequisite missing")
			}
			scenario := ps6140NewConstructorScenario(collection, flags, owner, method, 65536)
			proof := ps6140ScenarioWorkspaceUses(scenario, workspace, slot, 0, 65536)
			if (proof != nil) != test.want {
				t.Fatalf("all observations closed=%v want=%v", proof != nil, test.want)
			}
		})
	}
}
