package ps3091silent

// A textbook single-slot compile cache, but the test runs it with an EMPTY
// compiledResourceFuncs vocabulary, so the check must stay silent. No
// expectation comments.

type Graph struct{}

func compileGraph(sig string) *Graph { return &Graph{} }

var (
	lastSig   string
	lastGraph *Graph
)

func dispatch(sig string) *Graph {
	if sig != lastSig {
		lastGraph = compileGraph(sig)
		lastSig = sig
	}
	return lastGraph
}
