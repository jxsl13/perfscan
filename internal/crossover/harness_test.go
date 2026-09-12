package crossover

import (
	"bytes"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
)

func TestGenerateDeterministicTypedShape(t *testing.T) {
	t.Parallel()
	c := &config.DispatchCrossoverContract{SerialPolicy: "example.leaf", ParallelPolicy: "example.pool", WorkerRunner: "example.pool", SerialOperation: "example.forcedSerial", ParallelOperation: "example.forcedParallel", DispatchFunction: "example.operation", OperationInputFactory: "example.inputs", DiagnosticFunction: "example.TestDiagnostic"}
	m := &HarnessModel{Package: "example", PackagePath: "example", Signature: "func(src []float32) []float32", SerialBody: "{ dst:=make([]float32,len(src)); if true { leaf(dst,src) } else { pool(dst,src) }; return dst }", ParallelBody: "{ dst:=make([]float32,len(src)); if false { leaf(dst,src) } else { pool(dst,src) }; return dst }", ArgumentCount: 1, ResultCount: 1, OutputExtent: "len(_r0)", Dtype: "float32"}
	m.InputData = "_arg0"
	m.OutputData = "_r0"
	m.OutputGuard = "true"
	first, err := Generate(m, c)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Generate(m, c)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("regeneration changed identical instrumentation")
	}
	if _, err := parser.ParseFile(token.NewFileSet(), HarnessFile, first, parser.AllErrors); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"forced-operation", "production", "Float32bits", "Measure(_fixed", "len(_r0) != _n", "diagnostic instrumentation"} {
		if !strings.Contains(string(first), required) {
			t.Fatalf("missing integrity boundary %q", required)
		}
	}
	c.SerialOperation = c.DispatchFunction
	if _, err := Generate(m, c); err == nil {
		t.Fatal("accepted overwriting production operation")
	}
	if _, err := Generate(m, nil); err == nil {
		t.Fatal("accepted missing contract")
	}
}
