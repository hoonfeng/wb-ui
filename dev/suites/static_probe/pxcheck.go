// Command pxcheck reads a specific pixel from a rendered PNG (for probe
// validation): button interior should be transparent (activity-bar bg shows).
//go:build ignore

package main

import (
	"fmt"
	"image/png"
	"log"
	"os"
)

func main() {
	log.SetFlags(0)
	wd, _ := os.Getwd()
	path := wd + "\\dev\\static_probe\\desc_test.png"
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		log.Fatal(err)
	}
	// Button b1: .activity-bar at (0,0) 48x200, button 40x40 at top.
	pts := [][2]int{{10, 10}, {20, 20}, {10, 100}, {5, 5}, {30, 30}}
	for _, p := range pts {
		r, g, b, _ := img.At(p[0], p[1]).RGBA()
		fmt.Printf("(%3d,%3d) #%02x%02x%02x\n", p[0], p[1], r>>8, g>>8, b>>8)
	}
}
