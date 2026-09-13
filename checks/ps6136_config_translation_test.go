package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6136ConstructorConfigTranslation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body string
		want       bool
	}{
		{"literal", `cfg:=m.Config;c:=commonConfig{Max:cfg.Rows,Dim:cfg.Width};return common(c)`, true},
		{"copied-config", `cfg:=m.Config;original:=commonConfig{Max:cfg.Rows,Dim:cfg.Width};c:=original;return common(c)`, true},
		{"reordered-fields", `cfg:=m.Config;c:=commonConfig{Dim:cfg.Width,Max:cfg.Rows};return common(c)`, true},
		{"forwarded-config", `cfg:=m.Config;c:=commonConfig{Max:cfg.Rows,Dim:cfg.Width};return forward(c)`, true},
		{"wrong-field", `cfg:=m.Config;c:=commonConfig{Max:cfg.Rows,Dim:cfg.Other};return common(c)`, false},
		{"constant", `cfg:=m.Config;c:=commonConfig{Max:cfg.Rows,Dim:2048};return common(c)`, false},
		{"arithmetic", `cfg:=m.Config;c:=commonConfig{Max:cfg.Rows,Dim:cfg.Width+1};return common(c)`, false},
		{"foreign-model", `cfg:=other.Config;c:=commonConfig{Max:cfg.Rows,Dim:cfg.Width};return common(c)`, false},
		{"overwritten-field", `cfg:=m.Config;c:=commonConfig{Max:cfg.Rows,Dim:cfg.Width};c.Dim=cfg.Other;return common(c)`, false},
		{"conditional-field", `cfg:=m.Config;c:=commonConfig{Max:cfg.Rows};if b{c.Dim=cfg.Width};return common(c)`, false},
		{"escaped-config", `cfg:=m.Config;c:=commonConfig{Max:cfg.Rows,Dim:cfg.Width};escape(&c);return common(c)`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pkg := ps6125TestSSA(t, `package extent
type sourceConfig struct{Rows,Width,Other int};type model struct{Config sourceConfig}
type commonConfig struct{Max,Dim int};type owner struct{rows,width int}
func common(c commonConfig)*owner{return &owner{rows:c.Max,width:c.Dim}}
func forward(c commonConfig)*owner{return common(c)};func escape(*commonConfig)
func build(m,other *model,b bool)*owner{`+test.body+`}`)
			function := pkg.Func("build")
			context := ps6125NewSSAContext(function, nil, nil, 4096)
			owner := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			model := pkg.Pkg.Scope().Lookup("model").Type().(*types.Named)
			selected := &ps6136Selection{context: context, ownerType: owner, modelType: model, model: context.reference(function.Params[0]), maximumRows: ps6136FieldVar(owner, "rows"), width: ps6136FieldVar(owner, "width"), modelConfig: ps6136FieldVar(model, "Config")}
			selected.configRows = ps6136FieldVar(selected.modelConfig.Type(), "Rows")
			selected.configWidth = ps6136FieldVar(selected.modelConfig.Type(), "Width")
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					if returned, ok := instruction.(*ssa.Return); ok {
						selected.owner = ps6136OwnerRoot(context, returned.Results[0], owner)
					}
				}
			}
			if selected.owner.value == nil {
				t.Fatal("actual fresh common constructor owner prerequisite missing")
			}
			if got := selected.initializationValues(4096); got != test.want {
				t.Fatalf("translated config provenance=%v want=%v", got, test.want)
			}
			if selected.initializationValues(0) {
				t.Fatal("exhausted budget admitted")
			}
		})
	}
}
