package checks

import "testing"

func TestPS6084GroupedDenseCoverageRejectsGapsAndDuplicates(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		fill string
	}{
		{"missing element", `for j := range 3 { c0[j] = float32(p0[j]); c1[j] = float32(p1[j]) }`},
		{"duplicate element", `for j := range 4 { c0[j] = float32(p0[j]); c0[j] = float32(p0[j]); c1[j] = float32(p1[j]) }`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			source := `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)
func run(p0,p1 []byte,trips int){
	for block:=0;block<trips;block++{
		var c0,c1 [4]float32
		` + test.fill + `
		_,_=leaf(&p0[0],&p1[0],&c0[0],&c1[0])
	}
}`
			if got := ps6084GroupedDiagnostics(t, source); len(got) != 0 {
				t.Fatalf("diagnostics=%d want=0: %v", len(got), got)
			}
		})
	}
}
