package ps5111dot

import . "path"

func excluded(name string) string { return Clean(Dir(name)) }
