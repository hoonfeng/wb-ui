package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	lines := strings.Split(string(data), "\n")
	count := 0
	layoutStart := -1
	for i, line := range lines {
		opens := strings.Count(line, "{")
		closes := strings.Count(line, "}")
		count += opens - closes

		if strings.Contains(line, "func (c *FlexFormattingContext) Layout") {
			layoutStart = count
			fmt.Printf("Line %d: Layout START, count=%d\n", i+1, count)
		}
		if count < 0 {
			fmt.Printf("Line %d: BRACE NEGATIVE: %s  count=%d\n", i+1, strings.TrimSpace(line), count)
		}
	}
	fmt.Printf("Final count: %d\n", count)
}
