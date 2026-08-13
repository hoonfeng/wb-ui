package app

// Host 层 IME 组合输入集成测试（contenteditable / CodeMirror 6 场景）：
// 组合更新必须把组合文本写入 DOM 并派发 compositionstart/compositionupdate/
// input(insertCompositionText)；提交必须替换组合范围并派发 compositionend +
// input。历史根因：contenteditable 组合完全不写 DOM（预览不显示、提交文本
// 插到预览之后——「输入法输入时文字插入异常」）。
import (
	"encoding/json"
	"strings"
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/ime"
	"wb-ui/webkit"
)

func TestIMECompositionContentEditable(t *testing.T) {
	wv := webkit.NewWebView()
	wv.Resize(800, 600)
	if err := wv.LoadHTML(`<html><body style="font-family:Consolas,monospace;font-size:13px">
		<div id="ce" contenteditable="true" style="white-space:pre">func main</div>
	</body></html>`); err != nil {
		t.Fatal(err)
	}
	h := NewHostForTest(wv, 800, 600)
	el := wv.MainFrame().Document().GetElementById("ce")
	if el == nil {
		t.Fatal("element #ce not found")
	}
	h.MockFocus(el)

	// JS 记录组合事件序列。
	if _, err := wv.JSInterpreter().RunJS(`
		window.__comp = [];
		window.__inputs = [];
		var ce = document.getElementById('ce');
		ce.addEventListener('compositionstart', function(e){ window.__comp.push('start:'+e.data); });
		ce.addEventListener('compositionupdate', function(e){ window.__comp.push('update:'+e.data); });
		ce.addEventListener('compositionend', function(e){ window.__comp.push('end:'+e.data); });
		ce.addEventListener('input', function(e){ window.__inputs.push(e.inputType+':'+e.data); });
		window.getSelection().collapse(ce.firstChild, 5);
	`); err != nil {
		t.Fatal(err)
	}

	// 组合开始 + 更新：拼音预览 "ni"
	h.ApplyIMEEventsForTest([]ime.Event{{Kind: ime.EventCompositionUpdate, Composition: "ni"}})
	if got := el.TextContent(); got != "func nimain" {
		t.Fatalf("after composition update, text = %q, want %q", got, "func nimain")
	}
	// 组合更新：ni → 你（替换，不是追加）
	h.ApplyIMEEventsForTest([]ime.Event{{Kind: ime.EventCompositionUpdate, Composition: "你"}})
	if got := el.TextContent(); got != "func 你main" {
		t.Fatalf("after second update, text = %q, want %q", got, "func 你main")
	}
	// 提交：替换组合范围为最终字符
	h.ApplyIMEEventsForTest([]ime.Event{{Kind: ime.EventCharInput, Char: '你'}})
	if got := el.TextContent(); got != "func 你main" {
		t.Fatalf("after commit, text = %q, want %q", got, "func 你main")
	}
	// 组合结束（Windows 在提交字符之后发 ENDCOMPOSITION）：幂等，不再插文本
	h.ApplyIMEEventsForTest([]ime.Event{{Kind: ime.EventCompositionEnd}})
	if got := el.TextContent(); got != "func 你main" {
		t.Fatalf("after compositionend, text = %q, want %q", got, "func 你main")
	}
	// 组合提交后的普通字符：光标已在「你」之后，继续输入插到后面
	h.MockKeyChar('好')
	if got := el.TextContent(); got != "func 你好main" {
		t.Fatalf("after follow-up char, text = %q, want %q", got, "func 你好main")
	}

	// 事件序列验证
	v, err := wv.JSInterpreter().RunJS(`JSON.stringify({comp: window.__comp, inputs: window.__inputs})`)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Comp   []string `json:"comp"`
		Inputs []string `json:"inputs"`
	}
	if err := json.Unmarshal([]byte(v.ToString()), &state); err != nil {
		t.Fatalf("parse %q: %v", v.ToString(), err)
	}
	join := func(s []string) string { return strings.Join(s, "|") }
	gotComp := join(state.Comp)
	if !strings.Contains(gotComp, "start:ni") {
		t.Errorf("missing compositionstart, got %q", gotComp)
	}
	if !strings.Contains(gotComp, "update:ni") || !strings.Contains(gotComp, "update:你") {
		t.Errorf("missing compositionupdate, got %q", gotComp)
	}
	if !strings.Contains(gotComp, "end:你") {
		t.Errorf("missing compositionend with committed char, got %q", gotComp)
	}
	if c := strings.Count(gotComp, "end:"); c != 1 {
		t.Errorf("compositionend fired %d times, want exactly 1 (%q)", c, gotComp)
	}
	gotIn := join(state.Inputs)
	if !strings.Contains(gotIn, "insertCompositionText:ni") ||
		!strings.Contains(gotIn, "insertCompositionText:你") {
		t.Errorf("missing insertCompositionText input events, got %q", gotIn)
	}
}

// TestIMECompositionContentEditable_EmptyEditor 空编辑器（.cm-line <br>）上
// 组合输入：文本必须插入行内而非 contenteditable 根下。
func TestIMECompositionContentEditable_EmptyEditor(t *testing.T) {
	wv := webkit.NewWebView()
	wv.Resize(800, 600)
	if err := wv.LoadHTML(`<html><body style="font-family:Consolas,monospace;font-size:13px">
		<div id="ce" contenteditable="true"><div class="cm-line"><br></div></div>
	</body></html>`); err != nil {
		t.Fatal(err)
	}
	h := NewHostForTest(wv, 800, 600)
	el := wv.MainFrame().Document().GetElementById("ce")
	if el == nil {
		t.Fatal("element #ce not found")
	}
	h.MockFocus(el)
	if _, err := wv.JSInterpreter().RunJS(`
		var ce = document.getElementById('ce');
		window.getSelection().collapse(ce.firstChild, 0);
	`); err != nil {
		t.Fatal(err)
	}
	h.ApplyIMEEventsForTest([]ime.Event{{Kind: ime.EventCompositionUpdate, Composition: "ni"}})
	h.ApplyIMEEventsForTest([]ime.Event{{Kind: ime.EventCharInput, Char: '你'}})
	h.ApplyIMEEventsForTest([]ime.Event{{Kind: ime.EventCompositionEnd}})
	if got := el.TextContent(); got != "你" {
		t.Fatalf("text = %q, want %q", got, "你")
	}
	line := el.FirstChild()
	if line == nil || line.NodeType() != 1 {
		t.Fatalf(".cm-line missing, firstChild=%v", line)
	}
	if tn, ok := line.FirstChild().(*dom.Text); !ok || tn.Data() != "你" {
		t.Fatalf("first child of .cm-line = %#v, want Text(\"你\")", line.FirstChild())
	}
}
