package ps5114comment

import "path/filepath"

func retained(name string) string {
	return filepath.FromSlash( /* preserve platform-boundary rationale */ filepath.Base(name)) // want `filepath.FromSlash rescans the already-native result of filepath.Base through 1 redundant FromSlash layer`
}
