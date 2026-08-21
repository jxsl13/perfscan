package ps5109dot

import . "path"

func join(a, b, c string) string {
	return Join(Join(a, b), c)
}
