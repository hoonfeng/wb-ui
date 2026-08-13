package webkit

// 打开文件场景性能探针：模拟 CM6 编辑器打开一个 500 行文件——JS 侧构建
// .cm-line × token span DOM（≈5000 节点），随后引擎 RebuildRenderTree +
// Layout。分解「DOM 构建」「重建渲染树」耗时，找出文件打开慢的热点。
//
// 运行：
//   go test ./webkit/ -run TestFileOpenScale -v
//   go test ./webkit/ -run TestFileOpenScale -bench . -cpuprofile cpu.out
import (
	"fmt"
	"testing"
	"time"
)

func TestFileOpenScale(t *testing.T) {
	wv := NewWebView()
	wv.Resize(1280, 800)
	html := `<!DOCTYPE html><html><head><style>
		.cm-content { font-family: Consolas, monospace; font-size: 13px; white-space: pre; }
		.cm-line { min-height: 15.2px; }
		.tok-kw { color: #c678dd; } .tok-var { color: #e5c07b; } .tok-op { color: #56b6c2; }
	</style></head><body><div id="c" class="cm-content" contenteditable="true"></div></body></html>`
	if err := wv.LoadHTML(html); err != nil {
		t.Fatal(err)
	}
	wv.RebuildRenderTree()

	for _, size := range []int{100, 500, 1000} {
		// 1) JS 侧构建 DOM（模拟 CM6 DOMWriter 的文件打开重绘）。
		t0 := time.Now()
		_, err := wv.EvalJS(fmt.Sprintf(`(function(){
			var c = document.getElementById('c');
			while (c.firstChild) c.removeChild(c.firstChild);
			for (var i = 0; i < %d; i++) {
				var line = document.createElement('div');
				line.className = 'cm-line';
				var kw = document.createElement('span'); kw.className='tok-kw'; kw.textContent='func';
				var sp1 = document.createTextNode(' ');
				var v = document.createElement('span'); v.className='tok-var'; v.textContent='main';
				var sp2 = document.createTextNode('() { return ');
				var num = document.createElement('span'); num.className='tok-op'; num.textContent='42';
				var sp3 = document.createTextNode('; }');
				line.appendChild(kw); line.appendChild(sp1); line.appendChild(v);
				line.appendChild(sp2); line.appendChild(num); line.appendChild(sp3);
				c.appendChild(line);
			}
			return 1;
		})()`, size))
		if err != nil {
			t.Fatal(err)
		}
		domBuild := time.Since(t0)

		// 2) 引擎重建渲染树 + 布局（文件打开后的首个绘制帧成本）。
		t1 := time.Now()
		wv.RebuildRenderTree()
		rebuild := time.Since(t1)

		fmt.Printf("fileopen scale: lines=%d nodes≈%d domBuild=%v rebuild=%v\n",
			size, size*8, domBuild, rebuild)
	}
}

// BenchmarkFileOpenReplaceChildren 模拟切换文件时 CM6 用 replaceChildren
// 重建整个内容区 DOM 的引擎成本。
func BenchmarkFileOpenReplaceChildren(b *testing.B) {
	wv := NewWebView()
	wv.Resize(1280, 800)
	html := `<!DOCTYPE html><html><head><style>
		.cm-content { font-family: Consolas, monospace; font-size: 13px; white-space: pre; }
		.tok-kw { color: #c678dd; }
	</style></head><body><div id="c" class="cm-content" contenteditable="true"></div></body></html>`
	if err := wv.LoadHTML(html); err != nil {
		b.Fatal(err)
	}
	js := `(function(){
		var c = document.getElementById('c');
		var frag = [];
		for (var i = 0; i < 500; i++) {
			var line = document.createElement('div');
			line.className = 'cm-line';
			var kw = document.createElement('span'); kw.className='tok-kw'; kw.textContent='func main() { return 42; } // '+i;
			line.appendChild(kw);
			frag.push(line);
		}
		for (var j = 0; j < frag.length; j++) c.appendChild(frag[j]);
		return 1;
	})()`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := wv.EvalJS(js); err != nil {
			b.Fatal(err)
		}
		wv.RebuildRenderTree()
	}
}
