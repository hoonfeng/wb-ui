//go:build !windows

package main

import "os/exec"

// hideWindow 非 Windows 平台无需隐藏控制台。
func hideWindow(cmd *exec.Cmd) {}
