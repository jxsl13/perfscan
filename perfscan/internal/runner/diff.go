package runner

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strings"
)

// diffFixes is the -diff (dry-run) mode: it computes the exact bytes -fix
// would write via the shared patchedFiles helper, prints a unified diff of
// original → fixed per changed file to Stdout, modifies NOTHING, and
// returns 1 when at least one file would change (0 otherwise, 2 never —
// hard errors were already reported by patchedFiles on Stderr).
func diffFixes(findings []Finding, opts *Options) int {
	files, applied, overlapping, failed := patchedFiles(findings, opts)
	paths := make([]string, 0, len(files))
	for path, pf := range files {
		if !bytes.Equal(pf.orig, pf.fixed) {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	for _, path := range paths {
		pf := files[path]
		io.WriteString(opts.Stdout, unifiedDiff(relPath(path), pf.orig, pf.fixed))
	}
	if failed > 0 {
		fmt.Fprintf(opts.Stderr, "perfscan: %d fix(es) could not be rendered\n", failed)
	}
	if overlapping > 0 {
		fmt.Fprintf(opts.Stderr, "perfscan: %d overlapping fix(es) not shown (already covered by another)\n", overlapping)
	}
	if len(paths) > 0 {
		fmt.Fprintf(opts.Stderr, "perfscan: %d file(s) would change (%d fix(es)); run with -fix to apply\n", len(paths), applied)
		if !opts.ExitZero {
			return 1
		}
	}
	return 0
}

// diffContext is the number of unchanged context lines shown around each
// change, matching the diff/git default of 3.
const diffContext = 3

// unifiedDiff renders a standard unified diff between a (--- a/<path>) and
// b (+++ b/<path>) with diffContext lines of context — output that patch
// and git apply consume. Empty string when the contents are equal.
func unifiedDiff(path string, a, b []byte) string {
	if bytes.Equal(a, b) {
		return ""
	}
	al := splitLines(a)
	bl := splitLines(b)
	ops := diffLines(al, bl)
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- a/%s\n+++ b/%s\n", path, path)
	renderHunks(&sb, al, bl, ops)
	return sb.String()
}

// splitLines splits b into lines KEEPING each terminating "\n" (the last
// line may lack one; renderHunks marks that case).
func splitLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	lines := strings.SplitAfter(string(b), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// diffOp is one line of an alignment between two files: '=' keeps line
// a[ai] (== b[bi]), '-' deletes a[ai], '+' inserts b[bi].
type diffOp struct {
	tag    byte // '=', '-', '+'
	ai, bi int
}

// diffLines aligns a and b line-wise: common prefix and suffix are matched
// directly (fixes are localized, so the remaining middle is small), and the
// middle is aligned optimally via a longest-common-subsequence table.
func diffLines(a, b []string) []diffOp {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	ma, mb := a[p:len(a)-s], b[p:len(b)-s]

	// dp[i][j] = LCS length of ma[i:] and mb[j:], rows sliced from one slab.
	n, m := len(ma), len(mb)
	slab := make([]int, (n+1)*(m+1))
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = slab[i*(m+1) : (i+1)*(m+1) : (i+1)*(m+1)]
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if ma[i] == mb[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}

	ops := make([]diffOp, 0, len(a)+len(b))
	for i := 0; i < p; i++ {
		ops = append(ops, diffOp{'=', i, i})
	}
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case ma[i] == mb[j]:
			ops = append(ops, diffOp{'=', p + i, p + j})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, diffOp{'-', p + i, 0})
			i++
		default:
			ops = append(ops, diffOp{'+', 0, p + j})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{'-', p + i, 0})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{'+', 0, p + j})
	}
	for k := 0; k < s; k++ {
		ops = append(ops, diffOp{'=', len(a) - s + k, len(b) - s + k})
	}
	return ops
}

// renderHunks writes @@ -l,c +l,c @@ hunks for the aligned ops, with
// diffContext lines of context and change groups closer than 2*diffContext
// equal lines merged into one hunk (standard unified-diff grouping).
func renderHunks(sb *strings.Builder, a, b []string, ops []diffOp) {
	// aOff[k]/bOff[k]: lines of a/b consumed by ops[:k] — turns an op index
	// into 0-based line positions for the @@ header.
	aOff := make([]int, len(ops)+1)
	bOff := make([]int, len(ops)+1)
	for k, op := range ops {
		aOff[k+1], bOff[k+1] = aOff[k], bOff[k]
		if op.tag != '+' {
			aOff[k+1]++
		}
		if op.tag != '-' {
			bOff[k+1]++
		}
	}

	emit := func(prefix byte, line string) {
		sb.WriteByte(prefix)
		sb.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			sb.WriteString("\n\\ No newline at end of file\n")
		}
	}

	for k := 0; k < len(ops); {
		if ops[k].tag == '=' {
			k++
			continue
		}
		// Grow the change group: absorb following changes while the run of
		// equal lines between them is short enough that hunks would touch.
		last := k
		next := k + 1
		for next < len(ops) {
			if ops[next].tag != '=' {
				last = next
				next++
				continue
			}
			run := next
			for run < len(ops) && ops[run].tag == '=' {
				run++
			}
			if run < len(ops) && run-next <= 2*diffContext {
				next = run
				continue
			}
			break
		}
		lo := max(k-diffContext, 0)
		hi := min(last+diffContext+1, len(ops))

		aCount := aOff[hi] - aOff[lo]
		bCount := bOff[hi] - bOff[lo]
		aStart := aOff[lo] + 1
		if aCount == 0 {
			aStart = aOff[lo]
		}
		bStart := bOff[lo] + 1
		if bCount == 0 {
			bStart = bOff[lo]
		}
		fmt.Fprintf(sb, "@@ -%d,%d +%d,%d @@\n", aStart, aCount, bStart, bCount)
		for _, op := range ops[lo:hi] {
			switch op.tag {
			case '=':
				emit(' ', a[op.ai])
			case '-':
				emit('-', a[op.ai])
			case '+':
				emit('+', b[op.bi])
			}
		}
		k = hi
	}
}
