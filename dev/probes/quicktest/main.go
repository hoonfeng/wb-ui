package main

import (
	"fmt"
	"wb-ui/platform/graphics"
)

func main() {
	c := graphics.NewCanvas(400, 50)
	defer c.Release()

	// Test the default font (what the text actually uses)
	defaultFont := graphics.Font{Family: "sans-serif", Size: 14, Weight: 400, Style: "normal"}
	w3 := graphics.MeasureText(defaultFont, "...")
	w1 := graphics.MeasureText(defaultFont, "\u2026")

	// Also measure single period
	wDot := graphics.MeasureText(defaultFont, ".")

	// Draw all three for visual comparison
	c.DrawText(10, 20, "A...BC", defaultFont, graphics.Color{R: 255, G: 255, B: 255, A: 255})
	c.DrawText(10, 40, "A\u2026BC", defaultFont, graphics.Color{R: 255, G: 255, B: 255, A: 255})

	p1 := c.PixelAt(10, 20)
	p2 := c.PixelAt(40, 20)

	fmt.Printf("default font: '.'=%.1f '...'=%.1f '\u2026'=%.1f\n", wDot, w3, w1)
	fmt.Printf("pixels at (10,20)=#%02x%02x%02x (40,20)=#%02x%02x%02x\n", p1.R, p1.G, p1.B, p2.R, p2.G, p2.B)

	// Try monospace fallback
	mono := graphics.Font{Family: "Consolas", Size: 14, Weight: 400, Style: "normal"}
	wm3 := graphics.MeasureText(mono, "...")
	wm1 := graphics.MeasureText(mono, "\u2026")
	fmt.Printf("Consolas: '...'=%.1f '\u2026'=%.1f\n", wm3, wm1)
}
