package ps5086alias

import (
	ba "bytealias"
	b "bytes"
	sl "slices"
)

func aliasedPackages(data ba.Bytes) []byte {
	return b.ToUpper(sl.Clone(data)) // want `bytes.ToUpper already creates independent output but receives 1 throwaway Clone layer`
}
