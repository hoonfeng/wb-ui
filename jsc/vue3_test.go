package jsc

import (
	"fmt"
	"os"
	"testing"
)

func TestVue3BundleLoad(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	// Same polyfills as desktop/main.go
	pf := []string{
		`window.process={env:{NODE_ENV:"production"}}`,
		`Object.getOwnPropertyNames=function(o){if(!o)return [];var k=[];for(var n in o)k.push(n);return k}`,
		`Object.hasOwn=function(o,p){return Object.prototype.hasOwnProperty.call(o,p)}`,
		`Object.fromEntries=function(e){var r={};for(var i=0;e&&i<e.length;i++)if(e[i])r[e[i][0]]=e[i][1];return r}`,
		`if(!Array.prototype.flatMap)Array.prototype.flatMap=function(f){var r=[];for(var i=0;i<this.length;i++){var v=f(this[i],i,this);if(v&&v.length)for(var j=0;j<v.length;j++)r.push(v[j]);else r.push(v)}return r}`,
		`if(!Array.prototype.join)Array.prototype.join=function(s){s=s!==undefined?s:',';var r='';for(var i=0;i<this.length;i++){if(i>0)r+=s;if(this[i]!==null&&this[i]!==undefined)r+=this[i]}return r}`,
		`if(!Array.prototype.keys)Array.prototype.keys=function(){var i=0;return{next:function(){return i<this.length?{value:i++,done:false}:{done:true}}}}`,
		`if(!Array.prototype.reduceRight)Array.prototype.reduceRight=function(f,i){var a=this;var s=i!==undefined?a.length-1:a.length-2;var v=i!==undefined?i:a[a.length-1];for(var j=s;j>=0;j--)v=f(v,a[j],j,a);return v}`,
		`if(!Array.prototype.at)Array.prototype.at=function(i){var n=Number(i);if(isNaN(n))n=0;var l=this.length;n=n>=0?n:l+n;if(n<0||n>=l)return undefined;return this[n]}`,
	}
	for _, p := range pf {
		if _, err := vm.Run(p); err != nil {
			t.Fatalf("polyfill failed: %v", err)
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

	_, err = vm.Run(string(data))
	if err != nil {
		t.Fatalf("bundle error: %v", err)
	}
	fmt.Fprintf(os.Stderr, "Output:\n%s\n", logger.String())
}
