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
	passCount := 0
	failCount := 0
	assert := func(name string, ok bool, detail string) {
		if ok {
			passCount++
			fmt.Printf("  PASS: %s\n", name)
		} else {
			failCount++
			fmt.Printf("  FAIL: %s — %s\n", name, detail)
		}
	}

	rt := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	rt.SetupGlobal(log)

	doc := dom.NewDocument()
	bindings.RegisterDOMBindings(rt, doc)

	// Setup DOM tree
	htmlEl := doc.CreateElement("html")
	doc.AppendChild(htmlEl)
	head := doc.CreateElement("head")
	htmlEl.AppendChild(head)
	body := doc.CreateElement("body")
	htmlEl.AppendChild(body)
	appEl := doc.CreateElement("div")
	appEl.SetId("app")
	body.AppendChild(appEl)

	polyfills := []string{
		"if(!Object.getOwnPropertyNames)Object.getOwnPropertyNames=function(o){if(!o)return[];var k=[];for(var n in o)k.push(n);return k}",
		"if(!Object.fromEntries)Object.fromEntries=function(e){var r={};for(var i=0;e&&i<e.length;i++)if(e[i])r[e[i][0]]=e[i][1];return r}",
		"if(!Array.prototype.flatMap)Array.prototype.flatMap=function(f){var r=[];for(var i=0;i<this.length;i++){var v=f(this[i],i,this);if(v&&v.length)for(var j=0;j<v.length;j++)r.push(v[j]);else r.push(v)}return r}",
		"if(!Array.prototype.at)Array.prototype.at=function(i){var n=Number(i);if(isNaN(n))n=0;var l=this.length;n=n>=0?n:l+n;if(n<0||n>=l)return undefined;return this[n]}",
		"if(!Object.isExtensible)Object.isExtensible=function(){return true}",
		"class Node{}",
		"class Element extends Node{}",
		"class Document extends Node{}",
		"class Event{constructor(t,o){this.type=t}stopPropagation(){}preventDefault(){}}",
		"class SVGElement extends Element{}",
		"class SVGSVGElement extends SVGElement{}",
		"class HTMLElement extends Element{}",
		"class HTMLDivElement extends HTMLElement{}",
		"class HTMLSpanElement extends HTMLElement{}",
		"class HTMLButtonElement extends HTMLElement{}",
		"class HTMLTemplateElement extends HTMLElement{get content(){return document.createDocumentFragment()}}",
		"class Comment{}",
		"class CustomEvent extends Event{constructor(t,o){super(t);this.detail=o&&o.detail||null}}",
		"class DOMRect{constructor(x,y,w,h){this.x=x;this.y=y;this.width=w;this.height=h;this.top=y;this.right=x+w;this.bottom=y+h;this.left=x}}",
		"class MutationObserver{constructor(c){this.cb=c}observe(t,o){}disconnect(){}takeRecords(){return[]}}",
		"class IntersectionObserver{constructor(c,o){}observe(t){}unobserve(t){}disconnect(){}takeRecords(){return[]}}",
		"class ResizeObserver{constructor(c){}observe(t){}unobserve(t){}disconnect(){}}",
		"window.performance={now:function(){return Date.now()},mark:function(){},measure:function(){},getEntries:function(){return[]},getEntriesByType:function(){return[]}}",
		"window.addEventListener=function(n,fn){window['_ev_'+n]=fn}",
		"window.removeEventListener=function(){}",
	}
	for _, p := range polyfills {
		if _, err := rt.Run(p); err != nil {
			fmt.Fprintf(os.Stderr, "polyfill error: %v\n--- polyfill was: %s\n", err, p)
			os.Exit(1)
		}
	}

	vueData, err := os.ReadFile("dev/vue.global.prod.js")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read vue: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Vue bundle: %d bytes\n", len(vueData))
	if _, err := rt.Run(string(vueData)); err != nil {
		fmt.Fprintf(os.Stderr, "vue load error: %v\n", err)
		fmt.Println("Console:", log.String())
		os.Exit(1)
	}
	fmt.Println("Vue 3 global build loaded.")

	// Test 1: Vue global exposed (prod build exports as object, not function)
	fmt.Println("\n=== Test 1: Vue global API ===")
	assert("Vue is defined",
		checkBool(rt, "typeof Vue === 'object' && Vue.version"),
		"Vue should be an object with version")

	assert("Vue.createApp",
		checkBool(rt, "typeof Vue.createApp === 'function'"),
		"createApp should be a function")

	assert("Vue.ref",
		checkBool(rt, "typeof Vue.ref === 'function'"),
		"ref should be a function")

	assert("Vue.reactive",
		checkBool(rt, "typeof Vue.reactive === 'function'"),
		"reactive should be a function")

	assert("Vue.h",
		checkBool(rt, "typeof Vue.h === 'function'"),
		"h() should be a function")

	// Test 2: Create reactive state
	fmt.Println("\n=== Test 2: Reactive state (ref) ===")
	assert("ref create + read",
		checkBool(rt, "(function(){var r=Vue.ref(0);return r.value===0})()"),
		"ref(0).value should be 0")

	assert("ref set + read",
		checkBool(rt, "(function(){var r=Vue.ref(0);r.value=5;return r.value===5})()"),
		"ref(0).value=5 should be 5")

	assert("reactive create + mutate",
		checkBool(rt, "(function(){var s=Vue.reactive({count:0});s.count=10;return s.count===10})()"),
		"reactive({count:0}).count=10 should be 10")

	// Test 3: Render function via h()
	fmt.Println("\n=== Test 3: Render function (h) ===")
	assert("h() creates vnode",
		checkBool(rt, "(function(){var v=Vue.h('div',{class:'test'},'Hello');return v&&v.__v_isVNode})()"),
		"h() should create a vnode")

	// Test 4: Mount with render function (use closure for event handler)
	fmt.Println("\n=== Test 4: Vue 3 mount ===")
	mountResult, err := rt.RunJS(jsMount)
	if err != nil {
		assert("Vue mount", false, err.Error())
	} else {
		assert("Vue mount", strings.HasPrefix(mountResult.ToString(), "mounted"),
			fmt.Sprintf("result=%v", mountResult))
	}

	// Test 5: Check rendered DOM
	fmt.Println("\n=== Test 5: Rendered DOM ===")
	mountedHTML, _ := rt.RunJS("window.__mountedHTML || ''")
	html := mountedHTML.ToString()
	assert("DOM contains Count:", strings.Contains(html, "Count:"),
		fmt.Sprintf("innerHTML=%s", html[:min(100,len(html))]))

	// Test 6: Click reactivity (dispatch in one call, check in another)
	fmt.Println("\n=== Test 6: Reactive DOM update on click ===")
	rt.RunJS(jsClickDispatch)
	clickCheck, _ := rt.RunJS(jsClickCheck)
	assert("Click reactivity",
		clickCheck.ToString() == "Count: 1",
		fmt.Sprintf("expected 'Count: 1' got '%v'", clickCheck))

	// Test 7: Multi-click reactivity
	fmt.Println("\n=== Test 7: Multi-click reactivity ===")
	rt.RunJS(jsMultiClick)
	multiCheck, _ := rt.RunJS(jsMultiCheck)
	assert("Multi-click 4 total",
		multiCheck.ToString() == "Count: 4",
		fmt.Sprintf("expected 'Count: 4' got '%v'", multiCheck))

	// Test 8: v-if conditional rendering — two mounts
	fmt.Println("\n=== Test 8: v-if conditional rendering ===")
	vifResult, err := rt.RunJS(jsVIf)
	if err != nil {
		assert("v-if conditional", false, err.Error())
	} else {
		result := vifResult.ToString()
		assert("v-if toggle",
			strings.Contains(result, "showTrue=true") && strings.Contains(result, "showFalse=false"),
			fmt.Sprintf("result=%s", result))
	}

	// Test 9: v-for list rendering
	fmt.Println("\n=== Test 9: v-for list rendering ===")
	vforResult, err := rt.RunJS(jsVFor)
	if err != nil {
		assert("v-for list", false, err.Error())
	} else {
		result := vforResult.ToString()
		assert("v-for list",
			result == "items=3 first=A",
			fmt.Sprintf("result=%s", result))
	}

	fmt.Printf("\n=== Results: %d PASS, %d FAIL ===\n", passCount, failCount)
	if failCount > 0 {
		os.Exit(1)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func checkBool(rt *jsc.Interpreter, code string) bool {
	v, err := rt.RunJS(code)
	if err != nil {
		return false
	}
	return v.ToBoolean()
}

// JS test snippets — defined as Go variables to avoid backtick corruption
var jsMount = `
(function(){
	try {
		var app = Vue.createApp({
			render: function() {
				var self = this;
				return Vue.h('div', {class: 'counter-app'}, [
					Vue.h('span', {class: 'count'}, 'Count: ' + this.count),
					Vue.h('button', {class: 'btn', onClick: function() { self.count++ }}, '+1')
				]);
			},
			data: function() { return {count: 0}; }
		});
		var el = document.getElementById('app');
		app.mount(el);
		window.__mountedHTML = el.innerHTML;
		return el.innerHTML.indexOf('Count:') >= 0 ? 'mounted' : 'no-count';
	} catch(e) {
		return 'error:' + (e.message || String(e));
	}
})()
`

var jsClickDispatch = `
(function(){
	try {
		var btn = document.querySelector('button.btn');
		if (!btn) { window.__clickErr = 'no-btn'; return; }
		var ev = new Event('click', {bubbles: true});
		btn.dispatchEvent(ev);
		window.__clickErr = null;
	} catch(e) {
		window.__clickErr = 'error:' + (e.message || String(e));
	}
})()
`

var jsClickCheck = `
(function(){
	if (window.__clickErr) return window.__clickErr;
	var countEl = document.querySelector('span.count');
	return countEl ? countEl.textContent : 'no-el';
})()
`

var jsMultiClick = `
(function(){
	try {
		var btn = document.querySelector('button.btn');
		if (!btn) return;
		var ev = new Event('click', {bubbles: true});
		btn.dispatchEvent(ev); btn.dispatchEvent(ev); btn.dispatchEvent(ev);
		window.__multiErr = null;
	} catch(e) {
		window.__multiErr = 'error:' + (e.message || String(e));
	}
})()
`

var jsMultiCheck = `
(function(){
	if (window.__multiErr) return window.__multiErr;
	var countEl = document.querySelector('span.count');
	return countEl ? countEl.textContent : 'no-el';
})()
`

var jsVIf = `
(function(){
	try {
		var elShow = document.createElement('div');
		document.body.appendChild(elShow);
		Vue.createApp({
			render: function() {
				var children = [Vue.h('p', 'always visible')];
				if (this.show) children.push(Vue.h('span', {class: 'cond'}, 'SHOWN'));
				return Vue.h('div', children);
			},
			data: function() { return {show: true}; }
		}).mount(elShow);

		var elHide = document.createElement('div');
		document.body.appendChild(elHide);
		Vue.createApp({
			render: function() {
				var children = [Vue.h('p', 'always visible')];
				if (this.show) children.push(Vue.h('span', {class: 'cond'}, 'SHOWN'));
				return Vue.h('div', children);
			},
			data: function() { return {show: false}; }
		}).mount(elHide);

		var hasShowTrue = elShow.innerHTML.indexOf('SHOWN') >= 0;
		var hasShowFalse = elHide.innerHTML.indexOf('SHOWN') >= 0;
		return 'showTrue=' + hasShowTrue + ' showFalse=' + hasShowFalse;
	} catch(e) {
		return 'error:' + (e.message || String(e));
	}
})()
`

var jsVFor = `
(function(){
	try {
		var el = document.createElement('div');
		document.body.appendChild(el);
		Vue.createApp({
			render: function() {
				return Vue.h('ul', 
					this.items.map(function(item) {
						return Vue.h('li', {class: 'item'}, item);
					})
				);
			},
			data: function() { return {items: ['A','B','C']}; }
		}).mount(el);
		var lis = el.querySelectorAll('li.item');
		return 'items=' + (lis?lis.length:0) + ' first=' + (lis&&lis.length>0?lis[0].textContent:'none');
	} catch(e) {
		return 'error:' + (e.message || String(e));
	}
})()
`
