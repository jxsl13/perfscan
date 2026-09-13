package checks

import (
	"go/token"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6136InitializationInvocationOrdering(t *testing.T) {
	t.Parallel()
	pkg := ps6125TestSSA(t, `package extent
func leaf(int){}
func initialize(p *int){*p=1}
func conditional(p *int,b bool){if b {*p=1}}
func read(p *int){leaf(*p)}
func callback(fn func(*int),p *int){fn(p)}
func early(){p:=new(int);initialize(p);read(p)}
func late(){p:=new(int);read(p);initialize(p)}
func guarded(b bool){p:=new(int);conditional(p,b);read(p)}
func callbackEarly(){p:=new(int);callback(initialize,p);read(p)}
func callbackLate(){p:=new(int);read(p);callback(initialize,p)}
func aliasLate(){p:=new(int);q:=p;read(q);initialize(p)}
`)
	for _, name := range []string{"early", "late", "guarded", "callbackEarly", "callbackLate", "aliasLate"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := ps6125NewSSAContext(pkg.Func(name), nil, nil, 128)
			var store *ssa.Store
			var load *ssa.UnOp
			var storeContext, loadContext *ps6125SSAContext
			pending := []*ps6125SSAContext{root}
			for len(pending) > 0 {
				context := pending[len(pending)-1]
				pending = pending[:len(pending)-1]
				for _, block := range context.flow.function.Blocks {
					if !context.flow.blocks[block] {
						continue
					}
					for _, instruction := range block.Instrs {
						switch instruction := instruction.(type) {
						case *ssa.Store:
							store, storeContext = instruction, context
						case *ssa.UnOp:
							if instruction.Op == token.MUL {
								load, loadContext = instruction, context
							}
						case *ssa.Call:
							if child := ps6136Call(context, instruction); child != nil {
								pending = append(pending, child)
							}
						}
					}
				}
			}
			if store == nil || load == nil {
				t.Fatal("typed store/load boundary missing")
			}
			wanted := name == "early" || name == "callbackEarly"
			if got := ps6136ContextInstructionDominates(storeContext, store, loadContext, load, 128); got != wanted {
				t.Fatalf("ordered=%v want %v", got, wanted)
			}
		})
	}
}
