package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"wb-ui/bindings"
	"wb-ui/webkit"
)

func main() {
	distDir := `F:\syproject\gou-ide\cmd\desktop\web-ui\dist`
	absDist, _ := filepath.Abs(distDir)
	htmlData, err := os.ReadFile(filepath.Join(distDir, "index.html"))
	if err != nil {
		panic(err)
	}

	wv := webkit.NewWebView()
	mf := wv.MainFrame()
	fr := mf.Frame()
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
		panic(err)
	}

	rt := wv.JSInterpreter()
	doc := mf.Document()
	bindings.RegisterDOMBindings(rt, doc)

	// ─── Test 1: Can Vue load? ───
	fmt.Println("=== Test 1: Vue global ===")
	r, _ := wv.EvalJS(`(function(){
		var r = [];
		r.push('typeof Vue=' + typeof Vue);
		r.push('Vue.version=' + (typeof Vue !== 'undefined' ? Vue.version : 'N/A'));
		r.push('typeof Proxy=' + typeof Proxy);
		r.push('typeof Reflect=' + typeof Reflect);
		r.push('typeof Symbol=' + typeof Symbol);
		return r.join('\n');
	})()`)
	fmt.Println(r)

	// ─── Test 2: Minimal Vue 3 app (no components) ───
	fmt.Println("\n=== Test 2: Minimal Vue 3 ===")
	r2, _ := wv.EvalJS(`(function(){
		try {
			var app = Vue.createApp({
				template:'<div class="test">Hello {{name}}</div>',
				data:function(){return {name:'World'}}
			});
			app.mount('#app');
			var el = document.getElementById('app');
			return 'OK: childElementCount=' + el.childElementCount + ' innerHTML=' + el.innerHTML;
		} catch(e) {
			return 'FAIL: ' + (e.message || String(e));
		}
	})()`)
	fmt.Println(r2)

	// ─── Test 3: Simple render function ───
	fmt.Println("\n=== Test 3: Vue render function ===")
	r3, _ := wv.EvalJS(`(function(){
		try {
			var app = Vue.createApp({
				render: function() {
					return Vue.h('div', {class:'test'}, 'Hello World');
				}
			});
			app.mount('#app');
			var el = document.getElementById('app');
			return 'OK: childElementCount=' + el.childElementCount + ' innerHTML=' + el.innerHTML.slice(0,200);
		} catch(e) {
			return 'FAIL: ' + (e.message || String(e));
		}
	})()`)
	fmt.Println(r3)

	fmt.Println("\n=== Console ===")
	fmt.Println(wv.ConsoleOutput())
}
