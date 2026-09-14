
//go:build ignore

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"wb-ui/engine/js/goja"
)

func main() {
	for i := 0; ; i++ {
		p := fmt.Sprintf("F:/syproject/直播挂件助手/dev/l2d_tmpl_script_%d.js", i)
		b, err := os.ReadFile(p)
		if err != nil {
			break
		}
		code := string(b)
		_, cerr := goja.Compile(fmt.Sprintf("script%d", i), code, true)
		if cerr != nil {
			fmt.Printf("script#%d ERROR: %v\n", i, cerr)
			lines := strings.Split(code, "\n")
			// 找行号
			s := cerr.Error()
			ln := 0
			parts := strings.Split(s, ":")
			for _, p2 := range parts[1:] {
				if n, e := strconv.Atoi(p2); e == nil {
					ln = n
					break
				}
			}
			for li := ln - 4; li <= ln+2 && li >= 0 && li < len(lines); li++ {
				l := lines[li]
				if len(l) > 160 {
					l = l[:160] + "..."
				}
				fmt.Printf("  [%d] %s\n", li+1, l)
			}
		} else {
			fmt.Printf("script#%d OK (%d chars)\n", i, len(code))
		}
	}
	fmt.Println("CHECK_DONE")
}
