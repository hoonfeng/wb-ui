package jsc

import (
	"fmt"
	"os"
	"testing"
)

// TestCreateAppDiagnose 诊断 TestVue3CreateApp 失败的确切原因
func TestCreateAppDiagnose(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	// 与 TestVue3CreateApp 完全相同的 polyfill
	vm.Run(`window={process:{env:{NODE_ENV:"production"}}}`)
	vm.Run(`document={}`)
	vm.Run(`document.createElement=function(t){return{tagName:t,childNodes:[],insertBefore:function(){},textContent:'',setAttribute:function(){}}}`)

	// 测试：打印 container 的 keys
	vm.Run(`
var c = document.createElement('div');
console.log('KEYS: ' + Object.keys(c).join(','));
console.log('TAG: ' + c.tagName);
console.log('APPEND_CHILD: ' + (typeof c.appendChild));
console.log('INSERT_BEFORE: ' + (typeof c.insertBefore));
// 手动添加 appendChild
c.appendChild = function(child) { this.childNodes.push(child); };
console.log('APPEND_CHILD_AFTER: ' + (typeof c.appendChild));
// 测试调用
c.appendChild({});
console.log('CHILDREN: ' + c.childNodes.length);
`)

	out := logger.String()
	fmt.Fprintf(os.Stderr, "Results:\n%s\n", out)
}
