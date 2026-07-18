package jsc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestVue3RenderDirect(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	// Polyfills (same as main test)
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
		// DOM stub
		`document={}`,
		`document.getElementById=function(s){return this.querySelector(s)}`,
		`document.querySelector=function(s){var el={innerHTML:'',__vue_app__:null,_vnode:null,childNodes:[],appendChild:function(c){this.childNodes.push(c)},removeChild:function(c){var i=this.childNodes.indexOf(c);if(i>-1)this.childNodes.splice(i,1)},insertBefore:function(c,r){console.log('MOCK_IB: c='+(typeof c));this.childNodes.push(c)},setAttribute:function(){},getAttribute:function(){return''},addEventListener:function(){},style:{},parentNode:null,tagName:'DIV'};return el}`,
		`document.createElement=function(t){var el={tagName:t.toUpperCase(),innerHTML:'',textContent:'',childNodes:[],setAttribute:function(){},getAttribute:function(){return''},appendChild:function(c){this.childNodes.push(c)},removeChild:function(c){var i=this.childNodes.indexOf(c);if(i>-1)this.childNodes.splice(i,1)},insertBefore:function(c,r){console.log('CE_IB: c='+(typeof c));this.childNodes.push(c)},replaceChild:function(n,o){var i=this.childNodes.indexOf(o);if(i>-1)this.childNodes[i]=n},addEventListener:function(){},style:{},parentNode:null}}`,
		`document.body={appendChild:function(c){console.log('BODY_APPEND');},insertBefore:function(c,r){this.childNodes.push(c)},childNodes:[]}`,
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

	// Replace the end of the bundle to add direct render test
	// Current end:
	// const vl=_l(ml,[["render",bl]]);
	// console.log("A");
	// var yl=hl(vl);
	// console.log("B");
	// var xl=yl.mount("#app");
	// console.log("D ...");
	// 
	// We'll keep the bundle execution but add direct render test

	// Modify bundle end: add direct render test
	// Replace the end section
	oldEnd := `const vl=_l(ml,[["render",bl]]);console.log("A");var yl=hl(vl);console.log("B");var xl=yl.mount("#app");console.log("D root="+typeof xl);var Us=document.getElementById("app");console.log("E children="+Us.childNodes.length+" html=["+Us.innerHTML.substring(0,50)+"]");console.log("F __vue_app__="+(Us.__vue_app__?"yes":"no"))})();`

	newEnd := `const vl=_l(ml,[["render",bl]]);console.log("A");var yl=hl(vl);console.log("B");var xl=yl.mount("#app");console.log("D root="+typeof xl);var Us=document.getElementById("app");console.log("E children="+Us.childNodes.length+" html=["+Us.innerHTML.substring(0,50)+"]");console.log("F __vue_app__="+(Us.__vue_app__?"yes":"no"));
	// === DIRECT RENDER TEST ===
	try {
		var container = document.createElement('div');
		document.body.appendChild(container);
		console.log('RENDER_DIRECT: calling render...');
		// 获取 render 函数 (从 bundle 里找)
		// 在 bundle 中 render 是 _o() 返回的 render
		// 直接 vue 的 h('div', null, 'hello') 渲染
		// testComponent = {render: function() { return h('div', null, 'hello'); }}
		console.log('RENDER_DIRECT: h='+(typeof h));
		if(typeof h === 'function') {
			var vnode = h('div', null, 'hello world');
			console.log('VNODE: type='+(typeof vnode)+' tag='+vnode.type);
			// Try render with createApp
			var testApp = hl({render: function() { return h('div', null, 'direct render'); }});
			console.log('TEST_APP_CREATED');
			try {
				var testResult = testApp.mount(container);
				console.log('TEST_MOUNT: result='+(typeof testResult));
				console.log('TEST_CONTAINER: children='+container.childNodes.length+' html='+container.innerHTML.substring(0,100));
				console.log('TEST_VUE_APP: '+(container.__vue_app__?'yes':'no'));
			} catch(e) {
				console.log('TEST_MOUNT_ERR: '+e);
			}
		} else {
			console.log('RENDER_DIRECT: h is not available');
		}
	} catch(e) {
		console.log('RENDER_DIRECT_ERR: '+e);
	}
	// Check DOM after all operations
	try {
		var bd = document.body;
		console.log('FINAL_BODY: children='+(bd.childNodes?bd.childNodes.length:0));
		for(var vi=0; vi<(bd.childNodes?bd.childNodes.length:0); vi++) {
			var ch = bd.childNodes[vi];
			console.log('  CHILD['+vi+']: tag='+(ch.tagName||'#text')+' html='+(ch.innerHTML||'').substring(0,200));
		}
	} catch(e) {
		console.log('FINAL_ERR: '+e);
	}
})();`

	modSrc := strings.Replace(bundleSrc, oldEnd, newEnd, 1)
	if modSrc == bundleSrc {
		fmt.Fprintf(os.Stderr, "WARN: pattern not found, running original\n")
		_, err = vm.Run(bundleSrc)
	} else {
		_, err = vm.Run(modSrc)
	}
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "Bundle error: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "Bundle OK\n")
	}

	out := logger.String()
	fmt.Printf("=== OUTPUT ===\n%s\n=== END ===\n", out)
}
