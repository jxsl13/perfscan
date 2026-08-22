package pkgshadow

// useAll exercises, from a DIFFERENT file than the declarations, every call
// shape the PkgFuncCall-based checks match — but each qualifier is the
// package-level shadow from decl.go, not the stdlib package. NOTHING here
// may be reported or rewritten (this file deliberately carries zero
// expectation comments): the methods have different semantics (Sprintf
// returns "CUSTOM"), and any injected import would collide with the
// package-level names.
func useAll(lines []string, xs []int, buf []byte) []string {
	var out []string
	for i, s := range lines {
		out = append(out, fmt.Sprintf("%d", i))      // NOT fmt.Sprintf: PS2107 must not fire
		out = append(out, fmt.Sprintf("%s%s", s, s)) // NOT fmt.Sprintf: PS2103 must not fire
		var n int
		fmt.Sscanf(s, "%d", &n)                             // NOT fmt.Sscanf: PS3001 must not fire
		out = append(out, strings.Replace(s, "a", "b", -1)) // NOT strings.Replace: PS2003 must not fire
		out = append(out, strings.Repeat(s, 2))             // NOT strings.Repeat: PS2003 must not fire
		re := regexp.MustCompile(`\d+`)                     // NOT regexp.MustCompile: PS2005 must not fire
		_ = re.MatchString(s)
		_ = math.Exp(float64(i))                 // NOT math.Exp: PS4002 must not fire
		_ = math.Min(math.Max(float64(n), 0), 1) // NOT a clamp: PS3077 must not fire
		_ = math.Min(float64(i), 2)              // NOT math.Min: PS3082 must not fire
		_ = binary.LittleEndian.Uint64(buf)      // NOT encoding/binary: PS4001 must not fire
	}
	sinV := math.Sin(float64(len(xs))) // NOT math.Sin/Cos: PS5008 must not fire
	cosV := math.Cos(float64(len(xs)))
	_ = sinV + cosV
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] }) // NOT sort.Slice: PS3002/PS3005 must not fire
	return out
}
