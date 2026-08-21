package ps5111alias

import p "path"

func canonical(name string) string {
	return p.Clean(p.Clean(p.Dir(name))) // want `path.Clean rescans the canonical nonempty result of path.Dir through 2 redundant Clean layer`
}
