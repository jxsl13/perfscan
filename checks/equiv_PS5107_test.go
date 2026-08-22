package checks

import (
	"errors"
	"reflect"
	"testing"
)

type ps5107ProbeError struct {
	name  string
	match bool
	log   *[]string
}

func (probe *ps5107ProbeError) Error() string { return probe.name }

func (probe *ps5107ProbeError) Is(error) bool {
	*probe.log = append(*probe.log, probe.name)
	return probe.match
}

type ps5107SliceError []byte

func (ps5107SliceError) Error() string { return "slice target" }

func ps5107Search(flat bool, match string, target error) (bool, []string) {
	var log []string
	leaf := func(name string) error {
		return &ps5107ProbeError{name: name, match: name == match, log: &log}
	}
	if flat {
		return errors.Is(errors.Join(leaf("a"), leaf("b"), nil, leaf("c"), leaf("d")), target), log
	}
	return errors.Is(errors.Join(errors.Join(leaf("a"), leaf("b")), errors.Join(nil, leaf("c"), leaf("d"))), target), log
}

func TestEquiv_PS5107ErrorsIsTraversal(t *testing.T) {
	targets := []error{errors.New("missing"), ps5107SliceError{1, 2, 3}}
	for _, target := range targets {
		for _, match := range []string{"", "a", "b", "c", "d"} {
			beforeResult, beforeLog := ps5107Search(false, match, target)
			afterResult, afterLog := ps5107Search(true, match, target)
			if beforeResult != afterResult || !reflect.DeepEqual(beforeLog, afterLog) {
				t.Fatalf("match %q target %T differs: before=(%v,%v) after=(%v,%v)", match, target, beforeResult, beforeLog, afterResult, afterLog)
			}
		}
	}
}

func TestEquiv_PS5107ComparableLeafTarget(t *testing.T) {
	target := errors.New("target")
	var beforeLog, afterLog []string
	beforeA := &ps5107ProbeError{name: "a", log: &beforeLog}
	beforeC := &ps5107ProbeError{name: "c", log: &beforeLog}
	afterA := &ps5107ProbeError{name: "a", log: &afterLog}
	afterC := &ps5107ProbeError{name: "c", log: &afterLog}
	before := errors.Is(errors.Join(errors.Join(beforeA, target), beforeC), target)
	after := errors.Is(errors.Join(afterA, target, afterC), target)
	if before != after || !before || !reflect.DeepEqual(beforeLog, afterLog) || !reflect.DeepEqual(afterLog, []string{"a"}) {
		t.Fatalf("comparable leaf match differs: before=(%v,%v) after=(%v,%v)", before, beforeLog, after, afterLog)
	}
}

func TestEquiv_PS5107EvaluationOrder(t *testing.T) {
	run := func(flat bool) []string {
		var order []string
		leaf := func(name string) error {
			order = append(order, name)
			return errors.New(name)
		}
		target := func() error {
			order = append(order, "target")
			return errors.New("missing")
		}
		if flat {
			_ = errors.Is(errors.Join(leaf("a"), leaf("b"), leaf("c"), leaf("d")), target())
		} else {
			_ = errors.Is(errors.Join(errors.Join(leaf("a"), leaf("b")), errors.Join(leaf("c"), leaf("d"))), target())
		}
		return order
	}
	before, after := run(false), run(true)
	want := []string{"a", "b", "c", "d", "target"}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(after, want) {
		t.Fatalf("evaluation order differs: before=%v after=%v want=%v", before, after, want)
	}
}

func TestEquiv_PS5107AllNil(t *testing.T) {
	before := errors.Is(errors.Join(errors.Join(nil, nil), nil), nil)
	after := errors.Is(errors.Join(nil, nil, nil), nil)
	if before != after || !before {
		t.Fatalf("all-nil result differs: before=%v after=%v", before, after)
	}
}

func TestEquiv_PS5107ErrorsAsIsDeliberatelyExcluded(t *testing.T) {
	a, b, c := errors.New("a"), errors.New("b"), errors.New("c")
	nested := errors.Join(errors.Join(a, b), c)
	flat := errors.Join(a, b, c)
	var before, after interface{ Unwrap() []error }
	if !errors.As(nested, &before) || !errors.As(flat, &after) {
		t.Fatal("errors.As did not capture join roots")
	}
	if len(before.Unwrap()) == len(after.Unwrap()) {
		t.Fatalf("errors.As exclusion witness lost: nested root has %d children, flat root has %d", len(before.Unwrap()), len(after.Unwrap()))
	}
}
