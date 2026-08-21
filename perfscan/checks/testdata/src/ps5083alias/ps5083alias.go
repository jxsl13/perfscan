package ps5083alias

import s "strings"

func lastCloneImport(value string) int {
	return len(s.Clone(value)) // want "len consumes 1 throwaway standard-library Clone layer"
}
