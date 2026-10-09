//go:build !windows

package app

// 非 Windows 平台没有 WASAPI loopback；判据 B 在这类平台上仍走 ffmpeg dshow/
// 同类采集工具的回退路径（见 cmd/psai/mediaaudio_probe.go）。
import (
	"errors"

	"wb-ui/engine/rendering"
)

// CaptureDefaultRenderLoopback 在非 Windows 平台不可用。
func CaptureDefaultRenderLoopback(seconds float64) ([]byte, rendering.AudioFormat, error) {
	return nil, rendering.AudioFormat{}, errors.New("audio: 本平台未实现 WASAPI loopback 采集")
}

// LoopbackCaptureAvailable 在非 Windows 平台恒为 false。
func LoopbackCaptureAvailable() bool { return false }
