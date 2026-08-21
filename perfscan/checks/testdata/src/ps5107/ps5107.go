package ps5107

import "errors"

func nested(a, b, c, target error) bool {
	return errors.Is(errors.Join(errors.Join(a, b), c), target) // want "errors.Is traverses a nested errors.Join tree with 1 intermediate join node"
}

func branched(a, b, c, d, target error) bool {
	return errors.Is(errors.Join(errors.Join(a, b), errors.Join(c, d)), target) // want "errors.Is traverses a nested errors.Join tree with 2 intermediate join node"
}

func parenthesized(a, b, c, target error) bool {
	return errors.Is((errors.Join((errors.Join(a, b)), c)), target) // want "errors.Is traverses a nested errors.Join tree with 1 intermediate join node"
}

func nilLeaves(a, target error) bool {
	return errors.Is(errors.Join(errors.Join(nil, a), nil), target) // want "errors.Is traverses a nested errors.Join tree with 1 intermediate join node"
}

func expose(a, b, c error) error {
	return errors.Join(errors.Join(a, b), c)
}

type unwrapper interface {
	Unwrap() []error
}

func as(a, b, c error) (bool, unwrapper) {
	var found unwrapper
	matched := errors.As(errors.Join(errors.Join(a, b), c), &found)
	return matched, found
}

func unwrap(a, b, c error) error {
	return errors.Unwrap(errors.Join(errors.Join(a, b), c))
}

func functionConsumer(a, b, c, target error) bool {
	is := errors.Is
	return is(errors.Join(errors.Join(a, b), c), target)
}

func functionConstructor(a, b, c, target error) bool {
	join := errors.Join
	return errors.Is(errors.Join(join(a, b), c), target)
}

func empty(a, target error) bool {
	return errors.Is(errors.Join(errors.Join(), a), target)
}

func spread(values []error, target error) bool {
	return errors.Is(errors.Join(errors.Join(values...)), target)
}

func single(a, b, target error) bool {
	return errors.Is(errors.Join(a, b), target)
}

type joined struct{}

func (joined) Error() string { return "joined" }

func Join(...error) error { return joined{} }

func lookalike(a, b, c, target error) bool {
	return errors.Is(errors.Join(Join(a, b), c), target)
}
