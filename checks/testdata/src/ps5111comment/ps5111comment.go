package ps5111comment

import "path"

func retained(name string) string {
	return path.Clean( /* preserve why this normalization was expected */ path.Dir(name)) // want `path.Clean rescans the canonical nonempty result of path.Dir through 1 redundant Clean layer`
}
