package ps2045adv

import . "bytes"

// A dot import gives no qualifier to name bytes.Equal with (the fix
// never emits an unqualified Equal) — advisory.
var dotBuf, dotBuf2 Buffer

func dotImported() {
	_ = dotBuf.String() == dotBuf2.String() // want `no usable import of package bytes at this position .* the automatic fix is withheld`
}
