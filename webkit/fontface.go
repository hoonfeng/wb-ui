package webkit

import (
	"strings"

	"wb-ui/engine/page"
	"wb-ui/engine/platform/graphics"
)

// loadFontFace 处理页面里的一条 @font-face：按浏览器顺序尝试每个
// `src: url(...)` 候选，把第一个能被 Skia 解码的字体注册到全局字体管理器。
//
// 去重：同一 family + 同一候选集合在一次会话里只尝试一次。样式表会随 DOM
// 变更反复接入（extractAndAddStyles 先全撤再重扫），重复下载字体既浪费
// 带宽，也会让字体管理器反复重置缓存。
func (wv *WebView) loadFontFace(family string, srcs []string) {
	if wv == nil || wv.destroyed || family == "" || len(srcs) == 0 {
		return
	}
	key := strings.ToLower(strings.TrimSpace(family)) + "\x00" + strings.Join(srcs, "\x00")
	wv.fontFaceMu.Lock()
	if wv.fontFaces == nil {
		wv.fontFaces = make(map[string]bool)
	}
	if wv.fontFaces[key] {
		wv.fontFaceMu.Unlock()
		return
	}
	wv.fontFaces[key] = true
	wv.fontFaceMu.Unlock()

	fm := graphics.GetFontManager()
	if fm == nil {
		page.Logf("FontFace", "skip family=%q: no font manager", family)
		return
	}
	for _, src := range srcs {
		content, err := wv.loadExternalResource(src, PurposeFont)
		if err != nil {
			page.Logf("FontFace", "family=%q src=%q load FAIL: %v", family, src, err)
			continue
		}
		if content == "" {
			page.Logf("FontFace", "family=%q src=%q empty", family, src)
			continue
		}
		if err := fm.RegisterCustomFont(family, []byte(content), 0); err != nil {
			// 最常见的是 woff2（Skia 不内置 woff2 解码器）——继续下一个候选，
			// 与浏览器的 src 回退顺序一致（真实站点会同时给出 woff/ttf）。
			page.Logf("FontFace", "family=%q src=%q decode FAIL: %v", family, src, err)
			continue
		}
		page.Logf("FontFace", "registered family=%q src=%q bytes=%d", family, src, len(content))
		wv.invalidateForFontChange()
		return
	}
	page.Logf("FontFace", "family=%q: no decodable src (tried %d)", family, len(srcs))
}

// invalidateForFontChange 让已渲染内容按新字体重新排版/绘制：字体注册前
// 解析出的 *skia.Font 已被缓存（Canvas 级 + 包级测量缓存），旧缓存会让
// 「字体已注册但页面还是回退字型」；字体度量变化还会影响行宽，所以布局
// 也要置脏。
func (wv *WebView) invalidateForFontChange() {
	if wv.canvas != nil {
		wv.canvas.ResetFontCache()
	}
	mark := func(fr *page.Frame) {
		if fr == nil {
			return
		}
		if rv := fr.RenderView(); rv != nil {
			rv.MarkAllDirty()
		}
		if v := fr.View(); v != nil {
			v.SetNeedsLayout(true)
		}
	}
	if wv.mainFrame != nil {
		mark(wv.mainFrame.Frame())
	}
	for f := range wv.subframeJS {
		mark(f)
	}
}
