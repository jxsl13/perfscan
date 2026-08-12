package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTidyConfig materializes the generated custom-check config (the plumbing
// PX2101–PX2106 depend on: clang-tidy reads the query definitions from this
// file). Verify the file is created with a .clang-tidy suffix (so clang-tidy
// accepts it), holds the exact content, and that cleanup removes it.
func TestWriteTidyConfig(t *testing.T) {
	const content = "Checks: '-*,custom-x'\nCustomChecks:\n  - Name: x\n    Query: |\n      match cxxCatchStmt().bind(\"c\")\n"
	path, cleanup, err := writeTidyConfig(content)
	if err != nil {
		t.Fatalf("writeTidyConfig error: %v", err)
	}

	// clang-tidy keys config parsing off the .clang-tidy name.
	if filepath.Ext(path) != ".clang-tidy" && !strings.HasSuffix(path, ".clang-tidy") {
		t.Errorf("config path %q should end in .clang-tidy", path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written config: %v", err)
	}
	if string(got) != content {
		t.Errorf("written config = %q, want %q", got, content)
	}

	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("cleanup did not remove the temp config %q (stat err: %v)", path, err)
	}
	cleanup() // idempotent: a second call must not panic
}
