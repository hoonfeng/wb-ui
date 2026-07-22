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
		panic(fmt.Sprintf("read html: %v", err))
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
		panic(fmt.Sprintf("LoadHTML: %v", err))
	}

	rt := wv.JSInterpreter()
	doc := mf.Document()
	bindings.RegisterDOMBindings(rt, doc)

	// ─── DIAG: pre-mount DOM state ───
	fmt.Println("=== Pre-Mount DOM State ===")
	results, _ := wv.EvalJS(`(function(){
		var b = document.body;
		var r = [];
		r.push('body=' + (b?b.tagName:'null'));
		if (b) {
			r.push('body.childNodes.length=' + b.childNodes.length);
			for (var i=0; i<b.childNodes.length && i<10; i++) {
				var n = b.childNodes[i];
				var desc = '  ['+i+']: ';
				if (n === null) desc += 'NULL';
				else if (n === undefined) desc += 'UNDEFINED';
				else if (n.nodeType === 3) desc += 'Text(' + n.data.slice(0,20) + ')';
				else if (n.nodeType === 1) desc += n.tagName + (n.id?'#'+n.id:'') + (n.className?'.'+n.className:'');
				else desc += 'nodeType=' + n.nodeType;
				r.push(desc);
			}
		}
		var app = document.getElementById('app');
		r.push('app=' + (app?app.tagName:'null'));
		if (app) {
			r.push('app.childNodes.length=' + app.childNodes.length);
			r.push('app.innerHTML.len=' + app.innerHTML.length);
		}
		// Check body.firstChild via getter
		try {
			var fc = b.firstChild;
			r.push('body.firstChild=' + (fc === null ? 'null' : (fc === undefined ? 'undefined' : fc.toString().slice(0,40))));
		} catch(e) { r.push('body.firstChild ERROR: ' + e.message); }
		// Check body.childNodes[0]
		try {
			var c0 = b.childNodes[0];
			r.push('body.childNodes[0]=' + (c0 === null ? 'null' : (c0 === undefined ? 'undefined' : c0.toString().slice(0,40))));
		} catch(e) { r.push('body.childNodes[0] ERROR: ' + e.message); }
		return r.join('\n');
	})()`)
	fmt.Println(results)
	fmt.Println()

	// ─── Execute scripts with error trapping ───	
	fmt.Println("=== ExecuteScripts (with error detail) ===")
	// Use a simple error trap
	wv.EvalJS(`(function(){
		console.log = (function(oldLog) {
			return function() {
				var msg = ''; for (var i=0;i<arguments.length;i++) msg += arguments[i] + ' ';
				if (msg.indexOf('TypeError') >= 0 || msg.indexOf('Error') >= 0) {
					oldLog.call(console, 'TRAP:', msg);
				}
				return oldLog.apply(console, arguments);
			};
		})(console.log);
	})()`)

	// Now execute scripts
	fr.ExecuteScripts()

	// Check post-mount state
	fmt.Println("\n=== Post-Mount DOM ===")
	results2, _ := wv.EvalJS(`(function(){
		var app = document.getElementById('app');
		var r = [];
		r.push('app.childNodes.length=' + app.childNodes.length);
		r.push('app.firstChild=' + (app.firstChild?app.firstChild.toString().slice(0,40):'null'));
		r.push('app.innerHTML.length=' + app.innerHTML.length);
		return r.join('\n');
	})()`)
	fmt.Println(results2)
	fmt.Println()

	// Console output
	fmt.Println("=== Console ===")
	fmt.Println(wv.ConsoleOutput())
}
