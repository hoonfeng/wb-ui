package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"wb-ui/rendering"
	"wb-ui/platform/graphics"
	"wb-ui/webkit"
)

func main() {
	distDir := `F:\syproject\gou-ide\cmd\desktop\web-ui\dist`
	absDist, _ := filepath.Abs(distDir)

	htmlData, err := os.ReadFile(filepath.Join(distDir, "index.html"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ReadFile: %v\n", err)
		os.Exit(1)
	}

	wv := webkit.NewWebView()
	mf := wv.MainFrame()
	if mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.ScriptLoader = func(src string) (string, error) {
				p := filepath.Join(absDist, strings.TrimPrefix(strings.TrimPrefix(src, "file://"), "./"))
				data, _ := os.ReadFile(p)
				return string(data), nil
			fr.StyleSheetLoader = func(href string) (string, error) {
				p := filepath.Join(absDist, strings.TrimPrefix(strings.TrimPrefix(href, "file://"), "./"))
				data, _ := os.ReadFile(p)
				// Remove Vue scoped [data-v-XXXXXXXX] selectors
				re := regexp.MustCompile(`\[data-v-[a-f0-9]+\]`)
				return re.ReplaceAllString(string(data), ""), nil
			}
				return string(data), nil
			}
		}
	}

	// Pre-polyfill
	wv.EvalJS(`if(typeof TextEncoder==='undefined'){TextEncoder=function(){this.encode=function(s){var arr=new Uint8Array(s.length);for(var i=0;i<s.length;i++)arr[i]=s.charCodeAt(i);return arr;}}}`)
	wv.EvalJS(`if(typeof TextDecoder==='undefined'){TextDecoder=function(){this.decode=function(arr){var s='';for(var i=0;i<arr.length;i++)s+=String.fromCharCode(arr[i]);return s;}}}`)
	wv.EvalJS(`if(typeof structuredClone==='undefined'){structuredClone=function(obj){return JSON.parse(JSON.stringify(obj))}}`)

	if err := wv.LoadHTML(string(htmlData)); err != nil {
		fmt.Fprintf(os.Stderr, "LoadHTML: %v\n", err)
		os.Exit(1)
	}

	// Multiple layout passes for Vue mount
	for i := 0; i < 5; i++ {
		wv.EnsureLayout()
		wv.RebuildRenderTree()
	}
	wv.EnsureLayout()
	wv.RebuildRenderTree()

	// Render to PNG
	canvas := graphics.NewCanvas(1280, 800)
	defer canvas.Release()

	rv := wv.RenderView()
	if rv == nil {
		fmt.Println("ERROR: RenderView is nil")
		os.Exit(1)
	}
	rv.SetViewportSize(1280, 800)
	rv.Layout(nil)

	rendering.Paint(rv, canvas, rendering.Rect{X: 0, Y: 0, Width: 1280, Height: 800})

	outPath := filepath.Join(distDir, "..", "..", "screenshots", "wbui_headless_vue2.png")
	savePNG(canvas, outPath)
	fmt.Println("PNG saved to:", outPath)

	// Pixel grid analysis
	fmt.Println("\n=== PIXEL GRID (11x11) ===")
	cw, ch := canvas.Width(), canvas.Height()
	for sy := 0; sy < 11; sy++ {
		y := sy * ch / 11
		line := ""
		for sx := 0; sx < 11; sx++ {
			x := sx * cw / 11
			p := canvas.PixelAt(x, y)
			line += fmt.Sprintf("(%3d,%3d)#%02x%02x%02x ", x, y, p.R, p.G, p.B)
		}
		fmt.Println(line)
	}

	fmt.Println("\n=== SPOT CHECK ===")
	areas := []struct{ x, y int }{
		{10, 10}, {10, 50}, {70, 50}, {200, 50},
		{640, 400}, {1270, 790}, {70, 100}, {300, 100},
	}
	for _, a := range areas {
		p := canvas.PixelAt(a.x, a.y)
		fmt.Printf("  (%4d,%4d) #%02x%02x%02x a=%d\n", a.x, a.y, p.R, p.G, p.B, p.A)
	}

	// Console output
	if out := wv.ConsoleOutput(); out != "" {
		fmt.Println("\n[CONSOLE]\n" + out)
	}
}

func savePNG(canvas *graphics.Canvas, path string) {
	pixels := canvas.Pixels()
	w, h := canvas.Width(), canvas.Height()
	if len(pixels) < w*h*4 {
		return
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, pixels[:w*h*4])
	f, _ := os.Create(path)
	defer f.Close()
	png.Encode(f, img)
}
