package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"wb-ui/dom"
	"wb-ui/webkit"
)

func main() {
	distDir := `F:\syproject\gou-ide\cmd\desktop\web-ui\dist`
	absDist, _ := filepath.Abs(distDir)
	htmlData, _ := os.ReadFile(filepath.Join(distDir, "index.html"))

	wv := webkit.NewWebView()
	mf := wv.MainFrame()
	if mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.ScriptLoader = func(src string) (string, error) {
				clean := strings.TrimPrefix(src, "file://")
				clean = strings.TrimPrefix(clean, "/")
				clean = strings.TrimPrefix(clean, "./")
				data, err := os.ReadFile(filepath.Join(absDist, clean))
				return string(data), err
			}
			fr.StyleSheetLoader = func(href string) (string, error) {
				clean := strings.TrimPrefix(href, "file://")
				clean = strings.TrimPrefix(clean, "/")
				clean = strings.TrimPrefix(clean, "./")
				data, err := os.ReadFile(filepath.Join(absDist, clean))
				if err != nil {
					data2, _ := os.ReadFile(filepath.Join(absDist, "assets", clean))
					return string(data2), nil
				}
				re := regexp.MustCompile(`\[data-v-[a-f0-9]+\]`)
				return re.ReplaceAllString(string(data), ""), nil
			}
		}
	}

	wv.EvalJS(`if(typeof TextEncoder==='undefined')TextEncoder=function(){this.encode=function(s){var a=new Uint8Array(s.length);for(var i=0;i<s.length;i++)a[i]=s.charCodeAt(i);return a}}`)
	wv.EvalJS(`if(typeof structuredClone==='undefined')structuredClone=function(o){return JSON.parse(JSON.stringify(o))}`)

	if err := wv.LoadHTML(string(htmlData)); err != nil {
		fmt.Fprintf(os.Stderr, "LoadHTML: %v\n", err)
	}

	// Register DOM bindings
	doc := mf.Document()
	fmt.Printf("=== DOM Tree from Go ===\n")
	fmt.Printf("document.Body()=%v\n", doc.Body())
	body := doc.Body()
	if body != nil {
		fmt.Printf("body.TagName()=%s\n", body.TagName())
		fmt.Printf("body.LocalName()=%s\n", body.LocalName())
		fmt.Printf("body.FirstChild()=%v\n", body.FirstChild())
		fmt.Printf("body.HasChildNodes()=%v\n", body.HasChildNodes())
		fc := body.FirstChild()
		if fc != nil {
			if el, ok := fc.(*dom.Element); ok {
				fmt.Printf("  firstChild.TagName()=%s id=%s\n", el.TagName(), el.GetAttribute("id"))
			}
		}
		// Walk all children
		n := 0
		for c := body.FirstChild(); c != nil; c = c.NextSibling() {
			n++
			if el, ok := c.(*dom.Element); ok {
				fmt.Printf("  child[%d] tag=%s id=%s\n", n, el.TagName(), el.GetAttribute("id"))
			} else {
				fmt.Printf("  child[%d] type=%T\n", n, c)
			}
		}
		fmt.Printf("  total children: %d\n", n)
	}

	// Check JS side
	fmt.Printf("\n=== From JS ===\n")
	result, _ := wv.EvalJS(`(function(){
		var b = document.body;
		if (!b) return 'body is null';
		return 'body.tagName=' + b.tagName + 
			' firstChild=' + b.firstChild + 
			' hasChildNodes=' + b.hasChildNodes() +
			' childNodes.length=' + b.childNodes.length;
	})()`)
	fmt.Printf("JS: %v\n", result)
	fmt.Printf("Console:\n%s\n", wv.ConsoleOutput())
}
