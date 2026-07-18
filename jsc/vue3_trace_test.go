package jsc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestVue3TraceMountAccess 追踪 je.mount 属性访问
func TestVue3TraceMountAccess(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

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

	// 在末尾注入诊断，在 mount 前检查 je.mount
	oldEnd := `try{var Mi=document.createElement("div");document.body.appendChild(Mi),window.__BEFORE_MOUNT__=1,je.mount(Mi),window.__S7__="OK"}catch(e){window.__S7__=""+e,window.__AFTER_ERR__=je._instance?"instance_set":"instance_null"}})();`

	newEnd := `
try {
  var Mi=document.createElement("div");
  document.body.appendChild(Mi);
  window.__BEFORE_MOUNT__=1;
  
  // === 详细诊断 je.mount ===
  console.log('MTYPE1: '+(typeof je.mount));
  console.log('MKEYS1: '+(je?Object.keys(je).join(','):'no_je'));
  console.log('MHAS1: '+('mount' in je));
  console.log('MPROTO: '+(Object.getPrototypeOf(je)===Object.prototype));
  
  // 尝试获取 mount 的各种方式
  try { console.log('MDIRECT: '+(typeof je['mount'])); } catch(e) { console.log('MDIRECT_ERR: '+e); }
  try { console.log('MGET: '+Object.getOwnPropertyDescriptor(je,'mount')); } catch(e) { console.log('MGET_ERR: '+e); }
  
  // 保存引用再调用
  var savedMount = je.mount;
  console.log('MSAVED: '+(typeof savedMount));
  
  // 检查函数属性
  if(typeof savedMount === 'function') {
    console.log('MFN_NAME: '+savedMount.name);
    console.log('MFN_LEN: '+savedMount.length);
  }

  // 用 .call 方式调用
  je.mount(Mi);
  window.__S7__="OK";
} catch(e) {
  window.__S7__=""+e;
  window.__AFTER_ERR__=je._instance?"instance_set":"instance_null";
}
})();`

	modSrc := strings.Replace(bundleSrc, oldEnd, newEnd, 1)
	if modSrc == bundleSrc {
		fmt.Fprintf(os.Stderr, "WARN: pattern not found\n")
		vm.Run(bundleSrc)
	} else {
		fmt.Fprintf(os.Stderr, "DIAG INJECTED\n")
		vm.Run(modSrc)
	}

	vm.Run(`console.log('S7: '+(window.__S7__||'undef'))`)

	out := logger.String()
	fmt.Printf("=== DIAG ===\n%s\n=== END ===\n", out)
}
