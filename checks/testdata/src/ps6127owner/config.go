package ps6127owner

import (
	"path/filepath"
	"strings"
)

type config struct {
	root        string
	ignoreParts []string
}

// These are the unchanged relevant statements from GoAI
// 91f49d01b931995487af0fcd8a80edeb46fb790d/internal/cichange/config.go.
func newConfig(root string, ignore []string) *config {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	c := &config{root: absRoot}
	for _, p := range ignore {
		ap := p
		if !filepath.IsAbs(ap) {
			ap = filepath.Join(absRoot, ap)
		}
		c.ignoreParts = append(c.ignoreParts, filepath.Clean(ap))
	}
	return c
}

func (c *config) ignoredBy(rel string) (string, bool) { // want `GoAI spectackle bundle: configured any-depth non-embedded metadata directory ".spectackle" is filtered through a root-anchored ignore entry`
	abs := filepath.Clean(filepath.Join(c.root, filepath.FromSlash(rel)))
	for _, ip := range c.ignoreParts {
		if abs == ip || strings.HasPrefix(abs, ip+string(filepath.Separator)) {
			return "-ignore " + ip, true
		}
	}
	return "", false
}
