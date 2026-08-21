package ps5109comment

import "path"

func join(a, b, c string) string {
	return path.Join(( /* preserve prefix rationale */ path.Join(a, b)), c) // want "path.Join cleans and materializes 1 completed prefix layer"
}
