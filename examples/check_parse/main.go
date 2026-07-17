package main

import (
	"fmt"
	"wb-ui/jsc"
)

func main() {
	tests := []string{
		`let o = 1, [a, b] = [2, 3], c = 4;`,
		`let x = f(1,2,3), [y,z] = arr;`,
		`for(const [n,i] of Object.entries(e)){}`,
	}
	for _, src := range tests {
		rt := jsc.NewInterpreter()
		_, err := rt.Run(src)
		if err != nil {
			fmt.Printf("FAIL: %q\n  %v\n", src, err)
		} else {
			fmt.Printf("OK: %q\n", src)
		}
	}
}
