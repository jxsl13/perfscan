package ps5088alias

import (
	r "regexp"
	s "strings"
)

func aliasedPackages(compiled *r.Regexp, subject string) bool {
	return compiled.MatchString(s.Clone(subject)) // want `regexp.MatchString returns only match status but scans 1 throwaway Clone layer`
}
