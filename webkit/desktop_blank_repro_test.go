package webkit

// 复现「宿主前端加载后整页空白」：加载外部宿主构建出的同一份前端产物
// （目录由环境变量 WBUI_DESKTOP_DIST 指定），捕获 console/JS 错误并 dump DOM 状态。
// 跳过桥接——宿主侧桥接代码不在本仓库内，无法引用。
// 运行：WBUI_DESKTOP_DIST=<dist-dir> go test ./webkit/ -run TestDesktopDistLoads -v
import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wb-ui/engine/js/jsc"
)

// desktopDistDir 由环境变量指定；未设置则跳过本测试。
var desktopDistDir = os.Getenv("WBUI_DESKTOP_DIST")

func TestDesktopDistLoads(t *testing.T) {
	if desktopDistDir == "" {
		t.Skip("WBUI_DESKTOP_DIST not set (path to the host's built frontend dist)")
	}
	indexPath := filepath.Join(desktopDistDir, "index.html")
	htmlData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Skipf("desktop dist not available: %v", err)
	}
	wv := NewWebView()
	wv.Resize(1280, 800)
	_ = wv.JSInterpreter()
	absDist, _ := filepath.Abs(desktopDistDir)
	if mf := wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.ScriptLoader = func(src string) (string, error) {
				clean := strings.TrimPrefix(strings.TrimPrefix(src, "file://"), "./")
				data, err := os.ReadFile(filepath.Join(absDist, clean))
				return string(data), err
			}
			fr.StyleSheetLoader = func(href string) (string, error) {
				clean := strings.TrimPrefix(strings.TrimPrefix(href, "file://"), "./")
				data, err := os.ReadFile(filepath.Join(absDist, clean))
				return string(data), err
			}
		}
	}
	clog := &jsc.BufferLogger{}
	wv.SetConsoleLogger(clog)
	// ★ 在页面脚本执行前安装错误钩子（desktop 在 LoadHTML 之后才装，
	// 漏掉 mount 期间的错误——全空白时 errs 为空的原因）。
	BeforePageScripts = func(rt *jsc.Interpreter) {
		_, _ = rt.RunJS(`window.__errs = [];
window.addEventListener('error', function(e){ window.__errs.push('error: ' + (e && (e.message || e.type))); }, true);
window.addEventListener('unhandledrejection', function(e){ window.__errs.push('rejection: ' + ((e && e.reason && e.reason.message) || String(e))); }, true);
var _ce = console.error;
console.error = function(){
  window.__errs.push('console.error: ' + Array.prototype.slice.call(arguments).map(function(a){
    var m = typeof a === 'string' ? a : ((a && a.message) || String(a));
    if (a && a.stack) m += ' | STACK: ' + String(a.stack).split('\n').slice(0, 10).join(' <- ');
    return m;
  }).join(' | ').slice(0, 900));
  return _ce.apply(console, arguments);
};`)
	}
	defer func() { BeforePageScripts = nil }()

	start := time.Now()
	if err := wv.LoadHTML(string(htmlData)); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	t.Logf("LoadHTML took %v", time.Since(start))

	// 驱动事件循环（Vue mount + async 初始化）。
	for i := 0; i < 8; i++ {
		_, _ = wv.JSInterpreter().RunJS(`new Promise(function(res){ setTimeout(res, 300); })`)
		time.Sleep(350 * time.Millisecond)
		wv.JSInterpreter().RunJobs()
	}
	wv.RebuildRenderTree()
	wv.EnsureLayout()

	v, err := wv.EvalJS(`(function(){
		var out = {};
		out.bodyChildren = document.body ? document.body.children.length : -1;
		out.bodyHTML = document.body ? document.body.innerHTML.length : -1;
		var app = document.getElementById('app');
		out.appChildren = app ? app.children.length : -1;
		out.appHTML = app ? app.innerHTML.length : -1;
		out.errs = window.__errs || [];
		return JSON.stringify(out);
	})()`)
	if err != nil {
		t.Fatalf("dump: %v", err)
	}
	t.Logf("[DUMP] %s", v.ToString())
	// ★ 回归断言：Vue 必须完成挂载（此前惰性包装器缓存 accessor 活值 →
	// insertStaticContent 死循环 → 全空白）。
	if !strings.Contains(v.ToString(), `"appChildren":0`) && !strings.Contains(v.ToString(), `"appHTML":0`) {
		// ok
	} else {
		t.Fatalf("Vue app did not mount (blank screen): %s", v.ToString())
	}
	if strings.Contains(v.ToString(), `"errs":[`) && !strings.Contains(v.ToString(), `"errs":[]`) {
		t.Fatalf("JS errors during mount: %s", v.ToString())
	}

	if out := wv.ConsoleOutput(); out != "" {
		t.Logf("[CONSOLE]\n%s", out)
	}
}
