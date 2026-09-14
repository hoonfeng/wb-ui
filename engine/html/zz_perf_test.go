package html

import (
	"strings"
	"testing"
	"time"
)

// TestParseLongDataURI 回归：超长属性值（Live2D 纹理 data URI 1MB+）曾被
// 逐字符 `Value += string(r)` 打造成 O(n²) —— html.Parse 永久卡死，导致
// live2d 挂件启动阻塞（程序无窗口）。缓冲构建后解析应为线性（<5s 防线）。
func TestParseLongDataURI(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8"></head><body>`)
	for i := 0; i < 3; i++ {
		b.WriteString(`<img src="data:image/png;base64,`)
		b.WriteString(strings.Repeat("A", 1<<20)) // 每张 1MB
		b.WriteString(`">`)
	}
	b.WriteString(`</body></html>`)

	start := time.Now()
	doc, err := Parse(b.String())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("解析超长属性太慢: %v (O(n²) 回归?)", d)
	}
	_ = doc
}

// TestParseLongText 回归：大文本节点（长歌词/配置 JSON 内联）同样走
// appendToCharacter 缓冲，不应 O(n²)。
func TestParseLongText(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<html><body><div>`)
	b.WriteString(strings.Repeat("a", 4<<20)) // 4MB 纯文本
	b.WriteString(`</div></body></html>`)

	start := time.Now()
	doc, err := Parse(b.String())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("解析长文本太慢: %v (O(n²) 回归?)", d)
	}
	_ = doc
}
