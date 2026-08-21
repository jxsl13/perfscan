package benchmarks

import (
	"strings"
	"testing"
)

// These helpers copy Go 1.26 internal/filepathlite.replaceStringByte and run
// it with Windows' '\\' Separator. The host is macOS, where filepath's public
// ToSlash/FromSlash are intentional no-ops and cannot exercise the Windows
// allocation path.
func ps5113ReplaceStringByte(value string, old, replacement byte) string {
	if strings.IndexByte(value, old) == -1 {
		return value
	}
	data := []byte(value)
	for index := range data {
		if data[index] == old {
			data[index] = replacement
		}
	}
	return string(data)
}

func ps5113WindowsToSlash(path string) string {
	return ps5113ReplaceStringByte(path, '\\', '/')
}

func ps5113WindowsFromSlash(path string) string {
	return ps5113ReplaceStringByte(path, '/', '\\')
}

var (
	ps5113Input = strings.Repeat(`root\folder/mixed\leaf/`, 4096)
	ps5113Sink  string
)

func BenchmarkPS5113_Before(b *testing.B) {
	for b.Loop() {
		ps5113Sink = ps5113WindowsToSlash(ps5113WindowsFromSlash(ps5113Input))
	}
}

func BenchmarkPS5113_After(b *testing.B) {
	for b.Loop() {
		ps5113Sink = ps5113WindowsToSlash(ps5113Input)
	}
}
