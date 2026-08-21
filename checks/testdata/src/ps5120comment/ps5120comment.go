package ps5120comment

import "strings"

func retained(value string) string {
	head := strings.SplitN(value /* retain source */, ":", 2)[0] // want `strings.SplitN\(\.\.\.\)\[0\] allocates a piece slice only to assign its head; strings.Cut returns the identical head directly with no result-slice allocation`
	return head
}

func countComment(value string) string {
	head := strings.SplitN(value, ":", 2 /* preserve limit rationale */)[0] // want `strings.SplitN\(\.\.\.\)\[0\] allocates a piece slice only to assign its head; strings.Cut returns the identical head directly with no result-slice allocation`
	return head
}

func indexComment(value string) string {
	head := strings.SplitN(value, ":", 2)[0 /* preserve head rationale */] // want `strings.SplitN\(\.\.\.\)\[0\] allocates a piece slice only to assign its head; strings.Cut returns the identical head directly with no result-slice allocation`
	return head
}

func localCount(value string) string {
	const count = 2
	head := strings.SplitN(value, ":", count)[0] // want `strings.SplitN\(\.\.\.\)\[0\] allocates a piece slice only to assign its head; strings.Cut returns the identical head directly with no result-slice allocation`
	return head
}

func localIndex(value string) string {
	const index = 0
	head := strings.SplitN(value, ":", 2)[index] // want `strings.SplitN\(\.\.\.\)\[0\] allocates a piece slice only to assign its head; strings.Cut returns the identical head directly with no result-slice allocation`
	return head
}
