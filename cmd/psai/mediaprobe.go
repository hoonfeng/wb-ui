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
	"net/url"
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

// mediaPhase 是 manifest 里 video 样本的分段期望色（`phases` 字段），形如
// [start, end, [r,g,b]]。静止态 currentTime=0 落在第一段，因此判据取首段色。
type mediaPhase struct {
	From, To float64
	RGB      [3]int
}

func (p *mediaPhase) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if len(raw) != 3 {
		return fmt.Errorf("phase 应为 [start, end, color]，实际 %d 项", len(raw))
	}
	if err := json.Unmarshal(raw[0], &p.From); err != nil {
		return err
	}
	if err := json.Unmarshal(raw[1], &p.To); err != nil {
		return err
	}
	return json.Unmarshal(raw[2], &p.RGB)
}

// sampleSpec 是 manifest.json 的一个样本（由 gen_samples.py 生成）。
type sampleSpec struct {
	File     string           `json:"file"`
	Name     string           `json:"name"`
	Kind     string           `json:"kind"` // image|animated|svg|svg-inline|video|audio|broken|huge|skipped
	Format   string           `json:"format"`
	Mime     string           `json:"mime"`
	Width    int              `json:"width"`
	Height   int              `json:"height"`
	Pattern  string           `json:"pattern"` // quad|solid|gradient
	Quad     map[string][]int `json:"quad"`
	Solid    []int            `json:"solid"`
	Frames   int              `json:"frames"`
	Delays   []int            `json:"delays"`
	FrameRGB [][]int          `json:"frame_colors"`
	Phases   []mediaPhase     `json:"phases"`
	Skip     string           `json:"skip"`
	Inline   string           `json:"inline"`
	Note     string           `json:"note"`
	// AudioHz 是音频样本的期望基频（Hz，A3 判据 A 的比对基准）。gen_samples.py
	// 早就写进 manifest 了（`hz=440`），此前探针没解析——判据 A 就缺了这一半。
	AudioHz   float64 `json:"hz"`
	Generated bool    `json:"-"`
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
	Cells  []cellProbe               `json:"cells"`
	Events map[string]map[string]int `json:"events"`
	VW     float64                   `json:"vw"`
	VH     float64                   `json:"vh"`
	DocH   float64                   `json:"doch"`
}

// cellResult 是一格的最终判定。
type cellResult struct {
	ID       string      `json:"id"`
	Sample   string      `json:"sample"`
	Source   string      `json:"source"`
	Format   string      `json:"format"`
	Kind     string      `json:"kind"`
	Tag      string      `json:"tag"`
	BoxW     float64     `json:"box_w"`
	BoxH     float64     `json:"box_h"`
	NaturalW float64     `json:"natural_w"`
	NaturalH float64     `json:"natural_h"`
	Complete bool        `json:"complete"`
	Ready    float64     `json:"ready_state"`
	Duration float64     `json:"duration"`
	OnLoad   int         `json:"onload"`
	OnError  int         `json:"onerror"`
	// ErrCode 是元素 error.code 的实测值（media = MediaError.code；img 无此 IDL
	// 属性，恒 0）。它只用于报告备注的实测佐证（原始值见 geom-*.json 的
	// still[].err）；**归因本身**按「配置策略 + 来源形态」判定（见 mediaDenialNote）。
	ErrCode  float64     `json:"err_code,omitempty"`
	Sampled  []sampleDot `json:"sampled"`
	DrawOK   bool        `json:"draw_ok"`
	MetaOK   bool        `json:"meta_ok"`
	ContrOK  bool        `json:"contract_ok"`
	AnimOK   bool        `json:"anim_ok"`
	Grade    string      `json:"grade"`
	Note     string      `json:"note"`
	AnimNote string      `json:"anim_note,omitempty"`
	HTTPNote string      `json:"-"`
	// 音频取证（A3-1；kind=audio 时有效）：
	//   PCMFrames/PCMPeakHz/PCMPeakMag —— 判据 A（宿主输出回调的 PCM 频谱）；
	//   CTMid —— TC-M-602 定点（play() 之后 500ms 的 currentTime）；
	//   AudioOutput —— 本次运行是否有内置输出设备（支持矩阵的逐格依据）。
	PCMFrames   int64   `json:"pcm_frames,omitempty"`
	PCMPeakHz   float64 `json:"pcm_peak_hz,omitempty"`
	PCMPeakMag  float64 `json:"pcm_peak_mag,omitempty"`
	CTMid       float64 `json:"ct_mid,omitempty"`
	AudioOutput bool    `json:"audio_output,omitempty"`
}

// sampleDot 是一个采样点的实测色与期望色。
type sampleDot struct {
	Name    string `json:"name"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
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
	// Audio 是本配置下各音频格的取证（A3-1）：报告小节与逐格判定都用它。
	Audio []audioCellEvidence `json:"audio,omitempty"`
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
	// 注意：动画样本（GIF/WebP）在两个配置下各自独立加载，**截图时刻的动画相位
	// 天然可能不同**（帧相位差，与策略无关）。因此判定基于「排除动画样本格后」
	// 的差异，全图差异同时如实给出（见 writeMediaReport）。
	adDiff, adDiffExcl, adErr := -1.0, -1.0, error(nil)
	animRects := 0
	if len(results) == 4 {
		rects, n, rerr := animatedCellRects(results[0])
		if rerr != nil {
			fmt.Printf("  策略一致性：动画格矩形读取失败——%v\n", rerr)
			rects = nil
		}
		animRects = n
		adDiff, adDiffExcl, adErr = comparePNGFilesExcluding(results[0].StillPNG, results[3].StillPNG, rects)
	}

	baseline, berr := readBaseline(o.Baseline)
	reportPath := filepath.Join(outDir, "report.md")
	// 判据 B（可选）：环回录音复核。本机没有环回设备时返回 SKIP(no-loopback)，
	// 不阻塞主线验收（与 Edge 对照的 SKIP(no-edge) 同一处理方式）。
	expectHz := 0.0
	for _, s := range man.Samples {
		if s.Kind == "audio" && s.AudioHz > 0 {
			expectHz = s.AudioHz
			break
		}
	}
	if expectHz == 0 {
		expectHz = 440
	}
	loopback := probeLoopback(expectHz, 1.0)
	fmt.Printf("  判据 B（环回录音）：%s\n", loopback.Status)
	if err := writeMediaReport(reportPath, man, results, baseline, berr, matrixPath, adDiff, adDiffExcl, animRects, adErr, loopback); err != nil {
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
			// audio 无视觉内容：用包裹 div 承载采样框（期望：未绘制）。
			// ★ 探针采集用的 id 必须给**音频元素本身**——媒体状态挂在它上面。
			//   此前 div 用 `id`、audio 用 `id+"_a"`，采集到的是 div：
			//   readyState/duration 一律读不到 → 「加载/几何」两列恒 ❌、音频恒判
			//   L0，掩盖了「元数据其实可加载」（TC-M-601 的 L1）这一真实状态。
			fmt.Fprintf(&b, "<div class=\"cell\" id=%s style=\"%s\"></div>\n",
				htmlAttrValue(c.ID+"_box"), style)
			fmt.Fprintf(&b, "<audio id=%s style=\"position:absolute;left:%dpx;top:%dpx;width:1px;height:1px\" src=%s "+
				"onloadedmetadata=\"mvNote(%s,'loadedmetadata')\" onerror=\"mvNote(%s,'error')\"></audio>\n",
				htmlAttrValue(c.ID), c.X, c.Y, htmlAttrValue(c.URL),
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
				tag, htmlAttrValue(c.ID), htmlAttrValue(c.URL), style, events, tag)
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
// （双引号）传参 → 生成 `onload="mvNote("c0",'load')"`，属性在第二个双引号
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

// htmlAttrValue 把字符串编成**双引号包裹的 HTML 属性值**（敏感字符走实体）。
//
// ★ 不要用 jsString 拼 HTML 属性：它按 JS 字面量规则把 `"` 写成 `\"`，而 HTML
// 解析器不认这个转义——属性值会在第一个 `"` 处提前闭合（详见 dataURIText）。
func htmlAttrValue(s string) string { return `"` + htmlEscape(s) + `"` }

// dataURIText 生成 `data:<mime>,<payload>` 形式的 data URI（非 base64 载荷）。
//
// ★ 探针自身缺陷（**不是**引擎缺陷）：此前只把 `#` 换成 `%23`，SVG 文本里的
// 双引号原样进入 HTML 属性，而属性值又是 jsString（JS 字面量风格）拼的，于是
// 生成 `src="data:image/svg+xml,<svg xmlns=\"http://…\" width=\"120\" …>"`：
// HTML 解析器在第一个 `"` 处**提前结束属性值**，src 实际只剩
// `data:image/svg+xml,<svg xmlns=` → 该格必然 L0。曾据此把「内联 SVG 不绘制」
// 误判为引擎缺口 D9（引擎的 percent 解码路径本来就是通的）。
//
// 修法按 RFC 2397：非 base64 载荷一律 percent 编码（`+` 保持字面，**不是**
// 空格——那是 form-encoding 的约定），URI 里因此不再有 HTML 敏感字符。
func dataURIText(mime, text string) string {
	return "data:" + mime + "," + url.PathEscape(text)
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
	// 主线 A3-1 的宿主音频链路：ffmpeg 解码 s16le →（waveOut 输出 + tap）。
	// tap 是判据 A 的取证口：它拿到的就是**即将写进输出设备的同一份 PCM**。
	audioTap := newAudioTapCollector()
	mediaAudio := app.InstallMediaAudio(wv, "")
	mediaAudio.SetTap(audioTap.tap)

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
	playStart := time.Now()
	if err := startPlayback(wv, cells); err != nil {
		return res, err
	}
	// ★ TC-M-602 定点：play() 之后 **500ms** 采一次 currentTime（音频为主时钟的
	//   时序证据）。此前只有「1.5 秒后的终态」一个采样点，看不出时钟是否真的跟随
	//   输出位置——终态到 1.0 用挂钟累加同样做得到。
	//
	//   ★ 这里必须按**真实时间**到期，不能用 settleReal 数迭代：settleReal 每步
	//     都要渲染，而矩阵页渲染一次要几十毫秒（12 个 <video> 在抽帧），于是
	//     「50ms × 10 次」实际会跑成 800ms~1s——定点值随格式的解码启动速度整体
	//     漂移（实测 wav 1.00 / mp3 0.84 / ogg 0.68 / m4a 0.51），其中 mp3 因此
	//     被误判成「播放时钟与输出不同步」。这是测量伪影，不是引擎缺陷：同一批
	//     运行的 ctEnd 全部为 1.00（真的播完了）。
	if remain := 500*time.Millisecond - time.Since(playStart); remain > 0 {
		settleWall(wv, remain)
	}
	// 再等各格的首块 PCM 到齐（并发播放时排在后面的格子可能还没轮到），然后让
	// 真实时间流逝一小段再采样：定点判据的零点是**各格自己的首块时刻**，采样点
	// 只需落在「时钟已经起步」之后。
	waitAudioFirstPCM(wv, audioTap, cells, 2*time.Second)
	settleWall(wv, 150*time.Millisecond)
	sampleAt := time.Now()
	midProbes, _, err := collectPage(wv, cells)
	if err != nil {
		return res, err
	}
	settleReal(wv, 1000*time.Millisecond)
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

	// ③ 动图帧差异（L4 判据）：**等间隔连拍多帧**。
	//
	// ★ 为什么不是「再等 900ms 拍一张」（探针自身缺陷）：两帧像素是否不同取决于
	//   「采样间隔 mod 动图循环周期」——3 帧 ×100ms 的 GIF 周期正是 300ms，
	//   900ms 恰好是它的 3 倍，两次采样落在**同一帧**、像素完全相同 → 判成
	//   「未推进」。实测同一 HEAD 连跑两次全量：动图项的 L4/L3 会摇摆
	//   （`anim-3frames-rgb` 一次 L4、一次 L3，`anim-2frames.webp` 亦同），
	//   基线因此每次报警「等级下降」。
	//   改为等间隔连拍：步长取与常见帧时长**不成整数倍**的 130ms，判据放宽为
	//   「任意两帧不同」（见 framesDiffer），采样相位不再决定结论。
	//   ★ 但「等间隔」本身仍有洞（2026-10 实测，第 8 处测量伪影）：名义步长不成整数倍
	//   不等于**实际**步长不成整数倍——实际步长 = 名义步长 + 每步的渲染开销，可能恰好
	//   贴近某个样本的循环周期。`anim-uneven-delay.gif` 的帧延迟是 [50,200,100] ⇒ 周期
	//   350ms：全量第 3 跑时 Browser 配置下它的 data/file/rel 三格**同时**报「最大差异 0」
	//   （L4→L3），而同一轮其余动图样本全 L4、同一 HEAD 前两跑均正常（抖动特征，与 §9.7
	//   记录一致）。处置：步长改为**抖动**（两轮基步长不同 + 每步叠加不同抖动，相位锁不死）。
	//   ★ 两轮连拍（每轮 6 帧 ×130ms，轮间 250ms 让步，总窗口≈1.8s）：单轮
	//   780ms 窗口在满负载时（尤其排在最后的 Toolkit+AllowAll）仍可能整窗落在
	//   同一动画帧——实测连跑 6 次全量有 3 次报「动画未推进」的假降级，而把
	//   同一样本单独复跑恒为 L4（差异恒为 0，说明是采样期间动画根本没被调度，
	//   不是判据算错）。加一轮让步连拍把跨帧机会翻倍；**真静止的样本两轮也都
	//   无差异，判据不会因此放宽**。
	//   ★ 连拍前先停掉音频播放并让主循环喘息：见 stopAudioPlayback 的实测说明
	//   （音频播放期抢占主循环 → 动图帧不被调度 → 假降级），这是测量耦合而非
	//   样本缺陷。audio 元素不参与动图判据，停掉它不影响任何已有判据。
	stopAudioPlayback(wv, cells)
	settleReal(wv, 300*time.Millisecond)
	// ★ Q7-C（采样伪影根本解法，2026-10-07）：步长由**样本自己声明的帧时长**驱动，
	//   不再是全局固定基步长（推导与取舍见 mediaAnimSteps 的说明）。判据不变。
	animSteps := mediaAnimSteps(man)
	if len(animSteps) == 0 { // 无动画样本（或 manifest 无 delays）：退回旧口径，行为不变
		animSteps = []time.Duration{130 * time.Millisecond, 190 * time.Millisecond}
	}
	animFrames := make([][]byte, 0, len(animSteps)+1)
	for i, d := range animSteps {
		if i > 0 && i%6 == 0 { // 每 6 步让步一次：满负载时给主循环喘息（与旧版两轮让步等效）
			settleReal(wv, 250*time.Millisecond)
		}
		settleReal(wv, d)
		px, err := renderPixels(wv)
		if err != nil {
			return res, err
		}
		animFrames = append(animFrames, px)
	}
	probes3, _, err := collectPage(wv, cells)
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

	// 音频取证（A3-1）：PCM 来自 tap，时钟来自上面两个采集点（500ms / 终态）。
	midByID := map[string]cellProbe{}
	for _, p := range midProbes {
		midByID[p.ID] = p
	}
	audioEvidence := collectAudioEvidence(cfg.Name, cells, audioTap, midByID, byID2, app.AudioOutputAvailable(), sampleAt)
	audioByID := map[string]audioCellEvidence{}
	for _, ev := range audioEvidence {
		audioByID[ev.ID] = ev
	}
	res.Audio = audioEvidence

	for _, c := range cells {
		p, ok := byID[c.ID]
		if !ok {
			continue
		}
		r := judgeCell(c, man, p, byID2[c.ID], events2, stillPixels, playPixels, animFrames, vw, pageH,
			audioByID[c.ID], res.Policy)
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

// stopAudioPlayback 停掉所有 <audio> 元素的播放（pause，**不动 video**）。
//
// 用途：动图判据（帧间差异）采样前消除音频播放对主循环的抢占。音频会话在
// 播放期要持续做解码推送、waveOut 写入与 ended 巡检，满负载时会把动画帧的
// 调度挤到采样窗口之外，测出来就是「所有帧采样点相同（最大差异 0，共 14 帧）」
// 的**假降级**——实测含音频的全量连跑 8 次有 4 次命中（全落在最后跑的
// Toolkit+AllowAll），而只跑动画样本（不带音频）连跑 3 次零失败、同样本单独
// 复跑恒为 L4。动图是否推进与音频无因果关系，停掉音频再采样才是干净的测量。
//
// ★ video 绝不能一起 pause：动图判据同时覆盖 video 的「播放推进」，停掉它
// 等于把判据废掉。audio 元素不参与动图判据（其 PCM/时钟证据在更早的采样阶段
// 就已固化进 audioByID，且 pause 不改变 currentTime）。
func stopAudioPlayback(wv *webkit.WebView, cells []cell) {
	var ids []string
	for _, c := range cells {
		if c.Sample.Kind == "audio" {
			ids = append(ids, jsString(c.ID))
		}
	}
	if len(ids) == 0 {
		return
	}
	script := `(function(){
		var ids = [` + strings.Join(ids, ",") + `];
		for (var i = 0; i < ids.length; i++) {
			var el = document.getElementById(ids[i]);
			if (!el || typeof el.pause !== 'function') { continue; }
			try { el.pause(); } catch (e) {}
		}
		return 'ok';
	})()`
	_, _ = wv.EvalJS(script)
}

// ── 像素 ──────────────────────────────────────────────────────────────

// settleReal 交替「推进虚拟事件循环」与「真实 sleep」：渲染层的异步图片
// 取字节跑在 goroutine 上（见 engine/rendering/backgroundimage.go 的
// loadBackgroundImageWith → fetchImageViaLoaderAsync），只推进虚拟时间不会
// 让它完成——必须让真实时间流逝。
//
// ★ 每步还要**渲染一帧**：本引擎的 `<img>` 加载由绘制路径发起，`load` /
//
//	`error` 契约由 Render 之后的 flushImageEvents 派发（见
//	webkit/webview.go）。只推进事件循环而不绘制，采集到的
//	complete/onload/onerror 会**全是 false/0**——那是测量伪影，不是引擎能力
//	（实测：修前 96 格里契约列几乎全 ❌，Browser 下 0 个 L3）。
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

// settleWall 让**真实时间**流逝 d 毫秒（推进事件循环，但**不渲染**）。
//
// 与 settleReal 的分工：
//
//	settleReal 用于「等资源加载 + 契约事件派发」——它必须每步渲染，因为本引擎的
//	  `<img>`/SVG 加载与 complete/load 派发由绘制路径驱动；
//	settleWall 用于**测量**（音频的 TC-M-602 定点）：它要求间隔就是墙钟上的 d，
//	  渲染在这里只是干扰（矩阵页渲染一次几十毫秒，会把 500ms 拉成 800ms+），
//	  而音频推送跑在宿主自己的 goroutine 上，与渲染无关，不推进绘制也照常播。
func settleWall(wv *webkit.WebView, d time.Duration) {
	deadline := time.Now().Add(d)
	for {
		remain := time.Until(deadline)
		if remain <= 0 {
			return
		}
		step := 10 * time.Millisecond
		if remain < step {
			step = remain
		}
		driveEventLoop(wv, int64(step/time.Millisecond))
		time.Sleep(step)
	}
}

// mediaAnimSteps 按**样本自己声明的帧时长**生成动图连拍步长（Q7-C：采样伪影根本解法）。
//
// 背景（§9.9 第 8 处测量伪影）：旧实现是「全局调一个步长」——基步长 130/190ms +
// 固定抖动序列，对所有样本一视同仁。洞在于**名义步长不成整数倍 ≠ 实际步长不成整数倍**
// （实际步长 = 名义步长 + 每步渲染开销），`anim-uneven-delay.gif`（Delays=[50,200,100]
// ⇒ 周期 350ms）因此被锁相：全量第 3 跑时它的 data/file/rel 三格同时报「最大差异 0」
// （L4→L3），而同轮其余动图全 L4。
//
// 现在改为「按样本声明的帧时长驱动」：对每个**不同周期** P = Σ Delays 生成 3 个步长
// P×r，r ∈ {0.37, 0.61, 0.83}——互不相同，且刻意避开 1/2、1/3、2/3 这类对称点
//
//	（否则两帧样本互换相位后又落回同帧），各再叠加互不相同的抖动。步长按周期**轮转排列**，
//	使每个周期在整个采样窗口里都有 3 个独立的相位推进量；窗口总长（≈2.1s）远大于最长
//	周期（400ms）⇒ 覆盖完整循环周期。判据不变（framesDiffer 仍是「任意两帧不同」），
//	真静止的样本依旧无差异。
func mediaAnimSteps(man *manifestDoc) []time.Duration {
	if man == nil {
		return nil
	}
	const maxPeriods = 6 // 上限：周期种类过多时只取前几种，避免连拍窗口无限膨胀
	var periods []time.Duration
	seen := map[int64]bool{}
	for _, s := range man.Samples {
		if s.Kind != "animated" || len(s.Delays) == 0 {
			continue
		}
		sum := 0
		for _, d := range s.Delays {
			sum += d
		}
		if sum <= 0 || seen[int64(sum)] {
			continue
		}
		seen[int64(sum)] = true
		periods = append(periods, time.Duration(sum)*time.Millisecond)
		if len(periods) >= maxPeriods {
			break
		}
	}
	if len(periods) == 0 {
		return nil
	}
	ratios := [3]float64{0.37, 0.61, 0.83}
	jitter := [3]time.Duration{-17 * time.Millisecond, 23 * time.Millisecond, -9 * time.Millisecond}
	perPeriod := make([][3]time.Duration, 0, len(periods))
	for _, p := range periods {
		var st [3]time.Duration
		for i, r := range ratios {
			d := time.Duration(float64(p)*r) + jitter[i]
			if d < 40*time.Millisecond { // 下限：步长过短时渲染开销占主导，相位不可控
				d = 40 * time.Millisecond
			}
			st[i] = d
		}
		perPeriod = append(perPeriod, st)
	}
	out := make([]time.Duration, 0, len(perPeriod)*len(ratios))
	for round := 0; round < len(ratios); round++ { // 轮转：每个周期每轮贡献一个步长
		for _, st := range perPeriod {
			out = append(out, st[round])
		}
	}
	return out
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

// animatedCellRects 返回给定配置下「动画样本格」的矩形集合与数量。
// 用途：把 TC-M-905（Browser ↔ Toolkit+AllowAll）的差异归因到动画帧相位差——
// 动画样本在两个配置下各自独立加载与截图，停在不同帧，与策略开关无关。
func animatedCellRects(res configResult) ([]image.Rectangle, int, error) {
	data, err := os.ReadFile(res.GeomJSON)
	if err != nil {
		return nil, 0, err
	}
	var doc struct {
		Still []struct {
			ID string `json:"id"`
			X  int    `json:"x"`
			Y  int    `json:"y"`
			W  int    `json:"w"`
			H  int    `json:"h"`
		} `json:"still"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, 0, err
	}
	isAnim := map[string]bool{}
	for _, c := range res.Cells {
		if c.Kind == "animated" {
			isAnim[c.ID] = true
		}
	}
	var rects []image.Rectangle
	for _, s := range doc.Still {
		if isAnim[s.ID] {
			rects = append(rects, image.Rect(s.X, s.Y, s.X+s.W, s.Y+s.H))
		}
	}
	return rects, len(rects), nil
}

// comparePNGFilesExcluding 逐像素比较两张 PNG，返回「全图差异比例」与
// 「排除 exclude 矩形后的差异比例」。后者用于判定策略开关是否引入渲染差异：
// 动画样本格的帧相位差天然非零，不应计入该判定（全图值仍如实给出）。
func comparePNGFilesExcluding(a, b string, exclude []image.Rectangle) (allDiff, keptDiff float64, err error) {
	ia, err := readPNGFile(a)
	if err != nil {
		return -1, -1, err
	}
	ib, err := readPNGFile(b)
	if err != nil {
		return -1, -1, err
	}
	if ia.Bounds() != ib.Bounds() {
		return -1, -1, fmt.Errorf("尺寸不同：%v vs %v", ia.Bounds(), ib.Bounds())
	}
	bnd := ia.Bounds()
	total, diff, keptTotal, keptBad := 0, 0, 0, 0
	for y := bnd.Min.Y; y < bnd.Max.Y; y++ {
		for x := bnd.Min.X; x < bnd.Max.X; x++ {
			r1, g1, b1, a1 := ia.At(x, y).RGBA()
			r2, g2, b2, a2 := ib.At(x, y).RGBA()
			total++
			bad := absDiff32(r1, r2) > 256 || absDiff32(g1, g2) > 256 ||
				absDiff32(b1, b2) > 256 || absDiff32(a1, a2) > 256
			if bad {
				diff++
			}
			if !inRects(exclude, x, y) {
				keptTotal++
				if bad {
					keptBad++
				}
			}
		}
	}
	if total == 0 || keptTotal == 0 {
		return -1, -1, fmt.Errorf("空图")
	}
	return float64(diff) / float64(total), float64(keptBad) / float64(keptTotal), nil
}

func inRects(rects []image.Rectangle, x, y int) bool {
	for _, r := range rects {
		if x >= r.Min.X && x < r.Max.X && y >= r.Min.Y && y < r.Max.Y {
			return true
		}
	}
	return false
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

// mediaErrName 把 MediaError.code 映射为规范常量名（报告附注用）。数值与引擎
// 一致（engine/js/bindings/media_element.go：1=ABORTED、2=NETWORK、3=DECODE、
// 4=SRC_NOT_SUPPORTED）。
func mediaErrName(code int) string {
	switch code {
	case 1:
		return "MEDIA_ERR_ABORTED"
	case 2:
		return "MEDIA_ERR_NETWORK"
	case 3:
		return "MEDIA_ERR_DECODE"
	case 4:
		return "MEDIA_ERR_SRC_NOT_SUPPORTED"
	}
	return ""
}

// mediaDenialNote 给出「引用落 L0（未加载）」的**确定性**归因文案。
//
// 为什么不能只读 error.code：引擎侧 failUnavailableSrc 对两类「源不可用」派发
// **同一个** MEDIA_ERR_SRC_NOT_SUPPORTED——① 资源策略拒绝（MediaSrcAllowed
// 判否）② http(s) 无网络栈；页面侧 error.code 分不出两者。故归因按「配置策略 +
// 来源形态」判定，error.code 只作实测佐证（有则附注）。
//
// 覆盖范围（确定性 100%）：deny-external 下 file/rel 引用必被拒——探针在该配置
// **不装配**宿主 resolver（hostSamplesResolver 只在 allow-host-resolved 装配），
// allowsExternalURLs 为假即拒；而 data: 与策略解耦恒放行，故不在本列。
//
// ok=false 表示「不是策略拒绝」——调用方应改用后端缺失 / 样本等其他原因归因，
// 不要把「策略正确拒绝」写成「没有后端」（同一配置的 data: 行即证明后端存在）。
func mediaDenialNote(policy, source string, errCode float64) (string, bool) {
	if policy != webkit.DenyExternal.String() {
		return "", false
	}
	switch source {
	case "file", "rel":
	default:
		return "", false
	}
	n := "资源策略 deny-external 拒绝该引用（预期，非缺陷）"
	if errCode > 0 {
		n += fmt.Sprintf("；实测元素 error.code=%.0f（%s）", errCode, mediaErrName(int(errCode)))
	}
	return n, true
}

// judgeCell 按 §2 的判据给出一格的等级。
func judgeCell(c cell, man *manifestDoc, p, playing cellProbe, events map[string]map[string]int,
	still, play []byte, animFrames [][]byte, vw, pageH int, audio audioCellEvidence,
	policy string) cellResult {

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
	r.ErrCode = p.ErrCode
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
		changed, note := framesDiffer(c, sp, man, still, play, animFrames, vw)
		r.AnimOK, r.AnimNote = changed, note
	}

	switch {
	case sp.Kind == "audio":
		r.PCMFrames = audio.frames
		r.PCMPeakHz = audio.peakHz
		r.PCMPeakMag = audio.peakMag
		r.CTMid = audio.ctMid
		r.AudioOutput = audio.hasOutput
		// 音频的分级判据（文档 §2 的 L4「动态播放」对音频即「真实发声」）：
		//   L4-S：PCM 真的交付了 + 频谱主峰与样本频率一致（判据 A）+ 播放时钟与
		//         输出一致（TC-M-602 定点）；
		//   L3  ：PCM 有交付但 L4-S 判据不全（例如频谱偏离，说明链路仍有问题）；
		//   L1  ：只有元数据（没有输出后端 / 资源无音轨）；
		//   L0  ：连元数据都没有。
		switch {
		case audio.frames > 0 && audio.peakOK() && audio.clockOK(r.Duration):
			r.Grade = "L4"
			r.Note = fmt.Sprintf("音频输出：交付 %d 帧、PCM 主峰 %.2fHz（期望 %.0fHz，判据 A）；"+
				"play() 后 500ms currentTime=%.2fs、结束时 %.2fs（TC-M-602）",
				audio.frames, audio.peakHz, audio.ExpectHz, audio.ctMid, audio.ctEnd)
		case audio.frames > 0:
			why := "PCM 主峰偏离期望频率"
			if audio.peakOK() {
				why = "播放时钟与输出位置不同步"
			}
			r.Grade = "L3"
			r.Note = fmt.Sprintf("有 PCM 交付（%d 帧、主峰 %.2fHz）但 L4-S 判据不全：%s",
				audio.frames, audio.peakHz, why)
		case r.Ready >= 1 || r.Duration > 0:
			r.Grade = "L1"
			r.Note = "音频可加载（元数据可用）；未采到 PCM（无音轨 / 无输出后端）"
		default:
			r.Grade = "L0"
			// L0 的归因必须如实：deny-external 下 file/rel 引用是被**资源策略**
			// 拒绝的（引擎 finishLoad 判否 → failUnavailableSrc），不是「没有
			// 解码/输出后端」——音频后端全局存在（同配置的 data: 行即 L4，且
			// audio.hasOutput 记录了本机后端实测值）。
			if n, ok := mediaDenialNote(policy, c.Source, p.ErrCode); ok {
				r.Note = n
			} else if !audio.hasOutput {
				r.Note = "本机无内置音频输出后端（预期现状）"
			} else {
				r.Note = "音频未加载（元数据不可用）：输出后端存在、策略未拒该引用 → 需复核"
			}
		}
	case sp.Kind == "broken":
		if !r.DrawOK {
			r.Grade = "L0"
			// 归因先区分「资源策略拒绝」：deny-external 下 file/rel 引用在到达解码器
			// 之前就被拒，这一格的 onerror 派发与否与「样本本身损坏」无关 —— 把它写成
			// 「契约缺陷」与实际走的路径不符（Q7-B）。只改备注文案，等级/判定不变。
			if n, ok := mediaDenialNote(policy, c.Source, r.ErrCode); ok {
				r.Note = n
			} else if r.OnError >= 1 {
				r.Note = "失败路径：未绘制 + onerror 已派发（契约正确）"
			} else {
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
	// 视频的两条「非缺陷」解释：①data: 来源的媒体没有本地路径，宿主帧源
	// （ffmpeg 按路径抽帧）给不出画面——引擎侧的元数据与契约仍然正确；
	// ②纯色样本的帧色恒定，动画判据（帧间差异）不适用，画面正确即足。
	// ★ 二者都以「该格真的加载出内容」为前提：L0（未加载）谈帧色是错位
	//   （没加载哪来帧），此类格改由下方 L0 归因给出准确原因。
	if sp.Kind == "video" && r.Grade != "L4" {
		switch {
		case !r.DrawOK && c.Source == "data" && (r.Ready >= 1 || r.Duration > 0):
			r.Note = "data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期）"
		case !r.AnimOK && len(sp.Solid) == 3 && r.DrawOK:
			r.Note = "单色视频：帧色恒定，动画判据不适用（画面正确即足）"
		}
	}
	if sp.Format == "avif" || sp.Format == "tiff" {
		r.Note = "预期不支持（L0，写入基线，不投入）"
	}
	if p.Tag == "missing" {
		r.Grade = "L0"
		r.Note = "元素缺失（页面构建异常）"
	}
	// L0 归因兜底：落 L0 且还没有具体备注的格（没加载 → 没画面也没元数据），
	// 若成因是资源策略拒绝就如实写明——这是本轮门禁收口的**正确行为**，不是
	// 实现缺陷；不写会让用户把「策略正确」误读成「无后端的遗留缺陷」（§9.9）。
	if r.Grade == "L0" && r.Note == "" {
		if n, ok := mediaDenialNote(policy, c.Source, p.ErrCode); ok {
			r.Note = n
		}
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
	// ★ 视频（<video>）：画面由宿主按 (url, 时刻) 抽帧注入，静止态停在
	//   currentTime=0。manifest 给期望色的两种情形：纯色样本走上一条（solid）；
	//   分段样本（phases）取**第一段**的色。其余样本（testsrc 这类自然序列）
	//   没有单点期望色——按「非灰块」判定：中心点既不是 .cell 底色也不是页面
	//   白，即视为画出了画面（与文档 §5 TC-M-502「截图有画面（非灰块）」的
	//   达成标准一致）。此前这类样本落到末尾 return false → 「绘制」恒 ❌，
	//   把已经有画面的视频压在 L1——测量伪影，不是引擎能力。
	if sp.Kind == "video" {
		x := c.X + cellW/2
		y := c.Y + cellH/2
		got := pixelAt(px, vw, x, y)
		if len(sp.Phases) > 0 {
			want := sp.Phases[0].RGB
			matched := colorNear(got, want, toleranceFor(sp))
			return matched, []sampleDot{{Name: "center-phase0", X: x, Y: y, Got: got,
				Want: want, Inside: true, Matched: matched}}
		}
		mask := [3]int{238, 238, 238}
		white := [3]int{255, 255, 255}
		drawn := !colorNear(got, mask, 6) && !colorNear(got, white, 6)
		return drawn, []sampleDot{{Name: "center-drawn", X: x, Y: y, Got: got,
			Want: mask, Inside: true, Matched: drawn}}
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
func framesDiffer(c cell, sp sampleSpec, man *manifestDoc, still, play []byte, animFrames [][]byte, vw int) (bool, string) {
	frames := make([][]byte, 0, len(animFrames)+2)
	frames = append(frames, still, play)
	frames = append(frames, animFrames...)
	pts := samplePointsOf(c)
	best, bestPair := 0, ""
	for i := 0; i < len(frames); i++ {
		for j := i + 1; j < len(frames); j++ {
			d := framesMaxDiff(frames[i], frames[j], pts, vw)
			if d > best {
				best, bestPair = d, fmt.Sprintf("帧%d↔帧%d 差 %d", i, j, d)
			}
		}
	}
	if best > animFrameDiffThreshold {
		return true, fmt.Sprintf("%s（共 %d 帧采样）", bestPair, len(frames))
	}
	return false, fmt.Sprintf("所有帧采样点相同（最大差异 %d，共 %d 帧）", best, len(frames))
}

// animFrameDiffThreshold 是「两帧之间发生了变化」的像素通道差阈值（0-255）。
const animFrameDiffThreshold = 10

// samplePointsOf 返回一格的采样点：中心 + 四个 1/4 位置。
//
// ★ 不能只看中心点：动图样本多是四象限/双色块，单点既可能漏掉变化，也可能
// 被同色巧合骗过（原实现只取中心，判据因此脆弱）。
func samplePointsOf(c cell) [][2]int {
	qx, qy := cellW/4, cellH/4
	return [][2]int{
		{c.X + cellW/2, c.Y + cellH/2},
		{c.X + qx, c.Y + qy},
		{c.X + 3*qx, c.Y + qy},
		{c.X + qx, c.Y + 3*qy},
		{c.X + 3*qx, c.Y + 3*qy},
	}
}

// framesMaxDiff 返回两帧在给定采样点上的最大通道差（越界点忽略）。
func framesMaxDiff(p, q []byte, pts [][2]int, vw int) int {
	m := 0
	for _, pt := range pts {
		a := pixelAt(p, vw, pt[0], pt[1])
		b := pixelAt(q, vw, pt[0], pt[1])
		if a[0] < 0 || b[0] < 0 {
			continue
		}
		for i := 0; i < 3; i++ {
			v := a[i] - b[i]
			if v < 0 {
				v = -v
			}
			if v > m {
				m = v
			}
		}
	}
	return m
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
	matrixPath string, adDiff, adDiffExcl float64, animRects int, adErr error, lo loopbackEvidence) error {
	var b strings.Builder
	commit := gitShortHash()
	fmt.Fprintf(&b, "# 媒体格式真实可用性报告（%s %s）\n\n", time.Now().Format("2006-01-02 15:04"), commit)
	b.WriteString("## 环境\n\n")
	fmt.Fprintf(&b, "配置：%s ｜ 引擎：%s ｜ 样本：本地脚本生成、不入库（决策 5）｜ 模型：L0–L4（文档 §2）\n\n",
		configNames(), commit)
	b.WriteString("复现：`python dev/media/gen_samples.py` → `cmd/psai -media`（本机按需，不入 CI 门禁——决策 6）\n\n")

	b.WriteString("## 总表\n\n")
	// 列语义说明：音频行的「绘制」列 N/A（音频无视觉内容），其等级由音频输出
	// 判据决定——不说明会让「绘制 ❌ 而等级 L4」看起来自相矛盾。
	b.WriteString("> **列说明**：`加载`/`几何` 对音频行指「元数据可加载 / 时长可用」；" +
		"**`绘制` 列对音频行不适用（N/A，报告显示 `—`）**——音频无视觉内容，其等级由音频输出判据决定" +
		"（PCM 交付 + 频谱主峰（判据 A）+ 播放时钟（TC-M-602），见「音频」小节）。\n")
	b.WriteString("> **L0 归因**：L0 = 未加载。`deny-external` 下的 `file`/`rel` 引用是被资源策略拒绝——" +
		"**预期行为，非缺陷**（文档 §9.9）；元素 `error.code` 仅作实测佐证：" +
		"引擎对「策略拒绝」与「源不可达」共用 `MEDIA_ERR_SRC_NOT_SUPPORTED`（4）。\n\n")
	b.WriteString("| 配置 | 格式 | 样本 | 来源 | 加载 | 几何 | 绘制 | 契约 | 动画 | 等级 | 备注 |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|---|\n")
	// 按 (样本, 来源, 配置) 排序输出，便于横向对比四配置
	type row struct {
		res   configResult
		c     cellResult
		order int
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
		if r.c.Kind == "audio" {
			// 音频无视觉内容：绘制列 N/A（等级由音频输出判据决定，见总表上方列说明）。
			draw = "—"
		}
		contr := mark(r.c.ContrOK)
		anim := "—"
		if r.c.Kind == "animated" || r.c.Kind == "video" {
			anim = mark(r.c.AnimOK)
		}
		// 动画判据判否时把帧差诊断写进备注：`AnimNote` 原先只赋值不进报告，
		// 格子降级时看不出「最大差异是多少、采了几帧」，无法区分「真没动」与
		// 「采样期没被调度」。
		// ★ 只在该格**真的出过画面**（DrawOK）时拼：L0/L1（未加载 / 无画面）时帧差
		//   恒为 0，写进备注会变成「所有帧采样点相同（最大差异 0，共 14 帧）」——
		//   与「根本没加载」自相矛盾。
		note := r.c.Note
		if (r.c.Kind == "animated" || r.c.Kind == "video") && !r.c.AnimOK && r.c.AnimNote != "" && r.c.DrawOK {
			if note == "" {
				note = r.c.AnimNote
			} else {
				note += "；" + r.c.AnimNote
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | **%s** | %s |\n",
			r.res.Config, r.c.Format, r.c.Sample, r.c.Source, load, geom, draw, contr, anim, r.c.Grade,
			escapePipe(note))
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
		b.WriteString("> 标注「资源策略 … 拒绝该引用（预期，非缺陷）」的行是配置策略的**正确行为**" +
			"（文档 §9.9 门禁收口），不是实现缺陷；真正的缺陷行不含此标注。\n\n")
	}

	b.WriteString("## 策略一致性（决策 1 · TC-M-905）\n\n")
	if adErr != nil {
		fmt.Fprintf(&b, "无法比对 Browser 与 Toolkit+AllowAll 的截图：`%v`\n\n", adErr)
	} else if adDiff < 0 {
		b.WriteString("未采集（配置数不足）\n\n")
	} else {
		fmt.Fprintf(&b, "- 全图差异像素比例：**%.4f%%**（其中 %d 个动画样本格未计入下方判定）\n", adDiff*100, animRects)
		if adDiffExcl < 0 {
			b.WriteString("- 排除动画样本格后：未测量\n\n")
		} else {
			verdict := "✅ 非动画区域逐像素一致"
			if adDiffExcl > 0 {
				verdict = "❌ 存在差异（策略开关引入了渲染差异）"
			}
			fmt.Fprintf(&b, "- 排除动画样本格后：**%.4f%%** —— %s\n\n", adDiffExcl*100, verdict)
			if adDiff > 0 && adDiffExcl == 0 {
				b.WriteString("> **归因**：全图差异 **100% 落在动画样本格内**（`anim-noloop.gif`、`anim-2frames.webp` 等）——\n")
				b.WriteString("> 两个配置各是一次独立加载与截图，动画停在不同帧（**帧相位差**，与策略开关无关）；\n")
				b.WriteString("> 非动画区域（静态图 / SVG / 视频 / 音频格）**零差异**。详见 `media-format-verification-plan.md` §9.7。\n\n")
			}
		}
	}

	// 音频（A3-1）：判据 A（PCM 频谱）/ 判据 B（环回）/ TC-M-602 定点 / 支持矩阵。
	var audioAll []audioCellEvidence
	for _, res := range results {
		audioAll = append(audioAll, res.Audio...)
	}
	b.WriteString("## 音频（A3-1：宿主 ffmpeg 解码 → 引擎 PCM 通道 → 输出后端）\n\n")
	b.WriteString(audioSection(audioAll, lo))

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
