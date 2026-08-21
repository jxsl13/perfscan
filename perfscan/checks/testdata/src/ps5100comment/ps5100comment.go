package ps5100comment

import "io"

func copyTo(destination io.Writer, first, second, third io.Reader) (int64, error) {
	return io.Copy(destination, io.MultiReader(( /* preserve grouping rationale */ io.MultiReader(first, second)), third)) // want "io.Copy uses MultiReader's WriterTo path; 1 nested source adapter"
}
