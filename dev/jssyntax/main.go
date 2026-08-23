//go:build ignore

package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/dop251/goja"
)

func main() {
	b, _ := os.ReadFile("F:/syproject/直播挂件助手/internal/dev/cfgdump/config_dump.html")
	html := string(b)
	re := regexp.MustCompile(`<script>([\s\S]*?)</script>`)
	matches := re.FindAllStringSubmatch(html, -1)
	fmt.Println("script blocks:", len(matches))
	for i, m := range matches {
		code := m[1]
		if strings.TrimSpace(code) == "" {
			continue
		}
		vm := goja.New()
		_, err := vm.RunString(code)
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
