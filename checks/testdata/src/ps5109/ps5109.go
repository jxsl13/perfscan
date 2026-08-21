package ps5109

import (
	"path"
	"path/filepath"
)

func left(a, b, c string) string {
	return path.Join(path.Join(a, b), c) // want "path.Join cleans and materializes 1 completed prefix layer"
}

func deep(a, b, c, d string) string {
	return path.Join(path.Join(path.Join(a, b), c), d) // want "path.Join cleans and materializes 2 completed prefix layer"
}

func parenthesized(a, b, c string) string {
	return path.Join((path.Join((path.Join(a, b)), c))) // want "path.Join cleans and materializes 2 completed prefix layer"
}

func branched(a, b, c, d string) string {
	return path.Join(path.Join(a, b), path.Join(c, d)) // want "path.Join cleans and materializes 1 completed prefix layer"
}

func emptyValues(a string) string {
	return path.Join(path.Join("", a, ""), "") // want "path.Join cleans and materializes 1 completed prefix layer"
}

func soleEmpty() string {
	return path.Join(path.Join()) // want "path.Join cleans and materializes 1 completed prefix layer"
}

func soleSpread(parts []string) string {
	return path.Join(path.Join(parts...)) // want "path.Join cleans and materializes 1 completed prefix layer"
}

func deepSpread(parts []string) string {
	return path.Join(path.Join(path.Join(parts...))) // want "path.Join cleans and materializes 2 completed prefix layer"
}

// --- negatives ---

func right(a, b, c string) string {
	return path.Join(a, path.Join(b, c))
}

func middle(a, b, c, d string) string {
	return path.Join(a, path.Join(b, c), d)
}

func platform(a, b, c string) string {
	return filepath.Join(filepath.Join(a, b), c)
}

func emptyNested(a string) string {
	return path.Join(path.Join(), a)
}

func spreadNested(parts []string, tail string) string {
	return path.Join(path.Join(parts...), tail)
}

func functionOuter(a, b, c string) string {
	join := path.Join
	return join(path.Join(a, b), c)
}

func functionInner(a, b, c string) string {
	join := path.Join
	return path.Join(join(a, b), c)
}

func single(a, b string) string {
	return path.Join(a, b)
}

func Join(parts ...string) string { return "" }

func user(a, b, c string) string {
	return Join(Join(a, b), c)
}
