package ps5100dot

import . "io"

func copyTo(destination Writer, a, b, c Reader) (int64, error) {
	return Copy(destination, MultiReader(MultiReader(a, b), c))
}
