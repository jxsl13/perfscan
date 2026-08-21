package ps5114comment

import "path/filepath"

func retained(name string) string {
	return filepath.FromSlash( /* preserve platform-boundary rationale */ filepath.Clean(name)) // want `filepath.FromSlash rescans the already-native result of filepath.Clean through 1 redundant FromSlash layer`
}
