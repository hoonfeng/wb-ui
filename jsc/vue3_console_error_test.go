package jsc

import (
	"os"
	"strings"
	"testing"
	"fmt"
)

func TestVue3ConsoleError(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	// 测试 console.error 是否工作
	vm.Run(`console.error("test error message")`)
	vm.Run(`console.log("test log message")`)
	
	out := logger.String()
	fmt.Printf("=== OUTPUT ===\n%s\n=== END ===\n", out)

	// 现在运行 bundle 并检查 console.error 输出
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
		`var __appEl={innerHTML:'x',__vue_app__:null,_vnode:null,childNodes:[],appendChild:function(c){this.childNodes.push(c)},insertBefore:function(c,r){this.childNodes.push(c)},setAttribute:function(k,v){},getAttribute:function(k){return null},addEventListener:function(){},style:{},parentNode:null,tagName:'DIV'}`,
		`document={}`,
		`document.getElementById=function(s){return __appEl}`,
		`document.querySelector=function(s){return __appEl}`,
		`document.createElement=function(t){return{tagName:t.toUpperCase(),childNodes:[],innerHTML:'',setAttribute:function(k,v){},getAttribute:function(k){return null},appendChild:function(c){this.childNodes.push(c)},insertBefore:function(c,r){this.childNodes.push(c)},style:{},parentNode:null}}`,
		`document.body={appendChild:function(c){},insertBefore:function(c,r){},childNodes:[]}`,
		`document.createTextNode=function(t){return{nodeType:3,textContent:t}}`,
		`document.createComment=function(t){return{nodeType:8}}`,
	}
	for _, p := range pf {
		vm.Run(p)
	}

	// Inject console.error tracer into bundle
	data, _ := os.ReadFile("F:/syproject/gou-ide/cmd/desktop/web-ui-minimal/dist/assets/app-BwgZYNXz.js")
	src := string(data)

	// Override console.error to also use console.log
	injectCode := `
console.log('TRACE_BUNDLE_START');
var __origConsoleError = console.error;
console.error = function(msg) {
	console.log('CONSOLE_ERROR: ' + (msg && msg.message ? msg.message : msg));
	console.log('CONSOLE_ERROR_RAW: ' + msg);
	return __origConsoleError(msg);
};
`
	src = strings.Replace(src, `(function(){"use strict";`, `(function(){"use strict";`+injectCode, 1)

	// Run bundle
	_, err := vm.Run(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Bundle error: %v\n", err)
	}

	out = logger.String()
	fmt.Printf("=== FULL OUTPUT ===\n%s\n=== END ===\n", out)
}
