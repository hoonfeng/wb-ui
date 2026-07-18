package jsc

import (
	"fmt"
	"testing"
)

// TestExactVuePattern 精确复现 Vue bundle 的 ml 模式
func TestExactVuePattern(t *testing.T) {
	vm := NewInterpreter()
	log := &BufferLogger{}
	vm.SetupGlobal(log)

	code := `
		// 精确复制 Vue bundle 的 ml 模式
		var _l = {createApp: function(comp) {
			var app = {
				_container: null,
				_component: comp,
				mount: function(container) {
					this._container = container;
					return 'ok';
				}
			};
			return app;
		}};

		// 精确复制 Vue bundle 的 ml: 逗号表达式 + 函数覆盖
		var ml = (function() {
			return function() {
				var t = _l.createApp({});
				var s = t.mount;
				// 关键: 和 bundle 一模一样的逗号表达式
				return t.mount = function(n) {
					var i = n;
					var r = t._component;
					var o = s.call(t, i);
					return o;
				}, t;
			};
		})();

		var je = ml();
		console.log('M_TYP: '+(typeof je.mount));
		console.log('M_NAME: "'+je.mount.name+'"');
		
		// 测试方法调用
		try {
			je.mount('div');
			console.log('DIRECT_OK');
			console.log('CONT: '+je._container);
		} catch(e) {
			console.log('DIRECT_ERR: '+e);
			try {
				var fn = je.mount;
				fn.call(je, 'call_div');
				console.log('CALL_OK');
				console.log('CONT2: '+je._container);
			} catch(e2) {
				console.log('CALL_ERR2: '+e2);
			}
		}
	`

	if _, err := vm.Run(code); err != nil {
		t.Fatalf("err: %v", err)
	}
	out := log.String()
	fmt.Printf("=== EXACT VUE PATTERN ===\n%s\n=== END ===\n", out)
}

// TestCommaReturnPattern 测试 comma + return 模式
func TestCommaReturnPattern(t *testing.T) {
	vm := NewInterpreter()
	log := &BufferLogger{}
	vm.SetupGlobal(log)

	code := `
		function makeObj() {
			var obj = {val: 1, getVal: function(x) { return this.val + x; }};
			var old = obj.getVal;
			return obj.getVal = function(x) { return old.call(this, x); }, obj;
		}
		
		var o = makeObj();
		console.log('T: '+typeof o.getVal);
		console.log('R: '+o.getVal(5));
	`

	if _, err := vm.Run(code); err != nil {
		t.Fatalf("err: %v", err)
	}
	out := log.String()
	fmt.Printf("=== COMMA RETURN ===\n%s\n=== END ===\n", out)
}
