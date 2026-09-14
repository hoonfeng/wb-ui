//go:build ignore

package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"wb-ui/engine/js/goja"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: jssyntax <page.html>   # 检查页面内联 <script> 的 goja 语法")
		os.Exit(2)
	}
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "read error:", err)
		os.Exit(1)
	}
	html := string(b)
	re := regexp.MustCompile(`<script>([\s\S]*?)</script>`)
	matches := re.FindAllStringSubmatch(html, -1)
	fmt.Println("script blocks:", len(matches))
	for i, m := range matches {
		code := m[1]
		if strings.TrimSpace(code) == "" {
			continue
		}
		// 只编译不执行：页面脚本顶层就调 document/window，goja 无 DOM，
		// RunString 会误报 ReferenceError——语法检查只需 Compile。
		_, err := goja.Compile("script", code, true)
		if err != nil {
			fmt.Printf("--- script#%d ERROR: %v\n", i, err)
			lines := strings.Split(code, "\n")
			for li, ln := range lines {
				if strings.Contains(ln, "plSrc") || strings.Contains(ln, "列表") || strings.Contains(ln, "fromCharCode") {
					fmt.Printf("  [%d] %s\n", li+1, strings.TrimSpace(ln)[:min(120, len(strings.TrimSpace(ln)))])
				}
			}
		} else {
			fmt.Printf("script#%d OK (%d chars)\n", i, len(code))
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
