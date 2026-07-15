package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	path := `F:\syproject\wb-ui\app\host.go`
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	content := string(data)

	// Remove duplicate switch block in isTextFormControl
	target := `		case "checkbox", "radio", "range", "color", "file",\n\t\t\t"submit", "reset", "button", "image", "hidden":\n\t\t\treturn false\n\t\t}\n\t\treturn true\n\tdefault:\n\t\treturn false\n\t}\n}\n\n// calcTextControlOffset converts a CSS-pixel position`

	// Try to find the second, correct version and keep only it
	idx := strings.Index(content, "case \"checkbox\", \"radio\", \"range\", \"color\", \"file\",")
	if idx < 0 {
		fmt.Println("NOT FOUND 1")
		os.Exit(1)
	}
	// Find the second occurrence
	idx2 := strings.Index(content[idx+len("case \"checkbox\""):], "case \"checkbox\"")
	if idx2 >= 0 {
		idx2 += idx + len("case \"checkbox\"")
		// The first occurrence is the bad one (missing the rest of the case list).
		// Delete from the bad case line to "return false\n\t}\n"
		start := idx
		end := idx
		for i := start; i < len(content); i++ {
			if content[i] == '\n' {
				end = i
				break
			}
		}
		// Now find the return false + closing braces of the bad block
		afterBadCase := content[end:]
		closeIdx := strings.Index(afterBadCase, "}\n}\n\n// calcTextControlOffset")
		if closeIdx < 0 {
			closeIdx = strings.Index(afterBadCase, "}\n}\n\n// calcTextControlOffset")
		}
		if closeIdx < 0 {
			// Try to find where the good version starts
			restart := strings.Index(afterBadCase, "switch el.LocalName()")
			if restart >= 0 {
				content = content[:end] + afterBadCase[restart:]
				fmt.Println("Fixed via switch restart")
			} else {
				fmt.Println("Could not find repair point")
				os.Exit(1)
			}
		} else {
			content = content[:end] + afterBadCase[closeIdx:]
		}
	} else {
		fmt.Println("Only one occurrence found")
	}

	// Fix calcTextControlOffset signature error
	content = strings.Replace(content,
		"func (h *Host) calcTextControlOffset(el *dom.Element, cssX,",
		"func (h *Host) calcTextControlOffset(el *dom.Element, cssX, cssY float64) int {",
		1)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		panic(err)
	}
	fmt.Println("OK")
}
