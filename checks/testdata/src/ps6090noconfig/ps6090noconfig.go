package ps6090noconfig

import "testing"

func compute() (int, error) { return 0, nil }

func BenchmarkNoVocabulary(b *testing.B) {
	for b.Loop() {
		_, _ = compute()
	}
}
