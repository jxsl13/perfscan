package checks

// ps6136Residency describes geometry only. Callers must independently prove
// ownership, complete logical-use coverage and the reviewed workload policy.
// Numeric profile values are not inferred runtime dimensions.
type ps6136Residency struct {
	retained ps6125Extent
	common   ps6125Extent
	rows     *ps6125ExtentIdentity
	width    *ps6125ExtentIdentity
}

func ps6136OutputResidency(rows, width *ps6125ExtentIdentity) (ps6136Residency, bool) {
	if rows == nil || width == nil || rows == width || rows.pool != width.pool {
		return ps6136Residency{}, false
	}
	root := func(identity *ps6125ExtentIdentity) *ps6125ExtentIdentity {
		for identity.parent != nil {
			identity = identity.parent
		}
		return identity
	}
	if root(rows) != root(width) {
		return ps6136Residency{}, false
	}
	common := ps6125MultiplyExtents(ps6125ConstantExtent(4), ps6125SymbolicExtent(width))
	retained := ps6125MultiplyExtents(common, ps6125SymbolicExtent(rows))
	return ps6136Residency{retained: retained, common: common, rows: rows, width: width}, retained.known
}

func (r ps6136Residency) evaluate(rows, width, limit int64) (retained, common, idle, amplification int64, known bool) {
	if rows <= 1 || width <= 0 || r.rows == nil || r.width == nil {
		return 0, 0, 0, 0, false
	}
	dimensions := map[*ps6125ExtentIdentity]int64{r.rows: rows, r.width: width}
	retained, valid := ps6125EvaluateExtent(r.retained, dimensions, limit)
	if !valid {
		return 0, 0, 0, 0, false
	}
	common, valid = ps6125EvaluateExtent(r.common, dimensions, limit)
	if !valid || retained <= common {
		return 0, 0, 0, 0, false
	}
	return retained, common, retained - common, rows, true
}
