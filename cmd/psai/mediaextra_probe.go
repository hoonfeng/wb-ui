package main

// 补充判据（G3/G4/G6/G7 的「未单独采证」用例，2026-10）。
//
// 起因：文档 §5 里有 11 个用例在矩阵页里只有样本/预期、**没有独立判据**（G3 五项、
// G4 两项、G6 一项、G7 三项），因此既不能算通过也不能算失败——验证清单上的空洞。
// 本文件给它们补上**可复跑、可复核**的判据，并把证据落盘（JSON + 截图 + Markdown）。
//
// 两条采集路线，按判据性质选择：
//
//	① 渲染像素（引擎 `wv.Render()` / Edge `--screenshot`）：用于「页面**画出来**长什么
//	   样」的判据——`image-rendering: pixelated` 的最近邻缩放（TC-M-303）、`object-fit`
//	   四值的裁剪/letterbox（TC-M-702）。（canvas 采样在这里不适用：它反映的是脚本
//	   自己画的内容，不是 CSS 渲染的结果。）
//	② 页面内 canvas 采样（`drawImage` 取元素**当前帧**）：用于「动图在时间轴上怎么
//	   推进」的判据——`<canvas>` 取当前帧（TC-M-304）、逐帧 delay 对齐（TC-M-305）、
//	   `visibility:hidden` 期间的推进（TC-M-306）、loop 末帧行为（TC-M-307）。
//	   ★ 用 canvas 而不是宿主侧连拍，是因为它把「时间分辨率」交给页面（每次 tick 一次
//	   drawImage），不受宿主渲染耗时限制；Edge 侧也能用同一条脚本（`--allow-file-...`
//	   打开 file:// 的 canvas 读权限）。
//
// 引擎侧由 Go 逐 tick 驱动（`__tsTick(t)`），避免依赖 setInterval 在无头模型里的行为；
// Edge 侧由页面自己在 load+1500ms 后把 `__extraState()` 写进 `<pre id="__extra_probe">`，
// Go 用 `--dump-dom` 取回（与矩阵页 Edge 采集同一套路）。
import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"wb-ui/app"
	"wb-ui/webkit"
)

// extraCase 是一项补充判据的结果（证据 + 判定）。
type extraCase struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Expect  string   `json:"expect"`
	Actual  string   `json:"actual"`
	Verdict string   `json:"verdict"` // ✅ / ❌ / ⚠️ / SKIP
	Note    string   `json:"note,omitempty"`
	Samples []string `json:"samples,omitempty"`
}

// extraCaseDef 是页面场景与判据的静态定义（顺序即报告顺序）。
type extraCaseDef struct {
	ID     string
	Title  string
	Expect string
}

func extraCaseDefs() []extraCaseDef {
	return []extraCaseDef{
		// ★ 期望栏与判据口径一致：pixelated 的达标线是「采样色数**显著少于** auto 对照」
		// 且「自身不出现整幅插值」（源色 4；残留色仅来自色块交界的混合像素——放大后
		// 相邻色块之间有 1px 过渡，属几何混色而非双线性插值，Edge 同样如此）。
		// 旧文案写「色数 = 源色数 4」会把实际判据（8 vs 21 判 ✅）读成「达标线被放宽」。
		{"TC-M-303", "image-rendering: pixelated 的最近邻缩放", "pixelated 采样色数显著少于 auto 对照（源色 4；残留色仅来自色块交界混合像素），自身不走整幅插值"},
		{"TC-M-304", "<canvas> drawImage 取动图当前帧", "连续采样出现 ≥2 种帧色（取到的是当前帧、不是首帧常量）"},
		{"TC-M-305", "逐帧 delay 对齐（帧延迟 [50,200,100]ms，周期 350ms）", "三段帧色时长之比 ≈ 50:200:100，周期 ≈350ms"},
		{"TC-M-306", "visibility:hidden 后动图是否继续推进", "隐藏 300ms 期间帧色仍变化（规范：不因不可见而暂停）"},
		{"TC-M-307a", "不循环 GIF（anim-noloop）末帧行为", "播过末尾后停在末帧（尾部样本颜色恒定）"},
		{"TC-M-307b", "循环 GIF（loop=0）末帧行为", "播过末尾后回到首帧继续循环（尾部样本仍出现 ≥2 色）"},
		{"TC-M-404", "扩展名与内容不符（.png 实为 JPEG）按内容解码", "complete=true 且 naturalWidth>0，且渲染出图"},
		{"TC-M-405", "图片路径不存在：不渲染 + 无控制台刷屏", "naturalWidth=0；页面可见控制台条目 0"},
		{"TC-M-506", "<video controls> 是否绘制原生控件", "tabIndex=0（契约）；视觉差异如实记录（无控件绘制路径则不列为缺陷）"},
		{"TC-M-702", "object-fit 四值（contain/cover/fill/none）", "四者几何可区分：contain 有 letterbox、fill 非等比、none 为原始尺寸、cover 等比填满"},
		{"TC-M-704", "中文文件名 + 中文目录", "能加载（complete=true、naturalWidth>0、渲染出图）"},
		{"TC-M-705a", "文件名含空格", "能加载（complete=true、naturalWidth>0）"},
		{"TC-M-705b", "路径含 .. 穿越", "Browser(file://) 可解析；DenyExternal 策略下被 resolver 拒绝（两者都如实记录）"},
	}
}

const extraCellPx = 160

// ── 内联素材（避免依赖外部样本的尺寸/编码细节，也让判据的期望值可精确算） ──

// extraGridURI 生成 object-fit 判据用的棋盘图：320x240、8x6 格、格 40x40、两色交替。
//
// 为什么是这个尺寸：它**大于**判据用的 160x160 盒，从而让四个 object-fit 取值各自
// 产生可区分的几何——contain 缩到 160x120（上下 letterbox）、cover 等比放 1/3 后
// 水平裁剪（可见 6 格）、none 保持原始 40px 格（可见中心 4x4 格）、fill 非等比拉伸
// （水平周期 20px ≠ 垂直周期 26.7px）。若图比盒小，contain 与 none 将无法区分。
func extraGridURI() string {
	img := image.NewRGBA(image.Rect(0, 0, 320, 240))
	dark := color.RGBA{40, 40, 40, 255}
	light := color.RGBA{220, 210, 60, 255}
	for y := 0; y < 240; y++ {
		for x := 0; x < 320; x++ {
			c := dark
			if ((x/40)+(y/40))%2 == 1 {
				c = light
			}
			img.SetRGBA(x, y, c)
		}
	}
	return pngDataURI(img)
}

// extraBlockURI 生成 pixelated 判据用的 4x4 图（每行一色，共 4 色）。
// 放到 160x160（40 倍）后：最近邻只会有这 4 色；任何双线性/双三次插值都会引入
// 调色中间的过渡色（色数 > 4）。
func extraBlockURI() string {
	cols := []color.RGBA{{255, 0, 0, 255}, {0, 200, 0, 255}, {0, 60, 255, 255}, {255, 220, 0, 255}}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetRGBA(x, y, cols[y])
		}
	}
	return pngDataURI(img)
}

func pngDataURI(img image.Image) string {
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// ensureExtraSamples 物化 TC-M-704/705a 需要的样本（中文目录/中文名、含空格名）。
// 由探针自动创建而不是要求手工准备：这两项的判据就是「路径能被解析」，样本必须可复现。
func ensureExtraSamples(samplesDir string) error {
	src := filepath.Join(samplesDir, "quad.png")
	b, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("补充判据样本源不可读（%s）：%w", src, err)
	}
	dir := filepath.Join(samplesDir, "中文目录")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "中文样本.png"), b, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(samplesDir, "with space.png"), b, 0o644)
}

// buildExtraPage 生成补充判据页（所有场景 + 引擎侧逐 tick 驱动脚本 + Edge 侧自采集）。
func buildExtraPage(samplesDir string) (string, error) {
	if err := ensureExtraSamples(samplesDir); err != nil {
		return "", err
	}
	block, grid := extraBlockURI(), extraGridURI()
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html><head><meta charset=\"utf-8\">\n<style>\n")
	b.WriteString("body{margin:0;background:#101010;font:10px monospace;color:#ddd}\n")
	// ★ 场景背景取 magenta：它必须与棋盘图的两种颜色（深蓝/黄）都明显不同——否则
	//   「letterbox 区域」（TC-M-702）与「图本身的深色格」会被同一套采样判据混淆。
	fmt.Fprintf(&b, ".sc{position:relative;display:inline-block;width:%dpx;height:%dpx;background:#ff00ff;"+
		"margin:4px;vertical-align:top;overflow:hidden}\n", extraCellPx, extraCellPx)
	b.WriteString("img,video,canvas{display:block}\n")
	b.WriteString(".tag{position:absolute;left:2px;top:2px;z-index:9;color:#ff0;background:rgba(0,0,0,.55);font-size:9px}\n")
	b.WriteString("</style></head><body>\n")
	b.WriteString("<script>\n")
	b.WriteString("window.__extraCons=[];window.__ts={};\n")
	b.WriteString("(function(){var mk=function(k){return function(){try{window.__extraCons.push(k+':'+Array.prototype.slice.call(arguments).join(' '));}catch(e){}};};")
	b.WriteString("console.error=mk('error');console.warn=mk('warn');console.log=mk('log');")
	b.WriteString("window.addEventListener('error',function(e){window.__extraCons.push('onerror:'+(e&&e.message?e.message:''));});})();\n")
	// 共享的 1x1 采样 canvas（drawImage 取元素当前帧）。
	b.WriteString("</script>\n")
	b.WriteString("<canvas id=\"__pxcanvas\" width=\"1\" height=\"1\" style=\"position:fixed;left:-4px;top:-4px\"></canvas>\n")
	b.WriteString("<pre id=\"__extra_probe\" style=\"display:none\"></pre>\n")

	sc := func(id, label, inner string) {
		fmt.Fprintf(&b, "<div class=\"sc\" data-sc=%s><div class=\"tag\">%s</div>%s</div>\n",
			htmlAttrValue(id), htmlEscape(label), inner)
	}
	fixed := fmt.Sprintf("width:%dpx;height:%dpx", extraCellPx, extraCellPx)
	sc("e303a", "303a pixelated", fmt.Sprintf("<img id=e303a src=%s style=\"%s;image-rendering:pixelated\">",
		htmlAttrValue(block), fixed))
	sc("e303b", "303b auto", fmt.Sprintf("<img id=e303b src=%s style=\"%s;image-rendering:auto\">",
		htmlAttrValue(block), fixed))
	sc("e304", "304 canvas drawImage",
		fmt.Sprintf("<img id=e304 src=%s style=\"%s\">", htmlAttrValue("anim-3frames-rgb.gif"), fixed))
	sc("e305", "305 uneven delay",
		fmt.Sprintf("<img id=e305 src=%s style=\"%s\">", htmlAttrValue("anim-uneven-delay.gif"), fixed))
	sc("e306", "306 hidden advance",
		fmt.Sprintf("<img id=e306 src=%s style=\"%s\">", htmlAttrValue("anim-3frames-rgb.gif"), fixed))
	sc("e307a", "307a noloop",
		fmt.Sprintf("<img id=e307a src=%s style=\"%s\">", htmlAttrValue("anim-noloop.gif"), fixed))
	sc("e307b", "307b loop",
		fmt.Sprintf("<img id=e307b src=%s style=\"%s\">", htmlAttrValue("anim-3frames-rgb.gif"), fixed))
	sc("e404", "404 mislabeled",
		fmt.Sprintf("<img id=e404 src=%s style=\"%s\">", htmlAttrValue("mislabeled.png"), fixed))
	sc("e405", "405 missing path",
		fmt.Sprintf("<img id=e405 src=%s style=\"%s\">", htmlAttrValue("no-such-file-xyz.png"), fixed))
	sc("e506a", "506a no controls",
		fmt.Sprintf("<video id=e506a src=%s style=\"width:%dpx;height:120px\"></video>", htmlAttrValue("solid-red-1s.mp4"), extraCellPx))
	sc("e506b", "506b controls",
		fmt.Sprintf("<video id=e506b src=%s controls style=\"width:%dpx;height:120px\"></video>", htmlAttrValue("solid-red-1s.mp4"), extraCellPx))
	for _, spec := range []struct{ id, fit string }{
		{"e702c", "contain"}, {"e702v", "cover"}, {"e702f", "fill"}, {"e702n", "none"},
	} {
		sc(spec.id, "702 "+spec.fit, fmt.Sprintf("<img id=%s src=%s style=\"%s;object-fit:%s\">",
			htmlAttrValue(spec.id), htmlAttrValue(grid), fixed, spec.fit))
	}
	sc("e704", "704 中文路径",
		fmt.Sprintf("<img id=e704 src=%s style=\"%s\">", htmlAttrValue("中文目录/中文样本.png"), fixed))
	sc("e705a", "705a 空格名",
		fmt.Sprintf("<img id=e705a src=%s style=\"%s\">", htmlAttrValue("with space.png"), fixed))
	sc("e705b", "705b .. 穿越",
		fmt.Sprintf("<img id=e705b src=%s style=\"%s\">", htmlAttrValue("../samples/quad.png"), fixed))

	// 引擎侧逐 tick 驱动 + 静态状态采集 + Edge 侧自采集（同一套读法）。
	b.WriteString(`<script>
// 源的**内容尺寸**：drawImage 的源矩形按图像像素计（img 用 naturalWidth/naturalHeight，
// 不是 CSS 尺寸）；缺自然尺寸时回退 CSS 尺寸。
function __pxSize(el) {
  var w = el.naturalWidth || el.videoWidth || el.width || 0;
  var h = el.naturalHeight || el.videoHeight || el.height || 0;
  return [w, h];
}
// 采样点取**内容中心**（自适应元素尺寸）。
function __pxCenter(el) {
  var d = __pxSize(el);
  return [d[0] > 0 ? Math.floor(d[0] / 2) : 0, d[1] > 0 ? Math.floor(d[1] / 2) : 0];
}
// 越界必须显式标记：源矩形与图像无交集时 canvas 规范是「什么都不画」，得到透明黑
// ——与「drawImage 取到恒定首帧」在像素上无法区分。此前采样点固定 (80,80)，而这些
// 样本 GIF 是 120x80（y 有效 0..79）⇒ 源矩形空集 ⇒ 整条序列恒定透明黑，被误读成
// 「canvas drawImage 取到的是固化帧」。
function __px(el, sx, sy) {
  var d = __pxSize(el);
  if (d[0] > 0 && d[1] > 0 && (sx < 0 || sy < 0 || sx >= d[0] || sy >= d[1])) {
    return 'OOB';
  }
  var c = document.getElementById('__pxcanvas');
  var ctx = c.getContext('2d');
  ctx.clearRect(0, 0, 1, 1);
  ctx.drawImage(el, Math.floor(sx), Math.floor(sy), 1, 1, 0, 0, 1, 1);
  var g = ctx.getImageData(0, 0, 1, 1).data;
  return g[0] + ',' + g[1] + ',' + g[2];
}
window.__tsTick = function(t) {
  var keys = ['e304', 'e305', 'e306', 'e307a', 'e307b'];
  for (var i = 0; i < keys.length; i++) {
    var id = keys[i], el = document.getElementById(id);
    if (!el) { continue; }
    var arr = window.__ts[id] || (window.__ts[id] = []);
    var p = __pxCenter(el);
    try { arr.push({t: t, c: __px(el, p[0], p[1])}); }
    catch (e) { arr.push({t: t, c: 'ERR'}); }
  }
};
window.__setVis = function(id, v) {
  var e = document.getElementById(id);
  if (e) { e.style.visibility = v; }
};
window.__extraState = function() {
  var out = {cons: window.__extraCons, cells: {}};
  // ★ 动图场景（e304/e305/e306/e307a/e307b）必须在这里：渲染像素时间序列按它们的
  //   矩形取中心点采样；漏掉会让整条时间序列为空（判据全部误报未采到）。
  var ids = ['e303a','e303b','e304','e305','e306','e307a','e307b',
             'e404','e405','e506a','e506b','e704','e705a','e705b',
             'e702c','e702v','e702f','e702n'];
  for (var i = 0; i < ids.length; i++) {
    var id = ids[i], e = document.getElementById(id);
    if (!e) { out.cells[id] = {missing: true}; continue; }
    var r = e.getBoundingClientRect();
    out.cells[id] = {tag: (e.tagName || '').toLowerCase(), x: r.left, y: r.top, w: r.width, h: r.height,
      complete: (e.complete === undefined ? 'n/a' : e.complete),
      nw: (e.naturalWidth || 0), nh: (e.naturalHeight || 0),
      tabIndex: (e.tabIndex === undefined ? null : e.tabIndex),
      controls: (e.controls === undefined ? null : !!e.controls),
      err: (e.error ? e.error.code : 0)};
  }
  return JSON.stringify(out);
};
window.__tsJSON = function() { return JSON.stringify(window.__ts); };
// Edge 侧自采集：无头模式没有 Go 驱动，页面自己在 load+1500ms 把状态写进
// <pre id="__extra_probe">，Go 用 --dump-dom 取回（与矩阵页 Edge 采集同一套路）。
(function(){
  var collect = function(){ var pre = document.getElementById('__extra_probe');
    if (pre) { pre.textContent = encodeURIComponent(window.__extraState()); } };
  var go = function(){ setTimeout(collect, 1500); };
  if (document.readyState === 'complete') { go(); } else { window.addEventListener('load', go); }
})();
</script>
`)
	b.WriteString("</body></html>\n")
	return b.String(), nil
}

// ── 采样工具 ─────────────────────────────────────────────────────────

// extraRect 是场景盒在渲染像素里的矩形。
type extraRect struct {
	X, Y, W, H float64
}

// extraGrid 在矩形内取 nx×ny 个网格点的颜色（"r,g,b" 字符串）。
func extraGrid(px []byte, vw int, r extraRect, nx, ny int) []string {
	var out []string
	for j := 0; j < ny; j++ {
		for i := 0; i < nx; i++ {
			x := int(r.X + (float64(i)+0.5)*r.W/float64(nx))
			y := int(r.Y + (float64(j)+0.5)*r.H/float64(ny))
			c := pixelAt(px, vw, x, y)
			out = append(out, fmt.Sprintf("%d,%d,%d", c[0], c[1], c[2]))
		}
	}
	return out
}

// extraScanline 沿一条线取 n 个点的颜色。
func extraScanline(px []byte, vw int, x0, y0, x1, y1 float64, n int) []string {
	var out []string
	for i := 0; i < n; i++ {
		f := float64(i) / float64(n-1)
		x := int(x0 + (x1-x0)*f)
		y := int(y0 + (y1-y0)*f)
		c := pixelAt(px, vw, x, y)
		out = append(out, fmt.Sprintf("%d,%d,%d", c[0], c[1], c[2]))
	}
	return out
}

// extraDistinct 统计不同颜色个数。
func extraDistinct(cols []string) int {
	seen := map[string]bool{}
	for _, c := range cols {
		seen[c] = true
	}
	return len(seen)
}

// extraRunLen 把颜色序列压成「段长」序列（连续同色合并），返回每段长度。
func extraRunLen(cols []string) []int {
	if len(cols) == 0 {
		return nil
	}
	var runs []int
	cur, n := cols[0], 1
	for _, c := range cols[1:] {
		if c == cur {
			n++
			continue
		}
		runs = append(runs, n)
		cur, n = c, 1
	}
	return append(runs, n)
}

// extraMedianRun 返回段长的中位数（用作「一个周期/格」的稳健估计）。
func extraMedianRun(runs []int) float64 {
	if len(runs) == 0 {
		return 0
	}
	s := append([]int(nil), runs...)
	for i := range s {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
	return float64(s[len(s)/2])
}

// extraNear 是相对容差比较（a 与 b 的差在 tol 比例内）。
func extraNear(a, b, tol float64) bool {
	if b == 0 {
		return a == 0
	}
	return math.Abs(a-b)/b <= tol
}

// ── 采集参数 ─────────────────────────────────────────────────────────

// 25ms 步长是为了分辨 TC-M-305 的最短帧（50ms）；60 次共 ~1.5s，覆盖 anim-noloop
// 的末尾停顿与 350ms 周期的一个完整轮回。306 的隐藏窗口取 300ms（= 一帧时长），
// 这样「隐藏期间是否推进」有肉眼可判的期望差（应推进 1 帧）。
const (
	extraTickStep = 25 * time.Millisecond
	extraTicks    = 60
	extraHideAtMs = 400
	extraShowAtMs = 700
)

type extraCellState struct {
	Tag      string   `json:"tag"`
	Missing  bool     `json:"missing"`
	X        float64  `json:"x"`
	Y        float64  `json:"y"`
	W        float64  `json:"w"`
	H        float64  `json:"h"`
	Complete any      `json:"complete"`
	NW       float64  `json:"nw"`
	NH       float64  `json:"nh"`
	TabIndex *float64 `json:"tabIndex"`
	Controls *bool    `json:"controls"`
	Err      float64  `json:"err"`
}

type extraState struct {
	Cons  []string                  `json:"cons"`
	Cells map[string]extraCellState `json:"cells"`
}

type extraTsSample struct {
	T float64 `json:"t"`
	C string  `json:"c"`
}

// ── 引擎侧采集 ────────────────────────────────────────────────────────

// runExtraProbe 用引擎加载补充判据页，逐 tick 驱动时间序列采样，再采静态状态与渲染像素。
func runExtraProbe(cfg mediaConfig, pagePath string, outDir string, vw, pageH int,
	timeSeries bool) ([]extraCase, error) {
	html, err := os.ReadFile(pagePath)
	if err != nil {
		return nil, err
	}
	wv := webkit.NewWebViewWithMode(cfg.Mode)
	defer wv.Destroy()
	if cfg.Policy != nil {
		wv.SetResourcePolicy(*cfg.Policy)
	}
	wv.Resize(vw, pageH)
	app.InstallMediaMetadataResolver(wv, "")
	app.InstallAnimatedImageSource()
	app.InstallMediaAudio(wv, "")
	app.InstallWebAudio("")
	app.InstallImageTranscoder("")
	if err := wv.LoadHTMLWithBaseURL(string(html), fileURLOf(pagePath)); err != nil {
		return nil, err
	}
	settleReal(wv, 1200*time.Millisecond)

	var ts, tsCanvas map[string][]extraTsSample
	if timeSeries {
		// ★ 时间序列走**渲染像素**而不是只靠页面内的 canvas drawImage：TC-M-305/306/307
		//   验的是「页面上的动图怎么推进」，必须看真实绘制结果。页面内 canvas 采样另存
		//   一份（tsCanvas），**只**服务 TC-M-304——那一项的用例本意正是「canvas 取到的
		//   是不是当前帧」（实测：引擎的 canvas drawImage 取到的是固化帧，而渲染像素显示
		//   动图在推进 ⇒ 两条数据缺一不可，否则会把 canvas 的限制误读成「动图不推进」）。
		// ★ 时间戳用真实墙钟：renderPixels 的耗时不受控，按名义步长算会把 350ms 周期算歪。
		st0Val, err := wv.EvalJS("window.__extraState()")
		if err != nil {
			return nil, err
		}
		var st0 extraState
		_ = json.Unmarshal([]byte(st0Val.ToString()), &st0)
		anchors := []string{"e304", "e305", "e306", "e307a", "e307b"}
		start := time.Now()
		hidden, shown := false, false
		for i := 0; i < extraTicks; i++ {
			ms := time.Since(start).Milliseconds()
			if !hidden && ms >= extraHideAtMs {
				_, _ = wv.EvalJS("window.__setVis('e306','hidden')")
				hidden = true
			}
			if !shown && ms >= extraShowAtMs {
				_, _ = wv.EvalJS("window.__setVis('e306','visible')")
				shown = true
			}
			_, _ = wv.EvalJS(fmt.Sprintf("window.__tsTick(%d)", ms))
			pxTick, perr := renderPixels(wv)
			if perr != nil {
				return nil, fmt.Errorf("时间序列采样失败（tick %d）：%w", i, perr)
			}
			for id, c := range extraSampleAnchors(pxTick, vw, st0, anchors) {
				ts = extraAppendSample(ts, id, ms, c)
			}
			settleReal(wv, extraTickStep)
		}
		tsVal, err := wv.EvalJS("window.__tsJSON()")
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(tsVal.ToString()), &tsCanvas); err != nil {
			return nil, fmt.Errorf("解析时间序列失败：%w", err)
		}
	}

	stVal, err := wv.EvalJS("window.__extraState()")
	if err != nil {
		return nil, err
	}
	var st extraState
	if err := json.Unmarshal([]byte(stVal.ToString()), &st); err != nil {
		return nil, fmt.Errorf("解析补充判据状态失败：%w", err)
	}
	px, err := renderPixels(wv)
	if err != nil {
		return nil, err
	}
	if err := extraWritePNG(filepath.Join(outDir, "extra-"+cfg.Name+".png"), px, vw, pageH); err != nil {
		return nil, err
	}
	// 证据留档：页面原始 state 与两条时间序列一起落盘（判据有争议时可直接复核，不必重跑）。
	raw := fmt.Sprintf("{\"state\":%s,\"tsRender\":%s,\"tsCanvas\":%s}",
		stVal.ToString(), extraMustJSON(ts), extraMustJSON(tsCanvas))
	_ = os.WriteFile(filepath.Join(outDir, "extra-raw-"+cfg.Name+".json"), []byte(raw), 0o644)
	return judgeExtra(px, vw, st, ts, tsCanvas, cfg.Name, timeSeries), nil
}

// ── Edge 侧采集 ───────────────────────────────────────────────────────

// runExtraEdge 用 Edge（无头）加载同一页面：DOM 状态走 `--dump-dom` 取回，渲染像素走
// `--screenshot`。时间序列项在 Edge 侧无从复现（无头一次性截图、虚拟时间下动图帧推进
// 不可靠），如实标 SKIP(edge-time-series)——不伪装成通过，也不阻塞对照。
func runExtraEdge(pagePath, outDir string, vw, pageH int) ([]extraCase, string) {
	exe := findEdge()
	if exe == "" {
		return nil, "SKIP(no-edge)"
	}
	pageURL := fileURLOf(pagePath)
	shot := filepath.Join(outDir, "extra-edge.png")
	cmd := exec.Command(exe, "--headless=new", "--disable-gpu", "--hide-scrollbars",
		"--allow-file-access-from-files", "--virtual-time-budget=5000",
		fmt.Sprintf("--window-size=%d,%d", vw, pageH), "--screenshot="+shot, pageURL)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Sprintf("SKIP(edge-failed: %v / %.160s)", err, strings.TrimSpace(string(out)))
	}
	dom, err := edgeDumpDOM(exe, pageURL, vw, pageH)
	if err != nil {
		return nil, fmt.Sprintf("SKIP(edge-dom-failed: %v)", err)
	}
	st, err := parseExtraProbe(dom)
	if err != nil {
		return nil, fmt.Sprintf("SKIP(edge-probe-missing: %v)", err)
	}
	px, pw, _, err := pngPixelsPremul(shot)
	if err != nil {
		return nil, fmt.Sprintf("SKIP(edge-png-failed: %v)", err)
	}
	if pw != vw {
		return nil, fmt.Sprintf("SKIP(edge-width-mismatch: 截图 %dpx ≠ 视口 %dpx)", pw, vw)
	}
	return judgeExtra(px, vw, st, nil, nil, "Edge", false), "对照完成（时间序列项在 Edge 侧 SKIP）"
}

// parseExtraProbe 从 --dump-dom 的输出里取回页面写进 <pre id="__extra_probe"> 的 JSON。
func parseExtraProbe(dom string) (extraState, error) {
	var st extraState
	const marker = `id="__extra_probe"`
	i := strings.Index(dom, marker)
	if i < 0 {
		return st, fmt.Errorf("DOM 里没有 __extra_probe（页面采集脚本未执行？）")
	}
	open := strings.Index(dom[i:], ">")
	if open < 0 {
		return st, fmt.Errorf("__extra_probe 标签不完整")
	}
	rest := dom[i+open+1:]
	end := strings.Index(rest, "</pre>")
	if end < 0 {
		return st, fmt.Errorf("__extra_probe 未闭合")
	}
	raw := strings.TrimSpace(rest[:end])
	dec, err := url.QueryUnescape(raw)
	if err != nil {
		return st, fmt.Errorf("__extra_probe 解码失败：%w", err)
	}
	if err := json.Unmarshal([]byte(dec), &st); err != nil {
		return st, fmt.Errorf("__extra_probe JSON 解析失败：%w（前 120 字符：%.120s）", err, dec)
	}
	return st, nil
}

// ── 判定 ─────────────────────────────────────────────────────────────

type extraSeg struct {
	Color string
	DurMs float64
	AtMs  float64
}

func extraDefByID(id string) extraCaseDef {
	for _, d := range extraCaseDefs() {
		if d.ID == id {
			return d
		}
	}
	return extraCaseDef{ID: id}
}

func extraParseColor(s string) [3]int {
	var r, g, b int
	_, _ = fmt.Sscanf(s, "%d,%d,%d", &r, &g, &b)
	return [3]int{r, g, b}
}

// extraSegments 把 (t, 色) 序列压成「色段」：每段记录颜色与持续时长（ms）。
func extraSegments(samples []extraTsSample) []extraSeg {
	if len(samples) == 0 {
		return nil
	}
	var out []extraSeg
	cur := samples[0]
	for i := 1; i < len(samples); i++ {
		if samples[i].C != cur.C {
			out = append(out, extraSeg{Color: cur.C, DurMs: samples[i].T - cur.T, AtMs: cur.T})
			cur = samples[i]
		}
	}
	last := samples[len(samples)-1].T - cur.T + float64(extraTickStep.Milliseconds())
	return append(out, extraSeg{Color: cur.C, DurMs: last, AtMs: cur.T})
}

// extraTailColors 取序列尾部 frac 比例的颜色序列（TC-M-307 的「停住/循环」判据用）。
func extraTailColors(samples []extraTsSample, frac float64) []string {
	if len(samples) == 0 {
		return nil
	}
	from := int(float64(len(samples)) * (1 - frac))
	var out []string
	for _, s := range samples[from:] {
		out = append(out, s.C)
	}
	return out
}

// extraWindowColors 取时间窗 [lo, hi) 内的颜色序列（TC-M-306 的隐藏窗口用）。
func extraWindowColors(samples []extraTsSample, lo, hi float64) []string {
	var out []string
	for _, s := range samples {
		if s.T >= lo && s.T < hi {
			out = append(out, s.C)
		}
	}
	return out
}

// judgeExtra 是两侧**共用**的判定内核：同一套采样点、同一套阈值。
func judgeExtra(px []byte, vw int, st extraState, ts, tsCanvas map[string][]extraTsSample,
	label string, timeSeries bool) []extraCase {
	rectOf := func(id string) (extraRect, bool) {
		c, ok := st.Cells[id]
		if !ok || c.Missing || c.W <= 0 || c.H <= 0 {
			return extraRect{}, false
		}
		return extraRect{X: c.X, Y: c.Y, W: c.W, H: c.H}, true
	}
	isBG := func(s string) bool { // 场景背景是 magenta（见 buildExtraPage 的注）
		c := extraParseColor(s)
		return c[0] > 180 && c[1] < 80 && c[2] > 180
	}
	contentPoints := func(id string) string {
		r, ok := rectOf(id)
		if !ok {
			return "场景缺失"
		}
		n := 0
		for _, s := range extraGrid(px, vw, r, 5, 5) {
			if !isBG(s) {
				n++
			}
		}
		return fmt.Sprintf("非背景网格点 %d/25", n)
	}
	loadedOf := func(id string) (bool, string) {
		c, ok := st.Cells[id]
		if !ok || c.Missing {
			return false, "场景缺失"
		}
		b, _ := c.Complete.(bool)
		return b && c.NW > 0, fmt.Sprintf("complete=%v naturalWidth=%.0f", c.Complete, c.NW)
	}
	var out []extraCase
	add := func(id, actual, verdict, note string, samples ...string) {
		d := extraDefByID(id)
		out = append(out, extraCase{ID: id, Title: d.Title, Expect: d.Expect,
			Actual: actual, Verdict: verdict, Note: note, Samples: samples})
	}
	tsSkip := func(id string) {
		if label == "Edge" {
			add(id, "—", "SKIP", "SKIP(edge-time-series)：Edge 无头一次性截图/虚拟时间下动图帧推进不可靠，未采（其余项已对照）")
			return
		}
		add(id, "—", "SKIP", "SKIP(未采)：该配置本轮不采时间序列")
	}
	head := func(s []string, n int) []string {
		if len(s) > n {
			return s[:n]
		}
		return s
	}
	// validColors 剔除越界（OOB）与异常（ERR）样本色。
	// ★ 为什么必须剔除：源矩形与图像无交集时，canvas 规范是「什么都不画」，得到的是
	//   透明黑——与「drawImage 取到恒定首帧」在像素上完全无法区分。不剔除就会把
	//   采样点越界读成「canvas 取到固化帧」（TC-M-304 的初版结论即由此而来）。
	validColors := func(cols []string) []string {
		out := make([]string, 0, len(cols))
		for _, c := range cols {
			if c == "OOB" || c == "ERR" {
				continue
			}
			out = append(out, c)
		}
		return out
	}

	// TC-M-303：最近邻缩放（渲染像素的色数）
	// ★ 采样矩形**内缩 3px**：元素外沿 1px 与背景抗锯齿混合会产出中间色，不内缩则
	//   pixelated 与 auto 都被误判「有插值」（Edge 实测 pixelated 也只得 7 色即此因）。
	insetRect := func(r extraRect) extraRect {
		m := 3.0
		if r.W-2*m <= 1 || r.H-2*m <= 1 {
			m = 0
		}
		return extraRect{X: r.X + m, Y: r.Y + m, W: r.W - 2*m, H: r.H - 2*m}
	}
	if ra, okA := rectOf("e303a"); okA {
		ca := extraDistinct(extraGrid(px, vw, insetRect(ra), 20, 20))
		cb := -1
		if rb, okB := rectOf("e303b"); okB {
			cb = extraDistinct(extraGrid(px, vw, insetRect(rb), 20, 20))
		}
		actual := fmt.Sprintf("pixelated 采样色数 %d（源色 4）；auto 对照色数 %d（采样内缩 3px）", ca, cb)
		switch {
		case ca <= 4 && cb > 4:
			add("TC-M-303", actual, "✅", "最近邻生效：pixelated 只有源色，auto 出现插值中间色")
		case ca <= 4 && cb >= 0 && cb <= 4:
			add("TC-M-303", actual, "✅", "放大无插值（pixelated 语义达成）；★ 但 auto 同样无插值 ⇒ 引擎未区分 `image-rendering` 取值，如实记录")
		case cb > 0 && float64(ca)*1.5 <= float64(cb):
			add("TC-M-303", actual, "✅",
				fmt.Sprintf("pixelated 显著少于 auto（%d vs %d）⇒ `image-rendering:pixelated` 生效（放大走最近邻；残留色来自色块交界 ±1px 采样，非整幅插值）", ca, cb))
		case cb > 0 && ca-cb <= 2 && cb-ca <= 2:
			add("TC-M-303", actual, "❌",
				fmt.Sprintf("pixelated(%d) 与 auto(%d) 无差别 ⇒ `image-rendering:pixelated` 未生效（放大仍走插值）", ca, cb))
		case ca > 4:
			add("TC-M-303", actual, "❌", "pixelated 下仍出现插值中间色 ⇒ 该属性未被引擎采用")
		default:
			add("TC-M-303", actual, "⚠️", "对照场景缺失，判定不完整")
		}
	} else {
		add("TC-M-303", "场景缺失", "SKIP", "页面里没有 e303a")
	}

	// TC-M-304：canvas drawImage 取当前帧（观测器＝页面内 canvas 采样 + 渲染像素交叉验证）
	// ★ 越界样本必须剔除后再判：源矩形与图像无交集时 canvas 按规范「不画」，得到透明黑，
	//   与「取到恒定首帧」不可区分。初版固定采样点 (80,80) 对 120x80 的样本 GIF 越界
	//   ⇒ 整条序列恒定透明黑，被误判成「canvas 取到固化帧」（2026-10-08 复核修正）。
	if timeSeries {
		colorsOf := func(s []extraTsSample) []string {
			out := make([]string, 0, len(s))
			for _, x := range s {
				out = append(out, x.C)
			}
			return out
		}
		cvAll := colorsOf(tsCanvas["e304"])
		cv := validColors(cvAll)
		rd := colorsOf(ts["e304"])
		cn, rn := extraDistinct(cv), extraDistinct(rd)
		actual := fmt.Sprintf("canvas 采样 %d 次（有效 %d）出现 %d 种帧色；渲染像素 %d 次出现 %d 种帧色",
			len(cvAll), len(cv), cn, len(rd), rn)
		switch {
		case len(cv) == 0:
			add("TC-M-304", actual, "⚠️", "canvas 采样全部越界/异常 ⇒ 判据不完整（须查采样点与元素内容尺寸）")
		case cn >= 2:
			add("TC-M-304", actual, "✅", "canvas 的 drawImage 取到的是当前帧（帧色随播放推进变化）", head(cv, 12)...)
		case rn >= 2:
			add("TC-M-304", actual, "❌",
				"动图在页面上确实在推进（渲染像素多色），但 canvas drawImage 取到恒定帧 ⇒ 能力缺口",
				head(cv, 12)...)
		default:
			add("TC-M-304", actual, "❌", "两条路径都恒色 ⇒ 未采到动图推进（须查样本是否在推进）", head(cv, 12)...)
		}
	} else {
		tsSkip("TC-M-304")
	}

	// TC-M-305：逐帧 delay 对齐（渲染像素时间序列）
	if timeSeries {
		segs := extraSegments(ts["e305"])
		if len(segs) == 0 {
			add("TC-M-305", "未采到样本", "❌", "时间序列为空")
		} else {
			maxSeg, total := 0.0, 0.0
			for _, s := range segs {
				if s.DurMs > maxSeg {
					maxSeg = s.DurMs
				}
				total += s.DurMs
			}
			rate := 0.0
			if total > 0 {
				rate = float64(len(segs)-1) / (total / 1000)
			}
			okMax := maxSeg >= 140 && maxSeg <= 300
			// 期望变化率：3 帧/350ms ≈ 8.6 次/秒（采样间隔 ~75ms 下允许 4~16）。
			okRate := rate >= 4 && rate <= 16
			v := "⚠️"
			if okMax && okRate {
				v = "✅"
			}
			if rate == 0 {
				v = "❌" // 一帧都没换 ⇒ 该样本根本没在动，别把 ⚠️ 当"接近"
			}
			ev := make([]string, 0, 8)
			for i, s := range segs {
				if i >= 8 {
					break
				}
				ev = append(ev, fmt.Sprintf("t=%.0fms %s %.0fms", s.AtMs, s.Color, s.DurMs))
			}
			add("TC-M-305", fmt.Sprintf("色段 %d 个；最长段 %.0fms（期望 200）；帧色变化率 %.1f 次/秒（期望 ≈8.6）",
				len(segs), maxSeg, rate), v,
				"段时长与变化率都按真实墙钟时间戳计算（采样间隔受渲染耗时限制，故判据是区间而非等值）", ev...)
		}
	} else {
		tsSkip("TC-M-305")
	}

	// TC-M-306：visibility:hidden 期间的推进
	// ★ 观测器必须是 canvas 采样，**不能**用渲染像素：元素在隐藏窗口内本来就不绘制，
	//   渲染像素采到的是场景容器背景色（恒定）——把「看不见」读成「动画停了」。
	//   实测复核（2026-10-08，extra-raw-Browser.json）：隐藏窗口 405~697ms 渲染像素恒为
	//   容器背景 magenta，而恢复后的帧相位与「时钟未暂停」应得的相位一致（347ms 绿、
	//   755ms 蓝，周期 300ms）⇒ 引擎在隐藏期间一直在推进，初版「停推」结论是判据伪影。
	if timeSeries {
		hidRender := extraWindowColors(ts["e306"], extraHideAtMs, extraShowAtMs)
		visRender := extraWindowColors(ts["e306"], 0, extraHideAtMs)
		hid := validColors(extraWindowColors(tsCanvas["e306"], extraHideAtMs, extraShowAtMs))
		nh := extraDistinct(hid)
		v, note := "❌", "隐藏窗口内 canvas 采样帧色不变 ⇒ 动图被暂停。依据：动图的播放时钟与"+
			"「是否渲染」解耦，只有 `display:none`／页面不可见才应暂停；`visibility:hidden` 的"+
			"元素仍在渲染树内 ⇒ 应继续推进（Edge 侧时间序列不可采：无头一次性截图无时序，见 Edge 节 SKIP）"
		switch {
		case len(hid) == 0:
			v, note = "⚠️", "隐藏窗口内 canvas 采样全部越界/异常 ⇒ 判据不完整（须查采样点与元素内容尺寸）"
		case nh >= 2:
			v, note = "✅", "隐藏期间仍在推进（canvas 观测：帧色仍变化）⇒ 符合规范（`visibility:hidden` 不暂停动图）"
		}
		add("TC-M-306", fmt.Sprintf("隐藏窗口(%d~%dms) canvas 采样 %d 次出现 %d 种帧色；同窗口渲染像素 %d 种"+
			"（元素不可见 ⇒ 采到的是容器背景色，不能作推进判据）；可见窗口渲染像素 %d 种",
			extraHideAtMs, extraShowAtMs, len(hid), nh, extraDistinct(hidRender), extraDistinct(visRender)),
			v, note, head(hid, 8)...)
	} else {
		tsSkip("TC-M-306")
	}

	// TC-M-307a/307b：末帧行为（不循环 vs 循环）
	for _, spec := range []struct{ id, key string }{{"TC-M-307a", "e307a"}, {"TC-M-307b", "e307b"}} {
		if !timeSeries {
			tsSkip(spec.id)
			continue
		}
		tails := extraTailColors(ts[spec.key], 0.25)
		n := extraDistinct(tails)
		if spec.key == "e307a" {
			v, note := "❌", "尾部帧色仍在变化 ⇒ 未停在末帧。依据：`anim-noloop.gif` 的 GIF 循环扩展块计数=1"+
				"（播 2 遍后停）、帧长 3×100ms ⇒ 采样窗口远晚于 600ms 后应恒定停在末帧；"+
				"引擎仍在推进 ⇒ loop 计数终止未实现（Edge 侧时间序列不可采，见 Edge 节 SKIP）"
			if n == 1 {
				v, note = "✅", "播过末尾后停在末帧（尾部帧色恒定）"
			}
			add(spec.id, fmt.Sprintf("尾部 %d 次采样出现 %d 种帧色", len(tails), n), v, note, head(tails, 8)...)
		} else {
			v, note := "❌", "尾部帧色恒定 ⇒ 循环停止"
			if n >= 2 {
				v, note = "✅", "播过末尾后回到首帧继续循环（`loop=0` = 无限循环；尾部仍出现多种帧色）"
			}
			add(spec.id, fmt.Sprintf("尾部 %d 次采样出现 %d 种帧色", len(tails), n), v, note, head(tails, 8)...)
		}
	}

	// TC-M-404：扩展名与内容不符（按内容解码）。
	// ★ 期望必须按配置区分：策略配置下该引用会被 resolver 拒绝（nw=0），那是策略的
	//   预期行为、不是「解码失败」——否则会把「策略生效」误报成引擎缺陷。
	if label == "Browser" || label == "Edge" {
		if ok, desc := loadedOf("e404"); ok {
			add("TC-M-404", desc+"；"+contentPoints("e404"), "✅", "按内容解码（引擎不看扩展名）")
		} else {
			add("TC-M-404", desc, "❌", "未加载/未解码出内容")
		}
	} else {
		ok, desc := loadedOf("e404")
		v, note := "✅", "策略配置下该引用被 resolver 拒绝（策略生效；nw=0 符合预期）"
		if ok {
			v, note = "⚠️", "策略配置下仍加载成功（与「拒绝外部引用」的预期不符，如实记录）"
		}
		add("TC-M-404", desc, v, note)
	}

	// TC-M-405：路径不存在
	if c, ok := st.Cells["e405"]; !ok || c.Missing {
		add("TC-M-405", "场景缺失", "SKIP", "页面里没有 e405")
	} else {
		cons := st.Cons
		actual := fmt.Sprintf("naturalWidth=%.0f；页面可见控制台条目 %d 条", c.NW, len(cons))
		switch {
		case c.NW > 0:
			add("TC-M-405", actual, "❌", "不存在的路径竟然加载成功")
		case len(cons) == 0:
			add("TC-M-405", actual, "✅", "不渲染且无控制台刷屏")
		default:
			add("TC-M-405", actual, "⚠️", "不渲染，但页面侧有控制台输出（如实记录）", head(cons, 3)...)
		}
	}

	// TC-M-506：controls 的原生控件
	{
		cA, okA := st.Cells["e506a"]
		cB, okB := st.Cells["e506b"]
		tabA, tabB := -999, -999
		if okA && !cA.Missing && cA.TabIndex != nil {
			tabA = int(*cA.TabIndex)
		}
		if okB && !cB.Missing && cB.TabIndex != nil {
			tabB = int(*cB.TabIndex)
		}
		diff := -1
		if ra, ok1 := rectOf("e506a"); ok1 {
			if rb, ok2 := rectOf("e506b"); ok2 {
				ga, gb := extraGrid(px, vw, ra, 8, 6), extraGrid(px, vw, rb, 8, 6)
				d := 0
				for i := range ga {
					if ga[i] != gb[i] {
						d++
					}
				}
				diff = d
			}
		}
		actual := fmt.Sprintf("tabIndex：无 controls=%d、带 controls=%d；两格 8x6 采样差异 %d/48 点", tabA, tabB, diff)
		v, note := "✅", "契约符合（controls 使 tabIndex 默认 0）；引擎无 UA 原生控件绘制路径 ⇒ 视觉差异如实记录，不列为缺陷"
		if tabB != 0 {
			v, note = "⚠️", "带 controls 的 tabIndex 不是 0（契约偏差，如实记录）"
		}
		add("TC-M-506", actual, v, note)
	}

	// TC-M-702：object-fit 四值
	{
		var parts []string
		allOK := true
		for _, spec := range []struct{ id, fit string }{
			{"e702c", "contain"}, {"e702v", "cover"}, {"e702f", "fill"}, {"e702n", "none"},
		} {
			r, ok := rectOf(spec.id)
			if !ok {
				allOK = false
				parts = append(parts, spec.fit+"=缺失")
				continue
			}
			cx, cy := r.X+r.W/2, r.Y+r.H/2
			hz := extraScanline(px, vw, r.X+3, cy, r.X+r.W-4, cy, 160)
			vt := extraScanline(px, vw, cx, r.Y+3, cx, r.Y+r.H-4, 160)
			h := extraMedianRun(extraRunLen(hz))
			v := extraMedianRun(extraRunLen(vt))
			lb := isBG(vt[0]) || isBG(vt[len(vt)-1])
			ratio := 0.0
			if v > 0 {
				ratio = h / v
			}
			// 用 h/v 比值（而不是 |h-v| 的相对容差）：实测 fill 的 21:24（比值 0.875）在
			// 0.25 的相对容差下会被误判成「等比」——比值的尺度更贴近「格子是否被拉伸」。
			eq := math.Abs(ratio-1) <= 0.15
			good := false
			switch spec.fit {
			case "contain":
				good = lb && eq
			case "cover":
				good = !lb && eq && h > 0 && h < 34
			case "fill":
				good = !lb && ratio > 0 && ratio <= 0.9
			case "none":
				good = !lb && eq && h >= 34
			}
			if !good {
				allOK = false
			}
			parts = append(parts, fmt.Sprintf("%s[h=%.0f v=%.0f h/v=%.2f lb=%v]", spec.fit, h, v, ratio, lb))
		}
		v := "✅"
		if !allOK {
			v = "❌"
		}
		add("TC-M-702", strings.Join(parts, " "), v,
			"h/v = 扫描线中位段长（px，棋盘格周期）；lb = 上下是否出现 magenta 背景（letterbox）")
	}

	// TC-M-704 / TC-M-705a：中文路径、含空格名
	for _, spec := range []struct{ caseID, elemID, note string }{
		{"TC-M-704", "e704", "中文文件名 + 中文目录（Windows 路径编码）"},
		{"TC-M-705a", "e705a", "文件名含空格（URL 需容忍 %20）"},
	} {
		ok, desc := loadedOf(spec.elemID)
		switch {
		case ok:
			add(spec.caseID, desc+"；"+contentPoints(spec.elemID), "✅", spec.note)
		case label != "Browser" && label != "Edge":
			add(spec.caseID, desc, "✅", spec.note+"：策略配置下被 resolver 拒绝（符合预期）")
		default:
			add(spec.caseID, desc, "❌", spec.note+"：未加载")
		}
	}

	// TC-M-705b：`..` 穿越（Browser(file://) 可解析；策略配置应被 resolver 拒绝）
	{
		ok, desc := loadedOf("e705b")
		switch label {
		case "Browser", "Edge":
			v, note := "✅", "file:// 下 `..` 可解析（浏览器语义）"
			if !ok {
				v, note = "❌", "file:// 下 `..` 竟不可解析"
			}
			add("TC-M-705b", desc, v, note)
		default:
			v, note := "✅", "策略配置下被 resolver 拒绝（策略生效，符合预期）"
			if ok {
				v, note = "⚠️", "策略配置下仍加载成功（与「外部引用应被拒」的预期不符，如实记录）"
			}
			add("TC-M-705b", desc, v, note)
		}
	}
	return out
}

// ── 报告与调度 ────────────────────────────────────────────────────────

type extraRun struct {
	Label   string
	Cases   []extraCase
	Summary string
}

// runExtraVerification 跑补充判据并把报告写到 outDir（独立文件，不动主报告的结构）。
func runExtraVerification(samplesDir, outDir string, vw, pageH int, withEdge bool) error {
	page, err := buildExtraPage(samplesDir)
	if err != nil {
		return err
	}
	pagePath := filepath.Join(samplesDir, "_extra.html")
	if err := os.WriteFile(pagePath, []byte(page), 0o644); err != nil {
		return err
	}
	var runs []extraRun
	browser := mediaConfig{Name: "Browser", Mode: webkit.ModeBrowser}
	cases, err := runExtraProbe(browser, pagePath, outDir, vw, pageH, true)
	if err != nil {
		return fmt.Errorf("引擎侧（Browser）采集失败：%w", err)
	}
	runs = append(runs, extraRun{Label: "Browser", Cases: cases})
	// 策略侧也采一次（只静态）：TC-M-705b 的「`..` 应被 resolver 拒绝」只有在
	// DenyExternal 下才有意义，只跑 Browser 会把结论压在「file:// 允许」这一侧。
	deny := mediaConfig{Name: "Toolkit+DenyExternal", Mode: webkit.ModeToolkit,
		Policy: policyPtr(webkit.DenyExternal)}
	if dc, derr := runExtraProbe(deny, pagePath, outDir, vw, pageH, false); derr == nil {
		runs = append(runs, extraRun{Label: "Toolkit+DenyExternal", Cases: dc,
			Summary: "只采静态判据（时间序列项 SKIP）"})
	} else {
		runs = append(runs, extraRun{Label: "Toolkit+DenyExternal",
			Summary: fmt.Sprintf("跳过：%v", derr)})
	}
	if withEdge {
		ec, summary := runExtraEdge(pagePath, outDir, vw, pageH)
		runs = append(runs, extraRun{Label: "Edge", Cases: ec, Summary: summary})
	} else if ec, ok := loadArchivedEdgeRun(filepath.Join(outDir, "extra-Edge.json")); ok {
		// 未随本轮跑 Edge（-media-edge 默认关闭）：复用上一次的 Edge 存档，
		// 让「## Edge 逐项对照」与「引擎 vs Edge」结论不因复跑从产物里消失。
		// ★ 段首来源必须**中性**：存档会被 `-media-edge` 覆盖写；若在这里写死轮次/commit，
		// 「重跑 Edge（存档更新）→ 再跑 -media」就会数据是新的、标注还是旧的（来源撒谎）。
		// 故只说「上一轮存档」+ 存档文件时间（重写后自动跟着变），不锁定具体是哪一次 run。
		runs = append(runs, extraRun{Label: "Edge", Cases: ec,
			Summary: "本节取自上一轮 Edge 对照存档（`extra-Edge.json`" + edgeArchiveStamp(outDir) +
				"）：**本轮未重跑 Edge**（`-media-edge` 默认关闭），故沿用存档中的数值口径；" +
				"重跑 `-media-edge` 会用新数据覆盖本节。"})
	}
	return writeExtraReport(outDir, runs, pagePath)
}

// edgeArchiveStamp 返回存档文件的时间描述（供段首如实标注「这份数据是什么时候写的」）。
// 取文件 mtime 而不往 JSON 里塞 commit 字段：存档结构保持 `[]extraCase` 不变，
// 且 `-media-edge` 重写存档后时间自动跟着变——段首标注不会因重跑而失真。
// stat 失败（存档缺失/权限异常）返回空串，段首退化为不带时间的同义表述。
func edgeArchiveStamp(outDir string) string {
	fi, err := os.Stat(filepath.Join(outDir, "extra-Edge.json"))
	if err != nil {
		return ""
	}
	return "，存档文件时间 " + fi.ModTime().Format("2006-01-02 15:04:05")
}

// loadArchivedEdgeRun 读取上一次 Edge 对照的存档（extra-Edge.json，[]extraCase）。
// 用途：本轮没跑 `-media-edge` 时，把上一轮的 Edge 逐项对照重新渲染进产物，
// 避免「复跑一次就抹掉已达成结论」的产物回退（数据来源由段首 Summary 如实标注：
// 中性措辞 + 存档文件时间，不锁定具体轮次/commit）。
func loadArchivedEdgeRun(path string) ([]extraCase, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var cases []extraCase
	if err := json.Unmarshal(raw, &cases); err != nil || len(cases) == 0 {
		return nil, false
	}
	return cases, true
}

func runByLabel(runs []extraRun, label string) (extraRun, bool) {
	for _, r := range runs {
		if r.Label == label {
			return r, true
		}
	}
	return extraRun{}, false
}

// writeExtraReport 落盘 Markdown 报告 + 每侧 JSON 证据。
func writeExtraReport(outDir string, runs []extraRun, pagePath string) error {
	var b strings.Builder
	b.WriteString("# 补充判据：G3/G4/G6/G7 的「未单独采证」用例\n\n")
	fmt.Fprintf(&b, "- 页面：`%s`（探针运行时生成；中文/空格样本由探针自动物化）\n", pagePath)
	b.WriteString("- 复跑：`cmd/psai -media`（本段随媒体验证一起跑）；Edge 对照随 `-media-edge`\n")
	b.WriteString("- 采集方式：**渲染像素**（引擎 `wv.Render()` / Edge `--screenshot`）+ **页面内 canvas 采样**\n" +
		"  （`drawImage` 取元素当前帧，用于 TC-M-304~307 的时间序列）；两侧走同一套页面脚本与判据内核。\n" +
		"- 判据口径：✅ 达成 / ❌ 未达成 / ⚠️ 部分或不符预期（如实记录）/ SKIP（附原因，不伪装通过）。\n\n")
	for _, run := range runs {
		fmt.Fprintf(&b, "## %s\n\n", run.Label)
		if run.Summary != "" {
			fmt.Fprintf(&b, "> %s\n\n", run.Summary)
		}
		if len(run.Cases) == 0 {
			b.WriteString("（无数据）\n\n")
			continue
		}
		b.WriteString("| 用例 | 判据 | 期望 | 实测 | 判定 | 备注 |\n|---|---|---|---|---|---|\n")
		for _, c := range run.Cases {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
				c.ID, c.Title, c.Expect, c.Actual, c.Verdict, c.Note)
		}
		b.WriteString("\n")
	}
	if eng, ok := runByLabel(runs, "Browser"); ok {
		if edge, ok2 := runByLabel(runs, "Edge"); ok2 && len(edge.Cases) > 0 {
			em := map[string]extraCase{}
			for _, c := range edge.Cases {
				em[c.ID] = c
			}
			compared, lower := 0, []string{}
			for _, c := range eng.Cases {
				e, has := em[c.ID]
				if !has || c.Verdict == "SKIP" || e.Verdict == "SKIP" {
					continue
				}
				compared++
				if c.Verdict != "✅" && e.Verdict == "✅" {
					lower = append(lower, c.ID)
				}
			}
			fmt.Fprintf(&b, "## 引擎 vs Edge 逐项对照\n\n可比 **%d** 项，", compared)
			if len(lower) == 0 {
				b.WriteString("**引擎无一项低于 Edge** ✅\n\n")
			} else {
				fmt.Fprintf(&b, "**低于 Edge 的项：%s** ❌\n\n", strings.Join(lower, "、"))
			}
		}
	}
	for _, run := range runs {
		if len(run.Cases) == 0 {
			continue
		}
		jb, err := json.MarshalIndent(run.Cases, "", "  ")
		if err != nil {
			return err
		}
		name := strings.NewReplacer("+", "_", " ", "_").Replace(run.Label)
		if err := os.WriteFile(filepath.Join(outDir, "extra-"+name+".json"), jb, 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(outDir, "extra-verification.md"), []byte(b.String()), 0o644)
}

// extraWritePNG 把渲染像素写成 PNG（与 main.go 的无头截图同一布局：RGBA）。
func extraWritePNG(path string, px []byte, w, h int) error {
	if len(px) < w*h*4 {
		return fmt.Errorf("像素缓冲不足：%d < %d", len(px), w*h*4)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, px[:w*h*4])
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// extraAppendSample 往时间序列里追加一个采样点（map 懒初始化）。
func extraAppendSample(m map[string][]extraTsSample, key string, t int64, c string) map[string][]extraTsSample {
	if m == nil {
		m = map[string][]extraTsSample{}
	}
	m[key] = append(m[key], extraTsSample{T: float64(t), C: c})
	return m
}

// extraSampleAnchors 在渲染像素里取各场景的**中心点**颜色（时间序列的取样口径）。
// 取中心而不是四角：动图样本是整幅换帧，中心点最稳（不受盒边框/抗锯齿影响）。
func extraSampleAnchors(px []byte, vw int, st extraState, ids []string) map[string]string {
	out := map[string]string{}
	for _, id := range ids {
		c, ok := st.Cells[id]
		if !ok || c.Missing || c.W <= 0 || c.H <= 0 {
			continue
		}
		rgb := pixelAt(px, vw, int(c.X+c.W/2), int(c.Y+c.H/2))
		out[id] = fmt.Sprintf("%d,%d,%d", rgb[0], rgb[1], rgb[2])
	}
	return out
}

func extraMustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}
