package main

import (
	"fmt"
	"os"
	"wb-ui/jsc"
)

func main() {
	path := "F:\\syproject\\gou-ide\\cmd\\desktop\\web-ui\\dist\\assets\\index-1iBlH2r-.js"
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read error: %v\n", err)
		os.Exit(1)
	}
	src := string(data)
	fmt.Printf("Parsing %d bytes...\n", len(data))
	rt := jsc.NewInterpreter()
	_, err = rt.Run(src)
	if err != nil {
		errStr := fmt.Sprintf("%v", err)
		var line, col int
		if n, _ := fmt.Sscanf(errStr, "jsc parse error at line %d col %d:", &line, &col); n >= 2 {
			pos := 0
			currentLine := 1
			for i, b := range src {
				if b == '\n' { currentLine++ }
				if currentLine == line { pos = i + col; break }
			}
			start := pos - 60; if start < 0 { start = 0 }
			end := pos + 60; if end > len(src) { end = len(src) }
			fmt.Printf("PARSE ERROR at line %d (pos ~%d):\n%s<---HERE--->%s\n", line, pos, src[start:pos], src[pos:end])
		} else {
			fmt.Printf("Error: %s\n", errStr)
		}
		os.Exit(1)
	}
	fmt.Println("PARSE OK!")
}
