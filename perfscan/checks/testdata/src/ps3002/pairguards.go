package ps3002

// Two-return guard PAIRS and the bare `return false` tail. A pair
//
//	if xs[i].key < xs[j].key { return true }
//	if xs[i].key > xs[j].key { return false }
//
// is the `!=` tie-break guard in disguise (one if returns the '<' verdict
// when the fields differ, both fall through when equal), and a trailing
// `return false` after at least one guard unit is the canonical "all fields
// equal" tail, emitted as `return 0`. Only "sort" is imported here: the fix
// ADDS slices and cmp at the first fixable site of the file, and the
// sort.Sort call keeps a surviving sort reference so the golden compiles
// without the runner's orphan pruning.

import (
	"sort"
)

type pgRec struct {
	key   int
	rank  int
	name  string
	score float64
}

func pgKeepSort(keep sort.Interface) { sort.Sort(keep) }

// Ascending pair ('<' returns true) + final field compare: the pair is one
// ascending guard on key.
func pgAscPair(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key < xs[j].key {
			return true
		}
		if xs[i].key > xs[j].key {
			return false
		}
		return xs[i].rank < xs[j].rank
	})
}

// Descending pair ('>' returns true) + final field compare: the pair is one
// DESCENDING guard on key, so its cmp.Compare operands are swapped.
func pgDescPair(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key > xs[j].key {
			return true
		}
		if xs[i].key < xs[j].key {
			return false
		}
		return xs[i].rank < xs[j].rank
	})
}

// A `!=` guard, then a two-return pair, then a final field compare — the two
// guard spellings mix freely within one chain.
func pgMixedGuards(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key != xs[j].key {
			return xs[i].key < xs[j].key
		}
		if xs[i].rank < xs[j].rank {
			return true
		}
		if xs[i].rank > xs[j].rank {
			return false
		}
		return xs[i].name < xs[j].name
	})
}

// A chain of pairs closed by the bare `return false` tail ("all fields equal
// → i not before j") — every field becomes a guard and the tail `return 0`.
func pgPairTailFalse(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key < xs[j].key {
			return true
		}
		if xs[i].key > xs[j].key {
			return false
		}
		if xs[i].rank < xs[j].rank {
			return true
		}
		if xs[i].rank > xs[j].rank {
			return false
		}
		return false
	})
}

// A `!=` guard closed by the bare `return false` tail works too.
func pgNeqTailFalse(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key != xs[j].key {
			return xs[i].key < xs[j].key
		}
		return false
	})
}

// SliceStable with a pair becomes slices.SortStableFunc.
func pgStablePair(xs []pgRec) {
	sort.SliceStable(xs, func(i, j int) bool { // want `sort\.SliceStable swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key < xs[j].key {
			return true
		}
		if xs[i].key > xs[j].key {
			return false
		}
		return xs[i].rank < xs[j].rank
	})
}

// '<=' is not a strict ordering comparison — not the canonical pair.
// Advisory only.
func pgLeqPair(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key <= xs[j].key {
			return true
		}
		if xs[i].key > xs[j].key {
			return false
		}
		return xs[i].rank < xs[j].rank
	})
}

// Both ifs return true: a different predicate, not the `!=` guard. Advisory
// only.
func pgBothTrue(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key < xs[j].key {
			return true
		}
		if xs[i].key > xs[j].key {
			return true
		}
		return xs[i].rank < xs[j].rank
	})
}

// The same operator twice: the second if is dead, not a guard pair. Advisory
// only.
func pgSameOperator(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key < xs[j].key {
			return true
		}
		if xs[i].key < xs[j].key {
			return false
		}
		return xs[i].rank < xs[j].rank
	})
}

// The two ifs test DIFFERENT fields: not a pair on one field, the bool and
// cmp chains diverge. Advisory only.
func pgDifferentFields(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key < xs[j].key {
			return true
		}
		if xs[i].rank > xs[j].rank {
			return false
		}
		return xs[i].name < xs[j].name
	})
}

// A FLOAT field in a pair stays advisory: the NaN exclusion applies to the
// pair spelling exactly as it does to `!=` guards.
func pgFloatPair(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].score < xs[j].score {
			return true
		}
		if xs[i].score > xs[j].score {
			return false
		}
		return xs[i].rank < xs[j].rank
	})
}

// A WHOLE-ELEMENT pair (empty selector chain) is rejected like an empty `!=`
// guard: guards must name the field they order. Advisory only.
func pgWholeElemPair(xs []int) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i] < xs[j] {
			return true
		}
		if xs[i] > xs[j] {
			return false
		}
		return false
	})
}

// A `return true` tail is not a strict-order comparator on ties. Advisory
// only.
func pgTrueTail(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key < xs[j].key {
			return true
		}
		if xs[i].key > xs[j].key {
			return false
		}
		return true
	})
}

// An else branch on a pair half breaks the fall-through shape. Advisory only.
func pgPairElse(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key < xs[j].key {
			return true
		} else {
			return xs[i].rank < xs[j].rank
		}
	})
}

// An init statement on a pair half is not the canonical shape. Advisory only.
func pgPairInit(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if k := xs[i].key; k < xs[j].key {
			return true
		}
		if xs[i].key > xs[j].key {
			return false
		}
		return xs[i].rank < xs[j].rank
	})
}

// A lone `return false` with no guard unit before it is a degenerate constant
// comparator. Advisory only.
func pgLoneReturnFalse(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		return false
	})
}

// The FALSE-returning if may come first: ('<' → false, '>' → true) is the
// descending pair with the halves swapped — the true-returning '>' if carries
// the direction, so cmp.Compare's operands are swapped.
func pgFalseFirstDesc(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key < xs[j].key {
			return false
		}
		if xs[i].key > xs[j].key {
			return true
		}
		return xs[i].rank < xs[j].rank
	})
}

// ('>' → false, '<' → true) is the ascending pair with the halves swapped.
func pgFalseFirstAsc(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key > xs[j].key {
			return false
		}
		if xs[i].key < xs[j].key {
			return true
		}
		return xs[i].rank < xs[j].rank
	})
}

// The second half's operands are SWAPPED (xs[j] on the left): xs[j].key >
// xs[i].key is really the '<' condition again, so on key-greater the body
// falls through to the rank compare — NOT a fall-through-on-equal guard.
// Treating it as one would resurrect the rank tie-break the bool form skips.
// Advisory only.
func pgSwappedOperands(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key < xs[j].key {
			return true
		}
		if xs[j].key > xs[i].key {
			return false
		}
		return xs[i].rank < xs[j].rank
	})
}

// '>=' in the second half returns false ON EQUALITY too — the tie-break below
// it is dead code, so this is a single-field strict order, not a guard pair.
// Advisory only.
func pgGeqPair(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key < xs[j].key {
			return true
		}
		if xs[i].key >= xs[j].key {
			return false
		}
		return xs[i].rank < xs[j].rank
	})
}

// A LOCAL variable named `false` shadows the predeclared constant — its value
// may well be true, so `return false` is not the literal the pair hinges on.
// Advisory only.
func pgShadowFalse(xs []pgRec) {
	false := len(xs) > 0
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key < xs[j].key {
			return true
		}
		if xs[i].key > xs[j].key {
			return false
		}
		return xs[i].rank < xs[j].rank
	})
}

// Same for a shadowed `true`. Advisory only.
func pgShadowTrue(xs []pgRec) {
	true := len(xs) == 0
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key < xs[j].key {
			return true
		}
		if xs[i].key > xs[j].key {
			return false
		}
		return xs[i].rank < xs[j].rank
	})
}

var pgSink int

// A statement BETWEEN the two halves breaks the pair. Advisory only.
func pgInterleaved(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key < xs[j].key {
			return true
		}
		pgSink = xs[i].key
		if xs[i].key > xs[j].key {
			return false
		}
		return xs[i].rank < xs[j].rank
	})
}

// Both halves return false: on key-less it says "not before", on key-greater
// too — a different predicate, not a guard. Advisory only.
func pgBothFalse(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key < xs[j].key {
			return false
		}
		if xs[i].key > xs[j].key {
			return false
		}
		return xs[i].rank < xs[j].rank
	})
}

// A parenthesized `(false)` tail is not matched by the literal check —
// conservatively advisory.
func pgParenFalseTail(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].key != xs[j].key {
			return xs[i].key < xs[j].key
		}
		return (false)
	})
}

type pgCelsius float64

type pgTemp struct {
	temp pgCelsius
	rank int
}

// A NAMED float type in a pair is still a float underneath — the NaN
// exclusion applies through the name. Advisory only.
func pgNamedFloatPair(xs []pgTemp) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].temp < xs[j].temp {
			return true
		}
		if xs[i].temp > xs[j].temp {
			return false
		}
		return xs[i].rank < xs[j].rank
	})
}

type pgOrd interface{ ~int | ~string | ~float64 }

type pgBox[T pgOrd] struct {
	v    T
	rank int
}

// A TYPE-PARAMETER field is not an ordered basic type at check time — a
// float64 instantiation would smuggle NaN past the exclusion. Advisory only.
func pgGenericPair[T pgOrd](xs []pgBox[T]) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if xs[i].v < xs[j].v {
			return true
		}
		if xs[i].v > xs[j].v {
			return false
		}
		return xs[i].rank < xs[j].rank
	})
}

// --- Real-world near-miss shapes from kubernetes corpus (-fix validation,
// 2026-08-13). Each looks pair-like but must stay ADVISORY: the two-return
// pair matcher requires xs[i]<CHAIN> index-selector operands over an ordered
// basic field and a body of only if-guards + a final return. These pin that
// the new pair path does NOT false-positive on the trickiest real code. ---

type pgCPUSet struct{ n int }

func (c pgCPUSet) Size() int { return c.n }

func pgGetCPUs(id int) pgCPUSet { return pgCPUSet{id} }

// kubernetes pkg/kubelet/cm/cpumanager/cpu_assignment.go: the pair operands
// are METHOD CALLS on locals declared inside the comparator, not xs[i].field
// index chains — and leading `:=` statements are not guards. Advisory.
func pgMethodCallOperands(ids []int) {
	sort.Slice(ids, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		iCPUs := pgGetCPUs(ids[i])
		jCPUs := pgGetCPUs(ids[j])
		if iCPUs.Size() < jCPUs.Size() {
			return true
		}
		if iCPUs.Size() > jCPUs.Size() {
			return false
		}
		return ids[i] < ids[j]
	})
}

type pgNode int

func pgIsSet(n pgNode) bool { return n != 0 }

// kubernetes pkg/kubelet/cm/devicemanager/manager.go: a boolean-PREDICATE
// guard `if pred(xs[i]) { return true }` — the condition is not an ordering
// comparison, so it is neither a `!=` guard nor a pair half. Advisory.
func pgBoolPredicateGuard(nodes []pgNode) {
	sort.Slice(nodes, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		if pgIsSet(nodes[i]) {
			return true
		}
		if pgIsSet(nodes[j]) {
			return false
		}
		return nodes[i] < nodes[j]
	})
}

// kubernetes test/e2e/framework/bugs.go: a SWITCH leads the comparator body
// (there it was switch strings.Compare(...)); the walk accepts only if-guards
// and a final return, so a leading switch keeps it advisory.
func pgSwitchComparator(xs []pgRec) {
	sort.Slice(xs, func(i, j int) bool { // want `sort\.Slice swaps through reflection and calls its comparator indirectly; slices\.SortFunc sorts the concrete type directly`
		switch {
		case xs[i].key < xs[j].key:
			return true
		case xs[i].key > xs[j].key:
			return false
		}
		return xs[i].rank < xs[j].rank
	})
}
