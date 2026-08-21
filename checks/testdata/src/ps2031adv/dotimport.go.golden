package ps2031adv

import . "bytes"

// A dot import gives no qualifier to name bytes.Equal with (the fix
// never emits an unqualified Equal) — advisory.
var dotBuf Buffer

func dotImported() {
	_ = dotBuf.String() == "OK" // want `no usable import of package bytes at this position .* the automatic fix is withheld`
}
