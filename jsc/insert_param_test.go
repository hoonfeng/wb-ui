package jsc

import (
	"fmt"
	"testing"
)

// TestInsertParam 精确测试 Vue 3 bundle 中 insert 箭头函数的参数传递
// 问题: insert = (e,t,s) => { t.insertBefore(e, s||null) }
// 当在嵌套 IIFE + 箭头函数中调用 n(u.el, d, b) 时，t 参数可能被错误映射为 this
func TestInsertParam(t *testing.T) {
	vm := NewInterpreter()
	log := &BufferLogger{}
	vm.SetupGlobal(log)

	// 场景1: 基础箭头函数 insert 调用
	t.Run("basic_insert", func(t *testing.T) {
		log.Clear()
		code := `
			// 模拟 Vue 3 的 insert 箭头函数
			var insert = (e, t, s) => {
				console.log('I_E: '+(typeof e)+' val='+e);
				console.log('I_T: '+(typeof t)+' val='+t);
				console.log('I_S: '+(typeof s)+' val='+s);
				t.insertBefore(e, s || null);
				return 'ok';
			};
			
			// 模拟 DOM 元素
			var parent = {
				childNodes: [],
				insertBefore: function(c, r) { this.childNodes.push(c); console.log('IB: called'); }
			};
			var child = {tagName: 'DIV'};
			
			// 直接调用
			try {
				var r = insert(child, parent, null);
				console.log('DIRECT: '+r);
				console.log('CHILDREN: '+parent.childNodes.length);
			} catch(e) {
				console.log('DIRECT_ERR: '+e);
			}
		`
		if _, err := vm.Run(code); err != nil {
			t.Fatalf("run err: %v", err)
		}
		fmt.Printf("=== basic_insert ===\n%s\n", log.String())
	})

	// 场景2: 从另一个箭头函数中调用 insert（模拟 Vue 的 _o 内部 P 箭头函数）
	t.Run("arrow_call_insert", func(t *testing.T) {
		log.Clear()
		code := `
			// 模拟 Vue 的 _o 函数，内部有 insert 和 P 箭头函数
			function makeRenderer(options) {
				const {insert: n, ...rest} = options;
				// P 是箭头函数，类似 Vue 的 patch 函数
				const P = (c, u, d, b) => {
					console.log('P_ARGS: c='+(typeof c)+' u='+(typeof u)+' d='+(typeof d)+' b='+(typeof b));
					if(c != null) {
						// 这里模拟 insert 调用: n(u.el, d, b)
						n(u.el, d, b);
					}
				};
				return {patch: P};
			}
			
			// 创建 renderer options，包含 insert 箭头函数
			var options = {
				insert: (e, t, s) => {
					console.log('INSERT: e='+(typeof e)+' t='+(typeof t)+' s='+(typeof s));
					console.log('INSERT_THIS: '+(typeof this));
					t.insertBefore(e, s || null);
				},
				createElement: function(tag) { return {tagName: tag.toUpperCase(), childNodes: []}; }
			};
			
			// 创建 renderer
			var renderer = makeRenderer(options);
			
			// 准备参数
			var parentEl = {
				childNodes: [],
				insertBefore: function(c, r) { this.childNodes.push(c); console.log('IB_CALLED'); }
			};
			var vnode = {el: {tagName: 'SPAN'}};
			
			// 调用 P（模拟 Vue 的 patch 调用）
			try {
				renderer.patch(null, vnode, parentEl, null);
				console.log('PATCH_OK');
				console.log('PARENT_CHILDREN: '+parentEl.childNodes.length);
			} catch(e) {
				console.log('PATCH_ERR: '+e);
			}
		`
		if _, err := vm.Run(code); err != nil {
			t.Fatalf("run err: %v", err)
		}
		fmt.Printf("=== arrow_call_insert ===\n%s\n", log.String())
	})

	// 场景3: 完全模拟 Vue bundle 的 IIFE + 解构 + 箭头函数模式
	t.Run("iife_destruct_arrow", func(t *testing.T) {
		log.Clear()
		code := `
			// 完全模拟 Vue 3 bundle 模式
			var renderer = (function() {
				// 外层 options
				var options = {
					insert: (e, t, s) => {
						console.log('IIFE_INSERT: e='+(typeof e)+' t='+(typeof t)+' s='+s);
						if(t == null) {
							console.log('IIFE_INSERT_ERROR: t is null/undefined!');
							return;
						}
						t.insertBefore(e, s || null);
					},
					createElement: function(tag) {
						return {tagName: tag, childNodes: [], insertBefore: function(c,r){this.childNodes.push(c);console.log('IB_OK: '+c.tagName)}};
					}
				};

				// _o 函数 (createRenderer)
				function _o(e) {
					const {insert: n} = e;
					// P 是箭头函数 (patch)
					const P = (c, u, d, b) => {
						if(c != null) {
							// 这里调用 insert
							n(u.el, d, b);
						} else {
							// mount: createElement
							var el = e.createElement('div');
							u.el = el;
							n(el, d, b);
						}
					};
					return {patch: P};
				}

				// 合并 options 并调用 _o
				var merged = {};
				for(var k in options) merged[k] = options[k];
				return _o(merged);
			})();

			// 测试
			var parent = {
				childNodes: [],
				insertBefore: function(c,r) { this.childNodes.push(c); console.log('FINAL_IB: '+c.tagName); }
			};
			var vnode = {el: {tagName: 'SPAN'}};

			console.log('PATCH_EXISTS: '+(typeof renderer.patch));
			
			try {
				renderer.patch(vnode, vnode, parent, null);
				console.log('FINAL_OK');
			} catch(e) {
				console.log('FINAL_ERR: '+e);
			}
		`
		if _, err := vm.Run(code); err != nil {
			t.Fatalf("run err: %v", err)
		}
		fmt.Printf("=== iife_destruct_arrow ===\n%s\n", log.String())
	})

	// 场景4: 精确模拟 bundle 中的 _o 函数 + P 箭头函数调用 insert
	t.Run("exact_vue_pattern", func(t *testing.T) {
		log.Clear()
		code := `
			// 精确模拟 Vue bundle 的 _o + P 模式
			// Wo.insert = (e,t,s) => { t.insertBefore(e, s||null) }
			var Wo = {
				insert: (e, t, s) => {
					console.log('WO_INSERT: e='+(typeof e)+' t='+(typeof t)+' s='+(typeof s));
					if(t == null || t == undefined) {
						console.log('WO_INSERT_ERROR: t is null/undefined! THIS='+(typeof this));
						return;
					}
					t.insertBefore(e, s || null);
				}
			};

			// _o 函数
			function _o(e) {
				const {insert: n, remove: r} = e;

				// P 箭头函数 (patch 函数)
				const P = (c, u, d, b, m, g, x, y, v) => {
					// 这里应该准确传递参数
					// u = vnode, d = parent
					if(u && u.el) {
						console.log('P_CALL_INSERT: u.tag=' + (u.type || '?') + ' d='+(typeof d));
						n(u.el, d, b);
					}
				};

				// 返回包含 P 的对象，模拟 renderer
				return {p: P};
			}

			// 合并 Wo 到 options
			var options = {};
			for(var k in Wo) options[k] = Wo[k];

			// 创建 renderer
			var renderer = _o(options);

			// 模拟 DOM
			var parent = {
				childNodes: [],
				insertBefore: function(c, r) {
					this.childNodes.push(c);
					console.log('REAL_IB: '+c.tagName);
				}
			};

			// 创建 vnode 模拟
			var vnode = {
				el: {tagName: 'DIV'},
				type: 'div'
			};

			// 调用 P(nu... 像 Vue 那样传参
			console.log('CALL_P');
			try {
				renderer.p(null, vnode, parent, null);
				console.log('P_OK');
				console.log('CHILDREN: '+parent.childNodes.length);
			} catch(e) {
				console.log('P_ERR: '+e);
			}
		`
		if _, err := vm.Run(code); err != nil {
			t.Fatalf("run err: %v", err)
		}
		fmt.Printf("=== exact_vue_pattern ===\n%s\n", log.String())
	})
}
