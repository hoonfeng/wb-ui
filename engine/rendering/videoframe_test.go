package rendering

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// solidPNG 生成 w×h 纯色 PNG 字节（模拟宿主抽出的帧/解码器输出）。
func solidPNG(t *testing.T, c color.NRGBA, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("生成 PNG 失败: %v", err)
	}
	return buf.Bytes()
}

// solidDataURI 把纯色图包成 data: URI（poster 用它，避免测试依赖文件系统与
// 资源策略）。
func solidDataURI(t *testing.T, c color.NRGBA, w, h int) string {
	t.Helper()
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(solidPNG(t, c, w, h))
}

// paintVideoBox 造一个 100×100 的 <video> 渲染盒并绘制其前景（图片路径）。
func paintVideoBox(t *testing.T, el *dom.Element) *graphics.Canvas {
	t.Helper()
	canvas := graphics.NewCanvas(100, 100)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 100, Height: 100})
	st := style.NewComputedStyle()
	box := NewRenderBox(el, st)
	box.SetLocation(0, 0)
	box.SetSize(100, 100)
	PaintImage(box, info)
	return canvas
}

// newVideoElement 造一个文档内的 <video> 元素（带可选属性）。
func newVideoElement(t *testing.T, attrs map[string]string) *dom.Element {
	t.Helper()
	doc := dom.NewDocument()
	el := doc.CreateElement("video")
	for k, v := range attrs {
		el.SetAttribute(k, v)
	}
	return el
}

var (
	redFrame   = color.NRGBA{R: 255, A: 255}
	greenFrame = color.NRGBA{G: 200, A: 255}
	blueFrame  = color.NRGBA{B: 200, A: 255}
)

// TestVideoPosterWinsWhileFlagSet：show poster flag 置位期间（资源刚加载、还没
// 播放）显示的是 poster 替代画面——此时**不该**向宿主要帧（HTML §4.8.8）。
func TestVideoPosterWinsWhileFlagSet(t *testing.T) {
	calls := 0
	SetVideoFrameSource(func(url string, at float64) ([]byte, bool) {
		calls++
		return solidPNG(t, greenFrame, 40, 40), true
	})
	t.Cleanup(func() { SetVideoFrameSource(nil) })

	el := newVideoElement(t, map[string]string{
		"src":    "clip.mp4",
		"poster": solidDataURI(t, redFrame, 40, 40),
	})
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0, ShowPoster: true})

	canvas := paintVideoBox(t, el)
	defer canvas.Release()
	px := canvas.PixelAt(50, 50)
	if px.A < 200 || px.R < 200 || px.G > 60 {
		t.Fatalf("中心像素 = %+v，want poster 红色（flag 置位期间不显示视频帧）", px)
	}
	if calls != 0 {
		t.Fatalf("flag 置位期间取帧 %d 次，want 0", calls)
	}
}

// TestVideoFramePaintedAfterPosterCleared：play()/seek 之后 flag 清除，画的是
// **帧**（绿）而不是 poster（红）——主线 A1 的核心语义。
func TestVideoFramePaintedAfterPosterCleared(t *testing.T) {
	SetVideoFrameSource(func(url string, at float64) ([]byte, bool) {
		if url != "clip.mp4" || at != 0 {
			t.Errorf("帧源收到 url=%q at=%v，want clip.mp4/0", url, at)
		}
		return solidPNG(t, greenFrame, 40, 40), true
	})
	t.Cleanup(func() { SetVideoFrameSource(nil) })

	el := newVideoElement(t, map[string]string{
		"src":    "clip.mp4",
		"poster": solidDataURI(t, redFrame, 40, 40),
	})
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0, ShowPoster: false})

	canvas := paintVideoBox(t, el)
	defer canvas.Release()
	px := canvas.PixelAt(50, 50)
	if px.A < 200 || px.G < 150 || px.R > 60 {
		t.Fatalf("中心像素 = %+v，want 绿色帧（flag 清除后帧优先于 poster）", px)
	}
}

// TestVideoFrameShownWhenFlagSetWithoutPoster：show poster flag 置位但元素**没有
// poster** 时，poster frame 退化为当前帧（HTML 的 poster frame 定义）——加载完的
// 无 poster <video> 必须画出第一帧，而不是空白。flag ≠「不许画帧」。
func TestVideoFrameShownWhenFlagSetWithoutPoster(t *testing.T) {
	calls := 0
	SetVideoFrameSource(func(url string, at float64) ([]byte, bool) {
		calls++
		return solidPNG(t, greenFrame, 40, 40), true
	})
	t.Cleanup(func() { SetVideoFrameSource(nil) })

	el := newVideoElement(t, map[string]string{"src": "clip.mp4"})
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0, ShowPoster: true})

	canvas := paintVideoBox(t, el)
	defer canvas.Release()
	px := canvas.PixelAt(50, 50)
	if px.A < 200 || px.G < 150 {
		t.Fatalf("中心像素 = %+v，want 绿色帧（无 poster 时 flag 不拦帧）", px)
	}
	if calls != 1 {
		t.Fatalf("取帧 %d 次，want 1", calls)
	}
}

// TestVideoFrameWithoutStateIsNotRequested：没有状态记录（未加载/已切源）时，
// painter 不该向宿主取帧，元素保持空白（无 poster 时不画）。
func TestVideoFrameWithoutStateIsNotRequested(t *testing.T) {
	calls := 0
	SetVideoFrameSource(func(url string, at float64) ([]byte, bool) {
		calls++
		return solidPNG(t, greenFrame, 8, 8), true
	})
	t.Cleanup(func() { SetVideoFrameSource(nil) })

	el := newVideoElement(t, map[string]string{"src": "clip.mp4"})
	canvas := paintVideoBox(t, el)
	defer canvas.Release()
	if calls != 0 {
		t.Fatalf("帧源被调用了 %d 次，want 0（元素没有显示状态）", calls)
	}
	if px := canvas.PixelAt(50, 50); px.A != 0 {
		t.Fatalf("中心像素 = %+v，want 透明（无所画内容）", px)
	}
}

// TestVideoFrameFallsBackToPoster：flag 已清除但宿主给不出帧（解码失败/无
// ffmpeg）时回退 poster，且不会把 src（mp4）当图片去解码。
func TestVideoFrameFallsBackToPoster(t *testing.T) {
	SetVideoFrameSource(func(url string, at float64) ([]byte, bool) { return nil, false })
	t.Cleanup(func() { SetVideoFrameSource(nil) })

	el := newVideoElement(t, map[string]string{
		"src":    "clip.mp4",
		"poster": solidDataURI(t, redFrame, 40, 40),
	})
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0})

	canvas := paintVideoBox(t, el)
	defer canvas.Release()
	px := canvas.PixelAt(50, 50)
	if px.A < 200 || px.R < 200 || px.G > 60 {
		t.Fatalf("中心像素 = %+v，want poster 红色（宿主无帧时的替代画面）", px)
	}
}

// TestVideoFrameNegativeCachedAndTimeChangeRefetches：同一时间点只问宿主一次
// （负缓存避免每次绘制都跑解码器）；时间点变化才重新取帧。
func TestVideoFrameNegativeCachedAndTimeChangeRefetches(t *testing.T) {
	calls := 0
	SetVideoFrameSource(func(url string, at float64) ([]byte, bool) {
		calls++
		if at == 0 {
			return nil, false // 该时刻无帧（负缓存路径）
		}
		return solidPNG(t, blueFrame, 8, 8), true
	})
	t.Cleanup(func() { SetVideoFrameSource(nil) })

	el := newVideoElement(t, map[string]string{"src": "clip.mp4"})
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0})

	for i := 0; i < 3; i++ {
		canvas := paintVideoBox(t, el)
		canvas.Release()
	}
	if calls != 1 {
		t.Fatalf("同一时间点取帧 %d 次，want 1（负缓存失效）", calls)
	}

	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0.5})
	canvas := paintVideoBox(t, el)
	defer canvas.Release()
	if calls != 2 {
		t.Fatalf("时间点变化后取帧共 %d 次，want 2", calls)
	}
	if px := canvas.PixelAt(50, 50); px.A < 200 || px.B < 150 {
		t.Fatalf("中心像素 = %+v，want 蓝色帧（0.5s 处）", px)
	}
}

// TestVideoFrameClearedOnSourceChange：清状态（切源/出错）后回退 poster，且不再
// 向宿主取帧——旧视频的帧不允许留在新资源上。
func TestVideoFrameClearedOnSourceChange(t *testing.T) {
	calls := 0
	SetVideoFrameSource(func(url string, at float64) ([]byte, bool) {
		calls++
		return solidPNG(t, greenFrame, 8, 8), true
	})
	t.Cleanup(func() { SetVideoFrameSource(nil) })

	el := newVideoElement(t, map[string]string{
		"src":    "clip.mp4",
		"poster": solidDataURI(t, redFrame, 40, 40),
	})
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0})
	if st, ok := ElementVideoState(el); !ok || st.URL != "clip.mp4" {
		t.Fatalf("ElementVideoState = (%+v, %v)，want URL=clip.mp4", st, ok)
	}

	SetElementVideoState(el, VideoElementState{})
	if _, ok := ElementVideoState(el); ok {
		t.Fatalf("清除后 ElementVideoState 仍报告状态存在")
	}
	before := calls
	canvas := paintVideoBox(t, el)
	defer canvas.Release()
	if calls != before {
		t.Fatalf("清除后仍取帧 %d 次", calls-before)
	}
	if px := canvas.PixelAt(50, 50); px.R < 200 {
		t.Fatalf("中心像素 = %+v，want poster 红色", px)
	}
}

// TestSetVideoFrameSourceResetsCache：换帧源会丢弃旧帧缓存（旧帧可能来自另一份
// 文件/另一个解码器），下一次绘制必须重新取。
func TestSetVideoFrameSourceResetsCache(t *testing.T) {
	first := 0
	SetVideoFrameSource(func(url string, at float64) ([]byte, bool) {
		first++
		return solidPNG(t, greenFrame, 8, 8), true
	})
	t.Cleanup(func() { SetVideoFrameSource(nil) })

	el := newVideoElement(t, map[string]string{"src": "clip.mp4"})
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0})
	canvas := paintVideoBox(t, el)
	canvas.Release()
	if first != 1 {
		t.Fatalf("首次取帧 %d 次，want 1", first)
	}

	second := 0
	SetVideoFrameSource(func(url string, at float64) ([]byte, bool) {
		second++
		return solidPNG(t, redFrame, 8, 8), true
	})
	// 元素状态也被清掉了：绑定层会重新声明（这里手动补上）。
	if _, ok := ElementVideoState(el); ok {
		t.Fatalf("换帧源应清空元素状态记录")
	}
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0})
	canvas2 := paintVideoBox(t, el)
	defer canvas2.Release()
	if second != 1 {
		t.Fatalf("新帧源取帧 %d 次，want 1（旧缓存未丢弃）", second)
	}
	if px := canvas2.PixelAt(50, 50); px.R < 150 || px.G > 80 {
		t.Fatalf("中心像素 = %+v，want 新帧源的红色", px)
	}
}
