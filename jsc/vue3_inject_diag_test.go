package jsc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestVue3InjectDiag(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	pf := []string{
		`window={process:{env:{NODE_ENV:"production"}}}`,
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
		// Simple DOM
		`var __elCache={};document={}`,
		`document.getElementById=function(s){return __elCache[s]||null}`,
		`document.querySelector=function(s){if(!__elCache[s]){__elCache[s]={tagName:'DIV',childNodes:[],innerHTML:'<p>placeholder</p>',_vnode:null,__vue_app__:null,setAttribute:function(k,v){this[k]=v},getAttribute:function(k){return this[k]},appendChild:function(c){this.childNodes.push(c)},insertBefore:function(c,r){this.childNodes.push(c)},removeChild:function(c){var i=this.childNodes.indexOf(c);if(i>-1)this.childNodes.splice(i,1)},replaceChild:function(n,o){var i=this.childNodes.indexOf(o);if(i>-1)this.childNodes[i]=n},addEventListener:function(){},style:{},parentNode:null}};__elCache[s].tagName='DIV'}return __elCache[s]}`,
		`document.createElement=function(t){var el={tagName:t.toUpperCase(),childNodes:[],innerHTML:'',textContent:'',setAttribute:function(k,v){this[k]=v},getAttribute:function(k){return this[k]},appendChild:function(c){this.childNodes.push(c)},insertBefore:function(c,r){this.childNodes.push(c)},removeChild:function(c){var i=this.childNodes.indexOf(c);if(i>-1)this.childNodes.splice(i,1)},replaceChild:function(n,o){var i=this.childNodes.indexOf(o);if(i>-1)this.childNodes[i]=n},addEventListener:function(){},style:{},parentNode:null};return el}`,
		`document.body={appendChild:function(c){},childNodes:[]}`,
		`document.createTextNode=function(t){return{nodeType:3,textContent:t,nodeValue:t}}`,
		`document.createComment=function(t){return{nodeType:8,textContent:t,nodeValue:t}}`,
	}
	for _, p := range pf {
		if _, err := vm.Run(p); err != nil {
			t.Fatalf("polyfill: %v", err)
		}
	}

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

	// Run bundle
	_, err = vm.Run(bundleSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Bundle error: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "Bundle OK\n")
	}

	// Deep diagnostics
	diagCode := `
		console.log('=== DEEP_DIAG ===');
		// Check window.App
		console.log('WINDOW_APP: '+(window.App?'yes':'no'));
		console.log('WINDOW_createApp: '+(typeof window.createApp));
		
		// Check the mounted app's internal state
		var app = document.querySelector('#app');
		if(app && app.__vue_app__) {
			var vueApp = app.__vue_app__;
			console.log('VUE_APP_TYPE: '+(typeof vueApp));
			console.log('VUE_APP_UID: '+vueApp._uid);
			console.log('VUE_APP_INSTANCE: '+(vueApp._instance?'yes':'no'));
			
			var inst = vueApp._instance;
			if(inst) {
				console.log('INST_RENDER: '+(typeof inst.render));
				console.log('INST_RENDER_SRC: '+(inst.render?inst.render.toString().substring(0,100):'null'));
				console.log('INST_SUBTREE: '+(inst.subTree?'yes':'no'));
				console.log('INST_IS_MOUNTED: '+inst.isMounted);
				console.log('INST_VNODE: '+(inst.vnode?'yes':'no'));
				console.log('INST_TYPE_RENDER: '+(inst.type && typeof inst.type.render));
				if(inst.subTree) {
					console.log('SUBTREE_TYPE: '+(typeof inst.subTree));
					console.log('SUBTREE_TAG: '+(inst.subTree.type));
					console.log('SUBTREE_EL: '+(typeof inst.subTree.el));
					console.log('SUBTREE_CHILDREN: '+(inst.subTree.children));
					console.log('SUBTREE_SHAPEFLAG: '+inst.subTree.shapeFlag);
				}
				// Check what render returns
				if(typeof inst.render === 'function') {
					try {
						var proxy = inst.proxy;
						var cache = inst.renderCache;
						var result = inst.render.call(proxy, proxy, cache, {}, {}, {}, {});
						console.log('DIRECT_RENDER_RESULT: '+(typeof result));
						console.log('DIRECT_RENDER_TYPE: '+(result?result.type:'null'));
						console.log('DIRECT_RENDER_TAG: '+(result?result.type:'null'));
					} catch(e) {
						console.log('DIRECT_RENDER_ERR: '+e);
					}
				}
				// Check if the error handler was triggered
				console.log('INST_HANDLE_ERROR: '+(typeof inst.handleError));
			}
		}
		// Check _export_sfc result
		try {
			var sfc = {__vccOpts:null};
			var result = _export_sfc(sfc, [["render", function(){}]]);
			console.log('EXPORT_SFC_RENDER: '+(typeof result.render));
			console.log('EXPORT_SFC_KEYS: '+Object.keys(result).join(','));
		} catch(e) {
			console.log('EXPORT_SFC_ERR: '+e);
		}
	`
	_, err = vm.Run(diagCode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Diag error: %v\n", err)
	}

	out := logger.String()
	fmt.Printf("=== OUTPUT ===\n%s\n=== END ===\n", out)
}
