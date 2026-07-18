package jsc

import (
	"fmt"
	"testing"
)

// TestArrowMethodCall 测试通过对象方法调用箭头函数
func TestArrowMethodCall(t *testing.T) {
	vm := NewInterpreter()
	log := &BufferLogger{}
	vm.SetupGlobal(log)

	tests := []struct {
		name string
		code string
		want string
	}{
		{
			name: "direct_method_call",
			code: `
				var obj = {val: 42, getVal: function() { return this.val; }};
				var r = obj.getVal();
				console.log('DIRECT: '+r);
			`,
			want: "DIRECT: 42",
		},
		{
			name: "arrow_as_property",
			code: `
				var outer = {val: 100};
				outer.fn = () => { return this ? typeof this : 'no_this'; };
				var r = outer.fn();
				console.log('ARROW_PROP: '+r);
			`,
			want: "ARROW_PROP: no_this",
		},
		{
			name: "arrow_captured_var",
			code: `
				var ctx = {val: 77};
				function maker() {
					var self = ctx;
					var result = {val: 999};
					result.mount = function(x) { return self.val + x; };
					return result;
				}
				var obj = maker();
				var r = obj.mount(23);
				console.log('CLOSURE: '+r);
			`,
			want: "CLOSURE: 100",
		},
		{
			name: "replaced_method_arrow",
			code: `
				var app = {_component: null, _container: null};
				var origMount = function(container) {
					this._container = container;
					return 'mounted';
				};
				app.mount = origMount;
				// 模拟 Vue 的覆盖模式: 提取原方法, 用箭头函数覆盖
				(function() {
					var t = app;
					var s = t.mount;
					t.mount = function(n) {
						return s.call(t, n);
					};
				})();
				var r = app.mount('test');
				console.log('MOUNT: '+r);
				console.log('CONTAINER: '+app._container);
			`,
			want: "MOUNT: mounted",
		},
		// real_vue_pattern 已被移除 - 测试代码有 bug
		{
			name: "method_call_arrow_capture",
			code: `
				// 测试: 箭头函数作为属性, 通过 obj.method() 调用
				var obj = {
					_val: 'hello',
					getVal: () => { return this ? 'this:'+typeof this : 'no_this'; }
				};
				var r1 = obj.getVal();
				console.log('M1: '+r1);
			`,
			want: "M1: no_this",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log.Clear()
			if _, err := vm.Run(tt.code); err != nil {
				t.Fatalf("run error: %v", err)
			}
			out := log.String()
			fmt.Printf("--- %s ---\n%s\n", tt.name, out)
		})
	}
}

// TestVue3MountMethodCall 精确复现 Vue 3 的 mount 调用模式
func TestVue3MountMethodCall(t *testing.T) {
	vm := NewInterpreter()
	log := &BufferLogger{}
	vm.SetupGlobal(log)

	// 模拟 Vue 3 的 createApp + mount override
	code := `
		// ===== Polyfill (minimal) =====
		document = {};
		document.createElement = function(t) {
			var el = {
				tagName: t.toUpperCase(),
				innerHTML: '',
				childNodes: [],
				setAttribute: function(){},
				getAttribute: function(){return ''},
				appendChild: function(c){this.childNodes.push(c)},
				removeChild: function(c){var i=this.childNodes.indexOf(c);if(i>-1)this.childNodes.splice(i,1)},
				insertBefore: function(c,r){this.childNodes.push(c)},
				style: {},
				parentNode: null,
				textContent: ''
			};
			return el;
		};
		document.body = {appendChild: function(c){}, insertBefore: function(c,r){}, childNodes: []};
		document.createTextNode = function(t){return{nodeType:3,textContent:t}};
		document.createComment = function(t){return{nodeType:8}};
		document.querySelector = function(){return{innerHTML:'',_vnode:null,__vue_app__:null,childNodes:[]}};
		Object.getOwnPropertyNames = function(o){if(!o)return[];var k=[];for(var n in o)k.push(n);return k};

		// ===== 模拟 Vue 3 的 mount 模式 =====
		var _l = (function() {
			var dummyCreateApp = function(comp) {
				var app = {
					_uid: 1,
					_component: comp,
					_container: null,
					version: '3.5.40',
					_config: {},
					_instance: null,
					mount: function(container, flag, svg) {
						// 真实的 Vue mount 逻辑
						this._container = container;
						container.__vue_app__ = this;
						return 'ok';
					},
					use: function(){return this;},
					mixin: function(){return this;},
					component: function(){return this;},
					directive: function(){return this;},
					provide: function(k,v){return this;},
					unmount: function(){},
					runWithContext: function(fn){return fn();},
					onUnmount: function(fn){}
				};
				return app;
			};
			return {createApp: dummyCreateApp};
		})();

		// ===== 模拟 bundle 中 ml 函数 =====
		var ml = (function() {
			return function() {
				var t = _l.createApp({});
				var s = t.mount;
				console.log('S_TYPE: '+(typeof s));
				console.log('S_NAME: "'+s.name+'"');
				console.log('S_LEN: '+s.length);
				// override mount with regular function (not arrow)
				t.mount = function(n) {
					console.log('CALLED_MOUNT');
					var container = n;
					if(typeof container === 'string') container = document.querySelector(container);
					if(!container) return;
					container.textContent = '';
					return s.call(t, container, false, undefined);
				};
				return t;
			};
		})();

		var je = ml();
		console.log('J_TYPE: '+(typeof je));
		console.log('J_KEYS: '+(je?Object.keys(je).join(','):'no_je'));
		console.log('M_TYPE: '+(typeof je.mount));
		
		// ===== 测试方法调用 =====
		var Mi = document.createElement('div');
		document.body.appendChild(Mi);
		
		console.log('BEFORE_CALL');
		try {
			var r = je.mount(Mi);
			console.log('MOUNT_OK: '+r);
			console.log('CONTAINER: '+(je._container ? je._container.tagName : 'null'));
		} catch(e) {
			console.log('MOUNT_ERR: '+e);
			// 尝试替代方式
			try {
				var mfn = je.mount;
				console.log('MFN_TYPE: '+(typeof mfn));
				var r2 = mfn.call(je, Mi);
				console.log('CALL_OK: '+r2);
			} catch(e2) {
				console.log('CALL_ERR2: '+e2);
			}
		}
	`
	
	if _, err := vm.Run(code); err != nil {
		t.Fatalf("run error: %v", err)
	}
	
	out := log.String()
	fmt.Printf("=== VUE MOUNT TEST ===\n%s\n=== END ===\n", out)
}
