package ps5089alias

import (
	b "bytes"
	o "os"
	s "slices"
)

func aliasedPackages(file *o.File, data []byte) (int, error) {
	return file.Write(b.Clone(s.Clone(data))) // want `os.File.Write consumes its input synchronously but receives 2 throwaway standard-library Clone layer`
}
