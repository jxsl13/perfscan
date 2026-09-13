package checks

import (
	"github.com/jxsl13/perfscan/config"
	"go/ast"
	"golang.org/x/tools/go/analysis"
)

// No rewrite or numeric-equivalence claim is made by this diagnostic path.
func runPS6141WithContracts(pass *analysis.Pass, configured []config.SingleUseQuantizationContract) (any, error) {
	claims := map[string]int{}
	for i := range configured {
		c := &configured[i]
		claims[c.Quantizer+"\x00"+c.Consumer]++
	}
	for i := range configured {
		c := &configured[i]
		if !c.Valid() || claims[c.Quantizer+"\x00"+c.Consumer] != 1 {
			continue
		}
		for _, file := range pass.Files {
			for _, decl := range file.Decls {
				owner, ok := decl.(*ast.FuncDecl)
				if !ok || owner.Body == nil {
					continue
				}
				for _, candidate := range ps6141Candidates(pass, owner, c) {
					pass.Report(analysis.Diagnostic{Pos: candidate.quantizer.Pos(), End: candidate.quantizer.End(), Message: "fresh activation quantization/packing immediately feeds one source-proved dot boundary without visible reuse or fusion; conversion and scratch traffic may not amortize—benchmark the complete conversion-and-consumer boundary for this shape (PS6141 advisory; no automatic fix or numerical-equivalence claim)", Related: []analysis.RelatedInformation{{Pos: candidate.consumer.Pos(), End: candidate.consumer.End(), Message: "sole immediate consumer of this fresh packed activation"}}})
				}
			}
		}
	}
	return nil, nil
}
