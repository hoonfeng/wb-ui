//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"
	"wb-ui/jsc"
)

func main() {
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read error: %v\n", err)
		os.Exit(1)
	}

	src := string(data)
	rt := jsc.NewInterpreter()
	_, err = rt.Run(src)
	if err != nil {
		fmt.Printf("PARSE ERROR:\n%v\n", err)
		os.Exit(1)
	}
	fmt.Println("PARSE OK")
}
