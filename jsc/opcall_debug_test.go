package jsc

import (
	"fmt"
	"testing"
)

// TestOpCallMethodDebug 在 OpCallMethod 中添加调试输出
func TestOpCallMethodDebug(t *testing.T) {
	vm := NewInterpreter()
	log := &BufferLogger{}
	vm.SetupGlobal(log)

	// Polyfills (minimum for Vue bundle)
	pf := []string{
		`window={process:{env:{NODE_ENV:"production"}}}`,
		`Object.getOwnPropertyNames=function(o){if(!o)return[];var k=[];for(var n in o)k.push(n);return k}`,
		`document={}`,
		`document.createElement=function(t){return{tagName:t.toUpperCase(),childNodes:[],setAttribute:function(){},getAttribute:function(){return''},appendChild:function(c){this.childNodes.push(c)},insertBefore:function(c,r){},style:{}}}`,
		`document.body={appendChild:function(c){},childNodes:[]}`,
		`document.createTextNode=function(t){return{nodeType:3,textContent:t}}`,
		`document.createComment=function(t){return{nodeType:8}}`,
		`document.querySelector=function(){return{innerHTML:'',childNodes:[]}}`,
	}
	for _, p := range pf {
		vm.Run(p)
	}

	// Test: 简单方法调用
	t.Run("simple_method", func(t *testing.T) {
		log.Clear()
		code := `
			var obj = {val: 42, get: function(){return this.val;}};
			var r = obj.get();
			console.log('R: '+r);
		`
		if _, err := vm.Run(code); err != nil {
			t.Fatalf("err: %v", err)
		}
		fmt.Printf("%s\n", log.String())
	})

	// Test: 箭头函数作为属性被调用 (整个模式)
	t.Run("arrow_final", func(t *testing.T) {
		log.Clear()
		code := `
			// 创建类似 Vue app 的对象
			var app = (function() {
				var internal = {val: 999};
				var t = {
					_container: null,
					mount: function(container) {
						this._container = container;
						return internal.val;
					}
				};
				var s = t.mount;
				t.mount = function(n) {
					return s.call(t, n);
				};
				return t;
			})();
			
			console.log('M_TYP: '+(typeof app.mount));
			var r = app.mount('div');
			console.log('RES: '+r);
			console.log('CONT: '+app._container);
		`
		if _, err := vm.Run(code); err != nil {
			t.Fatalf("err: %v", err)
		}
		fmt.Printf("%s\n", log.String())
	})

	// Test: 真实错误追踪 - 在哪里失败
	t.Run("trace_call_fail", func(t *testing.T) {
		log.Clear()
		code := `
			// 创建一个简单的 app 对象
			var je = {_container: null};
			je.mount = function(c) { this._container = c; return 'ok'; };
			
			// 创建 Mi
			var Mi = document.createElement('div');
			
			// 测试方法调用
			console.log('TRY_DIRECT');
			try {
				je.mount(Mi);
				console.log('DIRECT_OK');
			} catch(e) {
				console.log('DIRECT_ERR: '+e);
				// fallback
				try {
					var fn = je.mount;
					console.log('FN_TYP: '+(typeof fn));
					var r = fn.call(je, Mi);
					console.log('CALL_OK: '+r);
				} catch(e2) {
					console.log('CALL_ERR2: '+e2);
				}
			}
			console.log('CONT: '+je._container);
		`
		if _, err := vm.Run(code); err != nil {
			t.Fatalf("err: %v", err)
		}
		fmt.Printf("%s\n", log.String())
	})

	// Test: 在 IIFE 中传参并用解构后调用方法  
	t.Run("iife_destructure", func(t *testing.T) {
		log.Clear()
		code := `
			// 模拟 Vue bundler 的 IIFE 模式
			var result = (function() {
				// 使用严格模式类似的 IIFE
				var exports = {};
				exports.createApp = function(comp) {
					var app = {
						_container: null,
						_component: comp,
						mount: function(container) {
							this._container = container;
							return 'ok';
						}
					};
					return app;
				};
				
				// Vue 的 ml 模式
				var ml = (function() {
					return function() {
						var t = exports.createApp({});
						var s = t.mount;
						t.mount = function(n) {
							return s.call(t, n);
						};
						return t;
					};
				})();
				
				return {ml: ml};
			})();
			
			var je = result.ml();
			console.log('MTYP: '+(typeof je.mount));
			var Mi = document.createElement('div');
			
			try {
				je.mount(Mi);
				console.log('DIRECT_OK');
			} catch(e) {
				console.log('DIRECT_ERR: '+e);
			}
			console.log('CONT: '+je._container);
		`
		if _, err := vm.Run(code); err != nil {
			t.Fatalf("err: %v", err)
		}
		fmt.Printf("%s\n", log.String())
	})
}
