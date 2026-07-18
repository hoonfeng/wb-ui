package jsc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestVue3CallDiag 精确诊断 je.mount(Mi) 调用失败根因
func TestVue3CallDiag(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	// 基础 polyfill - 只用最少的
	pf := []string{
		`window={process:{env:{NODE_ENV:"production"}}}`,
		// 必须的 DOM stub（只包含 mountElement 需要的）
		`document={}`,
		`document.createElement=function(t){return{tagName:t.toUpperCase(),innerHTML:'',childNodes:[],setAttribute:function(){},getAttribute:function(){return''},appendChild:function(c){this.childNodes.push(c)},removeChild:function(c){var i=this.childNodes.indexOf(c);if(i>-1)this.childNodes.splice(i,1)},insertBefore:function(c,r){this.childNodes.push(c)},style:{},parentNode:null,textContent:''}}`,
		`document.body={appendChild:function(c){},insertBefore:function(c,r){},childNodes:[]}`,
		`document.createTextNode=function(t){return{nodeType:3,textContent:t}}`,
		`document.createComment=function(t){return{nodeType:8}}`,
		`document.querySelector=function(){return{innerHTML:'',_vnode:null,__vue_app__:null,childNodes:[]}}`,
	}
	for _, p := range pf {
		if _, err := vm.Run(p); err != nil {
			t.Fatalf("polyfill err: %v", err)
		}
	}

	// 读取 bundle
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

	// Step 1: 先执行 bundle（IIFE），但不触发 mount
	// 让我们在 bundle 末尾注入诊断，在 mount 前做详尽检查
	oldEnd := `try{var Mi=document.createElement("div");document.body.appendChild(Mi),window.__BEFORE_MOUNT__=1,je.mount(Mi),window.__S7__="OK"}catch(e){window.__S7__=""+e,window.__AFTER_ERR__=je._instance?"instance_set":"instance_null"}})();`

	newEnd := `
try {
  var Mi=document.createElement("div");
  document.body.appendChild(Mi);
  window.__BEFORE_MOUNT__=1;

  // === 深入诊断 je.mount ===
  // 1) 先验证 je 对象本身
  console.log('JE_TYPE: '+(typeof je));
  console.log('JE_CLASS: '+(je&&je.constructor?je.constructor.name:'no_cons'));
  console.log('JE_KEYS: '+(je?Object.keys(je).join(','):'no_je'));

  // 2) 验证 mount 属性存在性 (4种方式)
  console.log('M_IN: '+('mount'in je));
  console.log('M_HAS: '+(Object.prototype.hasOwnProperty.call(je,'mount')));
  console.log('M_TYP: '+(typeof je.mount));
  console.log('M_BRK: '+(typeof je['mount']));
  
  // 3) 获取 descriptor
  var md = Object.getOwnPropertyDescriptor(je,'mount');
  console.log('M_DESC: '+(md?'exists('+typeof md.value+','+typeof md.get+')':'null'));
  if(md) {
    console.log('M_VAL: '+(typeof md.value));
    console.log('M_GET: '+(typeof md.get));
    console.log('M_CONF: '+md.configurable);
    console.log('M_ENUM: '+md.enumerable);
  }
  
  // 4) 尝试获取 mount 函数
  var mfn = je.mount;
  console.log('MFN_TYP: '+(typeof mfn));
  console.log('MFN_NAME: "'+mfn.name+'"');
  console.log('MFN_LEN: '+mfn.length);
  console.log('MFN_PROT: '+(typeof mfn.prototype));
  if(typeof mfn === 'function') {
    // 用 .call() 调用
    try {
      var r1 = mfn.call(je, Mi);
      console.log('CALL_OK: '+r1);
      window.__S7__="OK";
    } catch(e2) {
      console.log('CALL_ERR: '+e2);
      // 尝试直接调用
      try {
        var r2 = mfn(Mi);
        console.log('DIRECT_OK: '+r2);
      } catch(e3) {
        console.log('DIRECT_ERR: '+e3);
      }
    }
  } else {
    // 如果 mount 不是函数，检查原型链
    var proto = Object.getPrototypeOf(je);
    console.log('PROTO: '+(proto?'exists':'null'));
    if(proto) {
      console.log('PROTO_M: '+(typeof proto.mount));
      console.log('PROTO_KS: '+(Object.keys(proto).join(',')));
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
	fmt.Printf("=== DIAG OUTPUT ===\n%s\n=== END ===\n", out)
}
