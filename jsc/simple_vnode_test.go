package jsc

import (
	"testing"
	"fmt"
)

// TestSimpleVNode 测试最简单的 Vue3 vnode 创建
func TestSimpleVNode(t *testing.T) {
	vm := NewInterpreter()
	log := &BufferLogger{}
	vm.SetupGlobal(log)

	code := `
		// 直接测试 VNode 创建
		var xo = function(e) {
			var We = [];
			if(e === undefined) e = false;
			We.push(e ? null : []);
		};
		var fr = function(e, t, s, n, r, i, o) {
			var f = {__v_isVNode: true, type: e, props: t, children: s};
			return f;
		};
		var So = function(e) { return e; };
		var wo = function(e, t, s) { return So(fr(e, t, s)); };
		
		// 测试1: 基本 vnode
		try {
			var v1 = wo("div", null, "hello");
			console.log('VNODE_TYPE: '+(typeof v1));
			console.log('VNODE_ISTYPE: '+(v1.__v_isVNode));
			console.log('VNODE_TAG: '+v1.type);
		} catch(e) {
			console.log('VNODE_ERR: '+e);
		}
		
		// 测试2: 简单 render
		try {
			var renderResult = wo("div", null, "hello");
			console.log('RENDER_OK: type='+(typeof renderResult));
		} catch(e) {
			console.log('RENDER_ERR: '+e);
		}
		
		console.log('ALL_DONE');
	`
	
	if _, err := vm.Run(code); err != nil {
		t.Fatalf("run err: %v", err)
	}
	
	out := log.String()
	fmt.Printf("=== OUTPUT ===\n%s\n=== END ===\n", out)
}
