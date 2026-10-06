package rendering

// A4：动图帧推进（宿主注入多帧，引擎按帧时长选帧）。
//
// 用**假时钟**注入时间：动图推进依赖时间，真等会让测试又慢又不稳（而且「等了 250ms
// 是否正好跨帧」会随机器负载抖动）。

import (
	"testing"
	"time"
)

// useFakeAnimatedClock 装一个可控时钟并返回「推进时间」的函数。
func useFakeAnimatedClock(t *testing.T) func(time.Duration) {
	t.Helper()
	var now time.Duration
	SetAnimatedImageClock(func() time.Duration { return now })
	t.Cleanup(func() {
		SetAnimatedImageSource(nil)
		ResetAnimatedImages()
		SetAnimatedImageClock(func() time.Duration { return time.Duration(time.Now().UnixNano()) })
	})
	return func(d time.Duration) { now += d }
}

// TestAnimatedImageAdvancesWithClock：动图按帧时长推进，且循环回第一帧。
// 反向验证：把 frameAt 里的 `elapsed % totalMS` 去掉（或用固定帧）后，第 2/3 步就
// 会拿到同一帧，测试立刻失败。
func TestAnimatedImageAdvancesWithClock(t *testing.T) {
	advance := useFakeAnimatedClock(t)
	green := solidPNG(t, greenFrame, 4, 4)
	blue := solidPNG(t, blueFrame, 4, 4)
	SetAnimatedImageSource(func(url string, data []byte) ([]AnimatedFrame, int, bool) {
		return []AnimatedFrame{
			{Image: NewDecodedImage(green), DurationMS: 100},
			{Image: NewDecodedImage(blue), DurationMS: 100},
		}, -1, true
	})

	first := decodeImageOrAnimated("anim.gif", []byte("gif-bytes"))
	if first == nil {
		t.Fatal("动图应返回当前帧")
	}
	if !IsAnimatedImageURL("anim.gif") {
		t.Error("识别为动图后应登记（IsAnimatedImageURL = true）")
	}

	// 同一时刻多次取值必须稳定（painter 一帧里可能问多次）。
	if again, _ := animatedFrameForURL("anim.gif"); again != first {
		t.Error("同一时刻应返回同一帧")
	}

	advance(150 * time.Millisecond) // 跨到第 1 帧
	second, _ := animatedFrameForURL("anim.gif")
	if second == first {
		t.Error("150ms 后应换到第 1 帧（画面没变 = 动图没动）")
	}

	advance(100 * time.Millisecond) // 200ms = 总时长 → 回到第 0 帧（无限循环）
	third, _ := animatedFrameForURL("anim.gif")
	if third != first {
		t.Error("200ms 后应循环回第 0 帧")
	}
}

// TestAnimatedSourceDeclineFallsBackToSingleFrame：宿主不认这个数据（不是动图）时，
// 引擎必须走原来的单帧解码路径——A1/A2 与所有普通图片的行为不能被 A4 改掉。
func TestAnimatedSourceDeclineFallsBackToSingleFrame(t *testing.T) {
	useFakeAnimatedClock(t)
	SetAnimatedImageSource(func(url string, data []byte) ([]AnimatedFrame, int, bool) {
		return nil, 0, false
	})
	png := solidPNG(t, greenFrame, 4, 4)
	img := decodeImageOrAnimated("still.png", png)
	if img == nil || !img.Loaded() {
		t.Fatalf("宿主不认动图时应回退单帧解码，得到 %v", img)
	}
	if IsAnimatedImageURL("still.png") {
		t.Error("单帧图不应被登记为动图（否则会绕过单帧缓存）")
	}
}

// TestAnimatedLoopsZeroStopsAtLastFrame：容器声明「只播一次」（Skia 的 loops=0）时，
// 超过总时长后停在末帧，而不是循环回开头。
func TestAnimatedLoopsZeroStopsAtLastFrame(t *testing.T) {
	advance := useFakeAnimatedClock(t)
	green := solidPNG(t, greenFrame, 4, 4)
	blue := solidPNG(t, blueFrame, 4, 4)
	SetAnimatedImageSource(func(url string, data []byte) ([]AnimatedFrame, int, bool) {
		return []AnimatedFrame{
			{Image: NewDecodedImage(green), DurationMS: 100},
			{Image: NewDecodedImage(blue), DurationMS: 100},
		}, 0, true
	})
	first := decodeImageOrAnimated("once.gif", []byte("x"))
	advance(100 * time.Millisecond)
	last, _ := animatedFrameForURL("once.gif")
	if last == first {
		t.Fatal("第 2 帧应可见")
	}
	advance(10 * time.Second)
	if got, _ := animatedFrameForURL("once.gif"); got != last {
		t.Error("loops=0 时应停在末帧，不该循环")
	}
}
