package ps5099dot

import . "io"

func read(a, b, c Reader) ([]byte, error) {
	return ReadAll(MultiReader(MultiReader(a, b), c))
}
