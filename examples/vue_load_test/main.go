// Command vue_load_test loads a Vue 3 app bundle and runs it.
package main

import (
	"fmt"
	"os"

	"wb-ui/bindings"
	"wb-ui/dom"
	"wb-ui/jsc"
)

func main() {
	// 1. Create JS interpreter
	rt := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	rt.SetupGlobal(log)

	// 2. Create DOM
	doc := dom.NewDocument()
	bindings.RegisterDOMBindings(rt, doc)

	htmlEl := doc.CreateElement("html")
	doc.AppendChild(htmlEl)
	head := doc.CreateElement("head")
	htmlEl.AppendChild(head)
	body := doc.CreateElement("body")
	htmlEl.AppendChild(body)
	appEl := doc.CreateElement("div")
	appEl.SetId("app")
	body.AppendChild(appEl)

	// 3. Polyfills
	polyfills := []string{
		`window.process={env:{NODE_ENV:"production"}}`,
		`window.__VUE_OPTIONS_API__=true;window.__VUE_PROD_DEVTOOLS__=false`,
		`window.__VUE_PROD_HYDRATION_MISMATCH_DETAILS__=false`,
		`if(!Object.getOwnPropertyNames)Object.getOwnPropertyNames=function(o){if(!o)return[];var k=[];for(var n in o)k.push(n);return k}`,
		`if(!Object.fromEntries)Object.fromEntries=function(e){var r={};for(var i=0;e&&i<e.length;i++)if(e[i])r[e[i][0]]=e[i][1];return r}`,
		`if(!Array.prototype.flatMap)Array.prototype.flatMap=function(f){var r=[];for(var i=0;i<this.length;i++){var v=f(this[i],i,this);if(v&&v.length)for(var j=0;j<v.length;j++)r.push(v[j]);else r.push(v)}return r}`,
		`if(!Array.prototype.at)Array.prototype.at=function(i){var n=Number(i);if(isNaN(n))n=0;var l=this.length;n=n>=0?n:l+n;if(n<0||n>=l)return undefined;return this[n]}`,
		`if(!Object.isExtensible)Object.isExtensible=function(){return true}`,
		// 浏览器全局对象桩（供 Vue 引用，instanceof 暂不精确）
		`class Node{}`,
		`class Element extends Node{}`,
		`class Document extends Node{}`,
		`class Event{constructor(t,o){this.type=t}stopPropagation(){}preventDefault(){}}`,
		`class SVGElement extends Element{}`,
		`class SVGSVGElement extends SVGElement{}`,
		`class SVGPathElement extends SVGElement{}`,
		`class SVGCircleElement extends SVGElement{}`,
		`class SVGTextElement extends SVGElement{}`,
		`class HTMLElement{}`,
		`class HTMLDivElement extends HTMLElement{}`,
		`class HTMLSpanElement extends HTMLElement{}`,
		`class HTMLInputElement extends HTMLElement{}`,
		`class HTMLButtonElement extends HTMLElement{}`,
		`class HTMLAnchorElement extends HTMLElement{}`,
		`class HTMLImageElement extends HTMLElement{}`,
		`class HTMLFormElement extends HTMLElement{}`,
		`class HTMLTextAreaElement extends HTMLElement{}`,
		`class HTMLSelectElement extends HTMLElement{}`,
		`class HTMLOptionElement extends HTMLElement{}`,
		`class HTMLVideoElement extends HTMLElement{}`,
		`class HTMLCanvasElement extends HTMLElement{}`,
		`class HTMLScriptElement extends HTMLElement{}`,
		`class HTMLStyleElement extends HTMLElement{}`,
		`class HTMLLinkElement extends HTMLElement{}`,
		`class HTMLMetaElement extends HTMLElement{}`,
		`class HTMLHeadElement extends HTMLElement{}`,
		`class HTMLBodyElement extends HTMLElement{}`,
		`class HTMLHtmlElement extends HTMLElement{}`,
		`class HTMLTemplateElement extends HTMLElement{get content(){return document.createDocumentFragment()}}`,
		`class Comment{}`,
		`class CustomEvent{constructor(t,o){this.type=t;this.detail=o&&o.detail||null}}`,
		`class DOMRect{constructor(x=0,y=0,w=0,h=0){this.x=x;this.y=y;this.width=w;this.height=h;this.top=y;this.right=x+w;this.bottom=y+h;this.left=x}}`,
		`class MutationObserver{constructor(c){this.cb=c}observe(t,o){}disconnect(){}takeRecords(){return[]}}`,
		`class IntersectionObserver{constructor(c,o){}observe(t){}unobserve(t){}disconnect(){}takeRecords(){return[]}}`,
		`class ResizeObserver{constructor(c){}observe(t){}unobserve(t){}disconnect(){}}`,
		// Performance 桩
		`window.performance={now:function(){return Date.now()},mark:function(){},measure:function(){},getEntries:function(){return[]},getEntriesByType:function(){return[]}}`,
	}
	for _, p := range polyfills {
		if _, err := rt.Run(p); err != nil {
			fmt.Fprintf(os.Stderr, "polyfill error: %v\n", err)
			os.Exit(1)
		}
	}

	// 4. Load full Vue bundle
	bundlePath := `F:\syproject\gou-ide\cmd\desktop\web-ui-minimal\dist\assets\app-BzCHJTR_.js`
	data, err := os.ReadFile(bundlePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read bundle: %v\n", err)
		os.Exit(1)
	}
	code := string(data)

	// 5. Execute full bundle
	fmt.Printf("Bundle size: %d bytes\n", len(code))
	if _, err := rt.Run(code); err != nil {
		fmt.Fprintf(os.Stderr, "bundle execution error: %v\n", err)
		fmt.Fprintf(os.Stderr, "=== Console output ===\n%s\n", log.String())
		os.Exit(1)
	}

	// 6. Check result
	fmt.Println("\n=== Vue Bundle Load Test ===")
	fmt.Printf("#app children: %d\n", len(appEl.ChildNodes()))
	inner := appEl.GetInnerHTML()
	if len(inner) > 500 {
		inner = inner[:500]
	}
	fmt.Printf("#app innerHTML: %s\n", inner)
	fmt.Println("\n=== Console ===")
	fmt.Println(log.String())
	fmt.Println("\n=== SUCCESS ===")
}
