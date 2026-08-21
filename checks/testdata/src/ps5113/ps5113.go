package ps5113

import "path/filepath"

func toFrom(path string) string {
	return filepath.ToSlash(filepath.FromSlash(path)) // want `filepath.ToSlash absorbs 1 nested ToSlash/FromSlash layer`
}

func fromTo(path string) string {
	return filepath.FromSlash(filepath.ToSlash(path)) // want `filepath.FromSlash absorbs 1 nested ToSlash/FromSlash layer`
}

func repeatedTo(path string) string {
	return filepath.ToSlash(filepath.ToSlash(path)) // want `filepath.ToSlash absorbs 1 nested ToSlash/FromSlash layer`
}

func repeatedFrom(path string) string {
	return filepath.FromSlash(filepath.FromSlash(path)) // want `filepath.FromSlash absorbs 1 nested ToSlash/FromSlash layer`
}

func deep(path string) string {
	return filepath.ToSlash(filepath.FromSlash(filepath.ToSlash(filepath.FromSlash(path)))) // want `filepath.ToSlash absorbs 3 nested ToSlash/FromSlash layer`
}

func parenthesized(path string) string {
	return filepath.FromSlash((filepath.ToSlash((path)))) // want `filepath.FromSlash absorbs 1 nested ToSlash/FromSlash layer`
}

func nativeProducer(path string) string {
	return filepath.FromSlash(filepath.ToSlash(filepath.FromSlash(filepath.Base(path)))) // want `filepath.FromSlash restores the already-native filepath producer after 2 mixed slash-normalizer layer`
}

func windowsVolumeProducer(path string) string {
	return filepath.FromSlash(filepath.ToSlash(filepath.FromSlash(filepath.Clean(path)))) // want `filepath.FromSlash absorbs 2 nested ToSlash/FromSlash layer`
}

type definedString string

func converted(path definedString) any {
	return filepath.ToSlash(filepath.FromSlash(string(path))) // want `filepath.ToSlash absorbs 1 nested ToSlash/FromSlash layer`
}

// A single normalizer and an intervening call are already irreducible.
func single(path string) string   { return filepath.ToSlash(path) }
func identity(path string) string { return path }
func intervening(path string) string {
	return filepath.ToSlash(identity(filepath.FromSlash(path)))
}

func functionValues(path string) string {
	to := filepath.ToSlash
	from := filepath.FromSlash
	return to(from(path))
}

type fakeFilepath struct{}

func (fakeFilepath) ToSlash(path string) string   { return path }
func (fakeFilepath) FromSlash(path string) string { return path }

func shadowed(path string) string {
	filepath := fakeFilepath{}
	return filepath.ToSlash(filepath.FromSlash(path))
}
