package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscan/checks"
)

// docsDir is docs/checks relative to this package (perfscan/gendocs -> ../docs/checks).
const docsDir = "../docs/checks"

// TestDocsInSyncWithRegistry enforces the gendocs promise in package doc ("CI
// fails if the rendered docs drift from the registry"): the committed
// docs/checks/<ID>.md pages and README.md index must be EXACTLY what gendocs
// renders from the current registry, with no missing and no orphan pages. A
// check whose Doc changed without `make docs` fails here.
func TestDocsInSyncWithRegistry(t *testing.T) {
	for _, c := range checks.All() {
		path := filepath.Join(docsDir, c.ID+".md")
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: missing docs page %s — run `make docs`", c.ID, path)
			continue
		}
		if string(got) != renderCheckPage(c) {
			t.Errorf("%s: docs page is stale (differs from gendocs output) — run `make docs`", c.ID)
		}
	}

	if got, err := os.ReadFile(filepath.Join(docsDir, "README.md")); err != nil {
		t.Fatalf("missing docs index — run `make docs`: %v", err)
	} else if string(got) != renderIndex() {
		t.Error("docs/checks/README.md is stale (differs from gendocs output) — run `make docs`")
	}

	// No orphan pages (a .md left behind for a removed/renamed check).
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{"README.md": true}
	for _, c := range checks.All() {
		known[c.ID+".md"] = true
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if !known[e.Name()] {
			t.Errorf("orphan docs page %s (no such check) — remove it or run `make docs`", e.Name())
		}
	}
}
