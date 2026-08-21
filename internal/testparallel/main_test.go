package main

import (
	"reflect"
	"testing"
)

func TestParseTestNamesExcludesBenchmarksAndNoise(t *testing.T) {
	t.Parallel()
	got := parseTestNames("TestBeta\nBenchmarkHot\nExampleThing\nFuzzInput\nok pkg 0.1s\nTestAlpha\n")
	want := []string{"ExampleThing", "FuzzInput", "TestAlpha", "TestBeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseTestNames() = %v, want %v", got, want)
	}
}

func TestPartitionIsStableAndBalanced(t *testing.T) {
	t.Parallel()
	got := partition([]string{"A", "B", "C", "D", "E"}, 3)
	want := [][]string{{"A", "D"}, {"B", "E"}, {"C"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("partition() = %v, want %v", got, want)
	}
}

func TestPatternIsAnchoredAndEscaped(t *testing.T) {
	t.Parallel()
	if got, want := testPattern([]string{"TestA/B", "TestC+"}), `^(TestA/B|TestC\+)$`; got != want {
		t.Fatalf("testPattern() = %q, want %q", got, want)
	}
}
