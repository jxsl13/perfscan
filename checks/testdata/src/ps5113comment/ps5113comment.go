package ps5113comment

import "path/filepath"

func retained(path string) string {
	return filepath.ToSlash(filepath.FromSlash( /* preserve platform-boundary rationale */ path)) // want `filepath.ToSlash absorbs 1 nested ToSlash/FromSlash layer`
}
