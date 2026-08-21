package ps5107alias

import faults "errors"

func match(a, b, c, target error) bool {
	return faults.Is(faults.Join(faults.Join(a, b), c), target) // want "errors.Is traverses a nested errors.Join tree with 1 intermediate join node"
}
