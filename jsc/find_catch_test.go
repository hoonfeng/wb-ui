package jsc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestFindCatchInBundle(t *testing.T) {
	data, _ := os.ReadFile("F:/syproject/gou-ide/cmd/desktop/web-ui-minimal/dist/assets/app-BwgZYNXz.js")
	s := string(data)
	
	// 找 catch
	i := strings.Index(s, "catch")
	if i < 0 {
		t.Fatal("catch not found!")
	}
	start := i - 80
	if start < 0 { start = 0 }
	end := i + 150
	if end > len(s) { end = len(s) }
	fmt.Printf("CATCH at %d:\n%s\n", i, s[start:end])
	
	// 找第二个 catch
	s2 := s[i+1:]
	i2 := strings.Index(s2, "catch")
	if i2 >= 0 {
		start2 := i + 1 + i2 - 80
		if start2 < 0 { start2 = 0 }
		end2 := i + 1 + i2 + 150
		if end2 > len(s) { end2 = len(s) }
		fmt.Printf("\nCATCH2 at %d:\n%s\n", i+1+i2, s[start2:end2])
	}
	
	// 找 try
	i3 := strings.Index(s, "try{")
	if i3 >= 0 {
		end3 := i3 + 200
		if end3 > len(s) { end3 = len(s) }
		fmt.Printf("\nTRY at %d:\n%s\n", i3, s[i3:end3])
	}
	
	// Find ALL occurrences of qe(st)
	offset := 0
	count := 0
	for {
		i := strings.Index(s[offset:], "qe(st)")
		if i < 0 { break }
		count++
		absPos := offset + i
		start := absPos - 100
		if start < 0 { start = 0 }
		end := absPos + 30
		if end > len(s) { end = len(s) }
		fmt.Printf("\nqe(st) #%d at %d:\n%s\n", count, absPos, s[start:end])
		offset = absPos + 1
	}

	// 找 "Nt("
	i5 := strings.Index(s, "Nt(")
	if i5 >= 0 {
		fmt.Printf("\nNt( at %d\n", i5)
		start5 := i5 - 100
		if start5 < 0 { start5 = 0 }
		end5 := i5 + 50
		if end5 > len(s) { end5 = len(s) }
		fmt.Printf("Context:\n%s\n", s[start5:end5])
	}
	
	// 找 "Ci("
	i6 := strings.Index(s, "Ci(")
	if i6 >= 0 {
		fmt.Printf("\nCi( at %d\n", i6)
		start6 := i6 - 50
		if start6 < 0 { start6 = 0 }
		end6 := i6 + 50
		if end6 > len(s) { end6 = len(s) }
		fmt.Printf("Context:\n%s\n", s[start6:end6])
	}
	
	// 找 console.error
	i7 := strings.Index(s, "console.error")
	if i7 >= 0 {
		fmt.Printf("\nconsole.error at %d\n", i7)
		start7 := i7 - 50
		if start7 < 0 { start7 = 0 }
		end7 := i7 + 60
		if end7 > len(s) { end7 = len(s) }
		fmt.Printf("Context:\n%s\n", s[start7:end7])
	} else {
		fmt.Println("\nconsole.error NOT FOUND")
	}
	
	// 找第二个 console.error
	i8 := strings.Index(s[i7+1:], "console.error")
	if i7 >= 0 && i8 >= 0 {
		abs8 := i7 + 1 + i8
		fmt.Printf("\nconsole.error2 at %d\n", abs8)
		start8 := abs8 - 50
		if start8 < 0 { start8 = 0 }
		end8 := abs8 + 60
		if end8 > len(s) { end8 = len(s) }
		fmt.Printf("Context:\n%s\n", s[start8:end8])
	}
}
