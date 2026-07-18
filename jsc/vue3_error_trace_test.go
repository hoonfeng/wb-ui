package jsc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestVue3MountErrorTrace 追踪 mount 执行期间的错误堆栈
func TestVue3MountErrorTrace(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	pf := []string{
		`window={process:{env:{NODE_ENV:"production"}}}`,
		`Object.getOwnPropertyNames=function(o){if(!o)return[];var k=[];for(var n in o)k.push(n);return k}`,
		`document={}`,
		`document.createElement=function(t){var el={tagName:t.toUpperCase(),innerHTML:'',textContent:'',childNodes:[],setAttribute:function(){},getAttribute:function(){return''},appendChild:function(c){this.childNodes.push(c)},removeChild:function(c){var i=this.childNodes.indexOf(c);if(i>-1)this.childNodes.splice(i,1)},insertBefore:function(c,r){this.childNodes.push(c)},style:{},parentNode:null};return el}`,
		`document.body={appendChild:function(c){},insertBefore:function(c,r){this.childNodes.push(c)},childNodes:[]}`,
		`document.createTextNode=function(t){return{nodeType:3,textContent:t}}`,
		`document.createComment=function(t){return{nodeType:8}}`,
		`document.querySelector=function(){return{innerHTML:'',_vnode:null,__vue_app__:null,childNodes:[]}}`,
	}
	for _, p := range pf {
		if _, err := vm.Run(p); err != nil {
			t.Fatalf("polyfill err: %v", err)
		}
	}

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

	// 在末尾注入: 使用 try/catch + 错误堆栈追踪 mount
	oldEnd := `try{var Mi=document.createElement("div");document.body.appendChild(Mi),window.__BEFORE_MOUNT__=1,je.mount(Mi),window.__S7__="OK"}catch(e){window.__S7__=""+e,window.__AFTER_ERR__=je._instance?"instance_set":"instance_null"}})();`

	newEnd := `
try {
  var Mi=document.createElement("div");
  document.body.appendChild(Mi);
  window.__BEFORE_MOUNT__=1;

  // === 方法1: 直接调用 ===
  console.log('METH1: <direct>');
  try {
    je.mount(Mi);
    window.__S7__="OK";
    console.log('METH1_OK');
  } catch(e1) {
    console.log('METH1_ERR: '+e1);
    
    // === 方法2: 保存引用后 .call() ===
    console.log('METH2: <call>');
    try {
      var mfn = je.mount;
      console.log('MFN_TYP: '+(typeof mfn));
      if(typeof mfn === 'function') {
        var r = mfn.call(je, Mi);
        console.log('METH2_OK: '+r);
        window.__S7__="OK";
      } else {
        console.log('MFN_IS: '+(mfn===null?'null':mfn===undefined?'undef':Object.prototype.toString.call(mfn)));
      }
    } catch(e2) {
      console.log('METH2_ERR: '+e2);
      
      // === 方法3: 直接执行 mount 函数体 ===
      console.log('METH3: <manual>');
      try {
        // 手动设置 container
        var container = Mi;
        var t = je;
        var s = t.mount;
        console.log('S_TYP: '+(typeof s));
        if(typeof s === 'function') {
          var r2 = s.call(t, container, false, undefined);
          console.log('METH3_OK: '+r2);
          window.__S7__="OK";
        }
      } catch(e3) {
        console.log('METH3_ERR: '+e3);
      }
    }
  }
} catch(e) {
  window.__S7__=""+e;
  window.__AFTER_ERR__=je&&je._instance?"instance_set":"instance_null";
}
})();`

	modSrc := strings.Replace(bundleSrc, oldEnd, newEnd, 1)
	if modSrc == bundleSrc {
		fmt.Fprintf(os.Stderr, "WARN: pattern not found, running original\n")
		vm.Run(bundleSrc)
	} else {
		vm.Run(modSrc)
	}

	vm.Run(`console.log('S7: '+(window.__S7__||'undef'))`)
	vm.Run(`console.log('AFTER: '+(window.__AFTER_ERR__||'none'))`)

	out := logger.String()
	fmt.Printf("=== MOUNT ERROR TRACE ===\n%s\n=== END ===\n", out)
}
