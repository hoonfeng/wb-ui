package graphics

import (
	"testing"
)

func BenchmarkMeasureTextCached(b *testing.B) {
	font := Font{Family: "Microsoft YaHei", Size: 14, Weight: 400}
	texts := []string{
		"发送消息",
		"上次任务未完成，是否继续？",
		"思考中...",
		"Analyzing the request and planning next steps",
		"生成代码",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, t := range texts {
			MeasureText(font, t)
		}
	}
}

func BenchmarkMeasureTextUncached(b *testing.B) {
	font := Font{Family: "Microsoft YaHei", Size: 14, Weight: 400}
	texts := []string{
		"发送消息",
		"上次任务未完成，是否继续？",
		"思考中...",
		"Analyzing the request and planning next steps",
		"生成代码",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, t := range texts {
			measureTextUncached(font, t)
		}
	}
}

func TestMeasureTextCacheConsistent(t *testing.T) {
	font := Font{Family: "Microsoft YaHei", Size: 14, Weight: 400}
	text := "测试文本 abc 123"
	w1 := MeasureText(font, text)
	// Force cache hit path.
	for i := 0; i < 100; i++ {
		w2 := MeasureText(font, text)
		if w1 != w2 {
			t.Fatalf("cache mismatch: %v vs %v", w1, w2)
		}
	}
	if w1 <= 0 {
		t.Fatalf("zero width: %v", w1)
	}
	// Different font must not collide.
	font2 := Font{Family: "Consolas", Size: 16, Weight: 700}
	w3 := MeasureText(font2, text)
	if w1 == w3 {
		t.Fatalf("different fonts produced same cached width")
	}
}
