// SVG 资源来源的单测（U5 / 缺陷 D3）：`file://` 与文档相对路径此前一律
// 返回 nil（只有 `data:image/svg+xml` 能渲染），文件引用的 SVG 图标全不显示。
// 修复后非 data: 引用走宿主 loader（与栅格图同一条链：解析 → 逐 URL 策略
// 门禁 → 取字节），网络引用仍不在渲染线程同步取（会阻塞绘制）。

package rendering

import (
	"errors"
	"strings"
	"testing"
)

const testSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">` +
	`<rect width="10" height="10" fill="#ff0000"/></svg>`

// fakeSVGLoader 是最小的 ImageResourceLoader：只服务内存里登记的几个文件。
type fakeSVGLoader struct {
	base  string
	files map[string]string
	allow func(url string) bool
}

func (f *fakeSVGLoader) ResolveURL(ref string) string {
	if f.base == "" || ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "data:") || strings.HasPrefix(ref, "file://") ||
		strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ""
	}
	return f.base + "/" + ref
}

func (f *fakeSVGLoader) Load(abs string) ([]byte, error) {
	if s, ok := f.files[abs]; ok {
		return []byte(s), nil
	}
	return nil, errors.New("fake loader: not found: " + abs)
}

func (f *fakeSVGLoader) AllowsURL(url string) bool {
	if f.allow != nil {
		return f.allow(url)
	}
	return true
}

func TestLoadBackgroundSVGFromDocumentRelativePath(t *testing.T) {
	const base = "file:///F:/site"
	loader := &fakeSVGLoader{
		base:  base,
		files: map[string]string{base + "/icons/logo.svg": testSVG},
	}
	doc := loadBackgroundSVGWith("icons/logo.svg", loader)
	if doc == nil {
		t.Fatal("文档相对路径的 SVG 应能解析（U5/D3：修复前一律 nil）")
	}
}

func TestLoadBackgroundSVGFromFileURL(t *testing.T) {
	const fileURL = "file:///F:/site/icons/star.svg"
	loader := &fakeSVGLoader{files: map[string]string{fileURL: testSVG}}
	if doc := loadBackgroundSVGWith(fileURL, loader); doc == nil {
		t.Fatal("file:// 形式的 SVG 应能解析（U5/D3）")
	}
}

func TestLoadBackgroundSVGDeniedByPolicy(t *testing.T) {
	const fileURL = "file:///F:/site/icons/denied.svg"
	loader := &fakeSVGLoader{
		files: map[string]string{fileURL: testSVG},
		allow: func(string) bool { return false }, // DenyExternal 语义
	}
	if doc := loadBackgroundSVGWith(fileURL, loader); doc != nil {
		t.Fatal("策略拒绝时 SVG 不得渲染（安全默认不变，TC-M-902）")
	}
}

func TestLoadBackgroundSVGSkipsNetworkOnRenderThread(t *testing.T) {
	loader := &fakeSVGLoader{files: map[string]string{}}
	for _, u := range []string{"http://example.com/a.svg", "https://example.com/a.svg", "//example.com/a.svg"} {
		if doc := loadBackgroundSVGWith(u, loader); doc != nil {
			t.Fatalf("%s：网络 SVG 不应在渲染线程同步取（会阻塞绘制）", u)
		}
	}
}

func TestLoadBackgroundSVGWithoutLoaderKeepsLocalRead(t *testing.T) {
	// 无 loader（独立渲染/探针）：保留「本地路径直接读」的既有行为——
	// 路径不存在时返回 nil（不 panic、不猜测）。
	if doc := loadBackgroundSVGWith("F:/definitely/not/here.svg", nil); doc != nil {
		t.Fatal("不存在的本地路径应返回 nil")
	}
}
