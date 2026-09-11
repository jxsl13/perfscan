package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6127(t *testing.T) {
	t.Parallel()
	contracts := []config.RecursiveMetadataIgnoreContract{
		{Name: "recursive records", Builder: "ps6127.newConfig", Matcher: "ps6127.ignoredBy", RootField: "root", IgnoreField: "ignoreParts", DirectoryName: ".records", AnyDepthIntent: true, NonEmbeddedMetadataOnly: true},
		{Name: "GoAI spectackle bundle", Builder: "ps6127owner.newConfig", Matcher: "ps6127owner.config.ignoredBy", RootField: "root", IgnoreField: "ignoreParts", DirectoryName: ".spectackle", AnyDepthIntent: true, NonEmbeddedMetadataOnly: true},
	}
	analyzer := &analysis.Analyzer{Name: "PS6127", Doc: "PS6127 test", Run: func(pass *analysis.Pass) (any, error) {
		return runPS6127WithContracts(pass, contracts)
	}}
	analysistest.Run(t, analysistest.TestData(), analyzer, "ps6127", "ps6127owner")
}
