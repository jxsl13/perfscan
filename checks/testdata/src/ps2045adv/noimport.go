package ps2045adv

// This file sees the bytes.Buffers only through the package-level
// shared variables and has NO import of package bytes, so the fix has
// no qualifier to name bytes.Equal with — advisory.
func noImport() {
	_ = shared.String() == shared2.String() // want `no usable import of package bytes at this position .* the automatic fix is withheld`
}
