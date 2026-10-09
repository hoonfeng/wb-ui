//go:build windows

package app

// WASAPI loopback 采集（判据 B 的「采集侧」，Windows）。
//
// 为什么需要它：判据 B 原来只有一条路——用 ffmpeg dshow 录一个**环回录音设备**
// （立体声混音 / 虚拟声卡）。那条路的可用性完全取决于机器上有没有这样的设备、
// 以及虚拟声卡的内部路由配没配好；实测在「默认输出是虚拟声卡、而虚拟声卡引擎
// 未运行」的机器上，dshow 只录到底噪（rms ≈ 1e-5），判据 B 只能记 mismatch ——
// 这不是引擎的问题，却是「端到端那一环没有外部证据」。
//
// WASAPI loopback 把这件事变成**不依赖任何额外设备与路由**：它由音频引擎在
// render 端点内部做 tap，录到的就是「应用写给该端点、混音之后」的字节——这也正是
// 浏览器 / OBS 等现代采集路径在 Windows 上的做法（"系统音频采集"）。因此本文件
// 是判据 B 的首选路径，ffmpeg dshow 退化为回退路径。
//
// API 序列（每一步都取自 WASAPI 文档，实测行为见 dev/media/out 的环回记录）：
//
//	CoInitializeEx(MULTITHREADED)
//	CoCreateInstance(MMDeviceEnumerator)
//	IMMDeviceEnumerator::GetDefaultAudioEndpoint(eRender, eConsole)
//	IMMDevice::Activate(IAudioClient)
//	IAudioClient::GetMixFormat
//	IAudioClient::Initialize(SHARED, AUDCLNT_STREAMFLAGS_LOOPBACK, ...)
//	IAudioClient::GetService(IAudioCaptureClient)
//	IAudioClient::Start → 轮询 GetNextPacketSize/GetBuffer/ReleaseBuffer → Stop
//
// 为什么手写 vtable 而不是引第三方包：与 audioout_windows.go 同一条理由——
// 只需要几个方法（薄），引入 COM 包装库反而要背它的线程模型与生命周期假设。
import (
	"fmt"
	"math"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"wb-ui/engine/rendering"
)

const (
	clsctxAll                  = 0x17
	coinitMultithreaded        = 0x0
	audclntSharemodeShared     = 0
	audclntStreamflagsLoopback = 0x00020000
	waveFormatIEEEFloat        = 0x0003
	wFormatTagExtensible       = 0xFFFE
	// eRender / eConsole（EDataFlow / ERole）
	edataflowRender = 0
	eroleConsole    = 0
)

var (
	ole32DLL = syscall.NewLazyDLL("ole32.dll")

	procCoInitializeEx   = ole32DLL.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32DLL.NewProc("CoUninitialize")
	procCoCreateInstance = ole32DLL.NewProc("CoCreateInstance")
	procCoTaskMemFree    = ole32DLL.NewProc("CoTaskMemFree")
)

// comGUID 是 GUID（与 Windows 的内存布局一致）。
type comGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	clsidMMDeviceEnumerator = comGUID{0xBCDE0395, 0xE52F, 0x467C, [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnumerator  = comGUID{0xA95664D2, 0x9614, 0x4F35, [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	iidIAudioClient         = comGUID{0x1CB9AD4C, 0xDBFA, 0x4C32, [8]byte{0xB1, 0x78, 0xC2, 0xF5, 0x68, 0xA7, 0x03, 0xB2}}
	iidIAudioCaptureClient  = comGUID{0xC8ADBD64, 0xE71E, 0x48A0, [8]byte{0xA4, 0xDE, 0x18, 0x5C, 0x39, 0x5C, 0xD3, 0x17}}
	// KSDATAFORMAT_SUBTYPE_IEEE_FLOAT（WAVEFORMATEXTENSIBLE 的 SubFormat）
	subFormatIEEEFloat = comGUID{0x00000003, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}}
)

// waveFormatExtensible 是 WAVEFORMATEXTENSIBLE。
//
// ★ 这里必须**平铺**字段，不能写成 `struct { Format waveFormatEx; Samples uint16; ... }`：
//
//	C 的 WAVEFORMATEX 是 18 字节紧凑布局，而 Go 会把内嵌结构按最大字段对齐补到 20
//	字节，于是 Samples/ChannelMask/SubFormat 全部后移 2 字节，SubFormat GUID 读到垃圾
//	（实测症状：混音格式 tag=0xFFFE bits=32 却被判成非 float，采集直接报「不支持的
//	混音格式」）。平铺后 18+2=20 恰好 4 字节对齐，与 C 的偏移逐字段一致。
type waveFormatExtensible struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	Size           uint16
	Samples        uint16 // union：wValidBitsPerSample / wSamplesPerBlock / wReserved
	ChannelMask    uint32
	SubFormat      comGUID
}

// comCall 调用 COM 接口第 idx 个 vtable 方法（0/1/2 是 IUnknown 的三个）。
// 第一个参数固定是接口指针本身。
func comCall(iface uintptr, idx int, args ...uintptr) uintptr {
	if iface == 0 {
		return 0x80004003 // E_POINTER
	}
	vtbl := *(*uintptr)(unsafe.Pointer(iface))
	if vtbl == 0 {
		return 0x80004005 // E_FAIL
	}
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	if fn == 0 {
		return 0x80004005
	}
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{iface}, args...)...)
	return r
}

// comRelease 释放一个 COM 接口指针（IUnknown::Release）。
func comRelease(iface uintptr) {
	if iface != 0 {
		comCall(iface, 2)
	}
}

// CaptureDefaultRenderLoopback 录制「默认输出设备」的 WASAPI loopback。
//
// 返回 s16le 交错 PCM 与**实际**格式（采样率/声道取自设备的混音格式——不假设
// 48kHz/2ch，调用方用返回的格式做 FFT）。seconds 是录音窗口长度。
//
// ★ 已知语义（Windows 的 loopback 特性，不是缺陷）：端点**没有音频在播**时
//
//	GetNextPacketSize 返回 0、不产生静音包，因此「先起录音、再起播放」是必须的
//	（调用方 probeLoopback 即如此），否则窗口内可能一帧都拿不到。
func CaptureDefaultRenderLoopback(seconds float64) ([]byte, rendering.AudioFormat, error) {
	if seconds <= 0 {
		seconds = 1
	}
	format := rendering.AudioFormat{}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := procCoInitializeEx.Call(0, coinitMultithreaded)
	// S_OK=0、S_FALSE=1、RPC_E_CHANGED_MODE=0x80010106（别的组件已按单元线程
	// 初始化过——loopback 用不到消息泵，直接继续）。
	if hr != 0 && hr != 1 && hr != 0x80010106 {
		return nil, format, fmt.Errorf("audio: CoInitializeEx 失败：0x%08X", uint32(hr))
	}
	defer procCoUninitialize.Call()

	var enum uintptr
	hr, _, _ = procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)), 0, clsctxAll,
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)), uintptr(unsafe.Pointer(&enum)))
	runtime.KeepAlive(&clsidMMDeviceEnumerator)
	if hr != 0 || enum == 0 {
		return nil, format, fmt.Errorf("audio: CoCreateInstance(MMDeviceEnumerator) 失败：0x%08X", uint32(hr))
	}
	defer comRelease(enum)

	var dev uintptr
	if hr = comCall(enum, 4, edataflowRender, eroleConsole, uintptr(unsafe.Pointer(&dev))); hr != 0 || dev == 0 {
		return nil, format, fmt.Errorf("audio: GetDefaultAudioEndpoint(eRender) 失败：0x%08X", uint32(hr))
	}
	defer comRelease(dev)

	var audioClient uintptr
	if hr = comCall(dev, 3, uintptr(unsafe.Pointer(&iidIAudioClient)), clsctxAll, 0,
		uintptr(unsafe.Pointer(&audioClient))); hr != 0 || audioClient == 0 {
		return nil, format, fmt.Errorf("audio: IMMDevice::Activate(IAudioClient) 失败：0x%08X", uint32(hr))
	}
	defer comRelease(audioClient)

	var mix *waveFormatEx
	if hr = comCall(audioClient, 8, uintptr(unsafe.Pointer(&mix))); hr != 0 || mix == nil {
		return nil, format, fmt.Errorf("audio: IAudioClient::GetMixFormat 失败：0x%08X", uint32(hr))
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(mix)))

	channels := int(mix.Channels)
	sampleRate := int(mix.SamplesPerSec)
	if channels <= 0 || sampleRate <= 0 {
		return nil, format, fmt.Errorf("audio: 设备混音格式非法（%dch @ %dHz）", channels, sampleRate)
	}
	format = rendering.AudioFormat{SampleRate: sampleRate, Channels: channels}

	// 共享模式 loopback：periodicity 必须为 0；缓冲时长传 0 = 用引擎默认。
	if hr = comCall(audioClient, 3, audclntSharemodeShared, audclntStreamflagsLoopback,
		0, 0, uintptr(unsafe.Pointer(mix)), 0); hr != 0 {
		return nil, format, fmt.Errorf("audio: IAudioClient::Initialize(loopback) 失败：0x%08X", uint32(hr))
	}
	var capture uintptr
	if hr = comCall(audioClient, 14, uintptr(unsafe.Pointer(&iidIAudioCaptureClient)),
		uintptr(unsafe.Pointer(&capture))); hr != 0 || capture == 0 {
		return nil, format, fmt.Errorf("audio: GetService(IAudioCaptureClient) 失败：0x%08X", uint32(hr))
	}
	defer comRelease(capture)

	if hr = comCall(audioClient, 10); hr != 0 { // Start
		return nil, format, fmt.Errorf("audio: IAudioClient::Start 失败：0x%08X", uint32(hr))
	}
	defer comCall(audioClient, 11) // Stop（幂等）

	float32PCM := isFloatMixFormat(mix)
	bits := int(mix.BitsPerSample)
	var out []byte
	deadline := time.Now().Add(time.Duration(seconds * float64(time.Second)))
	for time.Now().Before(deadline) {
		var packet uint32
		if hr = comCall(capture, 5, uintptr(unsafe.Pointer(&packet))); hr != 0 {
			return out, format, fmt.Errorf("audio: GetNextPacketSize 失败：0x%08X", uint32(hr))
		}
		if packet == 0 {
			time.Sleep(2 * time.Millisecond)
			continue
		}
		var data uintptr
		var frames uint32
		var flags uint32
		var devPos, qpc uint64
		if hr = comCall(capture, 3, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&frames)),
			uintptr(unsafe.Pointer(&flags)), uintptr(unsafe.Pointer(&devPos)),
			uintptr(unsafe.Pointer(&qpc))); hr != 0 {
			break
		}
		if frames > 0 {
			switch {
			case float32PCM && bits == 32:
				src := unsafe.Slice((*float32)(unsafe.Pointer(data)), int(frames)*channels)
				out = appendFloat32AsS16(out, src)
			case !float32PCM && bits == 16:
				src := unsafe.Slice((*byte)(unsafe.Pointer(data)), int(frames)*channels*2)
				out = append(out, src...)
			default:
				comCall(capture, 4, uintptr(frames))
				return out, format, fmt.Errorf("audio: 不支持的混音格式（tag=0x%04X bits=%d float=%v）",
					mix.FormatTag, bits, float32PCM)
			}
		}
		comCall(capture, 4, uintptr(frames)) // ReleaseBuffer
	}
	runtime.KeepAlive(mix)
	return out, format, nil
}

// isFloatMixFormat 判断设备混音格式的样本是不是 IEEE float（WAVE_FORMAT_IEEE_FLOAT
// 或 WAVEFORMATEXTENSIBLE 且 SubFormat = IEEE_FLOAT）。
func isFloatMixFormat(mix *waveFormatEx) bool {
	switch mix.FormatTag {
	case waveFormatIEEEFloat:
		return true
	case wFormatTagExtensible:
		ext := (*waveFormatExtensible)(unsafe.Pointer(mix))
		return ext.SubFormat.Data1 == subFormatIEEEFloat.Data1 &&
			ext.SubFormat.Data4 == subFormatIEEEFloat.Data4
	default:
		return false
	}
}

// appendFloat32AsS16 把 [-1,1] 的 float32 交错样本追加成 s16le（截断到 int16 范围）。
func appendFloat32AsS16(dst []byte, src []float32) []byte {
	for _, f := range src {
		v := float64(f)
		if math.IsNaN(v) {
			v = 0
		}
		v = math.Max(-1, math.Min(1, v))
		s := int16(math.Round(v * 32767))
		dst = append(dst, byte(s), byte(uint16(s)>>8))
	}
	return dst
}

// LoopbackCaptureAvailable 报告本机能否做 WASAPI loopback 采集（探针的支持矩阵用）。
// 判定方式是**真的录一小段**：能拿到数据（哪怕全零）即视为可用——比「端点枚举
// 成功」可靠（被独占、格式不支持都在真正 Initialize/Start 时才暴露）。
//
// ★ 判定期间必须**有音频在播**：Windows 的 loopback 在端点静音时根本不产包
// （GetNextPacketSize 恒 0），否则这里会把「本机可用」误报成不可用。所以起一个
// 很短的测试音与录音重叠。
func LoopbackCaptureAvailable() bool {
	done := make(chan struct{})
	go func() {
		time.Sleep(120 * time.Millisecond)
		_ = PlayTestTone(440, 0.3, rendering.AudioFormat{SampleRate: 48000, Channels: 2})
		close(done)
	}()
	pcm, _, err := CaptureDefaultRenderLoopback(0.8)
	<-done
	return err == nil && len(pcm) > 0
}
