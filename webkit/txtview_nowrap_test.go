package webkit

import (
	"testing"
)

// renderWelcomeHTML 渲染完整 TextHTML 挂件结构（含 span.inner 包装），
// 返回 .txt 的 getBoundingClientRect。
func renderWelcomeHTML(t *testing.T, content string) string {
	wv := NewWebView()
	wv.Resize(600, 200)
	wv.LoadHTML(`<html><head><meta charset="utf-8"><style>
body{margin:0;background:transparent;overflow:hidden;font-family:'Segoe UI','Microsoft YaHei',sans-serif;user-select:none}
.txt{color:#fff;font-size:28px;font-weight:600;text-shadow:0 2px 8px rgba(0,0,0,0.7);line-height:1.3}
</style></head><body>
<div class="txt"><span class="inner">` + content + `</span></div>
</body></html>`)
	if mf := wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.RebuildRenderTree()
			fr.SetNeedsLayout(true)
		}
	}
	wv.EnsureLayout()
	v, err := wv.EvalJS(`JSON.stringify((function(){
		var e = document.querySelector('.txt');
		var r = e.getBoundingClientRect();
		return {w: Math.round(r.width), h: Math.round(r.height)};
	})())`)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	return v.ToString()
}

// TestTxtViewShortContent 短内容（如"新文字"2-4 字）时 txt-view 宽度应
// 收缩为内容宽（浏览器 shrink-to-fit），而非撑满 max-width:110——否则
// txt-view(110) + 编辑按钮(38) > pv 宽(128) → 属性面板内容行换行
// （用户报告"欢迎语属性内容换行"的候选根因）。
func TestTxtViewShortContent(t *testing.T) {
	wv := NewWebView()
	wv.Resize(900, 640)
	wv.LoadHTML(`<html><head><style>
*{box-sizing:border-box}
body{margin:0;background:#131722}
.prop-panel{width:220px;padding:8px 10px}
.prop-row{display:flex;align-items:center;gap:8px;margin-bottom:6px}
.prop-row label{width:64px;font-size:11px;color:#8fa0c0;flex-shrink:0}
.prop-row .pv{flex:1;min-width:0}
.txt-view{display:inline-block;max-width:110px;overflow:hidden;white-space:nowrap;text-overflow:ellipsis;vertical-align:middle;line-height:20px;font-size:11px;color:#aab8d4}
.btn{display:inline-block;padding:2px 8px;font-size:11px;background:#263352;color:#aab8d4;border-radius:4px}
</style></head><body>
<div class="prop-panel">
  <div class="prop-row"><label>内容</label><div class="pv"><div class="txt-view">欢迎来到直播间</div><div class="btn">编辑</div></div></div>
</div>
</body></html>`)
	if mf := wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.RebuildRenderTree()
			fr.SetNeedsLayout(true)
		}
	}
	wv.EnsureLayout()
	v, err := wv.EvalJS(`JSON.stringify((function(){
		var e = document.querySelector('.txt-view');
		var r = e.getBoundingClientRect();
		var b = document.querySelector('.btn');
		var br = b.getBoundingClientRect();
		return {txt:{w:Math.round(r.width),h:Math.round(r.height),y:Math.round(r.y)},
		        btn:{w:Math.round(br.width),h:Math.round(br.height),y:Math.round(br.y)}};
	})())`)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	t.Logf("short-content geom = %s", v.ToString())
	// txt-view 宽度应 < 110（内容 3 字 ≈ 33px），且与编辑按钮同行
}

// TestWelcomeTextLineBreak 复现欢迎语挂件（TextHTML）内容换行问题：
// 用户在 textarea 输入多行文本（含 \n / \r\n），浏览器 white-space:normal
// 折叠为空格→单行；wb-ui 若把 \n 当硬换行→多行（高度 > 36px 单行行高）。
func TestWelcomeTextLineBreak(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"default", "欢迎来到直播间！"},
		{"lf", "欢迎来到直播间\n感谢大家的关注"},
		{"crlf", "欢迎来到直播间\r\n感谢大家的关注"},
		{"multi_space", "欢迎来到  直播间 感谢关注"},
		{"c20", "欢迎来到直播间感谢大家的关注点赞转发"},
		{"c21", "欢迎来到直播间感谢大家的关注订阅点赞转发"},
		{"c22", "欢迎来到直播间感谢大家的关注订阅点赞转发评论"},
		{"long", "欢迎来到直播间感谢大家的关注订阅点赞转发评论转发关注点赞"},
	}
	for _, c := range cases {
		rect := renderWelcomeHTML(t, c.content)
		t.Logf("case=%s rect=%s", c.name, rect)
	}
}
