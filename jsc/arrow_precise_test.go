package jsc

import (
	"fmt"
	"testing"
)

// TestArrowMethodCallPrecise 精确隔离测试: 箭头函数在闭包中作为方法调用
func TestArrowMethodCallPrecise(t *testing.T) {
	vm := NewInterpreter()
	log := &BufferLogger{}
	vm.SetupGlobal(log)

	// 测试1: 外部函数内的箭头函数赋给对象属性后调用
	t.Run("arrow_in_closure_as_method", func(t *testing.T) {
		log.Clear()
		code := `
			function makeApp() {
				var internal = {val: 42};
				var app = {
					name: 'test',
					getVal: () => { return internal.val; }
				};
				return app;
			}
			var app = makeApp();
			console.log('RESULT: '+app.getVal());
		`
		if _, err := vm.Run(code); err != nil {
			t.Fatalf("error: %v", err)
		}
		fmt.Printf("--- arrow_in_closure_as_method ---\n%s\n", log.String())
	})

	// 测试2: 用中间变量 s 保存原方法, 箭头函数覆盖 - 精确模拟 Vue 模式
	t.Run("vue_mount_pattern_arrow", func(t *testing.T) {
		log.Clear()
		code := `
			// 创建一个带 mount 的 app
			var app = {
				_container: null,
				mount: function(container) {
					this._container = container;
					return 'ok';
				}
			};
			
			// Vue 的 ml 模式: 提取 mount, 用箭头函数覆盖
			(function() {
				var t = app;
				var s = t.mount;
				t.mount = (n) => {
					var result = s.call(t, n);
					return result;
				};
			})();
			
			console.log('M_TYPE: '+(typeof app.mount));
			try {
				var r = app.mount('div');
				console.log('RESULT: '+r);
				console.log('CONTAINER: '+app._container);
			} catch(e) {
				console.log('ERR: '+e);
			}
		`
		if _, err := vm.Run(code); err != nil {
			t.Fatalf("error: %v", err)
		}
		fmt.Printf("--- vue_mount_pattern_arrow ---\n%s\n", log.String())
	})

	// 测试3: 箭头函数在 IIFE 中创建后赋给属性 - JSC 核心问题
	t.Run("iife_arrow_method", func(t *testing.T) {
		log.Clear()
		code := `
			var obj = (function() {
				var self = {val: 1};
				return {
					val: 999,
					arrowFn: () => { return self.val; }
				};
			})();
			console.log('A_TYPE: '+(typeof obj.arrowFn));
			try {
				var r = obj.arrowFn();
				console.log('A_RESULT: '+r);
			} catch(e) {
				console.log('A_ERR: '+e);
			}
		`
		if _, err := vm.Run(code); err != nil {
			t.Fatalf("error: %v", err)
		}
		fmt.Printf("--- iife_arrow_method ---\n%s\n", log.String())
	})

	// 测试4: 用 OpDup+OpLoadProp+OpCallMethod 调箭头函数 (精确模拟JSC字节码)
	t.Run("dup_load_call_arrow", func(t *testing.T) {
		log.Clear()
		code := `
			// 最简单形式: 立即执行的 IIFE 返回带箭头方法调用的对象
			var maker = function() {
				var secret = 'secret_value';
				var result = {
					getIt: () => { return secret; }
				};
				return result;
			};
			var obj = maker();
			console.log('G_TYPE: '+(typeof obj.getIt));
			var r = obj.getIt();
			console.log('G_VAL: '+r);
		`
		if _, err := vm.Run(code); err != nil {
			t.Fatalf("error: %v", err)
		}
		fmt.Printf("--- dup_load_call_arrow ---\n%s\n", log.String())
	})

	// 测试5: 完全模拟 bundle 的 ml 模式 - destructure + arrow override + call
	t.Run("full_vue_pattern", func(t *testing.T) {
		log.Clear()
		code := `
			// 模拟 _l().createApp
			var _l = (function() {
				return {
					createApp: function(comp) {
						return {
							_component: comp,
							_container: null,
							mount: function(container) {
								this._container = container;
								return 'mounted:' + container;
							}
						};
					}
				};
			})();

			// 模拟 ml 函数
			var ml = (function() {
				return function() {
					var t = _l.createApp({render: function(){}});
					var s = t.mount;
					
					// 箭头函数覆盖 (这就是 Vue 3 bundle 的写法)
					t.mount = (n) => {
						return s.call(t, n);
					};
					console.log('MTYPE: '+(typeof t.mount));
					
					return t;
				};
			})();

			var je = ml();
			console.log('MTYPE2: '+(typeof je.mount));
			
			try {
				var r = je.mount('container');
				console.log('SUCCESS: '+r);
				console.log('STORED: '+je._container);
			} catch(e) {
				console.log('FAIL: '+e);
			}
		`
		if _, err := vm.Run(code); err != nil {
			t.Fatalf("error: %v", err)
		}
		fmt.Printf("--- full_vue_pattern ---\n%s\n", log.String())
	})

	// 测试6: 解构赋值后再用箭头函数 - 测试重点
	t.Run("destruct_then_arrow", func(t *testing.T) {
		log.Clear()
		code := `
			var app = {
				val: 42,
				getVal: function() { return this.val; }
			};
			
			// 解构提取方法 (Vue 写法的核心)
			var {getVal: s} = app;
			
			// 用箭头函数覆盖 (Vue 写法的核心)
			app.getVal = (n) => {
				return s.call(app, n);
			};
			
			console.log('T1: '+(typeof app.getVal));
			try {
				var r = app.getVal('x');
				console.log('T1_RES: '+r);
			} catch(e) {
				console.log('T1_ERR: '+e);
			}
		`
		if _, err := vm.Run(code); err != nil {
			t.Fatalf("error: %v", err)
		}
		fmt.Printf("--- destruct_then_arrow ---\n%s\n", log.String())
	})
}
