package runner

import (
	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
	"testing"
)

func TestPS6127PermanentVocabulary(t *testing.T) {
	t.Parallel()
	check := &lint.Check{ID: "PS6127", NeedsConfig: true, Vocab: []string{"recursiveMetadataIgnoreContracts"}}
	valid := config.RecursiveMetadataIgnoreContract{Name: "reviewed records", Builder: "example.com/policy.newConfig", Matcher: "example.com/policy.ignoredBy", RootField: "root", IgnoreField: "ignoreParts", DirectoryName: ".records", AnyDepthIntent: true, NonEmbeddedMetadataOnly: true}
	invalid := valid
	invalid.NonEmbeddedMetadataOnly = false
	for _, tc := range []struct {
		name      string
		contracts []config.RecursiveMetadataIgnoreContract
		want      int
	}{{"empty", nil, 1}, {"valid", []config.RecursiveMetadataIgnoreContract{valid}, 0}, {"invalid", []config.RecursiveMetadataIgnoreContract{invalid}, 1}, {"ambiguous", []config.RecursiveMetadataIgnoreContract{valid, valid}, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := missingVocab(check, &config.Config{RecursiveMetadataIgnoreContracts: tc.contracts})
			if len(got) != tc.want {
				t.Errorf("missingVocab=%v want length%d", got, tc.want)
			}
		})
	}
}
