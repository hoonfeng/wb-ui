package main

// MediaStudio 的应用 API（页面 fetch → bridge 进程内路由 → 宿主动作）。
//
// 与 cmd/psai 的桩路由同一机制（bridge.RegisterHTTP，全程无网络）：
// 页面按钮点击 → 页面 JS → fetch('/api/…') → 本文件的路由 → 宿主执行录制/剪辑/导出 →
// 结果回填页面（status 响应）。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"wb-ui/bridge"
)

// registerRoutes 注册应用 API。
func (s *studio) registerRoutes() {
	bridge.RegisterHTTP("GET", "/api/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, s.status())
	})

	bridge.RegisterHTTP("POST", "/api/record/start", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Seconds float64 `json:"seconds"`
			FPS     int     `json:"fps"`
			Audio   bool    `json:"audio"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &in)
		if in.Seconds <= 0 {
			in.Seconds = s.cfg.sec
		}
		if in.FPS <= 0 {
			in.FPS = s.cfg.fps
		}
		tk, err := s.record(in.Seconds, in.FPS, in.Audio, true)
		res := s.status()
		switch {
		case err != nil:
			res["ok"] = false
			res["message"] = "录制失败：" + err.Error()
		default:
			res["message"] = fmt.Sprintf("录制完成：%d 帧 @%dfps · 画面 %s · 音轨 %d 字节 · record.mp4 %d 字节",
				tk.frames, tk.fps, tk.stage, tk.pcmBytes, tk.mp4Bytes)
		}
		writeJSON(w, res)
	})

	bridge.RegisterHTTP("POST", "/api/clip/export", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			In  float64 `json:"in"`
			Out float64 `json:"out"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &in)
		ct, err := s.exportClip(in.In, in.Out)
		res := s.status()
		switch {
		case err != nil:
			res["ok"] = false
			res["message"] = "剪辑失败：" + err.Error()
		default:
			res["message"] = fmt.Sprintf("剪辑完成：%d 帧 · %.1fs（%.1f→%.1fs）· clip.mp4 %d 字节",
				ct.frames, ct.seconds, in.In, in.Out, ct.mp4Bytes)
		}
		writeJSON(w, res)
	})

	bridge.RegisterHTTP("POST", "/api/frame/export", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Format string `json:"format"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &in)
		path, err := s.exportFrame(in.Format)
		res := s.status()
		switch {
		case err != nil:
			res["ok"] = false
			res["message"] = "导出当前帧失败：" + err.Error()
		default:
			res["message"] = "已导出当前帧：" + path
		}
		writeJSON(w, res)
	})

	// 页面布局自上报：窗口模式下宿主主循环被 Run() 占用、无法从外部读页面几何，
	// 由页面自己把视口与关键面板的矩形报回来（写进应用日志/报告）。
	bridge.RegisterHTTP("POST", "/api/diag", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.note("布局自上报 %s", strings.TrimSpace(string(body)))
		writeJSON(w, map[string]any{"ok": true})
	})
}

// writeJSON 回写 JSON 响应。
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
