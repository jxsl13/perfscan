// Package compdb reads a clang compilation database (compile_commands.json) so
// perfscanxx can expand Go-style path patterns (./..., a directory) into the
// translation units to analyze — the C++ analog of `perfscan ./...`.
package compdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Name is the well-known compilation-database filename.
const Name = "compile_commands.json"

// entry is one compile_commands.json record; only directory+file are needed to
// enumerate translation units.
type entry struct {
	Directory string `json:"directory"`
	File      string `json:"file"`
}

// buildSubdirs are the conventional locations a build system drops the
// compilation database into, checked at each directory level (nearest wins) so
// `perfscanxx ./...` works from a project root without -p.
var buildSubdirs = []string{".", "build", "out", "cmake-build-debug", "cmake-build-release"}

// Find locates the compilation database. If buildDir is non-empty it must
// contain compile_commands.json. Otherwise, walking upward from start
// (default "."), each level is checked both directly and in the conventional
// build subdirectories (build/, out/, cmake-build-*) — the nearest one wins.
func Find(buildDir, start string) (string, error) {
	if buildDir != "" {
		p := filepath.Join(buildDir, Name)
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("no %s in -p %q", Name, buildDir)
		}
		return p, nil
	}
	if start == "" {
		start = "."
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		for _, sub := range buildSubdirs {
			p := filepath.Join(dir, sub, Name)
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s found (searched %q and build/, out/, cmake-build-* up from %q); pass -p <build-dir> or generate one with `cmake -DCMAKE_EXPORT_COMPILE_COMMANDS=ON`", Name, filepath.Base(start), start)
		}
		dir = parent
	}
}

// Load parses the database at path and returns the absolute translation-unit
// file paths it lists (deduplicated, sorted). A file recorded relative to its
// entry's directory is resolved against it.
func Load(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if len(entries) == 0 {
		return nil, errors.New(filepath.Base(path) + ": empty compilation database")
	}
	// The spec requires each entry's directory to be ABSOLUTE. When it is not
	// (a relative or empty directory, which real generators occasionally emit),
	// resolve the translation unit against the database's OWN location rather
	// than the process CWD, so `perfscanxx -p build` gives the same result no
	// matter which directory it is invoked from.
	base := filepath.Dir(path)
	seen := map[string]bool{}
	var out []string
	for _, e := range entries {
		f := e.File
		if f == "" {
			continue
		}
		if !filepath.IsAbs(f) {
			if filepath.IsAbs(e.Directory) {
				f = filepath.Join(e.Directory, f)
			} else {
				f = filepath.Join(base, e.Directory, f)
			}
		}
		f = filepath.Clean(f)
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	// Entries existed but none named a source file: a malformed database. Say so
	// clearly instead of returning an empty list that later reads as "no TUs
	// matched" and sends the user hunting in the wrong place.
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: %d entr%s but none names a source file (each needs a non-empty \"file\")",
			filepath.Base(path), len(entries), plural(len(entries)))
	}
	sort.Strings(out)
	return out, nil
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
