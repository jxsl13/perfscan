package ps3091

// Vocabulary for this fixture: compiledResourceFuncs = {compileGraph, newPipeline}.

type Graph struct{}
type Pipe struct{}

func compileGraph(sig string) *Graph { return &Graph{} }
func newPipeline(sig string) *Pipe  { return &Pipe{} }
func buildCheap(sig string) string  { return sig }

var (
	lastSig   string
	lastGraph *Graph
)

// --- POSITIVES ---

// Package-global single slot: compile stored in a global, signature remembered.
func dispatch(sig string) *Graph {
	if sig != lastSig { // want `single-slot cache`
		lastGraph = compileGraph(sig)
		lastSig = sig
	}
	return lastGraph
}

// Struct-field single slot on a receiver.
type Engine struct {
	lastSig  string
	pipeline *Pipe
}

func (e *Engine) run(sig string) *Pipe {
	if e.lastSig != sig { // want `single-slot cache`
		e.pipeline = newPipeline(sig)
		e.lastSig = sig
	}
	return e.pipeline
}

// --- GUARDS: none reported ---

// Lazy init: condition is ==, and there is no stored signature to update.
func lazy(sig string) *Graph {
	if lastGraph == nil {
		lastGraph = compileGraph(sig)
	}
	return lastGraph
}

// The compiled resource is stored into a LOCAL, not persistent state.
func local(sig string) *Graph {
	if sig != lastSig {
		g := compileGraph(sig)
		lastSig = sig
		return g
	}
	return nil
}

// The remembered signature is never updated (no `lastSig = sig`) — not the cache
// shape (and would recompile every call regardless).
func noUpdate(sig string) *Graph {
	if sig != lastSig {
		lastGraph = compileGraph(sig)
	}
	return lastGraph
}

// The constructor is not a compiled resource (cheap last-value memoization).
var lastCheap string

func cheap(sig string) string {
	if sig != lastSig {
		lastCheap = buildCheap(sig)
		lastSig = sig
	}
	return lastCheap
}
