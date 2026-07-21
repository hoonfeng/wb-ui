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
	pass := 0
	fail := 0
	assert := func(name string, ok bool, detail string) {
		if ok { pass++; fmt.Printf("  PASS: %s\n", name) 
		} else { fail++; fmt.Printf("  FAIL: %s — %s\n", name, detail) }
	}

	rt := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	rt.SetupGlobal(log)

	doc := dom.NewDocument()
	bindings.RegisterDOMBindings(rt, doc)

	htmlEl := doc.CreateElement("html")
	doc.AppendChild(htmlEl)
	body := doc.CreateElement("body")
	htmlEl.AppendChild(body)
	appEl := doc.CreateElement("div")
	appEl.SetId("app")
	body.AppendChild(appEl)

	polyfills := []string{
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
		`class HTMLElement extends Element{}`,
		`class HTMLDivElement extends HTMLElement{}`,
		`class HTMLSpanElement extends HTMLElement{}`,
		`class HTMLButtonElement extends HTMLElement{}`,
		`class HTMLInputElement extends HTMLElement{}`,
		`class HTMLTemplateElement extends HTMLElement{get content(){return document.createDocumentFragment()}}`,
		`class Comment{}`,
		`class CustomEvent extends Event{constructor(t,o){super(t);this.detail=o&&o.detail||null}}`,
		`class DOMRect{constructor(x=0,y=0,w=0,h=0){this.x=x;this.y=y;this.width=w;this.height=h;this.top=y;this.right=x+w;this.bottom=y+h;this.left=x}}`,
		`class MutationObserver{constructor(c){this.cb=c}observe(t,o){}disconnect(){}takeRecords(){return[]}}`,
		`class IntersectionObserver{constructor(c,o){}observe(t){}unobserve(t){}disconnect(){}takeRecords(){return[]}}`,
		`class ResizeObserver{constructor(c){}observe(t){}unobserve(t){}disconnect(){}}`,
		`window.performance={now:function(){return Date.now()},mark:function(){},measure:function(){},getEntries:function(){return[]},getEntriesByType:function(){return[]}}`,
	}
	for _, p := range polyfills { rt.Run(p) }

	vueData, _ := os.ReadFile("dev/vue.global.prod.js")
	rt.Run(string(vueData))
	fmt.Println("Vue 3 loaded.\n")

	// === FIXED TEST: use methods for event handlers ===
	fmt.Println("=== Test: Counter with methods ===")
	rt.RunJS(`
		window._counterApp = Vue.createApp({
			methods: {
				increment: function() { this.count++; console.log('increment -> ' + this.count); },
				decrement: function() { this.count--; console.log('decrement -> ' + this.count); }
			},
			render: function() {
				var self = this;
				return Vue.h('div', [
					Vue.h('span', {class: 'count-val'}, 'Count: ' + this.count),
					Vue.h('button', {class: 'inc', onClick: function() { self.increment(); }}, '+'),
					Vue.h('button', {class: 'dec', onClick: function() { self.decrement(); }}, '-')
				]);
			},
			data: function() { return {count: 0}; }
		});
		window._counterApp.mount('#app');
	`)

	checkText := func(sel string) string {
		v, _ := rt.RunJS(fmt.Sprintf(`document.querySelector('%s').textContent`, sel))
		return v.ToString()
	}

	initial := checkText(".count-val")
	assert("Initial count", initial == "Count: 0", initial)

	// Click +
	rt.RunJS(`var ev=new Event('click',{bubbles:true});document.querySelector('.inc').dispatchEvent(ev);`)
	afterInc := checkText(".count-val")
	assert("After increment", afterInc == "Count: 1", afterInc)

	// Click + twice more
	rt.RunJS(`var ev=new Event('click',{bubbles:true});var b=document.querySelector('.inc');b.dispatchEvent(ev);b.dispatchEvent(ev);`)
	after3 := checkText(".count-val")
	assert("After 3 increments", after3 == "Count: 3", after3)

	// Click -
	rt.RunJS(`var ev=new Event('click',{bubbles:true});document.querySelector('.dec').dispatchEvent(ev);`)
	afterDec := checkText(".count-val")
	assert("After decrement", afterDec == "Count: 2", afterDec)

	// === Test: v-if toggle ===
	fmt.Println("\n=== Test: v-if toggle (fixed) ===")
	rt.RunJS(`
		var el2 = document.createElement('div'); el2.id = 'app2';
		document.body.appendChild(el2);
		window._vifApp = Vue.createApp({
			methods: {
				toggle: function() { this.show = !this.show; }
			},
			render: function() {
				var self = this;
				var children = [Vue.h('p', 'always')];
				if (this.show) children.push(Vue.h('span', {class: 'conditional'}, 'SHOWN'));
				return Vue.h('div', [
					Vue.h('button', {class: 'toggle-btn', onClick: function(){ self.toggle(); }}, 'Toggle'),
					Vue.h('div', children)
				]);
			},
			data: function() { return {show: true}; }
		});
		window._vifApp.mount('#app2');
	`)

	hasShown := func() bool {
		v, _ := rt.RunJS(`document.querySelector('.conditional') !== null`)
		return v.ToBoolean()
	}

	assert("v-if initially shown", hasShown(), "conditional should be visible")

	rt.RunJS(`var ev=new Event('click',{bubbles:true});document.querySelector('.toggle-btn').dispatchEvent(ev);`)
	assert("v-if after toggle hidden", !hasShown(), "conditional should be hidden")

	rt.RunJS(`var ev=new Event('click',{bubbles:true});document.querySelector('.toggle-btn').dispatchEvent(ev);`)
	assert("v-if after toggle shown again", hasShown(), "conditional should be visible again")

	// === Test: v-for with reactivity ===
	fmt.Println("\n=== Test: v-for reactive list ===")
	rt.RunJS(`
		var el3 = document.createElement('div'); el3.id = 'app3';
		document.body.appendChild(el3);
		window._vforApp = Vue.createApp({
			methods: {
				add: function() { this.items.push('Item ' + (this.items.length + 1)); },
				remove: function() { this.items.pop(); }
			},
			render: function() {
				var self = this;
				return Vue.h('div', [
					Vue.h('button', {class: 'add-btn', onClick: function(){ self.add(); }}, 'Add'),
					Vue.h('button', {class: 'remove-btn', onClick: function(){ self.remove(); }}, 'Remove'),
					Vue.h('ul', 
						this.items.map(function(item) {
							return Vue.h('li', {class: 'vfor-item'}, item);
						})
					)
				]);
			},
			data: function() { return {items: ['A', 'B']}; }
		});
		window._vforApp.mount('#app3');
	`)

	itemCount := func() int {
		v, _ := rt.RunJS(`document.querySelectorAll('.vfor-item').length`)
		return int(v.ToNumber())
	}

	assert("v-for initial 2 items", itemCount() == 2, fmt.Sprintf("got %d", itemCount()))

	rt.RunJS(`var ev=new Event('click',{bubbles:true});document.querySelector('.add-btn').dispatchEvent(ev);`)
	assert("v-for after add 3 items", itemCount() == 3, fmt.Sprintf("got %d", itemCount()))

	rt.RunJS(`var ev=new Event('click',{bubbles:true});document.querySelector('.remove-btn').dispatchEvent(ev);`)
	assert("v-for after remove 2 items", itemCount() == 2, fmt.Sprintf("got %d", itemCount()))

	// === Test: computed property ===
	fmt.Println("\n=== Test: Computed property ===")
	rt.RunJS(`
		var el4 = document.createElement('div'); el4.id = 'app4';
		document.body.appendChild(el4);
		window._compApp = Vue.createApp({
			methods: {
				inc: function() { this.count++; }
			},
			computed: {
				double: function() { return this.count * 2; }
			},
			render: function() {
				var self = this;
				return Vue.h('div', [
					Vue.h('span', {class: 'double-val'}, 'Double: ' + this.double),
					Vue.h('button', {class: 'inc-btn2', onClick: function(){ self.inc(); }}, '+')
				]);
			},
			data: function() { return {count: 5}; }
		});
		window._compApp.mount('#app4');
	`)

	dval := checkText(".double-val")
	assert("Computed initial", dval == "Double: 10", dval)

	rt.RunJS(`var ev=new Event('click',{bubbles:true});document.querySelector('.inc-btn2').dispatchEvent(ev);`)
	dval2 := checkText(".double-val")
	assert("Computed after inc", dval2 == "Double: 12", dval2)

	// Console
	fmt.Println("\n=== Console ===")
	for _, line := range strings.Split(log.String(), "\n") {
		if t := strings.TrimSpace(line); t != "" { fmt.Printf("  %s\n", t) }
	}

	fmt.Printf("\n=== %d PASS, %d FAIL ===\n", pass, fail)
	if fail > 0 { os.Exit(1) }
}
