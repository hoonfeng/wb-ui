package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

var (
	user32     = syscall.NewLazyDLL("user32.dll")
	findWindow = user32.NewProc("FindWindowW")
	enumWin    = user32.NewProc("EnumWindows")
	getWinText = user32.NewProc("GetWindowTextW")
	getWinTextLen = user32.NewProc("GetWindowTextLengthW")
	postMsg    = user32.NewProc("PostMessageW")
	setFore    = user32.NewProc("SetForegroundWindow")
)

const (
	WM_MOUSEMOVE  = 0x0200
	WM_LBUTTONDOWN = 0x0201
	WM_LBUTTONUP   = 0x0202
)

// MAKELPARAM creates an LPARAM from x and y coordinates.
func MAKELPARAM(x, y int) uintptr {
	return uintptr((y << 16) | (x & 0xFFFF))
}

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: %s <title-substring> <x> <y>\n", os.Args[0])
		os.Exit(1)
	}
	titleSub := os.Args[1]
	cx, _ := strconv.Atoi(os.Args[2])
	cy, _ := strconv.Atoi(os.Args[3])

	hwnd := findWindowByTitle(titleSub)
	if hwnd == 0 {
		fmt.Fprintf(os.Stderr, "Window not found: %s\n", titleSub)
		os.Exit(1)
	}

	fmt.Printf("Found hwnd=0x%x\n", hwnd)
	setFore.Call(hwnd)

	// Send mouse events directly (client coordinates)
	fmt.Printf("Clicking at (%d,%d)\n", cx, cy)
	
	// Move first
	postMsg.Call(hwnd, WM_MOUSEMOVE, 0, MAKELPARAM(cx, cy))
	// Down
	postMsg.Call(hwnd, WM_LBUTTONDOWN, 1, MAKELPARAM(cx, cy))
	// Up
	postMsg.Call(hwnd, WM_LBUTTONUP, 0, MAKELPARAM(cx, cy))

	fmt.Println("PostMessage clicks sent.")
}

func findWindowByTitle(sub string) uintptr {
	var found uintptr
	enumWin.Call(syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		length, _, _ := getWinTextLen.Call(hwnd)
		if length == 0 {
			return 1
		}
		buf := make([]uint16, length+1)
		getWinText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(length+1))
		title := syscall.UTF16ToString(buf)
		if strings.Contains(strings.ToLower(title), strings.ToLower(sub)) {
			found = hwnd
			return 0
		}
		return 1
	}), 0)
	if found != 0 {
		fmt.Printf("Found window: hwnd=0x%x\n", found)
	}
	return found
}
