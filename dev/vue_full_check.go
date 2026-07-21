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

	// Polyfills for Vue 3 global build
	polyfills := []string{
		`if(!Object.getOwnPropertyNames)Object.getOwnPropertyNames=function(o){if(!o)return[];var k=[];for(var n in o)k.push(n);return k}`,
		`if(!Object.fromEntries)Object.fromEntries=function(e){var r={};for(var i=0;e&&i<e.length;i++)if(e[i])r[e[i][0]]=e[i][1];return r}`,
		`if(!Array.prototype.flatMap)Array.prototype.flatMap=function(f){var r=[];for(var i=0;i<this.length;i++){var v=f(this[i],i,this);if(v&&v.length)for(var j=0;j<v.length;j++)r.push(v[j]);else r.push(v)}return r}`,
		`if(!Array.prototype.at)Array.prototype.at=function(i){var n=Number(i);if(isNaN(n))n=0;var l=this.length;n=n>=0?n:l+n;if(n<0||n>=l)return undefined;return this[n]}`,
		`if(!Object.isExtensible)Object.isExtensible=function(){return true}`,
		// Stub classes Vue 3 needs
		`class Node{}`,
		`class Element extends Node{}`,
		`class Document extends Node{}`,
		`class Event{constructor(t,o){this.type=t}stopPropagation(){}preventDefault(){}}`,
		`class SVGElement extends Element{}`,
		`class SVGSVGElement extends SVGElement{}`,
		`class HTMLElement extends Element{}`,
		`class HTMLDivElement extends HTMLElement{}`,
		`class HTMLSpanElement extends HTMLElement{}`,
		`class HTMLButtonElement extends HTMLElement{}`,
		`class HTMLTemplateElement extends HTMLElement{get content(){return document.createDocumentFragment()}}`,
		`class Comment{}`,
		`class CustomEvent extends Event{constructor(t,o){super(t);this.detail=o&&o.detail||null}}`,
		`class DOMRect{constructor(x=0,y=0,w=0,h=0){this.x=x;this.y=y;this.width=w;this.height=h;this.top=y;this.right=x+w;this.bottom=y+h;this.left=x}}`,
		`class MutationObserver{constructor(c){this.cb=c}observe(t,o){}disconnect(){}takeRecords(){return[]}}`,
		`class IntersectionObserver{constructor(c,o){}observe(t){}unobserve(t){}disconnect(){}takeRecords(){return[]}}`,
		`class ResizeObserver{constructor(c){}observe(t){}unobserve(t){}disconnect(){}}`,
		`window.performance={now:function(){return Date.now()},mark:function(){},measure:function(){},getEntries:function(){return[]},getEntriesByType:function(){return[]}}`,
		`window.addEventListener=function(){console.log('window.addEventListener called:',arguments[0])}`,
		`window.removeEventListener=function(){}`,
	}
	for _, p := range polyfills {
		if _, err := rt.Run(p); err != nil {
			fmt.Fprintf(os.Stderr, "polyfill error: %v\n--- polyfill was: %s\n", err, p)
			os.Exit(1)
		}
	}

	// Load Vue 3 global build
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

	// Test 1: Vue global exposed
	fmt.Println("\n=== Test 1: Vue global API ===")
	assert("Vue is defined",
		checkBool(rt, `typeof Vue === 'function'`),
		"Vue should be a function")

	assert("Vue.createApp",
		checkBool(rt, `typeof Vue.createApp === 'function'`),
		"createApp should be a function")

	assert("Vue.ref",
		checkBool(rt, `typeof Vue.ref === 'function'`),
		"ref should be a function")

	assert("Vue.reactive",
		checkBool(rt, `typeof Vue.reactive === 'function'`),
		"reactive should be a function")

	assert("Vue.h",
		checkBool(rt, `typeof Vue.h === 'function'`),
		"h() should be a function")

	// Test 2: Create reactive state
	fmt.Println("\n=== Test 2: Reactive state (ref) ===")
	assert("ref create + read",
		checkBool(rt, `(function(){var r=Vue.ref(0);return r.value===0})()`),
		"ref(0).value should be 0")

	assert("ref set + read",
		checkBool(rt, `(function(){var r=Vue.ref(0);r.value=5;return r.value===5})()`),
		"ref(0).value=5 should be 5")

	assert("reactive create + mutate",
		checkBool(rt, `(function(){var s=Vue.reactive({count:0});s.count=10;return s.count===10})()`),
		"reactive({count:0}).count=10 should be 10")

	// Test 3: Render function via h()
	fmt.Println("\n=== Test 3: Render function (h) ===")
	assert("h() creates vnode",
		checkBool(rt, `(function(){var v=Vue.h('div',{class:'test'},'Hello');return v&&v.__v_isVNode})()`),
		"h() should create a vnode")

	// Test 4: Mount a component with render function
	fmt.Println("\n=== Test 4: Vue 3 mount ===")
	mountResult, err := rt.RunJS(`
		(function(){
			try {
				var app = Vue.createApp({
					render: function() {
						return Vue.h('div', {class: 'counter-app'}, [
							Vue.h('span', {class: 'count'}, 'Count: ' + this.count),
							Vue.h('button', {class: 'btn', onClick: function() { this.count++ }})
						]);
					},
					data: function() { return {count: 0}; }
				});
				var el = document.getElementById('app');
				app.mount(el);
				// After mount, check DOM
				var html = el.innerHTML;
				window.__mountedHTML = html;
				return html.indexOf('Count:') >= 0 ? 'mounted' : 'no-count:'+html.substring(0,50);
			} catch(e) {
				return 'error:' + (e.message || String(e));
			}
		})()
	`)
	if err != nil {
		assert("Vue mount", false, err.Error())
	} else {
		assert("Vue mount", strings.HasPrefix(mountResult.ToString(), "mounted"),
			fmt.Sprintf("result=%v", mountResult))
	}

	// Test 5: Check rendered DOM
	fmt.Println("\n=== Test 5: Rendered DOM ===")
	mountedHTML, _ := rt.RunJS(`window.__mountedHTML || ''`)
	html := mountedHTML.ToString()
	assert("DOM contains Count:", strings.Contains(html, "Count:"),
		fmt.Sprintf("innerHTML=%s", html[:min(100,len(html))]))

	// Test 6: Simulate click and check reactivity
	fmt.Println("\n=== Test 6: Reactive DOM update on click ===")
	clickResult, err := rt.RunJS(`
		(function(){
			try {
				var btn = document.querySelector('button.btn');
				if (!btn) return 'no-btn';
				var countEl = document.querySelector('span.count');
				if (!countEl) return 'no-count-el';
				var before = countEl.textContent;
				// Dispatch click event
				var ev = new Event('click', {bubbles: true});
				btn.dispatchEvent(ev);
				var after = countEl.textContent;
				window.__afterClick = after;
				return before + ' -> ' + after;
			} catch(e) {
				return 'error:' + (e.message || String(e));
			}
		})()
	`)
	if err != nil {
		assert("Click reactivity", false, err.Error())
	} else {
		result := clickResult.ToString()
		assert("Click reactivity", result == "Count: 0 -> Count: 1",
			fmt.Sprintf("expected 'Count: 0 -> Count: 1' got '%s'", result))
	}

	// Test 7: Texture multi-click reactivity
	fmt.Println("\n=== Test 7: Multi-click reactivity ===")
	multiResult, err := rt.RunJS(`
		(function(){
			try {
				var btn = document.querySelector('button.btn');
				var countEl = document.querySelector('span.count');
				if (!btn || !countEl) return 'no-elements';
				// Click 3 more times (count is already 1 from test 6)
				var ev = new Event('click', {bubbles: true});
				btn.dispatchEvent(ev);
				btn.dispatchEvent(ev);
				btn.dispatchEvent(ev);
				return countEl.textContent;
			} catch(e) {
				return 'error:' + (e.message || String(e));
			}
		})()
	`)
	if err != nil {
		assert("Multi-click", false, err.Error())
	} else {
		assert("Multi-click 4 total",
			multiResult.ToString() == "Count: 4",
			fmt.Sprintf("expected 'Count: 4' got '%v'", multiResult))
	}

	// Test 8: v-if conditional rendering
	fmt.Println("\n=== Test 8: v-if conditional rendering ===")
	vifResult, err := rt.RunJS(`
		(function(){
			try {
				// Create a new app in a fresh div for v-if test
				var el2 = document.createElement('div');
				el2.id = 'app2';
				document.body.appendChild(el2);
				var app = Vue.createApp({
					render: function() {
						var children = [Vue.h('p', 'always visible')];
						if (this.show) {
							children.push(Vue.h('span', {class: 'conditional'}, 'SHOWN'));
						}
						return Vue.h('div', children);
					},
					data: function() { return {show: true}; }
				});
				app.mount(el2);
				var before = el2.innerHTML.indexOf('SHOWN') >= 0;
				// Toggle show
				app._instance.proxy.show = false;
				// Need nextTick for DOM update
				window.__vifBefore = before;
				window.__vifHTML = el2.innerHTML;
				return 'before='+before+' after='+ (el2.innerHTML.indexOf('SHOWN') >= 0);
			} catch(e) {
				return 'error:' + (e.message || String(e));
			}
		})()
	`)
	if err != nil {
		assert("v-if conditional", false, err.Error())
	} else {
		result := vifResult.ToString()
		assert("v-if toggle",
			strings.Contains(result, "before=true") && strings.Contains(result, "after=false"),
			fmt.Sprintf("result=%s", result))
	}

	// Test 9: v-for list rendering
	fmt.Println("\n=== Test 9: v-for list rendering ===")
	vforResult, err := rt.RunJS(`
		(function(){
			try {
				var el3 = document.createElement('div');
				el3.id = 'app3';
				document.body.appendChild(el3);
				var app = Vue.createApp({
					render: function() {
						return Vue.h('ul', 
							this.items.map(function(item) {
								return Vue.h('li', {class: 'item'}, item);
							})
						);
					},
					data: function() { return {items: ['A','B','C']}; }
				});
				app.mount(el3);
				var lis = el3.querySelectorAll('li.item');
				var count = lis ? lis.length : 0;
				return 'items=' + count + ' first=' + (count>0?lis[0].textContent:'none');
			} catch(e) {
				return 'error:' + (e.message || String(e));
			}
		})()
	`)
	if err != nil {
		assert("v-for list", false, err.Error())
	} else {
		result := vforResult.ToString()
		assert("v-for list",
			result == "items=3 first=A",
			fmt.Sprintf("result=%s", result))
	}

	// Print console
	fmt.Println("\n=== Console ===")
	co := log.String()
	for _, line := range strings.Split(co, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			fmt.Printf("  %s\n", trimmed)
		}
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
