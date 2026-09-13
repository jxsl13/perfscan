package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6140ConstructorStorage(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, before, host, slot, after, wrapper string
		want                                     bool
	}{
		{name: "genuine", want: true},
		{name: "constant-dead-read", after: "if false{read(d)}", want: true},
		{name: "direct-read", after: "sink=d.scratch"},
		{name: "source-helper-read", after: "read(d)"},
		{name: "invoked-callback-read", after: "func(){read(d)}()"},
		{name: "returned-callback-read", after: "reader(d)()"},
		{name: "before-allocation-read", before: "read(d)"},
		{name: "slot-global-alias", slot: "sink=s"},
		{name: "slot-cross-field-alias", slot: "d.other=s"},
		{name: "slot-helper-alias", slot: "keep(s)"},
		{name: "host-global-alias", host: "hostSink=data"},
		{name: "host-observation", host: "count=len(data)"},
		{name: "whole-owner-copy", after: "copy:=*d;sink=copy.scratch"},
		{name: "field-address-alias", after: "fieldSink=&d.scratch"},
		{name: "wrapper-read", wrapper: "read(d)"},
		{name: "wrapper-field-alias", wrapper: "fieldSink=&d.scratch"},
		{name: "unknown-owner-read", after: "read(foreign)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, pkg := ps6136CellTestPackage(t, `package cells
type slot struct{b []float32};type owner struct{scratch,other *slot}
var sink *slot;var hostSink []float32;var fieldSink **slot;var count int;var foreign *owner
func upload(data []float32)*slot{return &slot{b:data}}
func read(d *owner){sink=d.scratch};func keep(s *slot){sink=s}
func reader(d *owner)func(){return func(){read(d)}}
func build(n int)*owner{d:=&owner{};`+test.before+`;data:=make([]float32,n);`+test.host+`;s:=upload(data);`+test.slot+`;d.scratch=s;`+test.after+`;return d}
func New(n int)*owner{d:=build(n);`+test.wrapper+`;return d}
`)
			ownerType := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			workspace := ps6136FieldVar(ownerType, "scratch")
			publication := ps6125NewSSAContext(pkg.Func("New"), nil, nil, 16384)
			var context *ps6125SSAContext
			for _, call := range ps6125ContextCalls(publication) {
				if call.Call.StaticCallee() == pkg.Func("build") {
					context = ps6136Call(publication, call)
				}
			}
			if context == nil {
				t.Fatal("actual constructor prerequisite missing")
			}
			owner := ps6140TestConstructorOwner(t, context, ownerType)
			proof := &ps6136ConstructorProof{}
			paths := ps6125AccessPaths{flow: context.flow}
			for _, block := range context.flow.function.Blocks {
				for _, instruction := range block.Instrs {
					store, ok := instruction.(*ssa.Store)
					if !ok {
						continue
					}
					path := paths.resolve(store.Addr)
					if !path.known || len(path.access.fields) != 1 || path.access.fields[0] != workspace {
						continue
					}
					call, ok := store.Val.(*ssa.Call)
					if !ok || proof.allocation != nil || ps6136AccessOwnerRoot(context, path, ownerType) != owner {
						t.Fatal("exact single selected allocation/store prerequisite missing")
					}
					proof.allocation, proof.factory = store, ps6136Call(context, call)
				}
			}
			if proof.allocation == nil || proof.factory == nil || proof.factory.flow.function != pkg.Func("upload") {
				t.Fatal("actual source factory/store prerequisite missing")
			}
			// This deliberately exercises only the constructor-use component;
			// native ownership and error receipts are not supplied by upload.
			if got := ps6140ConstructorStorage(publication, owner, ownerType, workspace, proof, 65536); got != test.want {
				ps6140ConstructorStorageCheck(publication, owner, ownerType, workspace, proof, 65536, func(stage string) { t.Log(stage) })
				t.Fatalf("constructor storage closure=%v want=%v", got, test.want)
			}
			if ps6140ConstructorStorage(publication, owner, ownerType, workspace, proof, 0) || ps6140ConstructorStorage(publication, owner, ownerType, workspace, nil, 65536) || ps6140ConstructorStorage(proof.factory, owner, ownerType, workspace, proof, 65536) {
				t.Fatal("exhausted/missing/unrelated constructor receipt admitted")
			}
		})
	}
}
