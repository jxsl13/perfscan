package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6140BlockNilInterface(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body string
		want       bool
	}{
		{"omitted", "b:=block{};return b", true},
		{"explicit", "b:=block{projection:nil};return b", true},
		{"whole-copy", "b:=block{};c:=b;return c", true},
		{"typed-nil", "var typed *linear;b:=block{projection:typed};return b", false},
		{"non-nil", "b:=block{projection:&linear{}};return b", false},
		{"unknown", "b:=block{projection:p};return b", false},
		{"conditional", "b:=block{};if choose{b.projection=p};return b", false},
		{"opaque-cell", "b:=block{};opaque(&b);return b", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pkg := ps6125TestSSA(t, `package extent
type api interface{use()};type linear struct{};func(*linear)use(){}
type block struct{projection api};func opaque(*block)
func build(p api,choose bool)block{`+test.body+`}`)
			block := pkg.Pkg.Scope().Lookup("block").Type().(*types.Named)
			context := ps6125NewSSAContext(pkg.Func("build"), nil, nil, 1024)
			found := false
			for _, bb := range context.flow.function.Blocks {
				for _, instruction := range bb.Instrs {
					ret, ok := instruction.(*ssa.Return)
					if !ok || len(ret.Results) != 1 {
						continue
					}
					found = true
					value := ps6125SSAReference{context: context, value: ret.Results[0]}
					if got := ps6140BlockFieldNil(value, ps6136FieldVar(block, "projection"), 128); got != test.want {
						t.Fatalf("nil interface=%v want=%v", got, test.want)
					}
					if ps6140BlockFieldNil(value, ps6136FieldVar(block, "projection"), 0) {
						t.Fatal("exhausted nil proof accepted")
					}
				}
			}
			if !found {
				t.Fatal("actual source struct return missing")
			}
		})
	}
}
