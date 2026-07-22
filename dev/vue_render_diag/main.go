package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"wb-ui/dom"
	"wb-ui/rendering"
	"wb-ui/webkit"
)

func main() {
	distDir := `F:\syproject\gou-ide\cmd\desktop\web-ui\dist`
	absDist, _ := filepath.Abs(distDir)

	wv := webkit.NewWebView()
	mf := wv.MainFrame()
	fr := mf.Frame()

	fr.ScriptLoader = func(src string) (string, error) {
		clean := strings.TrimPrefix(strings.TrimPrefix(src, "file://"), "./")
		data, err := os.ReadFile(filepath.Join(absDist, clean))
		return string(data), err
	}
	fr.StyleSheetLoader = func(href string) (string, error) {
		clean := strings.TrimPrefix(strings.TrimPrefix(href, "file://"), "./")
		data, _ := os.ReadFile(filepath.Join(absDist, clean))
		return string(data), nil
	}

	// Polyfills
	for _, p := range []string{
		`if(typeof TextEncoder=='undefined')TextEncoder=function(){this.encode=function(s){var a=new Uint8Array(s.length);for(var i=0;i<s.length;i++)a[i]=s.charCodeAt(i);return a}}`,
		`if(typeof TextDecoder=='undefined')TextDecoder=function(){this.decode=function(a){return String.fromCharCode.apply(null,a)}}`,
		`if(typeof structuredClone=='undefined')structuredClone=function(o){return JSON.parse(JSON.stringify(o))}`,
		`if(typeof crypto=='undefined')crypto={getRandomValues:function(arr){for(var i=0;i<arr.length;i++)arr[i]=Math.floor(Math.random()*256)}}`,
		`if(typeof fetch=='undefined')fetch=function(){return Promise.resolve({json:function(){return Promise.resolve({})},text:function(){return Promise.resolve('')}})}`,
		`if(typeof WebSocket=='undefined')WebSocket=function(){}`,
		`if(typeof CustomEvent=='undefined')CustomEvent=function(t,e){this.type=t||''}`,
		`if(typeof ResizeObserver=='undefined')ResizeObserver=function(){}`,
		`if(typeof MutationObserver=='undefined')MutationObserver=function(){}`,
		`if(typeof requestAnimationFrame=='undefined')requestAnimationFrame=function(fn){return setTimeout(fn,16)}`,
		`if(typeof Event=='undefined'){Event=function(t,e){this.type=t||'';for(var k in(e||{}))this[k]=e[k]};window.Event=Event}`,
		`if(!Array.from)Array.from=function(a,fn,ctx){var r=[];for(var i=0;i<a.length;i++)r.push(fn?fn.call(ctx||null,a[i],i):a[i]);return r}`,
		`if(typeof window.getSelection=='undefined')window.getSelection=function(){return{anchorNode:null,anchorOffset:0}}`,
	} {
		wv.EvalJS(p)
	}

	// Load HTML
	htmlData, _ := os.ReadFile(filepath.Join(distDir, "index.html"))
	s := strings.Replace(string(htmlData), `type="module"`, "", 1)
	s = strings.ReplaceAll(s, `crossorigin`, "")
	wv.LoadHTML(s)

	// Diagnostics
	rv := wv.RenderView()
	if rv == nil {
		fmt.Println("RenderView is nil!")
		return
	}

	fmt.Println("=== Render Tree ===")
	dumpRT(rv, 0)

	fmt.Println("\n=== Layout ===")
	ls := rv.LayoutState()
	if ls != nil {
		lb := rv.LayoutBox()
		if lb != nil {
			dumpLayout(lb, 0)
		}
	}

	// Paint to PNG
	canvas := rv.Paint()
	if canvas != nil {
		img := image.NewRGBA(image.Rect(0, 0, int(canvas.Width), int(canvas.Height)))
		pngPath := `F:\syproject\gou-ide\screenshots\wbui_external.png`
		f, _ := os.Create(pngPath)
		png.Encode(f, img)
		f.Close()
		fmt.Printf("\nSaved: %s\n", pngPath)
	}

	// Console
	if out := wv.ConsoleOutput(); out != "" {
		fmt.Printf("\n=== Console ===\n%s\n", out)
	}
}

func dumpRT(ro rendering.RenderObject, depth int) {
	if ro == nil {
		return
	}
	prefix := strings.Repeat("  ", depth)
	fr := ro.Frame()
	fmt.Printf("%s%s frame=(%.0f,%.0f %.0fx%.0f)\n", prefix, ro, fr.X, fr.Y, fr.Width, fr.Height)
	for _, c := range ro.Children() {
		dumpRT(c, depth+1)
	}
}

func dumpLayout(lb *dom.Element, depth int) {
	fmt.Printf("%s<%s> rect=(%.0f,%.0f %.0fx%.0f)\n",
		strings.Repeat("  ", depth),
		lb.LocalName(),
		// simplified
	)
}
