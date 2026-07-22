package main

import (
	"fmt"
	"os"
	"path/filepath"

	"wb-ui/bindings"
	"wb-ui/webkit"
)

func main() {
	wv := webkit.NewWebView()
	mf := wv.MainFrame()
	_ = mf.Frame()

	// Load vue.global.prod.js directly
	vuePath := filepath.Join("F:\\syproject\\wb-ui\\dev\\vue.global.prod.js")
	vueCode, err := os.ReadFile(vuePath)
	if err != nil {
		panic(err)
	}

	// Minimal HTML with body
	html := `<!DOCTYPE html><html><head><meta charset="utf-8"></head><body><div id="app"></div></body></html>`

	if err := wv.LoadHTML(html); err != nil {
		panic(err)
	}
	rt := wv.JSInterpreter()
	doc := mf.Document()
	bindings.RegisterDOMBindings(rt, doc)

	// ── Test 1: Load vue.global.prod.js ──
	fmt.Println("=== Loading vue.global.prod.js ===")
	_, err = wv.EvalJS(string(vueCode))
	fmt.Printf("EvalJS result: err=%v\n", err)

	// ── Test 2: Check Vue ──
	r, _ := wv.EvalJS(`(function(){
		return 'typeof Vue=' + typeof Vue + ' version=' + (typeof Vue !=='undefined'?Vue.version:'N/A');
	})()`)
	fmt.Println("Vue check:", r)

	// ── Test 3: Minimal Vue app ──
	r2, _ := wv.EvalJS(`(function(){
		try {
			var app = Vue.createApp({
				render: function() { return Vue.h('div', {class:'hello'}, 'Hello World'); }
			});
			app.mount('#app');
			var el = document.getElementById('app');
			return 'childCount=' + el.childElementCount + ' html=' + el.innerHTML.slice(0,100);
		} catch(e) {
			return 'ERROR: ' + e.message;
		}
	})()`)
	fmt.Println("Minimal app:", r2)

	fmt.Println("\n=== Console ===")
	fmt.Println(wv.ConsoleOutput())
}
