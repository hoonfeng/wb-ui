package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	path := `F:\syproject\wb-ui\rendering\renderformcontrol.go`
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	content := string(data)
	// Print the hex around "Truncate" to see exact whitespace
	idx := strings.Index(content, "Truncate rune")
	if idx < 0 {
		fmt.Println("FAIL: Truncate not found")
		os.Exit(1)
	}
	start := idx - 40
	if start < 0 {
		start = 0
	}
	end := idx + 100
	if end > len(content) {
		end = len(content)
	}
	fmt.Printf("Context around Truncate:\n%q\n", content[start:end])
}
