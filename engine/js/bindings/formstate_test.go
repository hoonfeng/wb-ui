// Tests for the HTMLInputElement.indeterminate IDL property (HTML §4.16.3) and
// its link to the :indeterminate pseudo-class. It is an IDL state, not a content
// attribute, so it must not show up in the attribute list, and every real change
// must go through the style-invalidation chain (otherwise a stylesheet rule on
// :indeterminate would not repaint).

package bindings

import (
	"testing"

	"wb-ui/engine/dom"
)

func TestInputIndeterminateProperty(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)
	cb := doc.CreateElement("input")
	cb.SetAttribute("type", "checkbox")
	cb.SetId("cb")
	body.AppendChild(cb)

	var invalidated int
	prev := OnClassChanged
	OnClassChanged = func(el *dom.Element) {
		if el == cb {
			invalidated++
		}
	}
	defer func() { OnClassChanged = prev }()

	mustRun(t, rt, `
		{
			const cb = document.getElementById("cb");
			if (cb.indeterminate !== false) throw new Error("初始 indeterminate 应为 false");
			if (cb.matches(":indeterminate")) throw new Error("初始不应匹配 :indeterminate");
			if (!("indeterminate" in cb)) throw new Error("'indeterminate' in input 应为 true");
			if (cb.hasAttribute("indeterminate")) throw new Error("初始不应有 indeterminate 属性");

			cb.indeterminate = true;
			if (cb.indeterminate !== true) throw new Error("设置后 false");
			if (!cb.matches(":indeterminate")) throw new Error("设置后应匹配 :indeterminate");
			if (document.querySelectorAll(":indeterminate").length !== 1) {
				throw new Error(":indeterminate 应只匹配 1 个元素");
			}
			// IDL 状态：不得写回内容属性（这不是 checked 那种反射属性）。
			if (cb.hasAttribute("indeterminate")) {
				throw new Error("indeterminate 是 IDL 状态，不应写内容属性");
			}
			cb.indeterminate = true; // 重复赋值不触发状态变化
		}
	`)
	if invalidated != 1 {
		t.Fatalf("indeterminate=true 后样式失效次数 = %d，want 1", invalidated)
	}

	mustRun(t, rt, `
		{
			const cb = document.getElementById("cb");
			cb.indeterminate = false;
			if (cb.indeterminate !== false) throw new Error("清零后应为 false");
			if (cb.matches(":indeterminate")) throw new Error("清零后不应匹配 :indeterminate");
			cb.indeterminate = false; // 同上：无变化
		}
	`)
	if invalidated != 2 {
		t.Fatalf("清零后样式失效次数 = %d，want 2（重复赋值不应计数）", invalidated)
	}

	// checked 与 indeterminate 是两个独立状态（取消勾选不清 indeterminate，
	// 浏览器也是如此）。
	mustRun(t, rt, `
		{
			const cb = document.getElementById("cb");
			cb.checked = true;
			cb.indeterminate = true;
			cb.checked = false;
			if (!cb.indeterminate) throw new Error("checked 变化不应清 indeterminate");
			if (!cb.matches(":indeterminate")) throw new Error("应仍匹配 :indeterminate");
			cb.indeterminate = false;
		}
	`)
}

// TestRadioIndeterminateFromScript <input type="radio"> 的 :indeterminate 由
// 「同组是否已有勾选」推导（没有 IDL 状态位），勾选别的 radio 后原 radio 的
// 匹配结果必须跟着变。
func TestRadioIndeterminateFromScript(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)
	a := doc.CreateElement("input")
	a.SetAttribute("type", "radio")
	a.SetAttribute("name", "g")
	a.SetId("a")
	body.AppendChild(a)
	b := doc.CreateElement("input")
	b.SetAttribute("type", "radio")
	b.SetAttribute("name", "g")
	b.SetId("b")
	body.AppendChild(b)

	mustRun(t, rt, `
		{
			const a = document.getElementById("a");
			const b = document.getElementById("b");
			if (!a.matches(":indeterminate") || !b.matches(":indeterminate")) {
				throw new Error("组内无勾选时两个 radio 都应匹配 :indeterminate");
			}
			b.checked = true;
			if (a.matches(":indeterminate") || b.matches(":indeterminate")) {
				throw new Error("组内已有勾选时都不应匹配 :indeterminate");
			}
			b.checked = false;
			if (!a.matches(":indeterminate")) {
				throw new Error("取消勾选后应恢复匹配 :indeterminate");
			}
		}
	`)
}
