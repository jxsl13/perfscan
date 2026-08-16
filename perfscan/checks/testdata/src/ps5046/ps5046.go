package ps5046

import "regexp"

var re = regexp.MustCompile("ab+c")

// --- POSITIVES ---

func idxNotNil(s string) bool {
	return re.FindStringIndex(s) != nil // want `re\.FindStringIndex\(x\) != nil allocates an index/submatch slice just to test for a match; re\.MatchString\(x\) returns the same bool with none`
}

func idxNil(s string) bool {
	return re.FindStringIndex(s) == nil // want `re\.FindStringIndex\(x\) == nil allocates an index/submatch slice just to test for a match; re\.MatchString\(x\) returns the same bool with none`
}

func findIndexBytes(b []byte) bool {
	return re.FindIndex(b) != nil // want `re\.FindIndex\(x\) != nil allocates an index/submatch slice just to test for a match; re\.Match\(x\) returns the same bool with none`
}

func submatchNil(b []byte) bool {
	return re.FindSubmatch(b) == nil // want `re\.FindSubmatch\(x\) == nil allocates an index/submatch slice just to test for a match; re\.Match\(x\) returns the same bool with none`
}

// nil on the left.
func nilLeft(s string) bool {
	return nil != re.FindStringSubmatch(s) // want `re\.FindStringSubmatch\(x\) != nil allocates an index/submatch slice just to test for a match; re\.MatchString\(x\) returns the same bool with none`
}

func findBytes(b []byte) bool {
	return re.Find(b) != nil // want `re\.Find\(x\) != nil allocates an index/submatch slice just to test for a match; re\.Match\(x\) returns the same bool with none`
}

// --- ADVISORY: reported, no fix ---

func commentInside(s string) bool {
	return re.FindStringIndex(s) != /* keep */ nil // want `re\.FindStringIndex\(x\) != nil allocates an index/submatch slice just to test for a match; re\.MatchString\(x\) returns the same bool with none`
}

// --- NEGATIVES: silent ---

// FindString returns a string, compared to "" not nil.
func findString(s string) bool {
	return re.FindString(s) != ""
}

// The result is used, not just tested against nil.
func usesResult(s string) []int {
	return re.FindStringIndex(s)
}

// FindAll* take two arguments.
func findAll(s string) bool {
	return re.FindAllStringIndex(s, -1) != nil
}
