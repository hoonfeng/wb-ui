package jsc

import (
	"fmt"
	"os"
	"testing"
)

func TestVue3HFunction(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	// Apply polyfills (same as desktop/main.go)
	pf := []string{
		`window.process={env:{NODE_ENV:"production"}}`,
		`Object.getOwnPropertyNames=function(o){if(!o)return[];var k=[];for(var n in o)k.push(n);return k}`,
		`Object.hasOwn=function(o,p){return Object.prototype.hasOwnProperty.call(o,p)}`,
		`Object.fromEntries=function(e){var r={};for(var i=0;e&&i<e.length;i++)if(e[i])r[e[i][0]]=e[i][1];return r}`,
		`if(!Array.prototype.flatMap)Array.prototype.flatMap=function(f){var r=[];for(var i=0;i<this.length;i++){var v=f(this[i],i,this);if(v&&v.length)for(var j=0;j<v.length;j++)r.push(v[j]);else r.push(v)}return r}`,
		`if(!Array.prototype.join)Array.prototype.join=function(s){s=s!==undefined?s:',';var r='';for(var i=0;i<this.length;i++){if(i>0)r+=s;if(this[i]!==null&&this[i]!==undefined)r+=this[i]}return r}`,
		`if(!Array.prototype.keys)Array.prototype.keys=function(){var i=0;return{next:function(){return i<this.length?{value:i++,done:false}:{done:true}}}}`,
		`if(!Array.prototype.at)Array.prototype.at=function(i){var n=Number(i);if(isNaN(n))n=0;var l=this.length;n=n>=0?n:l+n;if(n<0||n>=l)return undefined;return this[n]}`,
		// Object static methods needed by Vue 3
		`if(!Object.isExtensible)Object.isExtensible=function(){return true}`,
		`if(!Object.isSealed)Object.isSealed=function(){return false}`,
		`if(!Object.isFrozen)Object.isFrozen=function(){return false}`,
		`if(!Object.getPrototypeOf)Object.getPrototypeOf=function(o){return o?o.constructor?o.constructor.prototype:null:null}`,
		`if(!Object.setPrototypeOf)Object.setPrototypeOf=function(o,p){o.__proto__=p;return o}`,
		`if(!Object.preventExtensions)Object.preventExtensions=function(o){return o}`,
		`if(!Object.seal)Object.seal=function(o){return o}`,
		`if(!Array.prototype.reduceRight)delete Array.prototype.reduceRight`, // remove bad polyfill
		// Minimal DOM stubs for h() testing
		`document={}`,
		`document.querySelector=function(s){if(s==='#app')return{innerHTML:'',__vue_app__:null,_vnode:null,appendChild:function(c){}};return null}`,
		`document.createElement=function(t){return{tagName:t.toUpperCase(),innerHTML:'',textContent:'',setAttribute:function(){},getAttribute:function(){return''},appendChild:function(c){},removeChild:function(){},replaceChild:function(){},addEventListener:function(){},style:{},childNodes:[],parentNode:null}}`,
		`window.__h_steps=[];window.__h_err=null`,
	}
	for _, p := range pf {
		if _, err := vm.Run(p); err != nil {
			t.Fatalf("polyfill: %v", err)
		}
	}

	// Read Vue 3 bundle
	distDir := "F:/syproject/gou-ide/cmd/desktop/web-ui-minimal/dist/assets"
	entries, err := os.ReadDir(distDir)
	if err != nil {
		t.Skip("dist not found:", err)
	}
	var bp string
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > 3 && e.Name()[len(e.Name())-3:] == ".js" {
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
	bundle := string(data)

	// Run the bundle
	_, err = vm.Run(bundle)
	if err != nil {
		t.Fatalf("bundle error: %v", err)
	}
	// Reset logger output
	logger.Lines = nil

	// Check what globals were set
	tests := []string{
		`console.log('GLOBALS: S1='+(window.__S1__||'none')+' S2='+(window.__S2__||'none')+' S3='+(window.__S3__||'none')+' S4='+(window.__S4__||'none')+' S5='+(window.__S5__||'none')+' S6='+(window.__S6__||'none')+' S7='+(window.__S7__||'none'))`,
		`if(window.__APP_ERR__)console.log('ERR: '+window.__APP_ERR__)`,
	}
	for _, c := range tests {
		vm.Run(c)
	}
	fmt.Fprintf(os.Stderr, "Output:\n%s\n", logger.String())
}
