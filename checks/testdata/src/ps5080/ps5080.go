package ps5080

import (
	"bytes"
	"strings"
)

const packageSame = "package"

func stringReplaceAll(s string) string {
	return strings.ReplaceAll(s, "x", "x") // want `1 adjacent strings Replace/ReplaceAll call\(s\) are content no-ops`
}

func stringReplaceZero(s string) string {
	return strings.Replace(s, "old", "new", 0) // want `1 adjacent strings Replace/ReplaceAll call\(s\) are content no-ops`
}

func stringDeepMixed(s string) string {
	return strings.ReplaceAll(strings.Replace(strings.ReplaceAll(s, "a", "a"), "old", "new", 0), "z", "z") // want `3 adjacent strings Replace/ReplaceAll call\(s\) are content no-ops`
}

func stringPackageConstant(s string) string {
	return strings.ReplaceAll(s, packageSame, packageSame) // want `1 adjacent strings Replace/ReplaceAll call\(s\) are content no-ops`
}

func stringComment(s string) string {
	return strings.ReplaceAll( /* keep this explanation */ s, "x", "x") // want `1 adjacent strings Replace/ReplaceAll call\(s\) are content no-ops`
}

func stringLocalConstant(s string) string {
	const same = "local"
	return strings.ReplaceAll(s, same, same) // want `1 adjacent strings Replace/ReplaceAll call\(s\) are content no-ops`
}

// Removing the call would introduce a duplicate constant switch case.
func stringConstantSwitch(selected string) int {
	switch selected {
	case "x":
		return 1
	case strings.ReplaceAll("x", "y", "y"): // want `1 adjacent strings Replace/ReplaceAll call\(s\) are content no-ops`
		return 2
	}
	return 0
}

func bytesDeepMixed(b []byte) []byte {
	return bytes.ReplaceAll( // want `3 adjacent bytes Replace/ReplaceAll calls preserve content but each copies the slice`
		bytes.Replace(
			bytes.ReplaceAll(b, []byte("a"), []byte("a")),
			[]byte("old"), []byte("new"), 0,
		),
		[]byte("z"), []byte("z"),
	)
}

func bytesNilAndEmpty(b []byte) []byte {
	return bytes.Replace( // want `2 adjacent bytes Replace/ReplaceAll calls preserve content but each copies the slice`
		bytes.ReplaceAll(b, nil, []byte{}),
		[]byte("x"), []byte("x"), -1,
	)
}

func bytesComment(b []byte) []byte {
	return bytes.ReplaceAll( // want `2 adjacent bytes Replace/ReplaceAll calls preserve content but each copies the slice`
		bytes.ReplaceAll( /* preserve attribution */ b, []byte("x"), []byte("x")),
		[]byte("y"), []byte("y"),
	)
}

// A single byte call is the API's observable copy and stays silent.
func bytesSingleCopy(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("x"), []byte("x"))
}

func dynamicString(s, old string) string {
	return strings.ReplaceAll(s, old, old)
}

func dynamicBytes(b, old []byte) []byte {
	return bytes.ReplaceAll(bytes.ReplaceAll(b, old, old), old, old)
}

func realReplacement(s string) string {
	return strings.Replace(s, "x", "y", 1)
}

type helper struct{}

func (helper) ReplaceAll(s, old, new string) string { return s }

func shadowedMethod(h helper, s string) string {
	return h.ReplaceAll(h.ReplaceAll(s, "x", "x"), "y", "y")
}
