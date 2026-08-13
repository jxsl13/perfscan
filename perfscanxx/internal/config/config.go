// Package config loads an optional .perfscanxx.yml that supplies DEFAULTS for a
// project (checks selector, level, exclude patterns). Explicit command-line
// flags always win over the file, so a config never surprises a one-off run.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Names are the auto-discovered filenames, checked in order in the working
// directory (not walked up, so the active config is always predictable).
var Names = []string{".perfscanxx.yml", ".perfscanxx.yaml"}

// File is the parsed config. Level and Checks are pointers so an omitted key is
// distinguishable from a zero value ("" / 0) and simply leaves the flag default
// (or an explicit flag) in place. Exclude is nil when omitted.
type File struct {
	Level   *int     `yaml:"level"`
	Checks  *string  `yaml:"checks"`
	Exclude []string `yaml:"exclude"`
	// Tidy is the clang-tidy binary path (the -tidy flag). ExtraArgs are passed
	// to the compiler via clang-tidy (the repeatable -extra-arg), e.g. the
	// macOS `-isysroot <sdk>` pair — the flags most painful to repeat in CI.
	Tidy      *string  `yaml:"tidy"`
	ExtraArgs []string `yaml:"extra-args"`
}

// Discover returns the path of the first config filename that exists in dir,
// and whether one was found.
func Discover(dir string) (string, bool) {
	for _, name := range Names {
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, true
		}
	}
	return "", false
}

// Load parses the config at path. It rejects unknown keys (a typo'd option
// should fail loudly, not be silently ignored) and validates level ∈ 1..3.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var f File
	if err := dec.Decode(&f); err != nil {
		// An empty (or all-comment) file is a valid no-op config, not an error.
		if errors.Is(err, io.EOF) {
			return &File{}, nil
		}
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if f.Level != nil && (*f.Level < 1 || *f.Level > 3) {
		return nil, fmt.Errorf("%s: level %d out of range (want 1, 2, or 3)", filepath.Base(path), *f.Level)
	}
	return &f, nil
}
