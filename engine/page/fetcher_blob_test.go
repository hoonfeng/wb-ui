package page

import (
	"fmt"
	"strings"
	"testing"

	"wb-ui/engine/js/jsc"
)

// TestFetchBlobURL：URL.createObjectURL 生成的 blob: URL 由 fetch 直接
// 从内存表供给（浏览器同样不发起网络请求）。
func TestFetchBlobURL(t *testing.T) {
	u := jsc.RegisterBlobURL("https://example.com", "blob-body", "text/plain")
	defer jsc.RevokeBlobURL(u)

	in := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	in.SetupGlobal(log)
	RegisterFetch(in)

	script := fmt.Sprintf(`
		fetch(%q).then(function (resp) {
			console.log(resp.status);
			console.log(resp.ok);
			console.log(resp.url);
		});
	`, u)
	if _, err := in.Run(script); err != nil {
		t.Fatalf("Run: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(log.String()), "\n")
	if len(lines) < 3 {
		t.Fatalf("output = %q, want 3 lines", log.String())
	}
	if lines[0] != "200" {
		t.Errorf("status = %q, want 200", lines[0])
	}
	if lines[1] != "true" {
		t.Errorf("ok = %q, want true", lines[1])
	}
	if lines[2] != u {
		t.Errorf("url = %q, want %q", lines[2], u)
	}
}
