// jsread 在 wbui 管线里加载页面、执行一段 JS，并把**完整**结果原样落盘。
//
// 与 dev/probes/webshot 的 -js 的差别：webshot 经 summarizeJS 把结果截到 400
// 字符，读回长文本只能「分批」反复 go run（gprobe_cmp.sh 每批 3 行）。交叉扫描
// 表动辄 200+ 行，分批读回要 80+ 次 go run，且任一批被截断即静默丢尾部结果。
// 本工具一次读回、不截断，专供「长输出探针」（如 CJK 行盒度量扫描表）。
//
// 换行归一与 gprobe_cmp.sh 一致（CRLF/CR → LF、末尾恰一个 '\n'），保证两侧
// diff 只反映真实内容差异。
//
// 用法（仓库根，CGO 环境）：
//
//	go run ./dev/tools/jsread -html dev/fixtures/webshot/x.html \
//	    -js "document.getElementById('out').textContent" \
//	    -out dev/output/wbui-audit/x.wbui.txt
//
// 省略 -out 时打印到 stdout；结果为空的字符串一律拒绝落盘（防假 IDENTICAL）。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wb-ui/engine/js/jsc"
	"wb-ui/webkit"
)

func main() {
	htmlPath := flag.String("html", "", "HTML 文件路径（相对资源按文档目录解析）")
	w := flag.Int("w", 1280, "视口宽（须与 Edge 侧 gprobe_cmp.sh 的 1280x800 一致）")
	h := flag.Int("h", 800, "视口高")
	jsCode := flag.String("js", "", "加载完成后执行的 JS 表达式")
	out := flag.String("out", "", "输出文件（省略 = 打印到 stdout）")
	flag.Parse()

	if *htmlPath == "" || *jsCode == "" {
		fmt.Fprintln(os.Stderr, "用法: jsread -html <页面> -js <表达式> [-out <文件>]")
		os.Exit(2)
	}
	data, err := os.ReadFile(*htmlPath)
	if err != nil {
		fail("读取 HTML: %v", err)
	}
	abs, _ := filepath.Abs(*htmlPath)
	base := "file:///" + strings.ReplaceAll(filepath.ToSlash(filepath.Dir(abs)), " ", "%20") + "/"

	wv := webkit.NewWebView()
	defer wv.Destroy()
	wv.Resize(*w, *h)
	wv.SetConsoleLogger(&jsc.BufferLogger{})
	if err := wv.LoadHTMLWithBaseURL(string(data), base); err != nil {
		fail("LoadHTML: %v", err)
	}
	v, err := wv.EvalJS(*jsCode)
	if err != nil {
		fail("EvalJS: %v", err)
	}

	s := normalize(v.ToString())
	if s == "" {
		fail("JS 结果为空（拒绝产出空产物）")
	}
	if *out == "" {
		fmt.Print(s)
		return
	}
	if err := os.WriteFile(*out, []byte(s), 0o644); err != nil {
		fail("写文件: %v", err)
	}
	fmt.Printf("jsread %s（%d 行）\n", *out, strings.Count(s, "\n"))
}

// normalize 统一换行口径：CRLF/CR → LF，去掉末尾所有空行后恰补一个 '\n'。
func normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return ""
	}
	return s + "\n"
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", a...)
	os.Exit(1)
}
