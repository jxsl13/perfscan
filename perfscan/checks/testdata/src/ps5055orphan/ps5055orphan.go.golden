package ps5055orphan

import "slices"

// slices.Equal over byte slices is the file's only slices use — converting it
// would orphan the import, so the report stays advisory.
func onlyUse(a, b []byte) bool {
	return slices.Equal(a, b) // want `slices\.Equal over byte slices runs the generic element loop`
}
