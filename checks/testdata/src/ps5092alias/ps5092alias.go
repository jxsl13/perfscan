package ps5092alias

import text "strings"

func compare(left, right string) bool {
	return text.Clone(text.Clone(left)) != text.Clone(right) // want "!= comparison consumes 3 throwaway standard-library Clone layer[(]s[)] across 2 operand"
}
