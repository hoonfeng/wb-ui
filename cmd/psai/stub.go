// AI-PS 宿主的「桩实现」（stub）：把页面的请求与事件接管为固定应答。
//
// 分工：interceptor.go 负责「拦截 + 留痕」，本文件负责「作答」。
// 所有 handler 都是标准 http.HandlerFunc，但**没有任何网络**——wb-ui 的 bridge 把
// 页面的 fetch 直接 dispatch 到这里（bridge.RegisterHTTP），请求不离开本进程。
//
// 测试项目原则：能跑通链路优先 —— 未实现的真实能力（模型推理、图层合成、文件落盘）
// 一律先用桩响应，但每次调用都真实经过拦截器并留痕，便于核对链路完整性。
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"wb-ui/bridge"
)

func registerStubs(ic *Interceptor) {
	// ── 事件上报：页面事件（工具选择 / 图层选择 / 回车提交 / 按钮 emit）→ Go ──
	bridge.RegisterHTTP("POST", "/api/events", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		name, payload := parseEvent(string(raw))
		ic.recordEvent(name, payload)
		res, hits, reqs, evts, clicks, loaded := ic.Counts()
		writeJSON(w, map[string]any{
			"ok":      true,
			"stub":    true,
			"name":    name,
			"count":   evts,
			"message": "事件已由 Go 拦截器记录（桩）",
			"stats": map[string]int{
				"resourceRequests": res, "resourceHits": hits, "bridgeRequests": reqs,
				"events": evts, "hostClicks": clicks, "engineLoaded": loaded,
			},
		})
	})

	// ── AI 生成（桩）：不调用任何模型，返回占位结果与预览配色 ──
	bridge.RegisterHTTP("POST", "/api/ai/generate", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Prompt string `json:"prompt"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &in)
		ic.recordRequest("POST", "/api/ai/generate", string(raw))
		_, _, reqs, _, _, _ := ic.Counts()
		writeJSON(w, map[string]any{
			"stub":     true,
			"provider": "local-stub",
			"jobId":    fmt.Sprintf("stub-job-%04d", reqs),
			"prompt":   in.Prompt,
			"message":  "已受理（本地桩：未调用任何模型）",
			"steps":    []string{"解析提示词", "创建图层", "渲染占位位图"},
			"previewColors": []string{"#4A9EFF", "#2D9D78", "#E68619"},
			"model":         "stub-diffusion-0",
		})
	})

	// ── AI 变体（桩）：在原结果上给 3 个"变体"占位 ──
	bridge.RegisterHTTP("POST", "/api/ai/variant", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Prompt string `json:"prompt"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &in)
		ic.recordRequest("POST", "/api/ai/variant", string(raw))
		_, _, reqs, _, _, _ := ic.Counts()
		writeJSON(w, map[string]any{
			"stub":    true,
			"message": "已生成 3 个变体（本地桩）",
			"jobId":   fmt.Sprintf("stub-variant-%04d", reqs),
			"variants": []map[string]any{
				{"id": "v1", "seed": 1001, "delta": "色温 +120"},
				{"id": "v2", "seed": 1002, "delta": "对比 +8%"},
				{"id": "v3", "seed": 1003, "delta": "柔光晕影"},
			},
			"previewColors": []string{"#5CA8FF", "#3AA76D", "#E6A23C"},
		})
	})

	// ── 导出（桩）：不落盘，只回一个"计划写出"的路径 ──
	bridge.RegisterHTTP("POST", "/api/export", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		ic.recordRequest("POST", "/api/export", string(raw))
		writeJSON(w, map[string]any{
			"stub":    true,
			"message": "导出已受理（本地桩：未写盘）",
			"path":    "out/ai-ps-export.png",
			"format":  "png",
			"width":   1920,
			"height":  1080,
			"bytes":   0,
		})
	})

	// ── 图层列表（桩）：把设计稿里的三层回给页面 ──
	bridge.RegisterHTTP("GET", "/api/layers", func(w http.ResponseWriter, r *http.Request) {
		ic.recordRequest("GET", "/api/layers", "")
		writeJSON(w, map[string]any{
			"stub": true,
			"layers": []map[string]any{
				{"id": "L3", "name": "AI 生成层", "opacity": 80, "visible": true, "kind": "raster"},
				{"id": "L2", "name": "文字层", "opacity": 100, "visible": true, "kind": "text"},
				{"id": "L1", "name": "背景", "opacity": 100, "visible": true, "kind": "raster"},
			},
		})
	})

	// ── 运行状态（桩）：把拦截统计回给页面/自动化，形成可核对的回路 ──
	bridge.RegisterHTTP("GET", "/api/status", func(w http.ResponseWriter, r *http.Request) {
		res, hits, reqs, evts, clicks, loaded := ic.Counts()
		writeJSON(w, map[string]any{
			"stub": true, "mode": "toolkit", "transport": "in-process-bridge（无 HTTP）",
			"resourceRequests": res, "resourceHits": hits, "bridgeRequests": reqs,
			"events": evts, "hostClicks": clicks, "engineLoaded": loaded,
		})
	})
}

// parseEvent 解析事件上报体 {"name": "...", "payload": {...}}。
func parseEvent(raw string) (string, string) {
	var msg struct {
		Name    string          `json:"name"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal([]byte(raw), &msg); err != nil || msg.Name == "" {
		return "(unparsed)", truncate(strings.TrimSpace(raw), 200)
	}
	payload := strings.TrimSpace(string(msg.Payload))
	if payload == "" || payload == "null" {
		payload = "{}"
	}
	return msg.Name, payload
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
