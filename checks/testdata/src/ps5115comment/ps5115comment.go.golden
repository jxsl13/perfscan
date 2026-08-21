package ps5115comment

import "strings"

func retained(payload string) string {
	return strings.ToValidUTF8( /* preserve trust-boundary rationale */ strings.ToValidUTF8(payload, "?"), "outer") // want `strings.ToValidUTF8 is applied 2 times even though the retained call's valid replacement already guarantees valid UTF-8`
}
