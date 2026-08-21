package ps5114dot

import . "path/filepath"

func excluded(name string) string { return FromSlash(Clean(name)) }
