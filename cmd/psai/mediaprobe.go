package main

// 媒体格式真实可用性验证工装（文档 §6.2 第 2 条 / §8.2 阶段 0）。
//
// 设计要点：
//   - **四配置**：Browser / Toolkit+DenyExternal / Toolkit+AllowHostResolved /
//     Toolkit+AllowAll（决策 1、§4）。后两者靠 SetResourcePolicy 与宿主
//     resolver 区分；Browser 与 Toolkit+AllowAll 必须逐像素一致（TC-M-905）。
//   - **矩阵页**：由 manifest.json（gen_samples.py 产物）生成，一份页面覆盖
//     每个样本的多种来源（data: / file:// / 文档相对路径）。页面用绝对定位
//     网格排布，一次 Render 即可覆盖全页。
//   - **判定 L0–L4**（§2）：几何 + 像素（四象限采样，与样本期望色比对）+
//     事件/IDL 契约 + 帧差异（L4）。
//   - **基线比对**：与 baseline.json 的等级逐条对比，任何格式等级**下降**
//     都让本命令非零退出（§6.2 第 2 条；本机按需执行，不入 CI 门禁）。
//   - **证据纪律**（§6.3）：每条用例落盘 几何 JSON + 截图 PNG + 事件计数。
//     截图供 read_image 人眼复核。
//
// 用法见 docs/media-format-verification-plan.md §6.4。

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"wb-ui/app"
	"wb-ui/webkit"
)

// ── 配置与数据结构 ─────────────────────────────────────────────────────

type mediaOpts struct {
	Samples   string
	Out       string
	Baseline  string
	Edge      bool
	Update    bool
	Only      string
	ViewportW int
}

// mediaConfig 是一种执行配置（决策 1/2 的四种之一）。
type mediaConfig struct {
	Name string
	Mode webkit.Mode
	// Policy 为 nil 表示不显式设置（按模式推导，即历史默认）。
	Policy *webkit.ResourcePolicy
	// HostResolver 为 true 时宿主提供「读 samples 目录」的 resolver
	// （AllowHostResolved 的必要条件：只放行宿主明确提供的资源）。
	HostResolver bool
}

func policyPtr(p webkit.ResourcePolicy) *webkit.ResourcePolicy { return &p }

func mediaConfigs() []mediaConfig {
	return []mediaConfig{
		{Name: "Browser", Mode: webkit.ModeBrowser},
		{Name: "Toolkit+DenyExternal", Mode: webkit.ModeToolkit, Policy: policyPtr(webkit.DenyExternal)},
		{Name: "Toolkit+AllowHostResolved", Mode: webkit.ModeToolkit,
			Policy: policyPtr(webkit.AllowHostResolved), HostResolver: true},
		{Name: "Toolkit+AllowAll", Mode: webkit.ModeToolkit, Policy: policyPtr(webkit.AllowAll)},
	}
}

// sampleSpec 是 manifest.json 的一个样本（由 gen_samples.py 生成）。
type sampleSpec struct {
	File      string           `json:"file"`
	Name      string           `json:"name"`
	Kind      string           `json:"kind"` // image|animated|svg|svg-inline|video|audio|broken|huge|skipped
	Format    string           `json:"format"`
	Mime      string           `json:"mime"`
	Width     int              `json:"width"`
	Height    int              `json:"height"`
	Pattern   string           `json:"pattern"` // quad|solid|gradient
	Quad      map[string][]int `json:"quad"`
	Solid     []int            `json:"solid"`
	Frames    int              `json:"frames"`
	Delays    []int            `json:"delays"`
	FrameRGB  [][]int          `json:"frame_colors"`
	Skip      string           `json:"skip"`
	Inline    string           `json:"inline"`
	Note      string           `json:"note"`
	Generated bool             `json:"-"`
}

type manifestDoc struct {
	GeneratedBy  string `json:"generated_by"`
	QuadGeometry struct {
		Width        int              `json:"width"`
		Height       int              `json:"height"`
		SamplePoints map[string][]int `json:"sample_points"`
	} `json:"quad_geometry"`
	Samples []sampleSpec `json:"samples"`
}

// cell 是矩阵页上的一个「样本 × 来源」格。
type cell struct {
	ID     string
	Sample sampleSpec
	Source string // data | file | rel
	URL    string
	X, Y   int
}

// cellProbe 是页面侧采集回来的一格状态。
type cellProbe struct {
	ID         string  `json:"id"`
	Tag        string  `json:"tag"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	W          float64 `json:"w"`
	H          float64 `json:"h"`
	Complete   any     `json:"complete"`
	NaturalW   float64 `json:"nw"`
	NaturalH   float64 `json:"nh"`
	ReadyState float64 `json:"rs"`
	Duration   float64 `json:"duration"`
	CurTime    float64 `json:"ct"`
	Paused     any     `json:"paused"`
	ErrCode    float64 `json:"err"`
}

type pageReport struct {
	Cells  []cellProbe            `json:"cells"`
	Events map[string]map[string]int `json:"events"`
	VW     float64                `json:"vw"`
	VH     float64                `json:"vh"`
	DocH   float64                `json:"doch"`
}

// cellResult 是一格的最终判定。
type cellResult struct {
	ID       string       `json:"id"`
	Sample   string       `json:"sample"`
	Source   string       `json:"source"`
	Format   string       `json:"format"`
	Kind     string       `json:"kind"`
	Tag      string       `json:"tag"`
	BoxW     float64      `json:"box_w"`
	BoxH     float64      `json:"box_h"`
	NaturalW float64      `json:"natural_w"`
	NaturalH float64      `json:"natural_h"`
	Complete bool         `json:"complete"`
	Ready    float64      `json:"ready_state"`
	Duration float64      `json:"duration"`
	OnLoad   int          `json:"onload"`
	OnError  int          `json:"onerror"`
	Sampled  []sampleDot  `json:"sampled"`
	DrawOK   bool         `json:"draw_ok"`
	MetaOK   bool         `json:"meta_ok"`
	ContrOK  bool         `json:"contract_ok"`
	AnimOK   bool         `json:"anim_ok"`
	Grade    string       `json:"grade"`
	Note     string       `json:"note"`
	AnimNote string       `json:"anim_note,omitempty"`
	HTTPNote string       `json:"-"`
}

// sampleDot 是一个采样点的实测色与期望色。
type sampleDot struct {
	Name    string `json:"name"`
	X, Y    int    `json:"x"`
	Got     [3]int `json:"got"`
	Want    [3]int `json:"want"`
	Inside  bool   `json:"inside"`
	Matched bool   `json:"matched"`
}

type configResult struct {
	Config   string       `json:"config"`
	Policy   string       `json:"policy"`
	Cells    []cellResult `json:"cells"`
	StillPNG string       `json:"still_png"`
	PlayPNG  string       `json:"play_png,omitempty"`
	GeomJSON string       `json:"geom_json"`
	EdgeNote string       `json:"edge_note,omitempty"`
}

// ── 入口 ──────────────────────────────────────────────────────────────

func runMediaProbe(o mediaOpts) int {
	samplesDir, err := filepath.Abs(o.Samples)
	if err != nil {
		fmt.Printf("媒体验证：样本目录解析失败: %v\n", err)
		return 2
	}
	outDir, err := filepath.Abs(o.Out)
	if err != nil {
		fmt.Printf("媒体验证：输出目录解析失败: %v\n", err)
		return 2
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Printf("媒体验证：创建输出目录失败: %v\n", err)
		return 2
	}

	man, err := readManifest(filepath.Join(samplesDir, "manifest.json"))
	if err != nil {
		fmt.Printf("媒体验证：%v\n  提示：先运行 python dev/media/gen_samples.py\n", err)
		return 2
	}
	fmt.Printf("媒体验证：样本 %d 项（%s）\n", len(man.Samples), samplesDir)

	vw := o.ViewportW
	if vw <= 0 {
		vw = 1200
	}
	cells, html, pageH := buildMatrix(man, samplesDir, vw, o.Only)
	if len(cells) == 0 {
		fmt.Println("媒体验证：没有匹配的样本（检查 -media-only）")
		return 2
	}
	// ★ 矩阵页放在**样本目录内**：页面里的文档相对路径（`quad.png`）按
	//   文档基准解析，必须与样本同级才命中（TC-M-103/113/…）。此前放在
	//   out/ 目录，所有 rel 来源都解析到不存在的文件 → 假的 L0。
	matrixPath := filepath.Join(samplesDir, "_matrix.html")
	if err := os.WriteFile(matrixPath, []byte(html), 0o644); err != nil {
		fmt.Printf("媒体验证：写矩阵页失败: %v\n", err)
		return 2
	}
	fmt.Printf("媒体验证：矩阵页 %d 格 → %s（%dx%d）\n", len(cells), matrixPath, vw, pageH)

	var results []configResult
	for _, cfg := range mediaConfigs() {
		res, err := runMediaConfig(cfg, man, cells, matrixPath, samplesDir, outDir, vw, pageH)
		if err != nil {
			fmt.Printf("  配置 %s：失败——%v\n", cfg.Name, err)
			return 2
		}
		results = append(results, res)
		fmt.Printf("  配置 %-26s → %s\n", cfg.Name, filepath.Base(res.StillPNG))
	}

	if o.Edge {
		note := runEdgeComparison(results, matrixPath, vw, pageH, outDir)
		fmt.Printf("  Edge 对照：%s\n", note)
	}

	// 决策 1 的一致性断言（TC-M-905）：Browser 与 Toolkit+AllowAll 必须
	// 逐像素一致——证明「新增策略开关」没有引入渲染差异。
	adDiff, adErr := -1.0, error(nil)
	if len(results) == 4 {
		adDiff, adErr = comparePNGFiles(results[0].StillPNG, results[3].StillPNG)
	}

	baseline, berr := readBaseline(o.Baseline)
	reportPath := filepath.Join(outDir, "report.md")
	if err := writeMediaReport(reportPath, man, results, baseline, berr, matrixPath, adDiff, adErr); err != nil {
		fmt.Printf("媒体验证：写报告失败: %v\n", err)
		return 2
	}
	fmt.Printf("媒体验证：报告 → %s\n", reportPath)

	// 基线比对：等级下降 → 非零退出（§6.2 第 2 条）。
	if o.Update {
		if err := writeBaseline(o.Baseline, results); err != nil {
			fmt.Printf("媒体验证：写基线失败: %v\n", err)
			return 2
		}
		fmt.Printf("媒体验证：基线已更新 → %s\n", o.Baseline)
		return 0
	}
	if berr != nil {
		fmt.Printf("媒体验证：基线不可读（%v）——本次仅产出报告，未做升降比对\n", berr)
		return 0
	}
	regress := compareBaseline(baseline, results)
	if len(regress) == 0 {
		fmt.Println("媒体验证：与基线一致（无等级下降）")
		return 0
	}
	fmt.Println("媒体验证：等级下降（缺陷）：")
	for _, r := range regress {
		fmt.Println("  " + r)
	}
	return 1
}

// ── manifest 与矩阵页 ─────────────────────────────────────────────────

func readManifest(path string) (*manifestDoc, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读样本清单失败（%s）: %w", path, err)
	}
	var m manifestDoc
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("解析样本清单失败: %w", err)
	}
	if len(m.Samples) == 0 {
		return nil, fmt.Errorf("样本清单为空（%s）", path)
	}
	return &m, nil
}

// cellSize 是矩阵页每格的显示尺寸（与样本像素 1:1，DPR=1）。
const (
	cellW  = 120
	cellH  = 80
	gapX   = 12
	gapY   = 16
	margin = 8
)

// buildMatrix 生成矩阵页 HTML：每个样本 × 每种可用来源一格，绝对定位排布。
// 返回格子清单、HTML 与页面高度。
func buildMatrix(man *manifestDoc, samplesDir string, vw int, only string) ([]cell, string, int) {
	cols := (vw - 2*margin) / (cellW + gapX)
	if cols < 1 {
		cols = 1
	}
	var cells []cell
	add := func(sp sampleSpec, source, url string) {
		if only != "" && !strings.Contains(sp.Name, only) {
			return
		}
		cells = append(cells, cell{
			ID:     fmt.Sprintf("c%d", len(cells)),
			Sample: sp, Source: source, URL: url,
		})
	}

	for _, sp := range man.Samples {
		if sp.Kind == "skipped" || sp.Skip != "" {
			continue
		}
		switch sp.Kind {
		case "svg-inline":
			add(sp, "data", dataURIText(sp.Mime, sp.Inline))
			continue
		}
		if sp.File == "" {
			continue
		}
		abs := filepath.Join(samplesDir, filepath.FromSlash(sp.File))
		// data: 来源（内联；超大样本跳过以免页面膨胀）
		if b, err := os.ReadFile(abs); err == nil && len(b) > 0 && len(b) <= 1<<20 {
			add(sp, "data", "data:"+sp.Mime+";base64,"+base64.StdEncoding.EncodeToString(b))
		}
		// file:// 来源
		add(sp, "file", fileURLOf(abs))
		// 文档相对路径来源
		add(sp, "rel", sp.File)
	}

	rows := (len(cells) + cols - 1) / cols
	pageH := margin + rows*(cellH+gapY) + 40
	if pageH < 600 {
		pageH = 600
	}

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html><head><meta charset=\"utf-8\">\n<style>\n")
	b.WriteString("html,body{margin:0;padding:0;background:#ffffff;font:10px monospace;color:#333}\n")
	b.WriteString(".cell{position:absolute;background:#eeeeee;border:1px solid #bbbbbb;box-sizing:border-box}\n")
	b.WriteString(".lbl{position:absolute;font-size:9px;line-height:10px;overflow:hidden;white-space:nowrap}\n")
	b.WriteString("</style></head><body>\n")
	b.WriteString("<script>\nwindow.__mediaEvents={};\n")
	b.WriteString("function mvNote(id,ev){var e=window.__mediaEvents[id]||(window.__mediaEvents[id]={});e[ev]=(e[ev]||0)+1;}\n")
	b.WriteString("</script>\n")

	for i := range cells {
		c := &cells[i]
		row, col := i/cols, i%cols
		c.X = margin + col*(cellW+gapX)
		c.Y = margin + row*(cellH+gapY)
		tag := "img"
		if c.Sample.Kind == "video" {
			tag = "video"
		} else if c.Sample.Kind == "audio" {
			tag = "audio"
		}
		style := fmt.Sprintf("left:%dpx;top:%dpx;width:%dpx;height:%dpx", c.X, c.Y, cellW, cellH)
		if tag == "audio" {
			// audio 无视觉内容：用包裹 div 承载，采样点落在 div 上（期望：未绘制）
			fmt.Fprintf(&b, "<div class=\"cell\" id=%s style=\"%s\"></div>\n",
				jsString(c.ID), style)
			fmt.Fprintf(&b, "<audio id=%s style=\"position:absolute;left:%dpx;top:%dpx;width:1px;height:1px\" src=%s "+
				"onloadedmetadata=\"mvNote(%s,'loadedmetadata')\" onerror=\"mvNote(%s,'error')\"></audio>\n",
				jsString(c.ID+"_a"), c.X, c.Y, jsString(c.URL),
				jsAttrString(c.ID), jsAttrString(c.ID))
		} else {
			events := fmt.Sprintf("onload=\"mvNote(%s,'load')\" onerror=\"mvNote(%s,'error')\"", jsAttrString(c.ID), jsAttrString(c.ID))
			if tag == "video" {
				events = fmt.Sprintf("onloadedmetadata=\"mvNote(%s,'loadedmetadata')\" onloadeddata=\"mvNote(%s,'loadeddata')\""+
					" oncanplay=\"mvNote(%s,'canplay')\" onplay=\"mvNote(%s,'play')\" onplaying=\"mvNote(%s,'playing')\""+
					" ontimeupdate=\"mvNote(%s,'timeupdate')\" onended=\"mvNote(%s,'ended')\" onerror=\"mvNote(%s,'error')\"",
					jsAttrString(c.ID), jsAttrString(c.ID), jsAttrString(c.ID), jsAttrString(c.ID), jsAttrString(c.ID),
					jsAttrString(c.ID), jsAttrString(c.ID), jsAttrString(c.ID))
			}
			fmt.Fprintf(&b, "<%s class=\"cell\" id=%s src=%s style=\"%s\" %s></%s>\n",
				tag, jsString(c.ID), jsString(c.URL), style, events, tag)
		}
		label := fmt.Sprintf("%s|%s|%s", shortName(c.Sample.Name), c.Source, c.Sample.Format)
		fmt.Fprintf(&b, "<div class=\"lbl\" style=\"left:%dpx;top:%dpx;width:%dpx\">%s</div>\n",
			c.X, c.Y+cellH+2, cellW+gapX, htmlEscape(label))
	}
	b.WriteString("</body></html>\n")
	return cells, b.String(), pageH
}

func shortName(s string) string {
	s = strings.TrimSuffix(s, ".png")
	s = strings.TrimSuffix(s, ".jpg")
	s = strings.TrimSuffix(s, ".gif")
	s = strings.TrimSuffix(s, ".webp")
	s = strings.TrimSuffix(s, ".bmp")
	s = strings.TrimSuffix(s, ".ico")
	s = strings.TrimSuffix(s, ".tiff")
	s = strings.TrimSuffix(s, ".avif")
	return s
}

// jsAttrString 生成可安全嵌入 **HTML 属性值**（双引号包裹）的 JS 字符串
// 字面量：用单引号包裹，转义反斜杠/单引号，HTML 敏感字符走实体。
//
// ★ 缺陷源（探针自身缺陷，不是引擎缺陷）：内联事件属性此前用 jsString
//（双引号）传参 → 生成 `onload="mvNote("c0",'load')"`，属性在第二个双引号
// 处**提前闭合**，处理器只剩 `mvNote(` → 语法错误 → 页面侧
// window.__mediaEvents 恒为空。实测后果：96 格契约列全 ❌、Browser 下
// 0 个 L3——这是测量伪影，掩盖了 U2 修复后的真实等级。
func jsAttrString(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\\', '\'':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '"':
			b.WriteString("&quot;")
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}

func dataURIText(mime, text string) string {
	return "data:" + mime + "," + strings.ReplaceAll(text, "#", "%23")
}

// ── 单配置执行 ────────────────────────────────────────────────────────

func runMediaConfig(cfg mediaConfig, man *manifestDoc, cells []cell, matrixPath, samplesDir, outDir string,
	vw, pageH int) (configResult, error) {

	res := configResult{Config: cfg.Name}
	wv := webkit.NewWebViewWithMode(cfg.Mode)
	defer wv.Destroy()
	if cfg.Policy != nil {
		wv.SetResourcePolicy(*cfg.Policy)
	}
	if cfg.HostResolver {
		wv.SetResourceResolver(hostSamplesResolver(samplesDir))
	}
	wv.Resize(vw, pageH)

	// 主线 A0/A4 的宿主能力（与 psai 主流程一致：元数据探测 + Skia 多帧动图源）
	app.InstallMediaMetadataResolver(wv, "")
	app.InstallAnimatedImageSource()

	html, err := os.ReadFile(matrixPath)
	if err != nil {
		return res, err
	}
	base := fileURLOf(matrixPath)
	if err := wv.LoadHTMLWithBaseURL(string(html), base); err != nil {
		return res, err
	}
	// ★ 真实等待：file:// 与 http(s) 图片走渲染层的**异步**取字节
	//   （loadBackgroundImageWith 的 loader 分支是 goroutine），只推进虚拟
	//   事件循环时间不会让 goroutine 完成——必须「推进 + 真实 sleep」交替。
	settleReal(wv, 1500*time.Millisecond)

	res.Policy = wv.ResourcePolicy().String()

	// ① 静止态采集 + 截图
	probes, events, err := collectPage(wv, cells)
	if err != nil {
		return res, err
	}
	stillPixels, err := renderPixels(wv)
	if err != nil {
		return res, err
	}
	stillPath := filepath.Join(outDir, fmt.Sprintf("matrix-%s.png", sanitizeFile(cfg.Name)))
	if err := writePNG(stillPath, stillPixels, vw, pageH); err != nil {
		return res, err
	}
	res.StillPNG = stillPath

	// ② 播放态：对 video/audio 调 play()，推进事件循环后再次采集与截图
	if err := startPlayback(wv, cells); err != nil {
		return res, err
	}
	settleReal(wv, 1500*time.Millisecond)
	probes2, events2, err := collectPage(wv, cells)
	if err != nil {
		return res, err
	}
	playPixels, err := renderPixels(wv)
	if err != nil {
		return res, err
	}
	playPath := filepath.Join(outDir, fmt.Sprintf("matrix-%s-playing.png", sanitizeFile(cfg.Name)))
	if err := writePNG(playPath, playPixels, vw, pageH); err != nil {
		return res, err
	}
	res.PlayPNG = playPath
	_ = events

	// ③ 动图帧差异：再推进一段时间后重采样（L4 判据）
	settleReal(wv, 900*time.Millisecond)
	probes3, _, err := collectPage(wv, cells)
	if err != nil {
		return res, err
	}
	animPixels, err := renderPixels(wv)
	if err != nil {
		return res, err
	}

	geomPath := filepath.Join(outDir, fmt.Sprintf("geom-%s.json", sanitizeFile(cfg.Name)))
	if err := writeGeomJSON(geomPath, probes, probes2, events2, wv, cfg, res.Policy); err != nil {
		return res, err
	}
	res.GeomJSON = geomPath

	// ④ 判定
	byID := map[string]cellProbe{}
	for _, p := range probes {
		byID[p.ID] = p
	}
	byID2 := map[string]cellProbe{}
	for _, p := range probes2 {
		byID2[p.ID] = p
	}
	byID3 := map[string]cellProbe{}
	for _, p := range probes3 {
		byID3[p.ID] = p
	}
	_ = byID3
	for _, c := range cells {
		p, ok := byID[c.ID]
		if !ok {
			continue
		}
		r := judgeCell(c, man, p, byID2[c.ID], events2, stillPixels, playPixels, animPixels, vw, pageH)
		res.Cells = append(res.Cells, r)
	}
	return res, nil
}

// hostSamplesResolver 是 AllowHostResolved 配置的宿主资源通道：只提供
// samples 目录内的文件（file:// URL 或裸文件名），http(s) 一律不提供
// （TC-M-904：策略只放行宿主明确提供的资源，不代宿主联网）。
func hostSamplesResolver(samplesDir string) webkit.ResourceResolver {
	root, _ := filepath.Abs(samplesDir)
	return func(ref string) (string, bool) {
		p := ref
		switch {
		case strings.HasPrefix(ref, "file://"):
			p = webkit.FileURLPath(ref)
		case strings.HasPrefix(ref, "http://"), strings.HasPrefix(ref, "https://"):
			return "", false
		}
		abs, err := filepath.Abs(filepath.FromSlash(p))
		if err != nil {
			return "", false
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", false // 只放行样本目录内（路径穿越被拒，TC-M-705）
		}
		b, err := os.ReadFile(abs)
		if err != nil {
			return "", false
		}
		return string(b), true
	}
}

// collectPage 采集页面侧状态（几何 / IDL / 事件计数）。
func collectPage(wv *webkit.WebView, cells []cell) ([]cellProbe, map[string]map[string]int, error) {
	ids := make([]string, 0, len(cells))
	for _, c := range cells {
		ids = append(ids, jsString(c.ID))
	}
	script := `(function(){
		var ids = [` + strings.Join(ids, ",") + `];
		var out = {cells: [], events: window.__mediaEvents || {}, vw: window.innerWidth, vh: window.innerHeight,
		           doch: document.documentElement ? document.documentElement.scrollHeight : 0};
		for (var i = 0; i < ids.length; i++) {
			var id = ids[i];
			var el = document.getElementById(id);
			if (!el) { out.cells.push({id: id, tag: 'missing'}); continue; }
			var r = el.getBoundingClientRect();
			var num = function(v){ return (typeof v === 'number' && isFinite(v)) ? v : -1; };
			out.cells.push({
				id: id, tag: (el.tagName || '').toLowerCase(),
				x: r.left, y: r.top, w: r.width, h: r.height,
				complete: (el.complete === undefined ? 'n/a' : el.complete),
				nw: num(el.naturalWidth) < 0 ? 0 : (el.naturalWidth || 0),
				nh: num(el.naturalHeight) < 0 ? 0 : (el.naturalHeight || 0),
				rs: num(el.readyState),
				duration: num(el.duration),
				ct: num(el.currentTime),
				paused: (el.paused === undefined ? 'n/a' : el.paused),
				err: (el.error ? el.error.code : 0)
			});
		}
		return JSON.stringify(out);
	})()`
	v, err := wv.EvalJS(script)
	if err != nil {
		return nil, nil, err
	}
	var rep pageReport
	if err := json.Unmarshal([]byte(v.ToString()), &rep); err != nil {
		return nil, nil, fmt.Errorf("解析页面采集结果失败: %w（前 200 字符：%.200s）", err, v.ToString())
	}
	return rep.Cells, rep.Events, nil
}

// startPlayback 让页面里的 video/audio 开始播放（基线记录用；无解码器时
// 事件序列会停在 error/未就绪——这正是 G5/G6 要记录的现状）。
func startPlayback(wv *webkit.WebView, cells []cell) error {
	var ids []string
	for _, c := range cells {
		if c.Sample.Kind == "video" || c.Sample.Kind == "audio" {
			ids = append(ids, jsString(c.ID))
		}
	}
	if len(ids) == 0 {
		return nil
	}
	script := `(function(){
		var ids = [` + strings.Join(ids, ",") + `];
		for (var i = 0; i < ids.length; i++) {
			var el = document.getElementById(ids[i]);
			if (!el || typeof el.play !== 'function') { continue; }
			try { var p = el.play(); if (p && p.catch) { p.catch(function(){}); } } catch (e) {}
		}
		return 'ok';
	})()`
	_, err := wv.EvalJS(script)
	return err
}

// ── 像素 ──────────────────────────────────────────────────────────────

// settleReal 交替「推进虚拟事件循环」与「真实 sleep」：渲染层的异步图片
// 取字节跑在 goroutine 上（见 engine/rendering/backgroundimage.go 的
// loadBackgroundImageWith → fetchImageViaLoaderAsync），只推进虚拟时间不会
// 让它完成——必须让真实时间流逝。
//
// ★ 每步还要**渲染一帧**：本引擎的 `<img>` 加载由绘制路径发起，`load` /
//   `error` 契约由 Render 之后的 flushImageEvents 派发（见
//   webkit/webview.go）。只推进事件循环而不绘制，采集到的
//   complete/onload/onerror 会**全是 false/0**——那是测量伪影，不是引擎能力
//   （实测：修前 96 格里契约列几乎全 ❌，Browser 下 0 个 L3）。
func settleReal(wv *webkit.WebView, total time.Duration) {
	const step = 50 * time.Millisecond
	for elapsed := time.Duration(0); elapsed < total; elapsed += step {
		driveEventLoop(wv, int64(step/time.Millisecond))
		time.Sleep(step)
		if _, err := wv.Render(); err != nil {
			return
		}
	}
}

func renderPixels(wv *webkit.WebView) ([]byte, error) {
	px, err := wv.Render()
	if err != nil {
		return nil, err
	}
	w, h := wv.Width(), wv.Height()
	if len(px) < w*h*4 {
		return nil, fmt.Errorf("像素缓冲 %d 字节 < %dx%d", len(px), w, h)
	}
	return px, nil
}

// unpremul 把预乘 RGBA 还原为非预乘（与 renderPNG 同一套换算）。
func unpremul(px []byte, i int) [3]int {
	a := int(px[i+3])
	if a == 0 {
		return [3]int{0, 0, 0}
	}
	out := [3]int{}
	for c := 0; c < 3; c++ {
		v := int(px[i+c]) * 255 / a
		if v > 255 {
			v = 255
		}
		out[c] = v
	}
	return out
}

func pixelAt(px []byte, w, x, y int) [3]int {
	if x < 0 || y < 0 || x >= w {
		return [3]int{-1, -1, -1}
	}
	i := (y*w + x) * 4
	if i < 0 || i+3 >= len(px) {
		return [3]int{-1, -1, -1}
	}
	return unpremul(px, i)
}

func colorNear(got, want [3]int, tol int) bool {
	if got[0] < 0 {
		return false
	}
	for c := 0; c < 3; c++ {
		d := got[c] - want[c]
		if d < 0 {
			d = -d
		}
		if d > tol {
			return false
		}
	}
	return true
}

func writePNG(path string, px []byte, w, h int) error {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			rgb := unpremul(px, i)
			o := y*img.Stride + x*4
			img.Pix[o] = uint8(rgb[0])
			img.Pix[o+1] = uint8(rgb[1])
			img.Pix[o+2] = uint8(rgb[2])
			img.Pix[o+3] = px[i+3]
		}
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// comparePNGFiles 比较两张截图的差异像素比例（0 = 逐像素一致）——
// 决策 1 的一致性断言 TC-M-905（Browser vs Toolkit+AllowAll）。
func comparePNGFiles(a, b string) (float64, error) {
	ia, err := readPNGFile(a)
	if err != nil {
		return -1, err
	}
	ib, err := readPNGFile(b)
	if err != nil {
		return -1, err
	}
	if ia.Bounds() != ib.Bounds() {
		return -1, fmt.Errorf("尺寸不同：%v vs %v", ia.Bounds(), ib.Bounds())
	}
	total, diff := 0, 0
	bnd := ia.Bounds()
	for y := bnd.Min.Y; y < bnd.Max.Y; y++ {
		for x := bnd.Min.X; x < bnd.Max.X; x++ {
			r1, g1, b1, a1 := ia.At(x, y).RGBA()
			r2, g2, b2, a2 := ib.At(x, y).RGBA()
			total++
			if absDiff32(r1, r2) > 256 || absDiff32(g1, g2) > 256 ||
				absDiff32(b1, b2) > 256 || absDiff32(a1, a2) > 256 {
				diff++
			}
		}
	}
	if total == 0 {
		return -1, fmt.Errorf("空图")
	}
	return float64(diff) / float64(total), nil
}

func readPNGFile(p string) (image.Image, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func absDiff32(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

// ── 判定 ──────────────────────────────────────────────────────────────

// toleranceFor 返回像素容差：无损格式严格，有损/视频放宽。
func toleranceFor(sp sampleSpec) int {
	switch sp.Format {
	case "jpeg", "webp":
		return 26
	case "mp4", "webm":
		return 16
	case "svg", "png", "bmp", "gif", "ico":
		return 10
	}
	return 14
}

// judgeCell 按 §2 的判据给出一格的等级。
func judgeCell(c cell, man *manifestDoc, p, playing cellProbe, events map[string]map[string]int,
	still, play, anim []byte, vw, pageH int) cellResult {

	sp := c.Sample
	r := cellResult{ID: c.ID, Sample: sp.Name, Source: c.Source, Format: sp.Format, Kind: sp.Kind, Tag: p.Tag}
	r.BoxW, r.BoxH = p.W, p.H
	r.NaturalW, r.NaturalH = p.NaturalW, p.NaturalH
	r.Ready, r.Duration = p.ReadyState, p.Duration
	if b, ok := p.Complete.(bool); ok {
		r.Complete = b
	}
	if ev := events[c.ID]; ev != nil {
		r.OnLoad, r.OnError = ev["load"], ev["error"]
	}
	r.MetaOK = p.NaturalW > 0 && p.NaturalH > 0
	if sp.Kind == "video" || sp.Kind == "audio" {
		r.MetaOK = p.ReadyState >= 1 || p.Duration > 0
	}

	// 像素采样（四象限 / 单点）
	tol := toleranceFor(sp)
	allMatch, sampled := sampleCell(c, sp, man, still, vw)
	r.Sampled, r.DrawOK = sampled, allMatch

	// 契约：img → complete && onload；media → loadedmetadata
	if sp.Kind == "video" || sp.Kind == "audio" {
		ev := events[c.ID]
		r.ContrOK = ev != nil && (ev["loadedmetadata"] > 0 || ev["loadeddata"] > 0 || ev["canplay"] > 0)
	} else {
		r.ContrOK = r.Complete && r.OnLoad >= 1
	}

	// 动画/播放推进（L4）：静止态与播放/推进后两帧是否不同
	if sp.Kind == "animated" || sp.Kind == "video" {
		changed, note := framesDiffer(c, sp, man, still, play, anim, vw)
		r.AnimOK, r.AnimNote = changed, note
	}

	switch {
	case sp.Kind == "audio":
		if r.Ready >= 1 || r.Duration > 0 {
			r.Grade = "L1"
			r.Note = "音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无"
		} else {
			r.Grade = "L0"
			r.Note = "音频无解码/输出后端（预期现状）"
		}
	case sp.Kind == "broken":
		if !r.DrawOK {
			if r.OnError >= 1 {
				r.Grade = "L0"
				r.Note = "失败路径：未绘制 + onerror 已派发（契约正确）"
			} else {
				r.Grade = "L0"
				r.Note = "失败路径：未绘制，但 onerror 未派发（契约缺陷）"
			}
		} else {
			r.Grade = "L2"
			r.Note = "损坏样本被绘出（异常，需复核）"
		}
	default:
		switch {
		case r.DrawOK && r.ContrOK && r.AnimOK:
			r.Grade = "L4"
		case r.DrawOK && r.ContrOK:
			r.Grade = "L3"
		case r.DrawOK:
			r.Grade = "L2"
			if !r.MetaOK {
				r.Note = "画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷）"
			} else if !r.ContrOK {
				r.Note = "画得出但契约不完整（complete/onload）"
			}
		case r.MetaOK:
			r.Grade = "L1"
		default:
			r.Grade = "L0"
		}
	}
	if sp.Format == "avif" || sp.Format == "tiff" {
		r.Note = "预期不支持（L0，写入基线，不投入）"
	}
	if p.Tag == "missing" {
		r.Grade = "L0"
		r.Note = "元素缺失（页面构建异常）"
	}
	_ = tol
	return r
}

// sampleCell 在格内的采样点取色并与样本期望色比对。
func sampleCell(c cell, sp sampleSpec, man *manifestDoc, px []byte, vw int) (bool, []sampleDot) {
	if len(sp.Quad) > 0 {
		keys := []string{"tl", "tr", "bl", "br"}
		var dots []sampleDot
		ok := true
		for _, k := range keys {
			pt, has := man.QuadGeometry.SamplePoints[k]
			want, hasWant := sp.Quad[k]
			if !has || !hasWant {
				continue
			}
			x := c.X + pt[0]
			y := c.Y + pt[1]
			got := pixelAt(px, vw, x, y)
			matched := colorNear(got, [3]int{want[0], want[1], want[2]}, toleranceFor(sp))
			ok = ok && matched
			dots = append(dots, sampleDot{Name: k, X: x, Y: y, Got: got,
				Want: [3]int{want[0], want[1], want[2]}, Inside: true, Matched: matched})
		}
		return ok && len(dots) > 0, dots
	}
	if len(sp.Solid) == 3 {
		x := c.X + cellW/2
		y := c.Y + cellH/2
		got := pixelAt(px, vw, x, y)
		want := [3]int{sp.Solid[0], sp.Solid[1], sp.Solid[2]}
		matched := colorNear(got, want, toleranceFor(sp))
		return matched, []sampleDot{{Name: "center", X: x, Y: y, Got: got, Want: want, Inside: true, Matched: matched}}
	}
	// ★ 动图（GIF/WebP 多帧）：manifest 给的是**各帧颜色**，而截图停在哪一帧
	//   不确定（帧时机）——中心点接近**任一**帧色即算「画出来了」。
	//   此前这类样本（无 quad/solid）被直接判「未绘制」→ 动图全部压在 L1，
	//   而动画列明明显示帧在推进：测量伪影。
	if len(sp.FrameRGB) > 0 {
		x := c.X + cellW/2
		y := c.Y + cellH/2
		got := pixelAt(px, vw, x, y)
		want := [3]int{-1, -1, -1}
		matched := false
		for i, f := range sp.FrameRGB {
			if len(f) != 3 {
				continue
			}
			if i == 0 {
				want = [3]int{f[0], f[1], f[2]}
			}
			if colorNear(got, [3]int{f[0], f[1], f[2]}, toleranceFor(sp)) {
				want = [3]int{f[0], f[1], f[2]}
				matched = true
				break
			}
		}
		return matched, []sampleDot{{Name: "center-anyframe", X: x, Y: y, Got: got, Want: want, Inside: true, Matched: matched}}
	}
	// ★ 渐变样本（pattern=gradient）：manifest 没有单点期望色——按**渐变特征**
	//   判定：同一行上左/中/右三点都不是格子底色或页面白，且左右两点颜色差
	//   明显（渐变的方向性梯度）。同样修的是「无 quad/solid 就等于没画」的
	//   测量伪影。
	if sp.Pattern == "gradient" {
		y := c.Y + cellH/2
		xs := []int{c.X + cellW/8, c.X + cellW/2, c.X + cellW*7/8}
		names := []string{"left", "center", "right"}
		mask := [3]int{238, 238, 238} // .cell 背景色（未绘制时采样到的值）
		white := [3]int{255, 255, 255}
		var dots []sampleDot
		allDrawn := true
		for i, x := range xs {
			got := pixelAt(px, vw, x, y)
			drawn := !colorNear(got, mask, 6) && !colorNear(got, white, 6)
			allDrawn = allDrawn && drawn
			dots = append(dots, sampleDot{Name: names[i], X: x, Y: y, Got: got, Want: mask, Inside: true, Matched: drawn})
		}
		if allDrawn {
			d := 0
			for i := 0; i < 3; i++ {
				v := dots[0].Got[i] - dots[2].Got[i]
				if v < 0 {
					v = -v
				}
				if v > d {
					d = v
				}
			}
			if d < 16 {
				allDrawn = false // 三点同色：不是渐变（可能只是底色被别的元素盖住）
			}
		}
		return allDrawn, dots
	}
	return false, nil
}

// framesDiffer 判定「内容随时间变化」：在格中心取三点（静止/播放/再推进）
// 是否出现差异。
func framesDiffer(c cell, sp sampleSpec, man *manifestDoc, still, play, anim []byte, vw int) (bool, string) {
	x := c.X + cellW/2
	y := c.Y + cellH/2
	a := pixelAt(still, vw, x, y)
	b := pixelAt(play, vw, x, y)
	d := pixelAt(anim, vw, x, y)
	if a[0] < 0 || b[0] < 0 || d[0] < 0 {
		return false, "采样越界"
	}
	diff := func(p, q [3]int) int {
		m := 0
		for i := 0; i < 3; i++ {
			v := p[i] - q[i]
			if v < 0 {
				v = -v
			}
			if v > m {
				m = v
			}
		}
		return m
	}
	ctrl := 10
	switch {
	case diff(a, b) > ctrl || diff(b, d) > ctrl:
		return true, fmt.Sprintf("帧间差异 %d/%d（still→play→anim）", diff(a, b), diff(b, d))
	default:
		return false, fmt.Sprintf("三帧相同（差异 %d/%d）", diff(a, b), diff(b, d))
	}
}

// ── 落盘：几何 JSON / 报告 / 基线 ─────────────────────────────────────

func writeGeomJSON(path string, still, playing []cellProbe, events map[string]map[string]int,
	wv *webkit.WebView, cfg mediaConfig, policy string) error {

	doc := map[string]any{
		"config":      cfg.Name,
		"mode":        wv.Mode().String(),
		"policy":      policy,
		"viewport":    map[string]int{"w": wv.Width(), "h": wv.Height()},
		"still":       still,
		"playing":     playing,
		"events":      events,
		"generatedAt": time.Now().Format(time.RFC3339),
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, b, 0o644)
}

type baselineDoc struct {
	Version     int            `json:"version"`
	GeneratedAt string         `json:"generated_at"`
	Note        string         `json:"note"`
	Entries     []baselineItem `json:"entries"`
}

type baselineItem struct {
	Config    string `json:"config"`
	Sample    string `json:"sample"`
	Source    string `json:"source"`
	Format    string `json:"format"`
	Grade     string `json:"grade"`
	EdgeGrade string `json:"edge_grade,omitempty"`
}

func readBaseline(path string) (*baselineDoc, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d baselineDoc
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func writeBaseline(path string, results []configResult) error {
	d := baselineDoc{
		Version:     1,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Note: "媒体格式能力等级基线（引擎等级）。Edge 等级在 -media-edge 且本机有 Edge 时回填；" +
			"由 cmd/psai -media -media-update-baseline 生成（文档 §6.2 第 2 条）。",
	}
	for _, res := range results {
		for _, c := range res.Cells {
			d.Entries = append(d.Entries, baselineItem{
				Config: res.Config, Sample: c.Sample, Source: c.Source,
				Format: c.Format, Grade: c.Grade,
			})
		}
	}
	sort.Slice(d.Entries, func(i, j int) bool {
		a, b := d.Entries[i], d.Entries[j]
		if a.Config != b.Config {
			return a.Config < b.Config
		}
		if a.Sample != b.Sample {
			return a.Sample < b.Sample
		}
		return a.Source < b.Source
	})
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

var gradeOrder = map[string]int{"L0": 0, "L1": 1, "L2": 2, "L3": 3, "L4": 4}

func compareBaseline(base *baselineDoc, results []configResult) []string {
	type key struct{ config, sample, source string }
	want := map[key]string{}
	for _, e := range base.Entries {
		want[key{e.Config, e.Sample, e.Source}] = e.Grade
	}
	var regress []string
	for _, res := range results {
		for _, c := range res.Cells {
			k := key{res.Config, c.Sample, c.Source}
			w, ok := want[k]
			if !ok {
				continue // 新增条目不算回归
			}
			if gradeOrder[c.Grade] < gradeOrder[w] {
				regress = append(regress, fmt.Sprintf("%s / %s / %s：%s → %s（%s）",
					res.Config, c.Sample, c.Source, w, c.Grade, c.Note))
			}
		}
	}
	sort.Strings(regress)
	return regress
}

func writeMediaReport(path string, man *manifestDoc, results []configResult, base *baselineDoc, baseErr error,
	matrixPath string, adDiff float64, adErr error) error {
	var b strings.Builder
	commit := gitShortHash()
	fmt.Fprintf(&b, "# 媒体格式真实可用性报告（%s %s）\n\n", time.Now().Format("2006-01-02 15:04"), commit)
	b.WriteString("## 环境\n\n")
	fmt.Fprintf(&b, "配置：%s ｜ 引擎：%s ｜ 样本：本地脚本生成、不入库（决策 5）｜ 模型：L0–L4（文档 §2）\n\n",
		configNames(), commit)
	b.WriteString("复现：`python dev/media/gen_samples.py` → `cmd/psai -media`（本机按需，不入 CI 门禁——决策 6）\n\n")

	b.WriteString("## 总表\n\n")
	b.WriteString("| 配置 | 格式 | 样本 | 来源 | 加载 | 几何 | 绘制 | 契约 | 动画 | 等级 | 备注 |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|---|\n")
	// 按 (样本, 来源, 配置) 排序输出，便于横向对比四配置
	type row struct {
		res    configResult
		c      cellResult
		order  int
	}
	var rows []row
	for i, res := range results {
		for _, c := range res.Cells {
			rows = append(rows, row{res, c, i})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].c.Sample != rows[j].c.Sample {
			return rows[i].c.Sample < rows[j].c.Sample
		}
		if rows[i].c.Source != rows[j].c.Source {
			return rows[i].c.Source < rows[j].c.Source
		}
		return rows[i].order < rows[j].order
	})
	for _, r := range rows {
		load := mark(r.c.MetaOK)
		geom := mark(r.c.MetaOK)
		draw := mark(r.c.DrawOK)
		contr := mark(r.c.ContrOK)
		anim := "—"
		if r.c.Kind == "animated" || r.c.Kind == "video" {
			anim = mark(r.c.AnimOK)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | **%s** | %s |\n",
			r.res.Config, r.c.Format, r.c.Sample, r.c.Source, load, geom, draw, contr, anim, r.c.Grade,
			escapePipe(r.c.Note))
	}
	b.WriteString("\n")

	b.WriteString("## 缺陷清单（按影响排序）\n\n")
	type defect struct {
		config string
		c      cellResult
	}
	var defects []defect
	for _, res := range results {
		for _, c := range res.Cells {
			if c.Grade == "L0" && c.Kind != "audio" && c.Kind != "broken" {
				defects = append(defects, defect{res.Config, c})
			}
		}
	}
	if len(defects) == 0 {
		b.WriteString("（本次运行未发现 L0 缺陷——注意：L0 是否属缺陷取决于该配置的策略，见「基线差异」）\n\n")
	} else {
		b.WriteString("| 配置 | 样本 | 来源 | 现象 |\n|---|---|---|---|\n")
		for _, d := range defects {
			note := d.c.Note
			if note == "" {
				note = "无绘制、无几何"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", d.config, d.c.Sample, d.c.Source, escapePipe(note))
		}
		b.WriteString("\n")
	}

	b.WriteString("## 策略一致性（决策 1 · TC-M-905）\n\n")
	if adErr != nil {
		fmt.Fprintf(&b, "无法比对 Browser 与 Toolkit+AllowAll 的截图：`%v`\n\n", adErr)
	} else if adDiff < 0 {
		b.WriteString("未采集（配置数不足）\n\n")
	} else {
		verdict := "✅ 逐像素一致"
		if adDiff > 0 {
			verdict = "❌ 存在差异（策略开关引入了渲染差异）"
		}
		fmt.Fprintf(&b, "Browser vs Toolkit+AllowAll 首屏截图差异像素比例：%.4f%% —— %s\n\n", adDiff*100, verdict)
	}

	b.WriteString("## 与基线的差异\n\n")
	if baseErr != nil {
		fmt.Fprintf(&b, "基线不可读：`%v`——本次未做升降比对。\n\n", baseErr)
	} else {
		reg := compareBaseline(base, results)
		if len(reg) == 0 {
			b.WriteString("无等级下降（与基线一致）。\n\n")
		} else {
			for _, r := range reg {
				fmt.Fprintf(&b, "- ⚠️ %s\n", r)
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("## 原始证据索引\n\n")
	for _, res := range results {
		fmt.Fprintf(&b, "- %s：截图 `%s`", res.Config, relToCwd(res.StillPNG))
		if res.PlayPNG != "" {
			fmt.Fprintf(&b, "、播放态 `%s`", relToCwd(res.PlayPNG))
		}
		fmt.Fprintf(&b, "、几何/事件 `%s`\n", relToCwd(res.GeomJSON))
	}
	fmt.Fprintf(&b, "- 矩阵页：`%s`（与样本同级，文档相对路径来源依赖此位置）\n", relToCwd(matrixPath))
	fmt.Fprintf(&b, "- 样本清单：`%s`\n", filepath.Join("dev", "media", "samples", "manifest.json"))
	b.WriteString("\n> 截图为预乘 RGBA 反预乘后的 sRGB；判定基于像素采样（非人眼），截图供 `read_image` 复核。\n")

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func configOf(results []configResult, c cellResult) string {
	for _, res := range results {
		for _, x := range res.Cells {
			if x.ID == c.ID && x.Sample == c.Sample && x.Source == c.Source {
				return res.Config
			}
		}
	}
	return "?"
}

func configNames() string {
	var names []string
	for _, c := range mediaConfigs() {
		names = append(names, c.Name)
	}
	return strings.Join(names, " / ")
}

func mark(ok bool) string {
	if ok {
		return "✅"
	}
	return "❌"
}

func escapePipe(s string) string { return strings.ReplaceAll(s, "|", "\\|") }

func sanitizeFile(s string) string {
	r := strings.NewReplacer("+", "-", " ", "_")
	return r.Replace(s)
}

func relToCwd(p string) string {
	if p == "" {
		return ""
	}
	if rel, err := filepath.Rel(mustCwd(), p); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(p)
}

func mustCwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func gitShortHash() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "(unknown)"
	}
	return strings.TrimSpace(string(out))
}

// runEdgeComparison 尽力而为的 Edge 双端对照（决策 2）：找不到 Edge 就标
// SKIP(no-edge)（不得默认判通过）。找到时对矩阵页截图并保存，供人工比对。
func runEdgeComparison(results []configResult, matrixPath string, vw, pageH int, outDir string) string {
	exe := findEdge()
	if exe == "" {
		return "SKIP(no-edge)"
	}
	shot := filepath.Join(outDir, "edge-matrix.png")
	cmd := exec.Command(exe, "--headless=new", "--disable-gpu", "--hide-scrollbars",
		fmt.Sprintf("--window-size=%d,%d", vw, pageH),
		"--screenshot="+shot, fileURLOf(matrixPath))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Sprintf("SKIP(edge-failed: %v / %s)", err, strings.TrimSpace(string(out)))
	}
	if _, err := os.Stat(shot); err != nil {
		return "SKIP(edge-no-screenshot)"
	}
	return "已采集 " + relToCwd(shot) + "（等级对照需人工复核或后续扩展）"
}

func findEdge() string {
	cands := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Edge", "Application", "msedge.exe"),
	}
	for _, c := range cands {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	if p, err := exec.LookPath("msedge"); err == nil {
		return p
	}
	return ""
}
