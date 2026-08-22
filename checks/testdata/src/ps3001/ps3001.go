package ps3001

import "fmt"

func parse(lines []string) (int, int) {
	var a, b int
	for _, line := range lines {
		fmt.Sscanf(line, "%d %d", &a, &b) // want `fmt\.Sscanf in a loop pays format parsing and reflection per iteration; use strconv or manual parsing`
	}
	return a, b
}

func once(line string) (int, int) {
	var a, b int
	fmt.Sscanf(line, "%d %d", &a, &b)
	return a, b
}
