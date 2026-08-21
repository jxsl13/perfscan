package ps5099comment

import "io"

func read(first, second, third io.Reader) ([]byte, error) {
	return io.ReadAll(io.MultiReader(( /* preserve grouping rationale */ io.MultiReader(first, second)), third)) // want "io.ReadAll consumes only the flattened behavior of io.MultiReader; 1 nested adapter"
}
