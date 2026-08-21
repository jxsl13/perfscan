package ps5098dot

import . "io"

func read(a, b, c Reader, buffer []byte) (int, error) {
	return MultiReader(MultiReader(a, b), c).Read(buffer)
}
