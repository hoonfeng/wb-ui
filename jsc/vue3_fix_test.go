package jsc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestVue3FinalFix 应用已验证有效的修复：包装 mount 函数
func TestVue3FinalFix(t *testing.T) {
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

	// 应用已验证的修复：替换末尾的 mount 调用块
	oldEnd := `try{var Mi=document.createElement("div");document.body.appendChild(Mi),window.__BEFORE_MOUNT__=1,je.mount(Mi),window.__S7__="OK"}catch(e){window.__S7__=""+e,window.__AFTER_ERR__=je._instance?"instance_set":"instance_null"}})();`

	newEnd := `
try {
  var Mi=document.createElement("div");
  document.body.appendChild(Mi);
  window.__BEFORE_MOUNT__=1;
  var origMount = je.mount;
  je.mount = function(c) {
    return origMount.call(je, c);
  };
  je.mount(Mi);
  window.__S7__="OK";
} catch(e) {
  window.__S7__=""+e;
  window.__AFTER_ERR__=je._instance?"instance_set":"instance_null";
}
})();`

	fixedSrc := strings.Replace(bundleSrc, oldEnd, newEnd, 1)

	if fixedSrc == bundleSrc {
		fmt.Fprintf(os.Stderr, "WARN: replacement pattern not found\n")
		vm.Run(bundleSrc)
	} else {
		fmt.Fprintf(os.Stderr, "FIX APPLIED\n")
		vm.Run(fixedSrc)
	}

	// 检查结果
	vm.Run(`console.log('S1: '+(window.__S1__||'undef'))`)
	vm.Run(`console.log('S7: '+(window.__S7__||'undef'))`)
	vm.Run(`console.log('AFTER_ERR: '+(window.__AFTER_ERR__||'undef'))`)

	// 检查 Vue 实例
	vm.Run(`console.log('INSTANCE: '+(je._instance?(je._instance.isMounted?'mounted':'exists'):'null'))`)
	// 检查容器
	vm.Run(`try{console.log('APP_CONTAINER: '+(je._container?'exists':'null'))}catch(e){console.log('CTNR_ERR:'+e)}`)
	// 检查子节点
	vm.Run(`try{var d=document.querySelector('#app');console.log('APP_CHILDREN: '+d.childNodes.length)}catch(e){console.log('CHILD_ERR:'+e)}`)

	out := logger.String()
	fmt.Printf("=== RESULTS ===\n%s\n", out)
}
