package jsc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestVue3AutoDiagnose 自动诊断：在 bundle 中注入详细的错误诊断
func TestVue3AutoDiagnose(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	// Polyfills
	pf := []string{
		`window={process:{env:{NODE_ENV:"production"}}}`,
		`Object.getOwnPropertyNames=function(o){if(!o)return[];var k=[];for(var n in o)k.push(n);return k}`,
		`Object.hasOwn=function(o,p){return Object.prototype.hasOwnProperty.call(o,p)}`,
		`document={}`,
		`document.createElement=function(t){var el={tagName:t.toUpperCase(),innerHTML:'',textContent:'',childNodes:[],setAttribute:function(){},getAttribute:function(){return''},appendChild:function(c){this.childNodes.push(c)},removeChild:function(c){var i=this.childNodes.indexOf(c);if(i>-1)this.childNodes.splice(i,1)},insertBefore:function(c,r){this.childNodes.push(c)},style:{},parentNode:null};return el}`,
		`document.body={appendChild:function(c){},insertBefore:function(c,r){this.childNodes.push(c)},childNodes:[]}`,
		`document.createTextNode=function(t){return{nodeType:3,textContent:t}}`,
		`document.createComment=function(t){return{nodeType:8}}`,
		`document.querySelector=function(){return{innerHTML:'',_vnode:null,__vue_app__:null,childNodes:[]}}`,
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

	// 在 bundle 末尾注入诊断代码（替换最终的 try-catch-mount 块）
	// 原始代码以 })(); 结尾。我们在 })(); 之前添加诊断代码
	oldEnd := `try{var Mi=document.createElement("div");document.body.appendChild(Mi),window.__BEFORE_MOUNT__=1,je.mount(Mi),window.__S7__="OK"}catch(e){window.__S7__=""+e,window.__AFTER_ERR__=je._instance?"instance_set":"instance_null"}})();`

	newEnd := `
try {
  var Mi=document.createElement("div");
  document.body.appendChild(Mi);
  window.__BEFORE_MOUNT__=1;
  // 包装 mount 以捕获详细错误
  var origMount = je.mount;
  je.mount = function(c) {
    console.log('DIAG_MOUNT: container=' + (c ? c.tagName : 'null'));
    console.log('DIAG_MOUNT: this=' + (typeof this) + (this && this._component ? ' has_component' : ''));
    try {
      var result = origMount.call(je, c);
      console.log('DIAG_MOUNT_SUCCESS');
      window.__DIAG_MOUNT_OK__ = 1;
      return result;
    } catch(e) {
      console.log('DIAG_MOUNT_ERR: ' + e);
      window.__DIAG_ERR__ = '' + e;
      throw e;
    }
  };
  console.log('DIAG_MOUNT_CALLING');
  je.mount(Mi);
  window.__S7__="OK";
} catch(e) {
  window.__S7__=""+e;
  window.__AFTER_ERR__=je._instance?"instance_set":"instance_null";
}
})();`

	modSrc := strings.Replace(bundleSrc, oldEnd, newEnd, 1)
	if modSrc == bundleSrc {
		fmt.Fprintf(os.Stderr, "WARN: bundle end pattern not found, running original\n")
		vm.Run(bundleSrc)
	} else {
		fmt.Fprintf(os.Stderr, "Bundle modified at end - mount wrapped with diagnostics\n")
		vm.Run(modSrc)
	}

	// 输出诊断结果
	vm.Run(`console.log('S1: '+(window.__S1__||'undef'))`)
	vm.Run(`console.log('S7: '+(window.__S7__||'undef'))`)
	vm.Run(`console.log('AFTER_ERR: '+(window.__AFTER_ERR__||'undef'))`)

	out := logger.String()
	fmt.Printf("=== DIAGNOSTIC RESULTS ===\n%s\n", out)
}
