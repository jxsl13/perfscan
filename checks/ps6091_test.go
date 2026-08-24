package checks

import (
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6091MethodReceiverUnderlying(t *testing.T) {
	t.Parallel()
	interfaceType := types.NewInterfaceType(nil, nil).Complete()
	for _, test := range []struct {
		name               string
		underlying         types.Type
		wantMethodReceiver bool
		wantTilde          bool
	}{
		{name: "integer", underlying: types.Typ[types.Int], wantMethodReceiver: true, wantTilde: true},
		{name: "array", underlying: types.NewArray(types.Typ[types.Int], 1), wantMethodReceiver: true, wantTilde: true},
		{name: "channel", underlying: types.NewChan(types.SendRecv, types.Typ[types.Int]), wantMethodReceiver: true, wantTilde: true},
		{name: "map", underlying: types.NewMap(types.Typ[types.String], types.Typ[types.Int]), wantMethodReceiver: true, wantTilde: true},
		{name: "function", underlying: types.NewSignatureType(nil, nil, nil, nil, nil, false), wantMethodReceiver: true, wantTilde: true},
		{name: "slice", underlying: types.NewSlice(types.Typ[types.Byte]), wantMethodReceiver: true, wantTilde: true},
		{name: "struct", underlying: types.NewStruct(nil, nil), wantMethodReceiver: true, wantTilde: true},
		{name: "pointer", underlying: types.NewPointer(types.Typ[types.Int]), wantTilde: true},
		{name: "interface", underlying: interfaceType},
		{name: "unsafe pointer", underlying: types.Typ[types.UnsafePointer], wantTilde: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ps6091MethodReceiverUnderlying(test.underlying); got != test.wantMethodReceiver {
				t.Errorf("ps6091MethodReceiverUnderlying(%s) = %t, want %t", test.underlying, got, test.wantMethodReceiver)
			}
			if got := ps6091TildeUnderlying(test.underlying); got != test.wantTilde {
				t.Errorf("ps6091TildeUnderlying(%s) = %t, want %t", test.underlying, got, test.wantTilde)
			}
		})
	}
}

func TestPS6091WitnessPackage(t *testing.T) {
	t.Parallel()
	packageA := types.NewPackage("example.com/a", "a")
	packageB := types.NewPackage("example.com/b", "b")
	signature := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	methodA := types.NewFunc(token.NoPos, packageA, "sealedA", signature)
	methodB := types.NewFunc(token.NoPos, packageB, "sealedB", signature)
	samePackage := types.NewInterfaceType([]*types.Func{
		methodA,
		types.NewFunc(token.NoPos, packageA, "other", signature),
	}, nil).Complete()
	if got := ps6091WitnessPackage(samePackage); got == nil || got.Path() != packageA.Path() {
		t.Fatalf("ps6091WitnessPackage(same package) = %v, want %s", got, packageA.Path())
	}
	crossPackage := types.NewInterfaceType([]*types.Func{methodA, methodB}, nil).Complete()
	if got := ps6091WitnessPackage(crossPackage); got != nil {
		t.Fatalf("ps6091WitnessPackage(cross package) = %v, want nil", got)
	}
}

func TestPS6091(t *testing.T) {
	t.Parallel()
	contracts := []config.TopKOneContract{
		{Name: "resident TopKN", Function: "ps6091.DeviceBuffer.TopKN", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "value TopKN", Function: "ps6091.ValueBuffer.TopKN", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "host TopK", Function: "ps6091.TopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "generic TopK", Function: "ps6091.GenericTopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "generic method TopKN", Function: "ps6091.GenericBuffer.TopKN", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "generic any method TopKN", Function: "ps6091.GenericAnyBuffer.TopKN", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "named TopKN", Function: "ps6091.DeviceBuffer.NamedTopKN", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "alias TopKN", Function: "ps6091.DeviceBuffer.AliasTopKN", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "single TopK", Function: "ps6091.SingleTopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "unicode TopK", Function: "ps6091.TópK", KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "unicode method TopKN", Function: "ps6091.Búffer.TópKN", KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "collision function", Function: "ps6091kind.DeviceBuffer.TopKN", Kind: config.TopKOneContractFunction, KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "collision method", Function: "ps6091kind.DeviceBuffer.TopKN", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
	}
	analyzer := *PS6091.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6091WithContracts(pass, contracts)
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6091", "ps6091kind", "ps6091kind.DeviceBuffer")
}

func TestPS6091RejectsInvalidContracts(t *testing.T) {
	t.Parallel()
	for _, contract := range []config.TopKOneContract{
		{},
		{Function: "ps6091.TopK", KArgPosition: 0, IndicesResultPosition: 1},
		{Function: "ps6091.TopK", KArgPosition: 2, IndicesResultPosition: 0},
		{Function: "TopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: " ps6091.TopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "ps6091. TopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "ps6091..TopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com//ps6091.TopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/a:bad.TopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/a:bad.Buffer.TopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/a@bad.TopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/a%bad.TopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: `example.com/a\bad.TopK`, KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/café.TopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/CON.TopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/pkg~1.TopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/a:bad.Búffer.TópKN", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/a@bad.Búffer.TópKN", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/backend..TópKN", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/backend.Búffer.Top:KN", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/backend.Búffer.\u200bTopKN", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/project._", Kind: config.TopKOneContractFunction, KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/project._.TopKN", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/project.Buffer._", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/backend.DeviceBuffer.TopKN", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "gopkg.in/yaml.v3.Unmarshal", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "sort.Search", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/project.Búffer.TópKN", Kind: config.TopKOneContractFunction, KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "sort.Search", Kind: config.TopKOneContractKind("callable"), KArgPosition: 2, IndicesResultPosition: 1},
	} {
		if contract.Valid() {
			t.Fatalf("TopKOneContract.Valid(%+v) = true", contract)
		}
	}
	for _, contract := range []config.TopKOneContract{
		{Function: "sort.Search", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/project.TopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/project.Buffer.TopKN", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/a-b/c_d/v2.TopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/a-b/c_d/v2.Buffer.TopKN", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/c++/v2.TopK", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/project.TópK", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/project.Búffer.TópKN", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/dotted.pkg/backend.Búffer.TópKN", KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "gopkg.in/yaml.v3.Unmarshal", Kind: config.TopKOneContractFunction, KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "gopkg.in/yaml.v3.Decoder.Decode", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/backend.DeviceBuffer.TopKN", Kind: config.TopKOneContractFunction, KArgPosition: 2, IndicesResultPosition: 1},
		{Function: "example.com/backend.DeviceBuffer.TopKN", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
	} {
		if !contract.Valid() {
			t.Fatalf("TopKOneContract.Valid(%+v) = false", contract)
		}
	}
}

func TestPS6091SilentWithoutValidContracts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		contracts []config.TopKOneContract
	}{
		{name: "none"},
		{name: "argument out of bounds", contracts: []config.TopKOneContract{{
			Function: "ps6091silent.DeviceBuffer.TopKN", KArgPosition: 99, IndicesResultPosition: 1,
		}}},
		{name: "result out of bounds", contracts: []config.TopKOneContract{{
			Function: "ps6091silent.DeviceBuffer.TopKN", KArgPosition: 2, IndicesResultPosition: 99,
		}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			analyzer := *PS6091.Analyzer
			analyzer.Run = func(pass *analysis.Pass) (any, error) {
				return runPS6091WithContracts(pass, test.contracts)
			}
			analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6091silent")
		})
	}
}
