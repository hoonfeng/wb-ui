package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	findWindow           = user32.NewProc("FindWindowW")
	setForegroundWindow  = user32.NewProc("SetForegroundWindow")
	getWindowRect        = user32.NewProc("GetWindowRect")
	getSystemMetrics     = user32.NewProc("GetSystemMetrics")
	enumWindows          = user32.NewProc("EnumWindows")
	getWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	getWindowTextW       = user32.NewProc("GetWindowTextW")
	sendInput            = user32.NewProc("SendInput")
)

type RECT struct {
	Left, Top, Right, Bottom int32
}

type MOUSEINPUT struct {
	Dx          int32
	Dy          int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type INPUT struct {
	Type int32
	Mi   MOUSEINPUT
}

const (
	INPUT_MOUSE        = 0
	MOUSEEVENTF_MOVE   = 0x0001
	MOUSEEVENTF_LEFTDN = 0x0002
	MOUSEEVENTF_LEFTUP = 0x0004
	MOUSEEVENTF_ABS    = 0x8000
)

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

	setForegroundWindow.Call(hwnd)
	time.Sleep(200 * time.Millisecond)

	var rect RECT
	getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	sw, _, _ := getSystemMetrics.Call(0)
	sh, _, _ := getSystemMetrics.Call(1)

	absX := int32((rect.Left + int32(cx)) * 65535 / int32(sw))
	absY := int32((rect.Top + int32(cy)) * 65535 / int32(sh))

	fmt.Printf("Window: (%d,%d)-(%d,%d)\n", rect.Left, rect.Top, rect.Right, rect.Bottom)
	fmt.Printf("Screen: %dx%d\n", sw, sh)
	fmt.Printf("Click at client=(%d,%d) abs=(%d,%d)\n", cx, cy, absX, absY)

	sz := int32(unsafe.Sizeof(INPUT{}))

	move := INPUT{Type: INPUT_MOUSE, Mi: MOUSEINPUT{Dx: absX, Dy: absY, DwFlags: MOUSEEVENTF_MOVE | MOUSEEVENTF_ABS}}
	sendInput.Call(1, uintptr(unsafe.Pointer(&move)), uintptr(sz))
	time.Sleep(100 * time.Millisecond)

	down := INPUT{Type: INPUT_MOUSE, Mi: MOUSEINPUT{DwFlags: MOUSEEVENTF_LEFTDN}}
	sendInput.Call(1, uintptr(unsafe.Pointer(&down)), uintptr(sz))
	time.Sleep(50 * time.Millisecond)

	up := INPUT{Type: INPUT_MOUSE, Mi: MOUSEINPUT{DwFlags: MOUSEEVENTF_LEFTUP}}
	sendInput.Call(1, uintptr(unsafe.Pointer(&up)), uintptr(sz))

	fmt.Println("Click sent.")
}

func findWindowByTitle(sub string) uintptr {
	var found uintptr
	enumWindows.Call(syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		length, _, _ := getWindowTextLengthW.Call(hwnd)
		if length == 0 {
			return 1
		}
		buf := make([]uint16, length+1)
		getWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(length+1))
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
