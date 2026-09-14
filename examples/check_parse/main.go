package main

import (
	"fmt"
	"wb-ui/engine/js/jsc"
)

func main() {
	tests := []string{
		`class Foo {}; new Foo();`,
		`class Foo { constructor() { this.x = 42; } }; var f = new Foo(); f.x;`,
	}
	for _, src := range tests {
		rt := jsc.NewInterpreter()
		rt.SetupGlobal(nil)
		v, err := rt.Run(src)
		if err != nil {
			fmt.Printf("FAIL: %q\n  %v\n", src, err)
		} else {
			fmt.Printf("OK: %q => %v\n", src, v)
		}
	}
}
