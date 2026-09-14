// Tests for the constraint validation IDL (HTML §4.10.21.3): the element-level
// validity / validationMessage / willValidate / checkValidity() /
// reportValidity() / setCustomValidity(), the form-level checkValidity() /
// reportValidity() / noValidate, and their link to the :valid / :invalid
// pseudo-classes (the CSS side is wired through html5's init-injected resolver).

package bindings

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/html5"
)

func TestConstraintValidationElementIDL(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)

	form := doc.CreateElement("form")
	form.SetId("f")
	body.AppendChild(form)
	in := doc.CreateElement("input")
	in.SetAttribute("type", "email")
	in.SetAttribute("required", "required")
	in.SetId("email")
	form.AppendChild(in)

	var invalidated int
	prev := OnClassChanged
	OnClassChanged = func(el *dom.Element) {
		if el == in {
			invalidated++
		}
	}
	defer func() { OnClassChanged = prev }()

	mustRun(t, rt, `
		{
			const el = document.getElementById('email');
			if (!('validity' in el)) throw new Error("'validity' in input 应为 true");
			if (!('willValidate' in el)) throw new Error("'willValidate' in input 应为 true");
			if (!('validationMessage' in el)) throw new Error("'validationMessage' in input 应为 true");
			if (typeof el.checkValidity !== 'function') throw new Error("checkValidity 不是方法");
			if (typeof el.reportValidity !== 'function') throw new Error("reportValidity 不是方法");
			if (typeof el.setCustomValidity !== 'function') throw new Error("setCustomValidity 不是方法");

			// required 且为空
			if (el.willValidate !== true) throw new Error("required input willValidate 应为 true");
			if (el.validity.valueMissing !== true) throw new Error("空 required 应 valueMissing");
			if (el.validity.valid !== false) throw new Error("空 required 应 valid=false");
			if (el.checkValidity() !== false) throw new Error("空 required checkValidity 应为 false");
			if (el.validationMessage === '') throw new Error("空 required 应有 validationMessage");
			if (!el.matches(':invalid')) throw new Error("空 required 应匹配 :invalid");
			if (el.matches(':valid')) throw new Error("空 required 不应匹配 :valid");

			// 填入合法值
			el.value = 'user@example.com';
			if (el.validity.valid !== true) throw new Error("合法值应 valid=true");
			if (el.checkValidity() !== true) throw new Error("合法值 checkValidity 应为 true");
			if (!el.matches(':valid')) throw new Error("合法值应匹配 :valid");

			// type=email 格式错误
			el.value = 'not-an-email';
			if (el.validity.typeMismatch !== true) throw new Error("非法 email 应 typeMismatch");
			if (el.checkValidity() !== false) throw new Error("非法 email checkValidity 应为 false");

			// 自定义错误消息
			el.value = 'user@example.com';
			el.setCustomValidity('这个名字已经被占用了');
			if (el.validity.customError !== true) throw new Error("setCustomValidity 后应 customError");
			if (el.validity.valid !== false) throw new Error("setCustomValidity 后应 valid=false");
			if (el.checkValidity() !== false) throw new Error("setCustomValidity 后 checkValidity 应为 false");
			if (el.validationMessage !== '这个名字已经被占用了') throw new Error("validationMessage 应返回自定义消息");
			if (!el.matches(':invalid')) throw new Error("自定义错误应匹配 :invalid");
			el.setCustomValidity('');
			if (el.validity.customError !== false) throw new Error("清空后 customError 应为 false");
			if (el.checkValidity() !== true) throw new Error("清空后 checkValidity 应为 true");

			// validity 对象形态
			el.setCustomValidity('x');
			const keys = ['valueMissing','typeMismatch','patternMismatch','tooLong','tooShort',
				'rangeUnderflow','rangeOverflow','stepMismatch','badInput','customError','valid'];
			for (const k of keys) {
				if (typeof el.validity[k] !== 'boolean') throw new Error('validity.' + k + ' 不是布尔值');
			}
			el.setCustomValidity('');

			// 非表单控件没有这些 API
			const div = document.createElement('div');
			if (div.checkValidity !== undefined) throw new Error("div.checkValidity 应为 undefined");
			if (div.validity !== undefined) throw new Error("div.validity 应为 undefined");
		}
	`)
	// setCustomValidity 改变 :valid/:invalid 的匹配结果 → 必须走样式失效链。
	if invalidated < 2 {
		t.Fatalf("setCustomValidity 触发的样式失效次数 = %d, want >= 2", invalidated)
	}
}

func TestConstraintValidationBarredElements(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)

	mk := func(id string, attrs map[string]string) *dom.Element {
		el := doc.CreateElement("input")
		el.SetId(id)
		for k, v := range attrs {
			el.SetAttribute(k, v)
		}
		body.AppendChild(el)
		return el
	}
	mk("ro", map[string]string{"type": "text", "required": "required", "readonly": "readonly"})
	mk("dis", map[string]string{"type": "text", "required": "required", "disabled": "disabled"})
	mk("hid", map[string]string{"type": "hidden", "required": "required"})
	mk("cb", map[string]string{"type": "checkbox", "readonly": "readonly", "required": "required"})

	mustRun(t, rt, `
		{
			// readonly / disabled / hidden 不参与约束校验。
			for (const id of ['ro', 'dis', 'hid']) {
				const el = document.getElementById(id);
				if (el.willValidate !== false) throw new Error(id + '.willValidate 应为 false');
				if (el.checkValidity() !== true) throw new Error(id + '.checkValidity 应为 true（不参与校验）');
				if (el.matches(':valid') || el.matches(':invalid')) {
					throw new Error(id + ' 不应匹配 :valid/:invalid（barred）');
				}
			}
			// readonly 对 checkbox 没有意义 → 仍参与校验。
			const cb = document.getElementById('cb');
			if (cb.willValidate !== true) throw new Error("checkbox 的 readonly 不应排除校验");
			if (cb.checkValidity() !== false) throw new Error("未勾选的 required checkbox 应为无效");
			if (!cb.matches(':invalid')) throw new Error("未勾选的 required checkbox 应匹配 :invalid");
		}
	`)
}

func TestConstraintValidationReportValidity(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)

	form := doc.CreateElement("form")
	form.SetId("f")
	body.AppendChild(form)
	for _, id := range []string{"a", "b"} {
		in := doc.CreateElement("input")
		in.SetAttribute("type", "text")
		in.SetAttribute("required", "required")
		in.SetId(id)
		form.AppendChild(in)
	}

	mustRun(t, rt, `
		{
			const a = document.getElementById('a');
			const b = document.getElementById('b');
			const form = document.getElementById('f');

			let aInvalid = 0, bInvalid = 0, submitCount = 0;
			a.addEventListener('invalid', () => { aInvalid++; });
			b.addEventListener('invalid', () => { bInvalid++; });
			form.addEventListener('submit', () => { submitCount++; });

			// checkValidity 不派发 invalid 事件
			if (a.checkValidity() !== false) throw new Error('a.checkValidity 应为 false');
			if (aInvalid !== 0) throw new Error('checkValidity 不应派发 invalid 事件');

			// 元素级 reportValidity：派发 invalid 事件并返回 false
			if (a.reportValidity() !== false) throw new Error('a.reportValidity 应为 false');
			if (aInvalid !== 1) throw new Error('reportValidity 应派发 1 次 invalid，实际 ' + aInvalid);

			// 表单级 reportValidity：两个无效控件各派发一次
			if (form.reportValidity() !== false) throw new Error('form.reportValidity 应为 false');
			if (aInvalid !== 2 || bInvalid !== 1) {
				throw new Error('表单级 reportValidity 的 invalid 次数 = ' + aInvalid + '/' + bInvalid);
			}
			if (form.checkValidity() !== false) throw new Error('form.checkValidity 应为 false');

			// 填好一个：剩下的无效
			a.value = 'ok';
			if (form.checkValidity() !== false) throw new Error('还有无效控件时 form.checkValidity 应为 false');
			if (a.reportValidity() !== true) throw new Error('有效元素 reportValidity 应为 true');
			if (aInvalid !== 2) throw new Error('有效元素不应再派发 invalid');

			// 全部有效
			b.value = 'ok';
			if (form.checkValidity() !== true) throw new Error('全部有效时 form.checkValidity 应为 true');
			if (form.reportValidity() !== true) throw new Error('全部有效时 form.reportValidity 应为 true');
			if (submitCount !== 0) throw new Error('校验期间不应派发 submit 事件');
		}
	`)
}

func TestFormNoValidateIDL(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)
	form := doc.CreateElement("form")
	form.SetId("f")
	body.AppendChild(form)
	in := doc.CreateElement("input")
	in.SetAttribute("type", "text")
	in.SetAttribute("required", "required")
	in.SetId("x")
	form.AppendChild(in)

	mustRun(t, rt, `
		{
			const form = document.getElementById('f');
			if (!('noValidate' in form)) throw new Error("'noValidate' in form 应为 true");
			if (form.noValidate !== false) throw new Error('默认 noValidate 应为 false');
			form.noValidate = true;
			if (form.noValidate !== true) throw new Error('设置后应 true');
			if (!form.hasAttribute('novalidate')) throw new Error('noValidate 应反映 novalidate 内容属性');
			form.noValidate = false;
			if (form.hasAttribute('novalidate')) throw new Error('清除后不应有 novalidate 属性');
			if (typeof form.checkValidity !== 'function') throw new Error('form.checkValidity 应是方法');
			if (typeof form.reportValidity !== 'function') throw new Error('form.reportValidity 应是方法');
		}
	`)
}

// TestMarkUserInteractedEnablesUserPseudoClasses 覆盖引擎的用户交互路径：
// MarkUserInteracted（引擎在真实 change 派发点调用：失焦提交、点击 checkbox/
// radio、选择 option、拖动 range）之后 :user-valid / :user-invalid 才开始
// 匹配，并触发样式失效；脚本派发的 change 不算用户交互（与浏览器一致）。
func TestMarkUserInteractedEnablesUserPseudoClasses(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)
	mk := func(id string) *dom.Element {
		el := doc.CreateElement("input")
		el.SetAttribute("type", "text")
		el.SetAttribute("required", "required")
		el.SetId(id)
		body.AppendChild(el)
		return el
	}
	in := mk("x")
	mk("y")

	var invalidated int
	prev := OnClassChanged
	OnClassChanged = func(el *dom.Element) {
		if el == in {
			invalidated++
		}
	}
	defer func() { OnClassChanged = prev }()

	mustRun(t, rt, `
		{
			const el = document.getElementById('x');
			if (el.matches(':user-valid') || el.matches(':user-invalid')) {
				throw new Error("未交互时 :user-valid/:user-invalid 都不应匹配");
			}
		}
	`)

	MarkUserInteracted(in)
	mustRun(t, rt, `
		{
			const el = document.getElementById('x');
			if (!el.matches(':user-invalid')) throw new Error("交互后无效元素应匹配 :user-invalid");
			if (el.matches(':user-valid')) throw new Error("无效元素不应匹配 :user-valid");
			el.value = 'ok';
			if (!el.matches(':user-valid')) throw new Error("填好后应匹配 :user-valid");
			if (el.matches(':user-invalid')) throw new Error("填好后不应匹配 :user-invalid");
		}
	`)
	if invalidated == 0 {
		t.Error("MarkUserInteracted 应触发样式失效")
	}

	// 幂等：重复调用不重复失效。
	before := invalidated
	MarkUserInteracted(in)
	if invalidated != before {
		t.Errorf("重复 MarkUserInteracted 不应重复失效（%d → %d）", before, invalidated)
	}

	// 脚本派发 change 不代表用户交互。
	mustRun(t, rt, `
		{
			const el = document.getElementById('y');
			el.dispatchEvent(new Event('change'));
			if (el.matches(':user-invalid') || el.matches(':user-valid')) {
				throw new Error("脚本派发的 change 不应置 user validity");
			}
		}
	`)
}

// TestFocusSessionFlipEnablesUserPseudoClasses 覆盖 MDN :user-valid 第 3 条的
// 端到端链路（dom 焦点钩子 → html5 判定 → bindings 失效 → CSS 匹配）：控件
// 获得焦点时无效，用户在焦点内改值使其有效 → :user-valid 立即匹配（不必等
// 失焦）；镜像方向（聚焦时有效 → 改成无效）让 :user-invalid 立即匹配。
// 聚焦本身不置位，失焦会结束焦点会话（此后写值不再算用户交互）。
func TestFocusSessionFlipEnablesUserPseudoClasses(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	body := newHTMLBodyFixture(doc)
	mk := func(id, value string) *dom.Element {
		el := doc.CreateElement("input")
		el.SetAttribute("type", "text")
		el.SetAttribute("required", "required")
		if value != "" {
			el.SetAttribute("value", value)
		}
		el.SetId(id)
		body.AppendChild(el)
		return el
	}
	empty := mk("empty", "")     // 聚焦时无效
	filled := mk("filled", "ok") // 聚焦时有效

	var invalidated int
	prev := OnClassChanged
	OnClassChanged = func(el *dom.Element) {
		if el == empty || el == filled {
			invalidated++
		}
	}
	defer func() { OnClassChanged = prev }()

	mustRun(t, rt, `
		{
			for (const id of ['empty', 'filled']) {
				const el = document.getElementById(id);
				if (el.matches(':user-valid') || el.matches(':user-invalid')) {
					throw new Error(id + '：未交互时两个 user 伪类都不应匹配');
				}
			}
		}
	`)

	// 聚焦本身不是交互：记忆写入但两个伪类仍不匹配。
	empty.SetFocused(true)
	mustRun(t, rt, `
		{
			if (!document.getElementById('empty').matches(':invalid')) {
				throw new Error('空 required 应匹配 :invalid');
			}
			if (document.getElementById('empty').matches(':user-invalid')) {
				throw new Error('仅聚焦不应置 user validity');
			}
		}
	`)

	// 方向一：焦点内把无效值改成有效 → :user-valid 立即匹配。
	empty.SetAttribute("value", "ok")
	html5.NoteUserInput(empty)
	mustRun(t, rt, `
		{
			const el = document.getElementById('empty');
			if (!el.matches(':user-valid')) throw new Error('焦点内改有效后应匹配 :user-valid');
			if (el.matches(':user-invalid')) throw new Error('有效时不应匹配 :user-invalid');
		}
	`)
	if invalidated == 0 {
		t.Error("user validity 翻转应触发样式失效")
	}

	// 方向二（镜像）：焦点内把有效值改成无效 → :user-invalid 立即匹配。
	filled.SetFocused(true)
	filled.RemoveAttribute("value")
	html5.NoteUserInput(filled)
	mustRun(t, rt, `
		{
			const el = document.getElementById('filled');
			if (!el.matches(':user-invalid')) throw new Error('焦点内清空必填项后应匹配 :user-invalid');
			if (el.matches(':user-valid')) throw new Error('无效时不应匹配 :user-valid');
		}
	`)

	// 失焦结束焦点会话：此后写值不再算用户交互（记忆被清除）。
	_, fresh, _ := newRuntimeWithDoc(t)
	body2 := newHTMLBodyFixture(fresh)
	el2 := fresh.CreateElement("input")
	el2.SetAttribute("type", "text")
	el2.SetAttribute("required", "required")
	el2.SetId("b")
	body2.AppendChild(el2)
	el2.SetFocused(true)
	el2.SetFocused(false)
	el2.SetAttribute("value", "ok")
	html5.NoteUserInput(el2)
	if el2.UserInteracted() {
		t.Error("失焦后（焦点会话结束）写值不应置 user validity")
	}
}
