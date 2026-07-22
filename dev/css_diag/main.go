package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wb-ui/css"
	"wb-ui/style"
	"wb-ui/dom"
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
			}
			fr.StyleSheetLoader = func(href string) (string, error) {
				p := filepath.Join(absDist, strings.TrimPrefix(strings.TrimPrefix(href, "file://"), "./"))
				data, _ := os.ReadFile(p)
				return string(data), nil
			}
		}
	}

	wv.EvalJS(`if(typeof TextEncoder==='undefined'){TextEncoder=function(){this.encode=function(s){var arr=new Uint8Array(s.length);for(var i=0;i<s.length;i++)arr[i]=s.charCodeAt(i);return arr;}}}`)

	if err := wv.LoadHTML(string(htmlData)); err != nil {
		fmt.Fprintf(os.Stderr, "LoadHTML: %v\n", err)
		os.Exit(1)
	}
	for i := 0; i < 5; i++ {
		wv.EnsureLayout()
		wv.RebuildRenderTree()
	}

	// Access resolver directly from page.frame.resolver
	_ = mf
	// We can't access the resolver directly. Let's use the document.
	doc := mf.Frame().Document()

	// Count total elements with class attributes
	totalEls := 0
	withClass := 0
	withDataV := 0
	walkElements(doc, func(el *dom.Element) {
		totalEls++
		if el.ClassName() != "" {
			withClass++
		}
		if el.HasAttribute("data-v-245f3b5c") || el.HasAttribute("data-v-90f89230") {
			withDataV++
		}
	})
	fmt.Printf("Total elements: %d, with class: %d, with data-v: %d\n", totalEls, withClass, withDataV)

	// JS-side: list all elements with className
	r, _ := wv.EvalJS(`(function(){
		var r=[];
		var all=document.querySelectorAll('*');
		for(var i=0;i<Math.min(all.length,20);i++){
			var el=all[i];
			var info=el.tagName;
			if(el.className)info+='.'+el.className;
			if(el.id)info+='#'+el.id;
			var dv=el.getAttribute('data-v-245f3b5c');
			if(dv)info+='[data-v-245f3b5c]';
			r.push(info);
		}
		return JSON.stringify(r);
	})()`)
	fmt.Println("First 20 elements:", r.AsString())

	// Verify: does .menubar element have data-v-245f3b5c?
	r2, _ := wv.EvalJS(`(function(){
		var mb=document.querySelector('.menubar');
		if(!mb)return'no-menubar';
		var attrs=[];
		for(var i=0;i<mb.attributes.length;i++){
			attrs.push(mb.attributes[i].name+'='+mb.attributes[i].value);
		}
		return JSON.stringify(attrs);
	})()`)
	fmt.Println(".menubar attributes:", r2.AsString())

	// Parse external CSS and list the first few selectors
	extCss, _ := os.ReadFile(filepath.Join(absDist, "assets", "style-RwIVaaFr.css"))
	sheet := css.NewCSSStyleSheet()
	css.NewParser(string(extCss)).ParseStyleSheetInto(sheet)
	fmt.Println("\nFirst 10 external CSS selectors:")
	count := 0
	for _, rule := range sheet.Rules() {
		if sr, ok := rule.(*css.StyleRule); ok {
			fmt.Printf("  %s\n", sr.Selectors.String())
			count++
			if count >= 10 {
				break
			}
		}
	}
}

func walkElements(n dom.Node, fn func(*dom.Element)) {
	if n == nil { return }
	if el, ok := n.(*dom.Element); ok {
		fn(el)
		for _, c := range el.ChildNodes() {
			walkElements(c, fn)
		}
	}
}

// Initialize CSS
func init() {
	_ = style.NewResolver
}
