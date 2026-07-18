package jsc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestVue3MountTraceDiagnostic(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	// Polyfills (with caching querySelector for same-element behavior)
	pf := []string{
		`window={process:{env:{NODE_ENV:"production"}}}`,
		// Vue polyfills
		`Object.getOwnPropertyNames=function(o){if(!o)return[];var k=[];for(var n in o)k.push(n);return k}`,
		`Object.hasOwn=function(o,p){return Object.prototype.hasOwnProperty.call(o,p)}`,
		`Object.fromEntries=function(e){var r={};for(var i=0;e&&i<e.length;i++)if(e[i])r[e[i][0]]=e[i][1];return r}`,
		`if(!Array.prototype.flatMap)Array.prototype.flatMap=function(f){var r=[];for(var i=0;i<this.length;i++){var v=f(this[i],i,this);if(v&&v.length)for(var j=0;j<v.length;j++)r.push(v[j]);else r.push(v)}return r}`,
		`if(!Array.prototype.at)Array.prototype.at=function(i){var n=Number(i);if(isNaN(n))n=0;var l=this.length;n=n>=0?n:l+n;if(n<0||n>=l)return undefined;return this[n]}`,
		`if(!Object.isExtensible)Object.isExtensible=function(){return true}`,
		`if(!Object.isSealed)Object.isSealed=function(){return false}`,
		`if(!Object.isFrozen)Object.isFrozen=function(){return false}`,
		`if(!Object.getPrototypeOf)Object.getPrototypeOf=function(o){return o&&o.constructor?o.constructor.prototype:null}`,
		`if(!Object.setPrototypeOf)Object.setPrototypeOf=function(o,p){o.__proto__=p;return o}`,
		`if(!Object.preventExtensions)Object.preventExtensions=function(o){return o}`,
		`if(!Object.seal)Object.seal=function(o){return o}`,
		// DOM stub with CACHED elements (same element for same query)
		`var __elCache={};document={}`,
		`document.getElementById=function(s){return __elCache[s]||null}`,
		`document.querySelector=function(s){if(!__elCache[s]){__elCache[s]={innerHTML:'',__vue_app__:null,_vnode:null,childNodes:[],appendChild:function(c){this.childNodes.push(c);console.log('QS_APPEND: '+c.tagName)},removeChild:function(c){var i=this.childNodes.indexOf(c);if(i>-1)this.childNodes.splice(i,1)},insertBefore:function(c,r){console.log('QS_IB: c='+c.tagName);this.childNodes.push(c)},setAttribute:function(k,v){this[k]=v;console.log('QS_SETATTR: '+k+'='+v)},getAttribute:function(k){return this[k]},addEventListener:function(){},style:{},parentNode:null,tagName:'DIV'}};return __elCache[s]}`,
		`document.createElement=function(t){var el={tagName:t.toUpperCase(),innerHTML:'',textContent:'',childNodes:[],setAttribute:function(k,v){this[k]=v;console.log('CE_SETATTR: '+k+'='+v)},getAttribute:function(k){return this[k]},appendChild:function(c){this.childNodes.push(c);console.log('CE_APPEND: '+c.tagName)},removeChild:function(c){var i=this.childNodes.indexOf(c);if(i>-1)this.childNodes.splice(i,1)},insertBefore:function(c,r){console.log('CE_IB: c='+(c.tagName||'#text'));this.childNodes.push(c)},replaceChild:function(n,o){var i=this.childNodes.indexOf(o);if(i>-1)this.childNodes[i]=n},addEventListener:function(){},style:{},parentNode:null};return el}`,
		`document.body={appendChild:function(c){console.log('BODY_APPEND: '+c.tagName)},insertBefore:function(c,r){this.childNodes.push(c);console.log('BODY_IB')},childNodes:[]}`,
		`document.createTextNode=function(t){var tn={nodeType:3,textContent:t,nodeValue:t};console.log('CREATE_TEXT: '+t);return tn}`,
		`document.createComment=function(t){var cn={nodeType:8,textContent:t,nodeValue:t};console.log('CREATE_COMMENT');return cn}`,
	}
	for _, p := range pf {
		if _, err := vm.Run(p); err != nil {
			t.Fatalf("polyfill: %v", err)
		}
	}

	// Setup #app in cache
	vm.Run(`__elCache['#app']={innerHTML:'<p>placeholder</p>',__vue_app__:null,_vnode:null,childNodes:[],appendChild:function(c){this.childNodes.push(c);console.log('APP_APPEND: '+c.tagName)},removeChild:function(c){var i=this.childNodes.indexOf(c);if(i>-1)this.childNodes.splice(i,1)},insertBefore:function(c,r){console.log('APP_IB: c='+(c.tagName||'#text'));this.childNodes.push(c)},setAttribute:function(k,v){this[k]=v},getAttribute:function(k){return this[k]},addEventListener:function(){},style:{},parentNode:null,tagName:'DIV'}`)
	vm.Run(`__elCache['app']=__elCache['#app']`)

	// Read bundle
	distDir := "F:/syproject/gou-ide/cmd/desktop/web-ui-minimal/dist/assets"
	entries, err := os.ReadDir(distDir)
	if err != nil {
		t.Skip("dist not found:", err)
	}
	var bp string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".js") {
			bp = distDir + "/" + e.Name()
			break
		}
	}
	if bp == "" {
		t.Skip("no bundle found")
	}
	data, err := os.ReadFile(bp)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	bundleSrc := string(data)

	// Inject diagnostic tracing into the bundle
	// We need to trace the P function (patch), n (insert), and ye (mountElement)
	// 
	// The bundle is all one line. Let's add tracing at the end instead.
	// Remove the last 2 lines and add our own diagnostic code

	// Replace the final IIFE close with diagnostics
	// The bundle ends with: ... console.log("F __vue_app__="+(Us.__vue_app__?"yes":"no")) })();
	// We need to keep the IIFE execution and add diagnostics after

	// Just run the bundle first, then add diagnostics
	_, err = vm.Run(bundleSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Bundle error: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "Bundle OK\n")
	}

	// Post-bundle diagnostics
	diagCode := `
		console.log('=== POST_BUNDLE_DIAG ===');
		// Check the #app state
		var app = document.querySelector('#app');
		console.log('APP_TAG: '+(app?app.tagName:'null'));
		console.log('APP_CHILDREN: '+(app?app.childNodes.length:0));
		console.log('APP_VUE_APP: '+(app && app.__vue_app__ ? 'yes' : 'no'));
		console.log('APP_VNODE: '+(app && app._vnode ? typeof app._vnode : 'null'));
		
		// Check window state
		console.log('WIN_APP: '+(window.__VUE_APP__?'yes':'no'));
		
		// Direct render test
		try {
			// Create a new container
			var container = document.createElement('div');
			container.tagName = 'DIV-CONTAINER';
			document.body.appendChild(container);
			console.log('DIRECT_CONTAINER: created');
			
			// Create a simple component
			var comp = {render: function() { return h('div', null, 'direct render test'); }};
			
			// Create another app
			try {
				var app2 = hl(comp);
				console.log('APP2_CREATED');
				var result = app2.mount(container);
				console.log('APP2_MOUNTED: '+(typeof result));
				console.log('DIRECT_RESULT: children='+container.childNodes.length+' html='+(container.innerHTML||'').substring(0,100));
				console.log('DIRECT_VUE_APP: '+(container.__vue_app__?'yes':'no'));
			} catch(e2) {
				console.log('APP2_ERR: '+e2);
			}
		} catch(e) {
			console.log('DIRECT_ERR: '+e);
		}
	`
	vm.Run(diagCode)
	
	out := logger.String()
	fmt.Printf("=== OUTPUT ===\n%s\n=== END ===\n", out)
}
