// Package diff renders, WITHOUT leaving disk modified, the unified diff that
// perfscanxx -fix would produce.
//
// perfscanxx -fix drives clang-tidy with --fix, which rewrites files in place.
// A preview built by replaying the raw --export-fixes byte offsets ourselves
// would DIVERGE from -fix, because clang-tidy coalesces and cleans adjacent
// edits before writing (e.g. removing a whole ": x(0)" member-init rather than
// just "x(0)"). To be identical to -fix BY CONSTRUCTION, -diff instead
// snapshots the affected files, runs the real clang-tidy --fix over them, diffs
// original -> modified, and restores the snapshots — so the "+" side of the
// patch is exactly what -fix writes, and nothing is left changed on disk.
package diff

import (
	"bytes"
	"fmt"
	"strings"
)

// Unified renders a unified diff between orig and patched for a single file.
// aPath/bPath label the --- / +++ headers (typically the same cwd-relative
// path). Returns "" when orig == patched.
//
// Output is 3-line-context, line-based, and valid for `git apply` / `patch -p1`,
// including the "\ No newline at end of file" marker.
func Unified(aPath, bPath string, orig, patched []byte) string {
	if bytes.Equal(orig, patched) {
		return ""
	}
	aLines, aNoNL := splitLines(orig)
	bLines, bNoNL := splitLines(patched)

	hunks := diffHunks(aLines, bLines, 3)
	if len(hunks) == 0 {
		return ""
	}

	var out strings.Builder
	fmt.Fprintf(&out, "--- a/%s\n", aPath)
	fmt.Fprintf(&out, "+++ b/%s\n", bPath)
	for _, h := range hunks {
		writeHunk(&out, h, len(aLines), len(bLines), aNoNL, bNoNL)
	}
	return out.String()
}

// splitLines splits data into logical lines WITHOUT their trailing newline. The
// second result is true when data does not end in a newline (the last line has
// "no newline at end of file"). Empty input yields no lines.
func splitLines(data []byte) (lines []string, noFinalNewline bool) {
	if len(data) == 0 {
		return nil, false
	}
	s := string(data)
	noFinalNewline = !strings.HasSuffix(s, "\n")
	if !noFinalNewline {
		s = s[:len(s)-1] // drop trailing newline so Split doesn't add an empty tail
	}
	return strings.Split(s, "\n"), noFinalNewline
}

// op is one edit-script operation over lines, with the 1-based source line
// numbers it occupies on each side (a==original, b==patched).
type op struct {
	kind         byte // ' ' context, '-' delete, '+' insert
	line         string
	aLine, bLine int
}

// hunk is a contiguous run of operations with its 1-based header coordinates.
type hunk struct {
	aStart, aCount int
	bStart, bCount int
	ops            []op
}

// diffHunks computes a line-level diff (via LCS) and groups the differing
// regions into unified-diff hunks with `context` lines of surrounding context.
func diffHunks(a, b []string, context int) []hunk {
	raw := lcsDiff(a, b)
	if len(raw) == 0 {
		return nil
	}

	// Annotate each op with its 1-based line numbers.
	ops := make([]op, len(raw))
	ai, bi := 0, 0
	for i, o := range raw {
		o.aLine, o.bLine = ai, bi
		switch o.kind {
		case ' ':
			ai++
			bi++
			o.aLine, o.bLine = ai, bi
		case '-':
			ai++
			o.aLine = ai
			o.bLine = bi // position after last consumed b line
		case '+':
			bi++
			o.aLine = ai
			o.bLine = bi
		}
		ops[i] = o
	}

	changed := make([]bool, len(ops))
	any := false
	for i, o := range ops {
		if o.kind != ' ' {
			changed[i] = true
			any = true
		}
	}
	if !any {
		return nil
	}

	var hunks []hunk
	i := 0
	for i < len(ops) {
		if !changed[i] {
			i++
			continue
		}
		// Leading context: back up to `context` unchanged lines.
		start := i - context
		if start < 0 {
			start = 0
		}
		// Grow the cluster: extend while another change lies within 2*context.
		end := i
		for {
			next := -1
			for j := end + 1; j < len(ops) && j <= end+2*context; j++ {
				if changed[j] {
					next = j
				}
			}
			if next == -1 {
				break
			}
			end = next
		}
		// Trailing context.
		stop := end + context
		if stop >= len(ops) {
			stop = len(ops) - 1
		}

		h := hunk{ops: append([]op(nil), ops[start:stop+1]...)}
		h.aStart, h.aCount, h.bStart, h.bCount = coords(h.ops)
		hunks = append(hunks, h)
		i = stop + 1
	}
	return hunks
}

// coords derives the 1-based start line and line count for each side of a hunk.
// For an all-insert (or all-delete) hunk the empty side's start is the line it
// follows, with count 0 — matching git's convention.
func coords(ops []op) (aStart, aCount, bStart, bCount int) {
	aStart, bStart = 0, 0
	for _, o := range ops {
		switch o.kind {
		case ' ':
			if aStart == 0 {
				aStart = o.aLine
			}
			if bStart == 0 {
				bStart = o.bLine
			}
			aCount++
			bCount++
		case '-':
			if aStart == 0 {
				aStart = o.aLine
			}
			aCount++
		case '+':
			if bStart == 0 {
				bStart = o.bLine
			}
			bCount++
		}
	}
	// When a side is empty, git writes the line it follows (o.aLine/o.bLine of
	// the first op already holds the preceding line index for inserts/deletes).
	if aCount == 0 {
		aStart = ops[0].aLine
	}
	if bCount == 0 {
		bStart = ops[0].bLine
	}
	return aStart, aCount, bStart, bCount
}

// lcsDiff produces a line-level edit script between a and b. It trims the common
// prefix/suffix, then runs an LCS DP over the differing middle only.
func lcsDiff(a, b []string) []op {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}

	var ops []op
	for i := 0; i < p; i++ {
		ops = append(ops, op{kind: ' ', line: a[i]})
	}
	ops = append(ops, lcsMiddle(a[p:len(a)-s], b[p:len(b)-s])...)
	for i := len(a) - s; i < len(a); i++ {
		ops = append(ops, op{kind: ' ', line: a[i]})
	}
	return ops
}

// lcsMiddle diffs two slices (no common prefix/suffix) via LCS backtracking.
func lcsMiddle(a, b []string) []op {
	n, m := len(a), len(b)
	if n == 0 && m == 0 {
		return nil
	}
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var ops []op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, op{kind: ' ', line: a[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, op{kind: '-', line: a[i]})
			i++
		default:
			ops = append(ops, op{kind: '+', line: b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, op{kind: '-', line: a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, op{kind: '+', line: b[j]})
	}
	return ops
}

// writeHunk emits one @@ hunk. The "no newline at end of file" marker is placed
// after the op that renders the true final line of the side lacking a newline.
func writeHunk(out *strings.Builder, h hunk, aTotal, bTotal int, aNoNL, bNoNL bool) {
	fmt.Fprintf(out, "@@ -%s +%s @@\n", rangeSpec(h.aStart, h.aCount), rangeSpec(h.bStart, h.bCount))
	for _, o := range h.ops {
		out.WriteByte(o.kind)
		out.WriteString(o.line)
		out.WriteByte('\n')
		switch o.kind {
		case ' ':
			if aNoNL && o.aLine == aTotal {
				out.WriteString("\\ No newline at end of file\n")
			} else if bNoNL && o.bLine == bTotal {
				out.WriteString("\\ No newline at end of file\n")
			}
		case '-':
			if aNoNL && o.aLine == aTotal {
				out.WriteString("\\ No newline at end of file\n")
			}
		case '+':
			if bNoNL && o.bLine == bTotal {
				out.WriteString("\\ No newline at end of file\n")
			}
		}
	}
}

// rangeSpec formats a unified-diff range: "start" when count==1, "start,0" for
// an empty side, else "start,count".
func rangeSpec(start, count int) string {
	if count == 1 {
		return fmt.Sprintf("%d", start)
	}
	if count == 0 {
		return fmt.Sprintf("%d,0", start)
	}
	return fmt.Sprintf("%d,%d", start, count)
}
