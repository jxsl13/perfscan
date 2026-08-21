package main

import (
	"reflect"
	"testing"
	"time"
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

func TestTestArgsBoundNestedParallelismAndTimeout(t *testing.T) {
	t.Parallel()
	job := testJob{pkg: "example.com/p", names: []string{"TestA"}}
	got := testArgs(job, 1, 20*time.Minute, true)
	want := []string{
		"test",
		"-count=1",
		"-timeout=20m0s",
		"-parallel=1",
		"-run",
		"^(TestA)$",
		"-race",
		"example.com/p",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("testArgs() = %v, want %v", got, want)
	}
}
