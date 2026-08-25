//go:build ignore

// Command jscheck_ui 提取主项目 internal/configwin/ui.go 的 <script> 块，
// 用 goja Compile 做语法检查（面板 JS 出错的表现=树/面板/部件卡片空白）。
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"wb-ui/goja"
)

func main() {
	b, err := os.ReadFile("F:/syproject/直播挂件助手/internal/configwin/ui.go")
	if err != nil {
		fmt.Println("read:", err)
		os.Exit(1)
	}
	src := string(b)
	re := regexp.MustCompile(`(?s)<script>(.*?)</script>`)
	matches := re.FindAllStringSubmatch(src, -1)
	fmt.Println("script blocks:", len(matches))
	bad := 0
	for i, m := range matches {
		code := m[1]
		code = strings.Replace(code, "var widgets = %s;", "var widgets = [];", 1)
		code = strings.Replace(code, "var presets = %s;", "var presets = [];", 1)
		code = strings.Replace(code, "var videoDevices = %s;", "var videoDevices = [];", 1)
		code = strings.Replace(code, "::SCENEW::", "1280", -1)
		code = strings.Replace(code, "::SCENEH::", "720", -1)
		if strings.TrimSpace(code) == "" {
			continue
		}
		_, err := goja.Compile(fmt.Sprintf("ui%d.js", i), code, true)
		if err != nil {
			bad++
			fmt.Printf("--- script#%d ERROR: %v\n", i, err)
			lines := strings.Split(code, "\n")
			for li, ln := range lines {
				if strings.Contains(err.Error(), fmt.Sprintf("%d:", li+1)) || strings.Contains(err.Error(), fmt.Sprintf("line %d", li+1)) {
					fmt.Printf("    L%d: %s\n", li+1, strings.TrimSpace(ln))
					if li+5 < len(lines) {
						fmt.Printf("    L%d: %s\n", li+2, strings.TrimSpace(lines[li+1]))
					}
					break
				}
			}
		}
	}
	if bad == 0 {
		fmt.Println("OK: all script blocks compile")
	} else {
		fmt.Println("BAD:", bad, "block(s)")
		os.Exit(1)
	}
}
