package ps5107comment

import "errors"

func match(a, b, c, target error) bool {
	return errors.Is(errors.Join(( /* preserve grouping rationale */ errors.Join(a, b)), c), target) // want "errors.Is traverses a nested errors.Join tree with 1 intermediate join node"
}
