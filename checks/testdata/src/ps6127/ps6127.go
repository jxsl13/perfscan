package ps6127

import (
	"path/filepath"
	"strings"
)

type config struct {
	root        string
	ignoreParts []string
}

func newConfig(root string, ignore []string) *config {
	c := &config{root: root}
	for _, path := range ignore {
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		c.ignoreParts = append(c.ignoreParts, filepath.Clean(path))
	}
	return c
}

func ignoredBy(c *config, rel string) bool { // want `recursive records: configured any-depth non-embedded metadata directory ".records" is filtered through a root-anchored ignore entry`
	abs := filepath.Clean(filepath.Join(c.root, filepath.FromSlash(rel)))
	for _, ignored := range c.ignoreParts {
		if abs == ignored || strings.HasPrefix(abs, ignored+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// Recursive component matching is already consistent with any-depth intent.
func recursiveIgnored(c *config, rel string) bool {
	for _, component := range strings.Split(filepath.ToSlash(rel), "/") {
		if component == ".records" {
			return true
		}
	}
	return false
}

// Similar spelling without a typed descendant test must stay silent.
func equalityOnly(c *config, rel string) bool {
	abs := filepath.Clean(filepath.Join(c.root, rel))
	for _, ignored := range c.ignoreParts {
		if abs == ignored {
			return true
		}
	}
	return false
}
