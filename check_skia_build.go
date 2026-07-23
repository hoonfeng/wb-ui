package main

import (
	"fmt"
	"os"

	"github.com/hoonfeng/goskia/skia"
)

func main() {
	skia.Init()
	s, err := skia.NewRasterSurfaceN32Premul(100, 100)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	defer s.Release()
	c := s.Canvas()
	c.Clear(skia.NewColor(255, 0, 0, 255))
	fmt.Println("OK")
}
