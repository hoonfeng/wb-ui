package main

import (
	"fmt"
	"os"
	"wb-ui/jsc"
)

func main() {
	// Test 1: Parse hole patterns
	tests := []string{
		`let o = 1, [,a,b] = [10,20,30], c = 4;`,
		`const [,l,h,u,d] = [0,1,2,3,4];`,
		`let [,a,,b] = [1,2,3,4];`,
	}
	for _, src := range tests {
		rt := jsc.NewInterpreter()
		_, err := rt.Run(src)
		if err != nil {
			fmt.Printf("HOLE TEST FAIL: %q\n  %v\n", src, err)
		} else {
			fmt.Printf("HOLE OK: %q\n", src)
		}
	}

	// Test 2: Parse the full Vue bundle
	path := "F:\\syproject\\gou-ide\\cmd\\desktop\\web-ui\\dist\\assets\\index-1iBlH2r-.js"
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read error: %v\n", err)
		os.Exit(1)
	}
	src := string(data)
	fmt.Printf("Parsing %d bytes from %s...\n", len(data), path)
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
			start := pos - 120; if start < 0 { start = 0 }
			end := pos + 120; if end > len(src) { end = len(src) }
			fmt.Printf("PARSE ERROR at line %d col %d (pos ~%d):\n", line, col, pos)
			fmt.Printf(">>> BEFORE:\n%s\n>>> HERE:\n%s\n>>> AFTER:\n%s\n", src[start:pos], src[pos:pos+1], src[pos+1:end])
		} else {
			fmt.Printf("PARSE ERROR: %s\n", errStr)
		}
		os.Exit(1)
	}
	fmt.Println("PARSE OK - Full bundle parsed successfully!")
}
