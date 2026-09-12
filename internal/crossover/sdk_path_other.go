//go:build !windows

package crossover

import "path/filepath"

func canonicalSDKPath(path string) (string, error) { return filepath.EvalSymlinks(path) }
