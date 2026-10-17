package jsc

import (
	"strings"
	"testing"
)

// TestBlobStringPartKeepsBytes 回归：new Blob(["text"]) 的 part 是字符串时
// 必须直接取 UTF-8 字节。修复前走了「String 包装对象有 length/索引」的
// 类数组分支，字符经 ToInteger() 全变成 0，读回一串 NUL（前端矩阵探针
// 的 blob 场景复现）。
func TestBlobStringPartKeepsBytes(t *testing.T) {
	in := NewInterpreter()
	log := &BufferLogger{}
	in.SetupGlobal(log)
	in.RegisterWebAPIs()

	if _, err := in.Run(`
		var b = new Blob(["blob-body"], { type: "text/plain" });
		console.log(b.size);
		console.log(b.type);
		console.log(b.__wbBlobData);
	`); err != nil {
		t.Fatalf("Run: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(log.String()), "\n")
	if len(lines) < 3 {
		t.Fatalf("output = %q, want 3 lines", log.String())
	}
	if lines[0] != "9" {
		t.Errorf("size = %q, want 9", lines[0])
	}
	if lines[1] != "text/plain" {
		t.Errorf("type = %q, want text/plain", lines[1])
	}
	if lines[2] != "blob-body" {
		t.Errorf("data = %q, want blob-body", lines[2])
	}
}
