package ps5107dot

import . "errors"

func match(a, b, c, target error) bool {
	return Is(Join(Join(a, b), c), target)
}
