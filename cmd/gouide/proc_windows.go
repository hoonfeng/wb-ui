//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// createNoWindow 对应 Win32 CREATE_NO_WINDOW：控制台子进程不弹黑窗。
const createNoWindow = 0x08000000

// hideWindow 让后端子进程（控制台程序）不弹出控制台窗口。
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
