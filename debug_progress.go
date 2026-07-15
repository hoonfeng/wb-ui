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
	idx := strings.Index(content, "func paintProgressBar")
	if idx < 0 {
		fmt.Println("FAIL: not found")
		os.Exit(1)
	}
	end := idx + 1200
	if end > len(content) {
		end = len(content)
	}
	fmt.Println(content[idx:end])
}
