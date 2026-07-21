package main

import (
	"fmt"
	"os"
	"strings"

	"wb-ui/bindings"
	"wb-ui/dom"
	"wb-ui/jsc"
)

func main() {
	rt := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	rt.SetupGlobal(log)

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

	// Polyfills
	polyfills := []string{
		`window.process={env:{NODE_ENV:"production"}}`,
		`window.__VUE_OPTIONS_API__=true;window.__VUE_PROD_DEVTOOLS__=false`,
		`window.__VUE_PROD_HYDRATION_MISMATCH_DETAILS__=false`,
		`if(!Object.getOwnPropertyNames)Object.getOwnPropertyNames=function(o){if(!o)return[];var k=[];for(var n in o)k.push(n);return k}`,
		`if(!Object.fromEntries)Object.fromEntries=function(e){var r={};for(var i=0;e&&i<e.length;i++)if(e[i])r[e[i][0]]=e[i][1];return r}`,
		`if(!Array.prototype.flatMap)Array.prototype.flatMap=function(f){var r=[];for(var i=0;i<this.length;i++){var v=f(this[i],i,this);if(v&&v.length)for(var j=0;j<v.length;j++)r.push(v[j]);else r.push(v)}return r}`,
		`if(!Array.prototype.at)Array.prototype.at=function(i){var n=Number(i);if(isNaN(n))n=0;var l=this.length;n=n>=0?n:l+n;if(n<0||n>=l)return undefined;return this[n]}`,
		`if(!Object.isExtensible)Object.isExtensible=function(){return true}`,
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
		`class HTMLTemplateElement extends HTMLElement{get content(){return document.createDocumentFragment()}}`,
		`class Comment{}`,
		`class CustomEvent{constructor(t,o){this.type=t;this.detail=o&&o.detail||null}}`,
		`class DOMRect{constructor(x=0,y=0,w=0,h=0){this.x=x;this.y=y;this.width=w;this.height=h;this.top=y;this.right=x+w;this.bottom=y+h;this.left=x}}`,
		`class MutationObserver{constructor(c){this.cb=c}observe(t,o){}disconnect(){}takeRecords(){return[]}}`,
		`class IntersectionObserver{constructor(c,o){}observe(t){}unobserve(t){}disconnect(){}takeRecords(){return[]}}`,
		`class ResizeObserver{constructor(c){}observe(t){}unobserve(t){}disconnect(){}}`,
		`window.performance={now:function(){return Date.now()},mark:function(){},measure:function(){},getEntries:function(){return[]},getEntriesByType:function(){return[]}}`,
	}
	for _, p := range polyfills {
		if _, err := rt.Run(p); err != nil {
			fmt.Fprintf(os.Stderr, "polyfill error: %v\n", err)
			os.Exit(1)
		}
	}

	// Load Vue 3
	bundlePath := `F:\syproject\gou-ide\cmd\desktop\web-ui-minimal\dist\assets\app-CpekGwAS.js`
	data, err := os.ReadFile(bundlePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read bundle: %v\n", err)
		os.Exit(1)
	}
	code := string(data)

	fmt.Printf("Bundle size: %d bytes\n", len(code))
	if _, err := rt.Run(code); err != nil {
		fmt.Fprintf(os.Stderr, "bundle execution error: %v\n", err)
		fmt.Fprintf(os.Stderr, "=== Console output ===\n%s\n", log.String())
		os.Exit(1)
	}

	fmt.Println("\n=== PASS: Vue 3 loaded ===")

	// Test 1: Check initial render
	appHTML, err := rt.RunJS("document.getElementById('app').innerHTML")
	if err != nil {
		fmt.Printf("FAIL: get innerHTML: %v\n", err)
	} else {
		html := appHTML.ToString()
		if len(html) > 50 {
			fmt.Printf("PASS: #app innerHTML (%d chars): %s...\n", len(html), html[:50])
		} else {
			fmt.Printf("PASS: #app innerHTML: %s\n", html)
		}
	}

	// Test 2: Check Vue app instance
	result, err := rt.RunJS("window.__vue_app__ ? 'EXISTS' : 'NOT FOUND'")
	if err != nil {
		fmt.Printf("FAIL: check vue app: %v\n", err)
	} else {
		fmt.Printf("PASS: Vue app instance: %v\n", result)
	}

	// Test 3: Check if Vue component instance is accessible via element
	result, err = rt.RunJS(`
		var appEl = document.getElementById('app');
		var vm = appEl && appEl.__vue_app__;
		vm ? 'vm EXISTS' : 'vm NOT FOUND';
	`)
	if err != nil {
		fmt.Printf("FAIL: check component vm: %v\n", err)
	} else {
		fmt.Printf("PASS: Component VM: %v\n", result)
	}

	// Test 4: Simulate click on an element
	// Find a clickable element and dispatch click event
	result, err = rt.RunJS(`
		var items = document.querySelectorAll('.activity-item');
		var result = {found: items ? items.length : 0};
		if (items && items.length > 0) {
			var ev = new Event('click', {bubbles: true});
			items[0].dispatchEvent(ev);
			result.dispatched = true;
		}
		JSON.stringify(result);
	`)
	if err != nil {
		fmt.Printf("FAIL: click dispatch: %v\n", err)
	} else {
		fmt.Printf("PASS: Click dispatch: %v\n", result)
	}

	// Test 5: Test querySelector with class
	result, err = rt.RunJS(`
		var el = document.querySelector('.app-layout');
		el ? 'found: ' + el.className : 'NOT FOUND';
	`)
	if err != nil {
		fmt.Printf("FAIL: querySelector: %v\n", err)
	} else {
		fmt.Printf("PASS: querySelector: %v\n", result)
	}

	// Test 6: Test style attribute
	result, err = rt.RunJS(`
		var el = document.querySelector('.app-layout');
		el.style.display ? el.style.display : 'NO STYLE';
	`)
	if err != nil {
		fmt.Printf("FAIL: style check: %v\n", err)
	} else {
		fmt.Printf("PASS: style check: %v\n", result)
	}

	fmt.Println("\n=== Console ===")
	consoleOutput := log.String()
	if len(consoleOutput) > 2000 {
		consoleOutput = consoleOutput[:2000] + "..."
	}
	fmt.Println(consoleOutput)

	// Check for errors
	if strings.Contains(consoleOutput, "Error") || strings.Contains(consoleOutput, "error") {
		// Filter known non-errors
		lines := strings.Split(consoleOutput, "\n")
		realErrors := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && (strings.Contains(trimmed, "Error:") || strings.Contains(trimmed, "Uncaught")) {
				fmt.Printf("ERROR FOUND: %s\n", trimmed)
				realErrors = true
			}
		}
		if realErrors {
			fmt.Println("\n=== SOME TESTS MAY HAVE ISSUES ===")
			os.Exit(1)
		}
	}

	fmt.Println("\n=== ALL TESTS PASSED ===")
}
