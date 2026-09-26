package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// discoverBackend 按优先级定位 gou-ide 后端可执行文件：
//  1. -backend 显式指定
//  2. 环境变量 PAIRCODE_BACKEND
//  3. 本程序同目录：pair.exe / companion.exe / paircode.exe / pair
//  4. 本程序同目录 bin/：pair.exe / pair / companion.exe
//  5. PATH 中的 pair.exe / companion.exe
func discoverBackend(explicit string) (string, error) {
	var cands []string
	if explicit != "" {
		cands = append(cands, explicit)
	}
	if env := os.Getenv("PAIRCODE_BACKEND"); env != "" {
		cands = append(cands, env)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, n := range []string{"pair.exe", "companion.exe", "paircode.exe", "pair"} {
			cands = append(cands, filepath.Join(dir, n))
		}
		for _, n := range []string{"pair.exe", "pair", "companion.exe"} {
			cands = append(cands, filepath.Join(dir, "bin", n))
		}
	}
	for _, n := range []string{"pair.exe", "companion.exe"} {
		if p, err := exec.LookPath(n); err == nil {
			cands = append(cands, p)
		}
	}
	for _, c := range cands {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("未找到后端可执行文件（pair.exe / companion.exe）：请用 -backend 指定，" +
		"或把 pair.exe 放在本程序同目录，或设置环境变量 PAIRCODE_BACKEND")
}

// probeReady 探测后端是否已就绪（GET /api/system/info）。
func probeReady(base string, timeout time.Duration) bool {
	cli := &http.Client{Timeout: timeout}
	resp, err := cli.Get(strings.TrimSuffix(base, "/") + "/api/system/info")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 500
}

// startBackend 拉起后端子进程：工作目录 = 后端 exe 所在目录，
// WEB_PORT 指定端口（★ 不动默认 9090），stdout/stderr 落日志文件，
// Windows 下隐藏控制台窗口。
func startBackend(exe string, port int, logPath string) (*exec.Cmd, error) {
	cmd := exec.Command(exe)
	cmd.Dir = filepath.Dir(exe)
	cmd.Env = append(os.Environ(), fmt.Sprintf("WEB_PORT=%d", port))
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err == nil {
		if f, ferr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); ferr == nil {
			cmd.Stdout = f
			cmd.Stderr = f
		}
	}
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// waitReady 轮询等待后端就绪（首次启动要加载内核/插件，给足 120s）。
func waitReady(base string, total time.Duration) bool {
	deadline := time.Now().Add(total)
	for {
		if probeReady(base, 1500*time.Millisecond) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(400 * time.Millisecond)
	}
}

// stopBackend 结束后端进程（仅限本程序拉起的；proc 为 nil 时无操作）。
func stopBackend(proc *exec.Cmd) {
	if proc == nil || proc.Process == nil {
		return
	}
	logf("正在结束后端进程（PID %d）…", proc.Process.Pid)
	_ = proc.Process.Kill()
	_, _ = proc.Process.Wait()
}
