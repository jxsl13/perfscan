package ps5119comment

import "strings"

func conditionComment(value, prefix string) string {
	if strings.HasPrefix(value /* retain boundary source */, prefix) { // want `strings.HasPrefix proves the boundary and strings.TrimPrefix immediately repeats that proof; strings.CutPrefix returns the identical remainder and predicate in one direct call`
		return strings.TrimPrefix(value, prefix)
	}
	return value
}

// The Trim expression's comment would be deleted with the duplicate call, so
// the report remains advisory and the golden source remains unchanged.
func trimComment(value, prefix string) string {
	if strings.HasPrefix(value, prefix) { // want `strings.HasPrefix proves the boundary and strings.TrimPrefix immediately repeats that proof; strings.CutPrefix returns the identical remainder and predicate in one direct call`
		return strings.TrimPrefix(value /* retain extraction rationale */, prefix)
	}
	return value
}
