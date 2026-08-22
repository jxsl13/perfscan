package ps2031adv

// This file sees a bytes.Buffer only through the package-level shared
// variable and has NO import of package bytes, so the fix has no
// qualifier to name bytes.Equal with — advisory.
func noImport() {
	_ = shared.String() == "OK" // want `no usable import of package bytes at this position .* the automatic fix is withheld`
}
