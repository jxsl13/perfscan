package ps6006

import (
	"ps6006prod"
	"testing"
)

var useSplitK bool

func BenchmarkDefaultToggle(b *testing.B) {
	useSplitK = true // want `configured production selector/default useSplitK appears in a repeated leaf benchmark`
	for i := 0; i < b.N; i++ {
		_ = useSplitK
	}
}

type engine struct{}

func (engine) chooseSplitK() bool { return true }

func BenchmarkMethodSelector(b *testing.B) {
	e := engine{}
	for i := 0; i < b.N; i++ {
		_ = e.chooseSplitK() // want `configured production selector/default chooseSplitK appears in a repeated leaf benchmark`
	}
}

func BenchmarkImportedSelector(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = ps6006prod.UseSplitK() // want `configured production selector/default UseSplitK appears in a repeated leaf benchmark`
	}
}

// A configured spelling bound to a local object is not a production selector.
func BenchmarkLocalShadow(b *testing.B) {
	useSplitK := false
	for i := 0; i < b.N; i++ {
		_ = useSplitK
	}
}

// A benchmark without an explicit repetition loop is outside the detector's
// conservative evidence boundary.
func BenchmarkNoLoop(b *testing.B) {
	useSplitK = true
}

// Loops and production selectors outside a real testing benchmark are silent.
func helper(n int) {
	for i := 0; i < n; i++ {
		useSplitK = i%2 == 0
	}
}

// Nested closures are not attributed to the enclosing benchmark.
func BenchmarkNestedClosure(b *testing.B) {
	f := func() {
		for i := 0; i < b.N; i++ {
			useSplitK = i%2 == 0
		}
	}
	f()
}
