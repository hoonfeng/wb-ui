package bindings

// IME 组合输入 DOM 原语回归测试（contenteditable / CodeMirror 6）：
//   - TextOffsetOfSelection：DOM Selection 锚点 → root 文本内容 rune 偏移
//   - ReplaceTextRange：替换组合范围（单节点内、跨节点、纯插入、空行 <br>）
//   - 替换后选择同步到插入文本之后
import (
	"testing"

	"wb-ui/engine/dom"
)

// textOf 返回 root 的 TextContent（组合测试断言用）。
func textOf(root dom.Node) string {
	if el, ok := root.(*dom.Element); ok {
		return el.TextContent()
	}
	return ""
}

// TestIMETextOffsetOfSelection 验证文本节点内/元素锚点的 rune 偏移计算。
func TestIMETextOffsetOfSelection(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	root := doc.CreateElement("div")
	root.SetAttribute("contenteditable", "true")
	doc.AppendChild(root)
	l1 := doc.CreateElement("div")
	l1.AppendChild(doc.CreateTextNode("hello"))
	root.AppendChild(l1)
	l2 := doc.CreateElement("div")
	l2.AppendChild(doc.CreateTextNode("世界"))
	root.AppendChild(l2)

	// 光标在 "hello" 的 offset=2（l 和 l 之间）→ root 文本偏移 2
	mustRun(t, rt, `
		var lines = document.querySelectorAll('div[contenteditable] > div');
		window.getSelection().collapse(lines[0].firstChild, 2);
	`)
	off, ok := TextOffsetOfSelection(root)
	if !ok || off != 2 {
		t.Fatalf("TextOffsetOfSelection = (%d,%v), want (2,true)", off, ok)
	}

	// 光标在第二行 "世界" 的 offset=1 → root 文本偏移 5+1=6
	mustRun(t, rt, `
		var lines = document.querySelectorAll('div[contenteditable] > div');
		window.getSelection().collapse(lines[1].firstChild, 1);
	`)
	off, ok = TextOffsetOfSelection(root)
	if !ok || off != 6 {
		t.Fatalf("TextOffsetOfSelection = (%d,%v), want (6,true)", off, ok)
	}

	// 元素锚点（.cm-line 空行 <br> 光标，offset=0）→ 该行前文本总长
	l3 := doc.CreateElement("div")
	l3.AppendChild(doc.CreateElement("br"))
	root.AppendChild(l3)
	mustRun(t, rt, `
		var lines = document.querySelectorAll('div[contenteditable] > div');
		window.getSelection().collapse(lines[2], 0);
	`)
	off, ok = TextOffsetOfSelection(root)
	if !ok || off != 7 {
		t.Fatalf("TextOffsetOfSelection(element anchor) = (%d,%v), want (7,true)", off, ok)
	}
}

// TestIMEReplaceTextRange_CompositionFlow 模拟完整组合流程：
// 初始 "func main" → 更新 "ni" → 更新 "你" → 提交 "你" → 提交后继续输入 "好"。
func TestIMEReplaceTextRange_CompositionFlow(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	root := doc.CreateElement("div")
	root.SetAttribute("contenteditable", "true")
	doc.AppendChild(root)
	root.AppendChild(doc.CreateTextNode("func main"))

	// 光标定位到 "func " 之后（offset=5，main 之前）
	mustRun(t, rt, `
		var el = document.querySelector('div[contenteditable]');
		window.getSelection().collapse(el.firstChild, 5);
	`)
	from, ok := TextOffsetOfSelection(root)
	if !ok || from != 5 {
		t.Fatalf("initial offset = %d,%v want 5", from, ok)
	}

	// 组合开始：插入预览 "ni"
	end, ok := ReplaceTextRange(root, from, 0, "ni")
	if !ok {
		t.Fatal("ReplaceTextRange(insert) failed")
	}
	if got := textOf(root); got != "func ni"+"main" {
		t.Fatalf("after insert = %q, want %q", got, "func nimain")
	}
	if end != 7 {
		t.Fatalf("end after insert = %d, want 7", end)
	}

	// 组合更新：ni → 你（替换长度 2）
	end, ok = ReplaceTextRange(root, from, 2, "你")
	if !ok {
		t.Fatal("ReplaceTextRange(update) failed")
	}
	if got := textOf(root); got != "func 你main" {
		t.Fatalf("after update = %q, want %q", got, "func 你main")
	}
	if end != 6 {
		t.Fatalf("end after update = %d, want 6", end)
	}

	// 提交：你（替换长度 1 → 最终文本，实际内容不变，范围收窄）
	end, ok = ReplaceTextRange(root, from, 1, "你")
	if !ok {
		t.Fatal("ReplaceTextRange(commit) failed")
	}
	if got := textOf(root); got != "func 你main" {
		t.Fatalf("after commit = %q, want %q", got, "func 你main")
	}
	if end != 6 {
		t.Fatalf("end after commit = %d, want 6", end)
	}

	// 提交后普通字符：光标应在插入文本之后——用 selObj 定位继续输入 "好"
	if sstate.selObj == nil {
		t.Fatal("sstate.selObj nil after ReplaceTextRange")
	}
	an := sstate.selObj.GetStr("anchorNode")
	if an.IsNull() || an.IsUndefined() {
		t.Fatal("anchorNode null after ReplaceTextRange")
	}
	anchorEl := unwrapNode(an)
	if _, ok := anchorEl.(*dom.Element); !ok {
		t.Fatalf("anchorNode should be element after ReplaceTextRange, got %T", anchorEl)
	}
	if !InsertTextAtSelection("好") {
		t.Fatal("InsertTextAtSelection after commit failed")
	}
	if got := textOf(root); got != "func 你好main" {
		t.Fatalf("after follow-up char = %q, want %q", got, "func 你好main")
	}
}

// TestIMEReplaceTextRange_EmptyLine 空编辑器（.cm-line 只有 <br>）的组合插入。
func TestIMEReplaceTextRange_EmptyLine(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	root := doc.CreateElement("div")
	root.SetAttribute("contenteditable", "true")
	doc.AppendChild(root)
	line := doc.CreateElement("div")
	line.SetClassName("cm-line")
	line.AppendChild(doc.CreateElement("br"))
	root.AppendChild(line)

	mustRun(t, rt, `
		var el = document.querySelector('div[contenteditable]');
		window.getSelection().collapse(el.firstChild, 0);
	`)
	from, ok := TextOffsetOfSelection(root)
	if !ok || from != 0 {
		t.Fatalf("offset = %d,%v want 0", from, ok)
	}
	end, ok := ReplaceTextRange(root, from, 0, "你")
	if !ok {
		t.Fatal("ReplaceTextRange on empty line failed")
	}
	if end != 1 {
		t.Fatalf("end = %d, want 1", end)
	}
	// 文本必须插入 .cm-line 内（<br> 之前），而不是 contenteditable 根下。
	if got := textOf(root); got != "你" {
		t.Fatalf("textContent = %q, want %q", got, "你")
	}
	if line.FirstChild() == nil {
		t.Fatal(".cm-line has no children after insert")
	}
	if tn, ok := line.FirstChild().(*dom.Text); !ok || tn.Data() != "你" {
		t.Fatalf("first child of .cm-line = %#v, want Text(\"你\")", line.FirstChild())
	}
}

// TestIMEReplaceTextRange_AcrossNodes 组合范围跨多个文本节点（CM6 合并相邻
// 同样式文本后组合文本嵌入中间）的替换。
func TestIMEReplaceTextRange_AcrossNodes(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	root := doc.CreateElement("div")
	root.SetAttribute("contenteditable", "true")
	doc.AppendChild(root)
	root.AppendChild(doc.CreateTextNode("ab"))
	root.AppendChild(doc.CreateTextNode("cd"))
	root.AppendChild(doc.CreateTextNode("ef"))

	// 替换 "bcde"（跨 3 个节点）为 "X"
	mustRun(t, rt, `
		var el = document.querySelector('div[contenteditable]');
		window.getSelection().collapse(el.firstChild, 1);
	`)
	from, ok := TextOffsetOfSelection(root)
	if !ok || from != 1 {
		t.Fatalf("offset = %d,%v want 1", from, ok)
	}
	end, ok := ReplaceTextRange(root, from, 4, "X")
	if !ok {
		t.Fatal("ReplaceTextRange across nodes failed")
	}
	if got := textOf(root); got != "aXf" {
		t.Fatalf("textContent = %q, want %q", got, "aXf")
	}
	if end != 2 {
		t.Fatalf("end = %d, want 2", end)
	}
}
