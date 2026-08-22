package ps5083dot

import . "bytes"

func dotImportStaysSilent(data []byte) int {
	return len(Clone(data))
}
