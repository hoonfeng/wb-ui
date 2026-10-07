//go:build windows

package app

// Windows 输出后端（主线 A3-1）：waveOut。
//
// 为什么是 waveOut 而不是 WASAPI：waveOut 是 winmm 里最薄的一层（几个 libcall，
// 没有 COM、没有设备枚举回调、没有线程模型要求），完全够用——本阶段要证明的
// 是「PCM 真的到了设备、且设备位置能当播放时钟」，不是低延迟专业音频。WASAPI
// 的价值（独占模式、极低延迟、精确事件驱动）在本阶段的判据里用不上，却要引入
// COM 生命周期与线程亲和性两套坑。升级路径记在 docs/TECH_DEBT.md。
//
// 关键 API 与语义（都取自 winmm 文档，实测行为见 docs 的验收记录）：
//
//	waveOutOpen(WAVE_MAPPER)      打开默认设备；WAVE_MAPPER = (UINT)-1
//	waveOutPrepareHeader/Write    提交一块 PCM；缓冲在设备队列里不能被改写
//	waveOutGetPosition(TIME_SAMPLES)
//	                              设备**已播放**的采样帧数——A3 的时钟真相源
//	waveOutReset                  丢弃队列并把位置归零（seek/flush 用）
//
// 缓冲模型：audioDeviceBuffers 个固定缓冲轮流复用。write 返回前保证数据已复制
// 进缓冲（调用方的切片可以立刻释放），缓冲全部在飞时阻塞等待——这个阻塞就是
// 推送循环的节流点。
import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"wb-ui/engine/rendering"
)

var (
	winmmDLL = syscall.NewLazyDLL("winmm.dll")

	procWaveOutGetNumDevs      = winmmDLL.NewProc("waveOutGetNumDevs")
	procWaveOutOpen            = winmmDLL.NewProc("waveOutOpen")
	procWaveOutClose           = winmmDLL.NewProc("waveOutClose")
	procWaveOutPrepareHeader   = winmmDLL.NewProc("waveOutPrepareHeader")
	procWaveOutUnprepareHeader = winmmDLL.NewProc("waveOutUnprepareHeader")
	procWaveOutWrite           = winmmDLL.NewProc("waveOutWrite")
	procWaveOutReset           = winmmDLL.NewProc("waveOutReset")
	procWaveOutPause           = winmmDLL.NewProc("waveOutPause")
	procWaveOutRestart         = winmmDLL.NewProc("waveOutRestart")
	procWaveOutGetPosition     = winmmDLL.NewProc("waveOutGetPosition")
)

const (
	// waveMapper 是 WAVE_MAPPER ((UINT)-1)：让系统挑默认输出设备。
	waveMapper = uintptr(0xFFFFFFFF)
	// wFormatPCM 是 WAVE_FORMAT_PCM。
	wFormatPCM = 0x0001
	// callbackNull 表示不用回调（我们靠轮询 WHDR_DONE 判断缓冲可复用）——
	// 回调会在系统线程上执行，把 Go 函数暴露给系统回调没必要（见 writeChunk）。
	callbackNull = 0
	// whdrDone 是 WAVEHDR.dwFlags 的 WHDR_DONE 位：该缓冲已播完。
	whdrDone = 0x00000001
	// MMTIME.wType 的时基常量（winmm 的 TIME_* 取值）。
	//
	// ★ 这里踩过一次坑：TIME_* 不是从 0 开始的枚举，TIME_SAMPLES 是 0x0002。
	//   写成 0 时 waveOutGetPosition 会把请求当成无效时基、**忽略它**并按自己
	//   支持的时基回答（本机驱动固定回 TIME_BYTES = 0x0004）——症状是「写入全部
	//   成功、设备位置却恒为 0」，播放时钟因此永远停在 0。
	timeMS      = 0x0001
	timeSamples = 0x0002
	timeBytes   = 0x0004
	// mmNoError 是 winmm 的成功码（MMSYSERR_NOERROR）。
	mmNoError = 0
)

// errAudioDeviceClosed 是「设备已关闭」的写入错误（会话关闭与推送循环的竞态）。
var errAudioDeviceClosed = errors.New("audio: 输出设备已关闭")

// waveFormatEx 是 WAVEFORMATEX（winmm 的 PCM 格式描述）。
type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	Size           uint16
}

// waveHdr 是 WAVEHDR（一块待播/在播 PCM）。
type waveHdr struct {
	Data          uintptr
	BufferLength  uint32
	BytesRecorded uint32
	User          uintptr
	Flags         uint32
	Loops         uint32
	Next          uintptr
	Reserved      uintptr
}

// mmTime 是 MMTIME：wType 之后是联合体，TIME_SAMPLES 用第一个 DWORD。
// 联合体里最大成员是 SMPTE（6 个 DWORD），这里留足 28 字节并把首 DWORD 当样本数读。
type mmTime struct {
	Type uint32
	rest [28]byte
}

// waveOutDevice 是 waveOut 输出设备。
type waveOutDevice struct {
	mu     sync.Mutex
	hwo    uintptr
	format rendering.AudioFormat

	bufs      [][]byte // 固定缓冲（持有引用，防止 GC 回收 Data 指向的内存）
	hdrs      []waveHdr
	prepared  []bool
	submitted []bool // 已交给设备、尚未 DONE

	closed bool
}

// openPlatformAudioDevice 打开默认 waveOut 设备。任何一步失败都返回 ok=false
// ——宿主随即进入降级模式（PCM 仍交付给 tap），播放语义不受影响。
func openPlatformAudioDevice(format rendering.AudioFormat) (audioDevice, bool) {
	if n, _, _ := procWaveOutGetNumDevs.Call(); n == 0 {
		return nil, false // 本机没有任何 waveOut 设备（无声卡/纯服务器环境）
	}
	f := waveFormatEx{
		FormatTag:      wFormatPCM,
		Channels:       uint16(format.Channels),
		SamplesPerSec:  uint32(format.SampleRate),
		AvgBytesPerSec: uint32(format.SampleRate * format.BytesPerFrame()),
		BlockAlign:     uint16(format.BytesPerFrame()),
		BitsPerSample:  16,
	}
	var hwo uintptr
	ret, _, _ := procWaveOutOpen.Call(
		uintptr(unsafe.Pointer(&hwo)),
		waveMapper,
		uintptr(unsafe.Pointer(&f)),
		0, 0, callbackNull,
	)
	if int32(ret) != mmNoError {
		return nil, false
	}
	bufFrames := format.SampleRate * audioDeviceBufferMs / 1000
	if bufFrames <= 0 {
		procWaveOutClose.Call(hwo)
		return nil, false
	}
	bufBytes := bufFrames * format.BytesPerFrame()
	d := &waveOutDevice{
		hwo:       hwo,
		format:    format,
		bufs:      make([][]byte, audioDeviceBuffers),
		hdrs:      make([]waveHdr, audioDeviceBuffers),
		prepared:  make([]bool, audioDeviceBuffers),
		submitted: make([]bool, audioDeviceBuffers),
	}
	for i := range d.bufs {
		d.bufs[i] = make([]byte, bufBytes)
		d.hdrs[i].Data = uintptr(unsafe.Pointer(&d.bufs[i][0]))
	}
	return d, true
}

// write 提交一块 PCM：按缓冲大小切段并逐段等待空闲缓冲。
func (d *waveOutDevice) write(data []byte) error {
	bpf := d.format.BytesPerFrame()
	bufSize := len(d.bufs[0])
	for off := 0; off < len(data); {
		n := len(data) - off
		if n > bufSize {
			n = bufSize
		}
		if r := n % bpf; r != 0 {
			// 段尾对齐到采样帧：半帧数据进设备只会被丢掉或产生毛刺。
			if n-r > 0 {
				n -= r
			} else {
				n = len(data) - off // 尾块本身不足一帧：原样提交（总比丢好）
			}
		}
		if err := d.writeChunk(data[off : off+n]); err != nil {
			return err
		}
		off += n
	}
	return nil
}

// writeChunk 把一段（不超过一个缓冲）交给设备。没有空闲缓冲时轮询等待。
func (d *waveOutDevice) writeChunk(chunk []byte) error {
	for {
		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			return errAudioDeviceClosed
		}
		idx := d.freeBufferLocked()
		if idx < 0 {
			d.mu.Unlock()
			// 4ms 轮询：一个 100ms 缓冲大约每 25ms 播完一块，4ms 足够细（不会
			// 因等待过久而 underrun），又不至于空转烧 CPU。
			time.Sleep(4 * time.Millisecond)
			continue
		}
		buf := d.bufs[idx]
		copy(buf, chunk)
		hdr := &d.hdrs[idx]
		hdr.BufferLength = uint32(len(chunk))
		hdr.Flags &^= whdrDone // 复用缓冲：先清「已播完」标志
		if !d.prepared[idx] {
			if ret, _, _ := procWaveOutPrepareHeader.Call(d.hwo, uintptr(unsafe.Pointer(hdr)), unsafe.Sizeof(*hdr)); int32(ret) != mmNoError {
				d.mu.Unlock()
				return fmt.Errorf("waveOutPrepareHeader 失败：%d", int32(ret))
			}
			d.prepared[idx] = true
		}
		if ret, _, _ := procWaveOutWrite.Call(d.hwo, uintptr(unsafe.Pointer(hdr)), unsafe.Sizeof(*hdr)); int32(ret) != mmNoError {
			d.mu.Unlock()
			return fmt.Errorf("waveOutWrite 失败：%d", int32(ret))
		}
		d.submitted[idx] = true
		d.mu.Unlock()
		return nil
	}
}

// freeBufferLocked 返回一个可复用的缓冲下标（-1 = 全在飞）。
func (d *waveOutDevice) freeBufferLocked() int {
	for i := range d.bufs {
		if !d.submitted[i] {
			return i
		}
		if d.hdrs[i].Flags&whdrDone != 0 {
			d.submitted[i] = false
			d.hdrs[i].Flags &^= whdrDone
			return i
		}
	}
	return -1
}

// playedFrames 返回设备已播放的采样帧数。
//
// ★ 必须是**设备位置**而不是「已写入的帧数」：写入的样本还在缓冲里排队（最多
// 4 × 100ms），拿写入量当时钟会让 currentTime 领先声音 300ms。设备位置是声卡
// 真正播到的位置——这正是「音频为主时钟」要的东西。
//
// ★ 时基要按**驱动实际返回**的类型解释：请求 TIME_SAMPLES 时，驱动可以（实测
// 本机驱动就是）忽略请求、用 TIME_BYTES 回答。只认一种时基会让位置恒为 0。
func (d *waveOutDevice) playedFrames() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.hwo == 0 {
		return 0
	}
	var mmt mmTime
	mmt.Type = timeSamples
	if ret, _, _ := procWaveOutGetPosition.Call(d.hwo, uintptr(unsafe.Pointer(&mmt)), unsafe.Sizeof(mmt)); int32(ret) != mmNoError {
		return 0
	}
	raw := int64(*(*uint32)(unsafe.Pointer(&mmt.rest[0])))
	switch mmt.Type {
	case timeSamples:
		return raw
	case timeBytes:
		bpf := int64(d.format.BytesPerFrame())
		if bpf <= 0 {
			return 0
		}
		return raw / bpf
	case timeMS:
		return raw * int64(d.format.SampleRate) / 1000
	default:
		// 未知时基（TIME_TICKS/SMPTE/MIDI）：宁可报 0 也不要按错误时基解释数字。
		return 0
	}
}

func (d *waveOutDevice) pause() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	procWaveOutPause.Call(d.hwo)
	return nil
}

func (d *waveOutDevice) resume() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	procWaveOutRestart.Call(d.hwo)
	return nil
}

// flush 丢弃排队数据并把设备位置归零（seek 用）。
func (d *waveOutDevice) flush() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	procWaveOutReset.Call(d.hwo) // 阻塞到所有缓冲归还，且位置归零
	for i := range d.submitted {
		d.submitted[i] = false
	}
	return nil
}

func (d *waveOutDevice) close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	procWaveOutReset.Call(d.hwo)
	for i := range d.hdrs {
		if d.prepared[i] {
			procWaveOutUnprepareHeader.Call(d.hwo, uintptr(unsafe.Pointer(&d.hdrs[i])), unsafe.Sizeof(d.hdrs[i]))
			d.prepared[i] = false
		}
	}
	procWaveOutClose.Call(d.hwo)
	d.hwo = 0
	return nil
}
