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

	// Pre-polyfills
	wv.EvalJS(`if(!Object.getPrototypeOf)Object.getPrototypeOf=function(o){return o&&o.constructor?o.constructor.prototype:null}`)
	wv.EvalJS(`if(!Object.setPrototypeOf)Object.setPrototypeOf=function(o,p){o.__proto__=p;return o}`)
	wv.EvalJS(`if(typeof TextEncoder==='undefined')TextEncoder=function(){this.encode=function(s){var a=new Uint8Array(s.length);for(var i=0;i<s.length;i++)a[i]=s.charCodeAt(i);return a}}`)
	wv.EvalJS(`if(typeof structuredClone==='undefined')structuredClone=function(o){return JSON.parse(JSON.stringify(o))}`)

	// Load HTML (REPLACE type=module before parsing)
	s := string(htmlData)
	s = strings.Replace(s, `type="module"`, "", 1)
	s = strings.ReplaceAll(s, `crossorigin`, "")
	if err := wv.LoadHTML(s); err != nil {
		panic(err)
	}

	rt := wv.JSInterpreter()
	doc := mf.Document()
	bindings.RegisterDOMBindings(rt, doc)

	// Add error tracer
	wv.EvalJS(`(function(){
		var _onerror = window.onerror;
		window.onerror = function(msg, url, line, col, err) {
			console.log("ONERROR:", msg, "at line", line, "col", col);
			if (err && err.stack) console.log(err.stack);
			if (_onerror) return _onerror.apply(this, arguments);
		};
	})()`)

	// Execute scripts
	fmt.Println("=== Executing scripts ===")
	fr.ExecuteScripts()

	// Check result
	fmt.Println("\n=== Post-Mount ===")
	r, _ := wv.EvalJS(`(function(){
		var r = [];
		r.push('typeof Vue=' + typeof Vue);
		if (typeof Vue !== 'undefined') r.push('Vue.version=' + Vue.version);
		var app = document.getElementById('app');
		r.push('app.innerHTML.length=' + (app?app.innerHTML.length:'null'));
		return r.join('\n');
	})()`)
	fmt.Println(r)
	fmt.Println("\n=== Console ===")
	fmt.Println(wv.ConsoleOutput())
}
