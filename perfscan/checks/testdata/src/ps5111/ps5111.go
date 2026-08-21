package ps5111

import (
	"path"
	"path/filepath"
)

const fixed = "fixed"
const folded = "fi" + "xed"

func pathDir(name string) string {
	return path.Clean(path.Dir(name)) // want `path.Clean rescans the canonical nonempty result of path.Dir through 1 redundant Clean layer`
}

func pathBase(name string) string {
	return path.Clean((path.Base(name))) // want `path.Clean rescans the canonical nonempty result of path.Base through 1 redundant Clean layer`
}

func filepathDir(name string) string {
	return filepath.Clean(filepath.Dir(name)) // want `path/filepath.Clean rescans the canonical nonempty result of path/filepath.Dir through 1 redundant Clean layer`
}

func filepathBase(name string) string {
	return filepath.Clean(filepath.Base(name)) // want `path/filepath.Clean rescans the canonical nonempty result of path/filepath.Base through 1 redundant Clean layer`
}

func deep(name string) string {
	return path.Clean(path.Clean(path.Clean(path.Dir(name)))) // want `path.Clean rescans the canonical nonempty result of path.Dir through 3 redundant Clean layer`
}

func knownJoin(root string) string {
	return path.Clean(path.Join(root, fixed)) // want `path.Clean rescans the canonical nonempty result of path.Join through 1 redundant Clean layer`
}

func foldedJoin(root string) string {
	return path.Clean(path.Join(root, folded)) // want `path.Clean rescans the canonical nonempty result of path.Join through 1 redundant Clean layer`
}

func knownFilepathJoin(tail string) string {
	return filepath.Clean(filepath.Join("root", tail)) // want `path/filepath.Clean rescans the canonical nonempty result of path/filepath.Join through 1 redundant Clean layer`
}

// Join can return "" when every argument is empty, but Clean("") is ".".
func dynamicJoin(root string) string { return path.Clean(path.Join(root)) }
func emptyJoin() string              { return path.Clean(path.Join()) }
func constantEmptyJoin() string      { return path.Clean(path.Join("", "")) }

// Cross-package path syntax is not interchangeable on every target OS.
func crossPackage(name string) string { return path.Clean(filepath.Dir(name)) }

func functionValue(name string) string {
	clean := path.Clean
	return clean(path.Dir(name))
}

func producerValue(name string) string {
	dir := path.Dir
	return path.Clean(dir(name))
}

func Clean(name string) string { return name }
func Dir(name string) string   { return name }

func user(name string) string { return Clean(Dir(name)) }
