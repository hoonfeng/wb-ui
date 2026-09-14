// page/frame_script_test.go — <script> 执行路径的 currentScript 契约。
package page

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"wb-ui/bindings"
)

// TestAsyncScriptCurrentScript 覆盖异步（外部）脚本执行期间的
// document.currentScript：外部脚本经 ResourceLoader 异步加载，执行时必须与
// 同步路径一样把 currentScript 指向正在执行的 <script> 元素（React 19 的水合
// 流程据此定位宿主脚本；读到 undefined 会让整段脚本中断）。
func TestAsyncScriptCurrentScript(t *testing.T) {
	dir := t.TempDir()
	jsPath := filepath.Join(dir, "async.js")
	if err := os.WriteFile(jsPath, []byte("// async script\n"), 0o644); err != nil {
		t.Fatalf("write temp js: %v", err)
	}
	jsURL := "file://" + filepath.ToSlash(jsPath)

	type seen struct {
		src  string
		attr string
	}
	executed := make(chan seen, 1)
	f := &Frame{
		ScriptEngine: func(code string) error {
			s := seen{}
			if cur := bindings.CurrentScriptElement; cur != nil {
				s.src = cur.GetAttribute("src")
				s.attr = cur.LocalName()
			}
			executed <- s
			return nil
		},
		ResourceLoader: NewCachedResourceLoader(jsURL),
	}
	html := `<html><body><script id="boot" src="` + jsURL + `"></script></body></html>`
	if err := f.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	f.ExecuteScripts()

	select {
	case got := <-executed:
		if got.src != jsURL {
			t.Fatalf("异步脚本执行期间 currentScript.src = %q，want %q", got.src, jsURL)
		}
		if got.attr != "script" {
			t.Fatalf("currentScript 的标签应为 script，实际 %q", got.attr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("异步脚本没有被执行（超时）")
	}
}

// TestSyncScriptCurrentScript 覆盖同步（内联）脚本的同一契约，作为对照：
// 异步路径的修复不能改变同步路径的行为。
func TestSyncScriptCurrentScript(t *testing.T) {
	executed := make(chan string, 1)
	f := &Frame{
		ScriptEngine: func(code string) error {
			src := "<nil>"
			if cur := bindings.CurrentScriptElement; cur != nil {
				src = cur.GetAttribute("id")
			}
			executed <- src
			return nil
		},
	}
	if err := f.LoadHTML(`<html><body><script id="inline">var x = 1;</script></body></html>`); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	f.ExecuteScripts()
	select {
	case got := <-executed:
		if got != "inline" {
			t.Fatalf("内联脚本执行期间 currentScript.id = %q，want inline", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("内联脚本没有被执行")
	}
}
