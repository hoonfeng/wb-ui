package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"wb-ui/bindings"
	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/rendering"
	"wb-ui/style"
	"wb-ui/webkit"
)

func main() {
	distDir := `F:\syproject\gou-ide\cmd\desktop\web-ui\dist`
	absDist, _ := filepath.Abs(distDir)
	htmlData, err := os.ReadFile(filepath.Join(distDir, "index.html"))
	if err != nil {
		panic(fmt.Sprintf("read html: %v", err))
	}

	// ─── 1. Create WebView, set loaders, load HTML ───
	wv := webkit.NewWebView()
	mf := wv.MainFrame()
	if mf == nil {
		panic("no MainFrame")
	}
	fr := mf.Frame()
	if fr == nil {
		panic("no Frame")
	}
	fr.ScriptLoader = func(src string) (string, error) {
		clean := strings.TrimPrefix(src, "file://")
		clean = strings.TrimPrefix(clean, "/")
		data, err := os.ReadFile(filepath.Join(absDist, clean))
		return string(data), err
	}
	fr.StyleSheetLoader = func(href string) (string, error) {
		clean := strings.TrimPrefix(href, "file://")
		clean = strings.TrimPrefix(clean, "/")
		data, err := os.ReadFile(filepath.Join(absDist, clean))
		if err != nil {
			data2, _ := os.ReadFile(filepath.Join(absDist, "assets", clean))
			return string(data2), nil
		}
		re := regexp.MustCompile(`\[data-v-[a-f0-9]+\]`)
		return re.ReplaceAllString(string(data), ""), nil
	}

	wv.EvalJS(`if(typeof TextEncoder==='undefined')TextEncoder=function(){this.encode=function(s){var a=new Uint8Array(s.length);for(var i=0;i<s.length;i++)a[i]=s.charCodeAt(i);return a}}`)
	wv.EvalJS(`if(typeof structuredClone==='undefined')structuredClone=function(o){return JSON.parse(JSON.stringify(o))}`)

	if err := wv.LoadHTML(string(htmlData)); err != nil {
		panic(fmt.Sprintf("LoadHTML: %v", err))
	}

	// Register DOM bindings + execute scripts
	rt := wv.JSInterpreter()
	doc := mf.Document()
	bindings.RegisterDOMBindings(rt, doc)
	fr.ExecuteScripts()

	// ─── 2. Rebuild render tree ───
	fr.RebuildRenderTree()
	resolver := fr.Resolver()
	rv := fr.RenderView()
	if rv == nil {
		panic("no RenderView")
	}

	// ─── DIAG A: DOM tree ───
	fmt.Println("==================== A: DOM Tree ====================")
	dumpDOM(doc.DocumentElement(), 0)

	// ─── DIAG B: Render Tree (post-rebuild) ───
	fmt.Println("\n==================== B: Render Tree ====================")
	dumpRenderTree(rv, 0)

	// ─── DIAG C: Layout (1280x800 viewport, matching desktop) ───
	fmt.Println("\n==================== C: Layout ====================")
	rv.SetViewportSize(1280, 800)
	rv.Layout(nil)
	fmt.Println("Layout done. Layout boxes:")
	dumpLayoutBoxes(rv, 0)

	// ─── DIAG D: Computed styles ───
	fmt.Println("\n==================== D: Computed Styles ====================")
	body := doc.Body()
	if body != nil {
		dumpComputed(body, resolver, 0)
	} else {
		fmt.Println("body is nil!")
	}

	// ─── DIAG E: Custom properties on :root ───
	fmt.Println("\n==================== E: CSS Custom Properties (:root) ====================")
	htmlEl := doc.DocumentElement()
	if htmlEl != nil {
		cs := resolver.ResolveElement(htmlEl)
		if cs != nil {
			fmt.Printf("CustomProperties count: %d\n", len(cs.CustomProperties))
			count := 0
			for k, v := range cs.CustomProperties {
				if count < 20 {
					fmt.Printf("  %s = %s\n", k, v)
				}
				count++
			}
			if count > 20 {
				fmt.Printf("  ... and %d more\n", count-20)
			}
		}
	}

	// ─── DIAG F: Paint → headless PNG ───
	fmt.Println("\n==================== F: Paint ====================")
	canvas := graphics.NewCanvas(1280, 800)
	defer canvas.Release()

	// Find body bg color
	bg := findBodyBgColor(rendering.RenderObject(rv))
	fmt.Printf("body bg: #%02x%02x%02x a=%d\n", bg.R, bg.G, bg.B, bg.A)
	canvas.Clear(bg)

	rendering.Paint(rv, canvas, rendering.Rect{X: 0, Y: 0, Width: 1280, Height: 800})

	outPath := "F:\\syproject\\gou-ide\\screenshots\\wbui_pipeline.png"
	savePNG(canvas, outPath)
	fmt.Printf("Saved: %s\n", outPath)

	// ─── DIAG G: Pixel grid sample ───
	fmt.Println("\n==================== G: Pixel Grid (7x7) ====================")
	cw, ch := canvas.Width(), canvas.Height()
	for sy := 0; sy < 7; sy++ {
		y := sy * ch / 7
		line := fmt.Sprintf("  y=%4d:", y)
		for sx := 0; sx < 7; sx++ {
			x := sx * cw / 7
			p := canvas.PixelAt(x, y)
			line += fmt.Sprintf(" #%02x%02x%02x", p.R, p.G, p.B)
		}
		fmt.Println(line)
	}

	// Console
	if out := wv.ConsoleOutput(); out != "" {
		fmt.Printf("\n==================== Console ====================\n%s\n", out)
	}
}

func dumpDOM(el *dom.Element, depth int) {
	if el == nil {
		return
	}
	pfx := strings.Repeat("  ", depth)
	tag := el.TagName()
	id := el.GetAttribute("id")
	cls := el.GetAttribute("class")
	extra := ""
	if id != "" {
		extra += "#" + id
	}
	if cls != "" {
		if len(cls) > 40 {
			cls = cls[:40]
		}
		extra += "." + cls
	}
	fmt.Printf("%s<%s%s>\n", pfx, tag, extra)
	for c := el.FirstChild(); c != nil; c = c.NextSibling() {
		if childEl, ok := c.(*dom.Element); ok {
			dumpDOM(childEl, depth+1)
		}
	}
}

func dumpRenderTree(ro rendering.RenderObject, depth int) {
	if ro == nil {
		return
	}
	pfx := strings.Repeat("  ", depth)
	info := ""
	box := asRenderBox(ro)
	if box != nil {
		fr := box.FrameRect()
		info = fmt.Sprintf(" frame=(%.0f,%.0f %.0fx%.0f)", fr.X, fr.Y, fr.Width, fr.Height)
	}
	node := ro.Node()
	if node != nil {
		if el, ok := node.(*dom.Element); ok {
			id := el.GetAttribute("id")
			cls := el.GetAttribute("class")
			tag := el.TagName()
			extra := ""
			if id != "" {
				extra += "#" + id
			}
			if cls != "" {
				if len(cls) > 35 {
					cls = cls[:35]
				}
				extra += "." + cls
			}
			fmt.Printf("%s[%s] <%s>%s\n", pfx, ro.RenderName(), tag+extra, info)
		} else if _, ok := node.(*dom.Text); ok {
			t := node.(*dom.Text)
			txt := t.Data()
			if len(txt) > 25 {
				txt = txt[:25] + "..."
			}
			fmt.Printf("%s[%s] Text(%q)%s\n", pfx, ro.RenderName(), txt, info)
		} else {
			fmt.Printf("%s[%s] %T%s\n", pfx, ro.RenderName(), node, info)
		}
	} else {
		fmt.Printf("%s[%s] (anon)%s\n", pfx, ro.RenderName(), info)
	}
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		dumpRenderTree(c, depth+1)
	}
}

func dumpLayoutBoxes(ro rendering.RenderObject, depth int) {
	if ro == nil {
		return
	}
	pfx := strings.Repeat("  ", depth)
	box := asRenderBox(ro)
	if box != nil {
		fr := box.FrameRect()
		st := box.Style()
		bgStr := ""
		if st != nil {
			bgStr = fmt.Sprintf(" bg=#%02x%02x%02x a=%d disp=%s",
				st.BackgroundColor.R, st.BackgroundColor.G, st.BackgroundColor.B, st.BackgroundColor.A,
				st.Display,
			)
		}
		node := ro.Node()
		tag := "anon"
		if node != nil {
			if el, ok := node.(*dom.Element); ok {
				tag = el.TagName()
				if id := el.GetAttribute("id"); id != "" {
					tag += "#" + id
				}
			} else if _, ok := node.(*dom.Text); ok {
				tag = "Text"
			}
		}
		fmt.Printf("%s<%s> (%.0f,%.0f) %.0fx%.0f%s\n", pfx, tag, fr.X, fr.Y, fr.Width, fr.Height, bgStr)
	}
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		dumpLayoutBoxes(c, depth+1)
	}
}

func dumpComputed(el *dom.Element, resolver *style.Resolver, depth int) {
	if el == nil || resolver == nil {
		return
	}
	pfx := strings.Repeat("  ", depth)
	cs := resolver.ResolveElement(el)
	if cs != nil {
		fmt.Printf("%s<%s> color=#%02x%02x%02x bg=#%02x%02x%02x a=%d disp=%s\n",
			pfx, el.TagName(),
			cs.Color.R, cs.Color.G, cs.Color.B,
			cs.BackgroundColor.R, cs.BackgroundColor.G, cs.BackgroundColor.B, cs.BackgroundColor.A,
			cs.Display,
		)
	}
	for c := el.FirstChild(); c != nil; c = c.NextSibling() {
		if childEl, ok := c.(*dom.Element); ok {
			dumpComputed(childEl, resolver, depth+1)
		}
	}
}

func findBodyBgColor(ro rendering.RenderObject) graphics.Color {
	if ro == nil {
		return graphics.Color{}
	}
	if n := ro.Node(); n != nil {
		if el, ok := n.(*dom.Element); ok && strings.EqualFold(el.TagName(), "body") {
			if st := ro.Style(); st != nil && st.BackgroundColor.A > 0 {
				return graphics.Color{R: st.BackgroundColor.R, G: st.BackgroundColor.G, B: st.BackgroundColor.B, A: st.BackgroundColor.A}
			}
		}
	}
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		if col := findBodyBgColor(c); col.A > 0 {
			return col
		}
	}
	return graphics.Color{}
}

func asRenderBox(ro rendering.RenderObject) *rendering.RenderBox {
	if ro == nil {
		return nil
	}
	// Direct type assertion
	if box, ok := ro.(*rendering.RenderBox); ok {
		return box
	}
	// Embedded chain: RenderBlockFlow/RenderBlock embed RenderBox
	if bf, ok := ro.(*rendering.RenderBlockFlow); ok {
		return &bf.RenderBox
	}
	if b, ok := ro.(*rendering.RenderBlock); ok {
		return &b.RenderBox
	}
	if rv, ok := ro.(*rendering.RenderView); ok {
		return &rv.RenderBox
	}
	return nil
}

func savePNG(canvas *graphics.Canvas, path string) {
	pixels := canvas.Pixels()
	w, h := canvas.Width(), canvas.Height()
	if len(pixels) < w*h*4 {
		fmt.Printf("savePNG: pixels too short (%d < %d*%d*4=%d)\n", len(pixels), w, h, w*h*4)
		return
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, pixels[:w*h*4])
	f, err := os.Create(path)
	if err != nil {
		fmt.Printf("savePNG: create %s: %v\n", path, err)
		return
	}
	defer f.Close()
	png.Encode(f, img)
	fmt.Printf("savePNG: %s (%dx%d, %d bytes)\n", path, w, h, w*h*4)
}
