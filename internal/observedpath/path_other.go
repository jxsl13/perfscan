//go:build !windows

package observedpath

import "path/filepath"

// Canonical observes the physical path. Resolution errors never fall back to
// an uncanonicalized spelling; non-Windows behavior remains EvalSymlinks.
func Canonical(path string) (string, error) { return filepath.EvalSymlinks(path) }
