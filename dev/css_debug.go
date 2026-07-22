package main

import (
	"fmt"

	"wb-ui/css"
)

func main() {
	p := css.NewParser("flex:1;background:red;width:1280px")
	decls := p.ParseDeclarationList()
	fmt.Println("Declarations:")
	for _, d := range decls {
		fmt.Printf("  %s: %s\n", d.Name, d.ValueString())
	}
}
