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
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"wb-ui/app"
	"wb-ui/engine/rendering"
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
	all := []mediaConfig{
		{Name: "Browser", Mode: webkit.ModeBrowser},
		{Name: "Toolkit+DenyExternal", Mode: webkit.ModeToolkit, Policy: policyPtr(webkit.DenyExternal)},
		{Name: "Toolkit+AllowHostResolved", Mode: webkit.ModeToolkit,
			Policy: policyPtr(webkit.AllowHostResolved), HostResolver: true},
		{Name: "Toolkit+AllowAll", Mode: webkit.ModeToolkit, Policy: policyPtr(webkit.AllowAll)},
	}
	// ★ 调试开关（`PSAI_MEDIA_CONFIGS=Browser`，逗号分隔）：只跑指定配置。
	//   排查「加载完成速度 vs 等待窗口」这类**竞态**必须做多组对照实验，而一次四
	//   配置全量要 ~14 分钟、窗口参数又要逐组变 ⇒ 单配置把每组压到约 1/4。
	//   未设置时仍是四配置全跑（交付口径不变）。
	sel := strings.TrimSpace(os.Getenv("PSAI_MEDIA_CONFIGS"))
	if sel == "" {
		return all
	}
	want := map[string]bool{}
	for _, n := range strings.Split(sel, ",") {
		want[strings.TrimSpace(n)] = true
	}
	out := make([]mediaConfig, 0, len(all))
	for _, c := range all {
		if want[c.Name] {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		fmt.Printf("媒体验证：PSAI_MEDIA_CONFIGS=%q 没匹配到配置，按四配置全跑\n", sel)
		return all
	}
	return out
}

// envInt 读整数环境变量（调试开关用）；未设置/非法/非正数时取 def。
func envInt(name string, def int) int {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
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
	ID       string  `json:"id"`
	Sample   string  `json:"sample"`
	Source   string  `json:"source"`
	Format   string  `json:"format"`
	Kind     string  `json:"kind"`
	Tag      string  `json:"tag"`
	BoxW     float64 `json:"box_w"`
	BoxH     float64 `json:"box_h"`
	NaturalW float64 `json:"natural_w"`
	NaturalH float64 `json:"natural_h"`
	Complete bool    `json:"complete"`
	Ready    float64 `json:"ready_state"`
	Duration float64 `json:"duration"`
	OnLoad   int     `json:"onload"`
	OnError  int     `json:"onerror"`
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
	// WebAudio 是 TC-M-603 的自检取证：在**引擎真实 JS 环境**里验证
	// AudioContext / decodeAudioData（只在一个配置上跑一次——它验的是引擎能力，
	// 与资源策略/配置无关，报告因此不重复四遍同一份证据）。
	WebAudio *webAudioEvidence `json:"webaudio,omitempty"`
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

	// Edge 双端对照（决策 2）：开启时把逐格对照节写进报告；未开启时报告里
	// 明确标「未执行」，不留一个看不出跑没跑的空白（§8.3 第 1 条的证据纪律）。
	edgeSection := ""
	if o.Edge {
		summary, section := runEdgeComparison(results, cells, man, matrixPath, vw, pageH, outDir)
		fmt.Printf("  Edge 对照：%s\n", summary)
		edgeSection = section
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
	if err := writeMediaReport(reportPath, man, results, baseline, berr, matrixPath, adDiff, adDiffExcl, animRects, adErr, loopback, edgeSection); err != nil {
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
	// ★ Edge 双端对照的**页面内**采集脚本（§8.3 第 1/2 条）：Edge 侧没有引擎的
	// Go 采集接口（引擎侧走 collectPage → EvalJS），所以把**同一套采集字段**挂进
	// 页面，让 Edge 自己执行、把结果写进 <pre id="__edge_probe">，Go 侧用
	// `--dump-dom` 取回后交给**同一个判定内核**（judgeCell）定级。
	//
	// 为什么先 encodeURIComponent 再落 DOM：DOM 转储层面就不必处理 HTML 实体
	// 转义，也不怕 JSON 里出现 `<`/`&`（URL 里确实可能有）。
	// 采集时机 = load 之后 1500ms（与引擎侧「settleReal 之后采集」的语义对齐）；
	// Edge 侧靠 --virtual-time-budget 让这个定时器在无头模式下真的触发。
	edgeIDs := make([]string, 0, len(cells))
	for i := range cells {
		edgeIDs = append(edgeIDs, jsString(cells[i].ID))
	}
	b.WriteString("<pre id=\"__edge_probe\" style=\"display:none\"></pre>\n<script>\n(function(){\n")
	b.WriteString("var ids=[" + strings.Join(edgeIDs, ",") + "];\n")
	b.WriteString("function collect(){var out={cells:[],events:window.__mediaEvents||{},vw:window.innerWidth," +
		"vh:window.innerHeight,doch:document.documentElement?document.documentElement.scrollHeight:0};")
	b.WriteString("var num=function(v){return (typeof v==='number'&&isFinite(v))?v:-1;};")
	b.WriteString("for(var i=0;i<ids.length;i++){var id=ids[i],el=document.getElementById(id);")
	b.WriteString("if(!el){out.cells.push({id:id,tag:'missing'});continue;}")
	b.WriteString("var r=el.getBoundingClientRect();")
	b.WriteString("out.cells.push({id:id,tag:(el.tagName||'').toLowerCase(),x:r.left,y:r.top,w:r.width,h:r.height,")
	b.WriteString("complete:(el.complete===undefined?'n/a':el.complete),")
	b.WriteString("nw:num(el.naturalWidth)<0?0:(el.naturalWidth||0),nh:num(el.naturalHeight)<0?0:(el.naturalHeight||0),")
	b.WriteString("rs:num(el.readyState),duration:num(el.duration),ct:num(el.currentTime),")
	b.WriteString("paused:(el.paused===undefined?'n/a':el.paused),err:(el.error?el.error.code:0)});}")
	b.WriteString("var pre=document.getElementById('__edge_probe');")
	b.WriteString("if(pre){pre.textContent=encodeURIComponent(JSON.stringify(out));}}\n")
	b.WriteString("var go=function(){setTimeout(collect,1500);};")
	b.WriteString("if(document.readyState==='complete'){go();}else{window.addEventListener('load',go);}\n")
	b.WriteString("})();\n</script>\n")
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
	// ★ TC-M-603（WebAudio 最小子集）：把宿主 ffmpeg 解码器接到
	//   `AudioContext.decodeAudioData`。不装的话自检里的 decodeAudioData 会以
	//   EncodingError 失败（那是如实失败，不是通过）——探针要验的是**端到端可用**，
	//   所以这里必须与 psai 主宿主（app/host.go）一样装配。
	app.InstallWebAudio("")
	// ★ AVIF（对齐浏览器，2026-10-08）：Skia 二进制无 AVIF 解码，浏览器有 ⇒
	//   探针必须与 psai 主宿主（app/host.go）一样装配宿主转码器，否则 AVIF 会
	//   被判成 L0（那是「探针没装能力」，不是引擎能力）。
	app.InstallImageTranscoder("")

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
	// ★ 就绪即走（2026-10-08 修复）：固定窗口换成「轮询到稳定」，见 waitImagesReady。
	waitImagesReady(wv, cells, samplesDir, 30*time.Second)

	res.Policy = wv.ResourcePolicy().String()

	// ★ TC-M-603：WebAudio 最小子集自检。只在一个配置上跑（见 configResult.WebAudio
	//   的注释）；样本是真音频字节，解码走宿主 ffmpeg 通道——失败也在证据里如实记录，
	//   不按通过处理。
	if cfg.Name == "Browser" {
		if p, ok := firstWAVSample(samplesDir); ok {
			ev := probeWebAudio(wv, p)
			res.WebAudio = &ev
		}
	}

	// ① 静止态采集 + 截图
	probes, events, err := collectPage(wv, cells)
	if err != nil {
		return res, err
	}
	// ★ Q7-D 截图相位锁定（2026-10-07）：动图显示哪一帧 = 「登记时刻到绘制时刻之间
	//   真实流逝了多久」，那段时长包含加载、事件派发、主循环调度的全部抖动 ⇒ 同一
	//   HEAD 重跑会截到**不同帧**（实测 matrix-*.png 差异 1.98%~5.95%，逐格定位后
	//   差异像素 100% 落在动画样本格上）。截图要当验收证据（§6.3 供 read_image 复核、
	//   供跨跑比对），就必须逐像素可复现。
	//   处置：仅在**这一次截图绘制**期间把相位钉在 0（各动图首帧），截完立即解除。
	//   ★ 这不会掩盖缺陷：「动图是否真的在推进」由连拍判据（L4，见 ③）负责，它用的
	//     是**真实相位**，不受本锁定影响；本锁定只决定「取证那一帧取哪一帧」。
	app.PinAnimatedImagePhase(0)
	stillPixels, err := renderPixels(wv)
	app.UnpinAnimatedImagePhase()
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
	// ★ 播放态截图同样钉相位（理由同 ①）：播放态与静止态各自的动画相位都不可复现。
	app.PinAnimatedImagePhase(0)
	playPixels, err := renderPixels(wv)
	app.UnpinAnimatedImagePhase()
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

// waitImagesReady 轮询等待页面内所有 `<img>` 格子进入稳定态，返回就绪格数、
// img 格总数与收敛所用步数。
//
// 为什么必须「就绪即走」而不是固定时长：本引擎的 `<img>` 取字节跑在 goroutine 上、
// load/error 契约由绘制路径派发（见 settleReal 注释），**每步绘制能推进的图片数量有限**。
// 矩阵页 96 格时固定的 settleReal(1500ms)=30 步只够约半数样本，其余格被采集到
// complete=false / naturalWidth=0，判据如实给出「画得出但无固有尺寸（D4）」⇒ 全量报告
// 出现 51 条 L4/L3→L2 的**批量假降级**。对照证据：①只降 file/rel，data 来源是页面内联
// 同步解码故不受影响；②同一份代码跑单样本（3 格）时「与基线一致」；③既有基线本身记录的
// 就是这些格的 L4/L3 ⇒ 引擎能力没问题，是探针等待窗口与就绪状态解耦。
//
// 收敛条件：连续 8 步没有新的格子就绪即认为稳定（样本里本就有故意损坏的
// corrupt.png / mislabeled.png，永远不就绪，因此不能等「全部就绪」）。
func waitImagesReady(wv *webkit.WebView, cells []cell, samplesDir string, maxWait time.Duration) (ready, total, steps int) {
	ids := make([]string, 0, len(cells))
	for _, c := range cells {
		ids = append(ids, jsString(c.ID))
	}
	if len(ids) == 0 {
		return 0, 0, 0
	}
	script := `(function(){
		var ids = [` + strings.Join(ids, ",") + `];
		var total = 0, ready = 0, miss = [];
		for (var i = 0; i < ids.length; i++) {
			var el = document.getElementById(ids[i]);
			if (!el || String(el.tagName || '').toLowerCase() !== 'img') { continue; }
			total++;
			if (el.complete === true || (el.naturalWidth || 0) > 0) { ready++; } else { miss.push(ids[i]); }
		}
		return ready + '/' + total + ' ' + miss.join(',');
	})()`
	const step = 100 * time.Millisecond
	// ★ 采样节流：EvalJS 每 5 步（500ms）一次。每步都注入脚本会打断引擎的加载/重绘推进
	// （实测每步 EvalJS 时 Browser 配置恒停在 30/72、50 步零推进；而节流后恢复）。
	const sampleEvery = 5
	// minSamples 保证「长尾也能等到」：AVIF 走宿主 ffmpeg 子进程转码、huge-4096.png 解码
	// 都可能在上一次就绪后停顿 1s 以上；只按空窗判稳会提前收敛（实测 8 步空窗时曾停在
	// 62~63/72，造成 12 条零星 L2 假降级）。因此既要求最短等待，也要求空窗足够长。
	// ★ 调试开关：PSAI_MEDIA_MIN_SAMPLES / PSAI_MEDIA_STABLE_SAMPLES 可覆盖下述收敛
	//   参数，用于「人为缩短/延长等待窗口 ⇒ 未就绪格是否补齐」的对照实验。
	minSamples := envInt("PSAI_MEDIA_MIN_SAMPLES", 10)      // 至少 10 次采样（= 5s）
	stableSamples := envInt("PSAI_MEDIA_STABLE_SAMPLES", 5) // 连续 5 次采样（2.5s）无新增就绪才认为收敛
	samples, stable := 0, 0
	prev := -1
	notReady := ""
	for elapsed := time.Duration(0); elapsed < maxWait; elapsed += step {
		driveEventLoop(wv, int64(step/time.Millisecond))
		time.Sleep(step)
		if _, err := wv.Render(); err != nil {
			break
		}
		steps++
		if steps%sampleEvery != 0 {
			continue
		}
		samples++
		v, err := wv.EvalJS(script)
		if err != nil {
			break
		}
		var r, t int
		if _, err := fmt.Sscanf(v.ToString(), "%d/%d", &r, &t); err != nil {
			break
		}
		ready, total = r, t
		if sp := strings.IndexByte(v.ToString(), ' '); sp >= 0 {
			notReady = v.ToString()[sp+1:]
		} else {
			notReady = ""
		}
		if r > prev {
			prev, stable = r, 0
			continue
		}
		stable++
		if stable >= stableSamples && samples >= minSamples {
			break
		}
	}
	// 未就绪格 id 打进日志：收敛后仍不就绪的应**只有**故意坏样本（corrupt.png /
	// mislabeled.png）与策略拒绝的格（DenyExternal 的 file/rel）——多出来的即是缺陷线索。
	fmt.Printf("  图片就绪：%d/%d（%d 步 / %d 次采样收敛）", ready, total, steps, samples)
	if notReady != "" {
		fmt.Printf("；未就绪：%s", notReady)
	}
	fmt.Println()
	// ★ 就绪诊断（排查 file/rel 批量未就绪）：把「未就绪」分成两类，直接区分两条
	//   互斥的根因方向——
	//     ①缓存里**已解码**（IsImageReady 为真）：取字节/解码已完成，缺陷在**契约
	//       派发**（load 事件没到 DOM，el.complete/naturalWidth 不更新）；
	//     ②缓存里**也没有**：缺陷在**取字节通道**（loader → loadExternalResource）
	//       或解码失败，即该 URL 没能在等待窗口内完成。
	//   再打印 goroutine 数：「加载 goroutine 挂起 ⇒ loading[url] 永久为真 ⇒ 永不
	//   重试 ⇒ 零推进」这条假设若成立，goroutine 数会显著高于稳态。
	if notReady != "" {
		byID := make(map[string]cell, len(cells))
		for _, c := range cells {
			byID[c.ID] = c
		}
		var cached, uncached int
		var uncachedItems []string
		for _, id := range strings.Split(notReady, ",") {
			c, ok := byID[id]
			if !ok {
				continue
			}
			// 缓存键是**解析后的绝对 URL**：file 形态本就是绝对 URL；rel 形态在渲染层
			// 经 loader.ResolveURL 解析为 samples 目录下的同一文件。
			u := c.URL
			if c.Source != "data" && c.Sample.File != "" {
				u = fileURLOf(filepath.Join(samplesDir, filepath.FromSlash(c.Sample.File)))
			}
			if rendering.IsImageReady(u) {
				cached++
			} else {
				uncached++
				uncachedItems = append(uncachedItems, id+"@"+c.Source)
			}
		}
		fmt.Printf("  未就绪诊断：%d 格（缓存已解码 %d / 缓存无 %d）；goroutines=%d\n",
			cached+uncached, cached, uncached, runtime.NumGoroutine())
		if len(uncachedItems) > 0 {
			fmt.Printf("    缓存无：%s\n", strings.Join(uncachedItems, " "))
		}
	}
	return ready, total, steps
}

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
	// TIFF：浏览器同样不支持（Edge 里 `<img src=x.tiff>` 同样是失败路径）⇒
	// **保持不支持就是对齐浏览器**，L0 属预期而非缺陷。
	// （AVIF 曾与此同列，2026-10-08 起已支持：Skia 二进制没有编入 AVIF 解码，
	// 但浏览器支持它，故由宿主用 ffmpeg 转 PNG 再交引擎——见
	// app/imagetranscode.go。它的 L0 归因见下方「未装配转码器」分支。）
	if sp.Format == "tiff" {
		r.Note = "预期不支持（浏览器同样不支持 TIFF，L0 即对齐）"
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
	// AVIF 落到 L0 且没有策略成因 ⇒ 是**没有可用的转码器**（宿主没装配，或
	// 本机 ffmpeg 不可用/转码失败）。如实写明，不写成「预期不支持」——那是把
	// 能力缺口说成设计取舍。
	if sp.Format == "avif" && r.Grade == "L0" && r.Note == "" {
		r.Note = "AVIF 需宿主图像转码器（ffmpeg）——本机未装配或转码失败"
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
	matrixPath string, adDiff, adDiffExcl float64, animRects int, adErr error, lo loopbackEvidence,
	edgeSection string) error {
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

	// TC-M-603（WebAudio 最小子集）：在真实引擎环境里的自检取证。
	for _, r := range results {
		if r.WebAudio != nil {
			b.WriteString(webAudioSection(*r.WebAudio, r.WebAudio.Sample))
			break
		}
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

	// Edge 双端对照节（决策 2 / §8.3 第 1-2 条）：程序化逐格对照。未启用
	// `-media-edge` 时同样写一节并标「未执行」——报告里不该出现「看不出跑没跑」
	// 的空白（§6.3 证据纪律）。
	if strings.TrimSpace(edgeSection) == "" {
		edgeSection = edgeSectionSkip("SKIP(未启用 -media-edge)")
	}
	b.WriteString(edgeSection)

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

// runEdgeComparison Edge 双端对照（决策 2、§8.3 第 1/2 条）。
//
// 早先这里只把矩阵页交给 Edge 截一张图、**不做任何等级判定**（文档 §8.3 记为
// 「本轮未做」），于是「引擎等级不低于 Edge 等级」这条判据一直只能人工复核。
// 现在补成程序化逐格对照，做法是让两侧走**同一把尺子**：
//
//	Edge 侧：页面内采集脚本（buildMatrix 注入）+ `--dump-dom` 取回 → cellProbe，
//	         像素用 Edge 自己的截图按格矩形采样；
//	引擎侧：既有的 collectPage + Render 像素（一字未改）；
//	判定：两侧都调 judgeCell —— 同一内核、同一阈值、同一等级映射。
//
// ★ 比较尺度是 0..3（normGrade3）。
// Edge 侧不做连拍采样、测不到「动画推进」维度，故引擎的 L4 在比较时按 L3 计。
// 方向只有「引擎不得低于 Edge」一个。
// ★ 找不到 Edge → SKIP(no-edge)，**不得默认通过**（决策 2 原文）。
//
// 返回 (summary, section)：summary 供终端一行，section 是写进 report.md 的节。
func runEdgeComparison(results []configResult, cells []cell, man *manifestDoc,
	matrixPath string, vw, pageH int, outDir string) (string, string) {
	exe := findEdge()
	if exe == "" {
		return "SKIP(no-edge)", edgeSectionSkip("SKIP(no-edge)")
	}
	pageURL := fileURLOf(matrixPath)
	shot := filepath.Join(outDir, "edge-matrix.png")
	cmd := exec.Command(exe, "--headless=new", "--disable-gpu", "--hide-scrollbars",
		fmt.Sprintf("--window-size=%d,%d", vw, pageH),
		"--screenshot="+shot, pageURL)
	if out, err := cmd.CombinedOutput(); err != nil {
		s := fmt.Sprintf("SKIP(edge-failed: %v / %s)", err, strings.TrimSpace(string(out)))
		return s, edgeSectionSkip(s)
	}
	if _, err := os.Stat(shot); err != nil {
		return "SKIP(edge-no-screenshot)", edgeSectionSkip("SKIP(edge-no-screenshot)")
	}
	dom, err := edgeDumpDOM(exe, pageURL, vw, pageH)
	if err != nil {
		s := fmt.Sprintf("SKIP(edge-dom-failed: %v)", err)
		return s, edgeSectionSkip(s)
	}
	probes, events, err := parseEdgeProbe(dom)
	if err != nil {
		s := fmt.Sprintf("SKIP(edge-probe-missing: %v)", err)
		return s, edgeSectionSkip(s)
	}
	px, pw, _, err := pngPixelsPremul(shot)
	if err != nil {
		s := fmt.Sprintf("SKIP(edge-png-failed: %v)", err)
		return s, edgeSectionSkip(s)
	}
	if pw != vw {
		// 截图宽度与视口不一致（DPR ≠ 1 等）会让按格矩形采样整体错位——如实报错，
		// 不拿错位的采样当判定依据。
		s := fmt.Sprintf("SKIP(edge-width-mismatch: 截图 %dpx ≠ 视口 %dpx)", pw, vw)
		return s, edgeSectionSkip(s)
	}
	edgeCells := judgeEdgeCells(cells, man, probes, events, px, vw)

	engByID := make(map[string]cellResult, len(results[0].Cells))
	for _, c := range results[0].Cells {
		engByID[c.ID] = c
	}
	var worse, higher []string
	compared, equal := 0, 0
	for _, c := range cells {
		e, okE := edgeCells[c.ID]
		g, okG := engByID[c.ID]
		if !okE || !okG || e.Tag == "missing" || g.Tag == "missing" {
			continue
		}
		compared++
		eg, gg := normGrade3(e.Grade), normGrade3(g.Grade)
		label := fmt.Sprintf("%s|%s|%s", shortName(c.Sample.Name), c.Source, c.Sample.Format)
		switch {
		case gg < eg:
			worse = append(worse, fmt.Sprintf("%s：引擎 %s < Edge %s", label, g.Grade, e.Grade))
		case gg > eg:
			higher = append(higher, fmt.Sprintf("%s：引擎 %s > Edge %s", label, g.Grade, e.Grade))
		default:
			equal++
		}
	}
	sort.Strings(worse)
	sort.Strings(higher)
	verdict := "✅ 逐格不低于 Edge 等级"
	if len(worse) > 0 {
		verdict = fmt.Sprintf("❌ %d 格低于 Edge 等级", len(worse))
	}
	summary := fmt.Sprintf("%s（对照 %d 格：一致 %d、低于 %d、高于 %d；截图 %s）",
		verdict, compared, equal, len(worse), len(higher), relToCwd(shot))

	var b strings.Builder
	b.WriteString("## Edge 双端对照（决策 2、§8.3 第 2 条）\n\n")
	fmt.Fprintf(&b, "执行：`cmd/psai -media -media-edge` ｜ Edge：`%s` ｜ 截图：`%s`（同时用页面内采集 + `--dump-dom` 回传每格状态）\n\n",
		exe, relToCwd(shot))
	b.WriteString("> 判据（§8.3 第 2 条原文）：**引擎等级不低于 Edge 等级**。逐格比较，" +
		"比较尺度 0..3 —— Edge 侧不做连拍采样、测不到「动画推进」维度，故引擎的 L4 在比较时按 L3 计；" +
		"两侧等级都由**同一个判定内核**（`judgeCell`，同一阈值同一映射）算出。\n\n")
	b.WriteString("| 项 | 值 |\n|---|---|\n")
	fmt.Fprintf(&b, "| 对照格数 | %d |\n", compared)
	fmt.Fprintf(&b, "| 等级一致 | %d |\n", equal)
	fmt.Fprintf(&b, "| **引擎低于 Edge** | **%d** |\n", len(worse))
	fmt.Fprintf(&b, "| 引擎高于 Edge | %d |\n", len(higher))
	fmt.Fprintf(&b, "| 结论 | %s |\n\n", verdict)
	if len(worse) > 0 {
		b.WriteString("**低于 Edge 的格（须复核）**\n\n")
		for _, w := range worse {
			fmt.Fprintf(&b, "- %s\n", w)
		}
		b.WriteString("\n")
	}
	if len(higher) > 0 {
		fmt.Fprintf(&b, "<details><summary>引擎高于 Edge 的格（%d 格，供参考：多为动画推进与音频输出——浏览器侧未采这类维度）</summary>\n\n", len(higher))
		for _, h := range higher {
			fmt.Fprintf(&b, "- %s\n", h)
		}
		b.WriteString("\n</details>\n\n")
	}
	return summary, b.String()
}

// edgeSectionSkip 是没有 Edge（或对照失败）时写进报告的节：如实标明未执行，
// **不作为通过**（决策 2：无 Edge 环境时标 SKIP(no-edge)，不得默认通过）。
func edgeSectionSkip(reason string) string {
	return "## Edge 双端对照（决策 2、§8.3 第 2 条）\n\n" +
		"判据：引擎等级不低于 Edge 等级。\n\n" +
		"| 项 | 值 |\n|---|---|\n" +
		"| 结论 | " + reason + " —— **未执行，不算通过** |\n\n"
}

// normGrade3 把等级压到 0..3：Edge 侧只判 L/G/P/E 四维（没有连拍采样，测不到
// 动画推进），因此「不低于 Edge」的判据在 0..3 的尺度上比较——引擎的 L4 按 L3
// 计，避免用「浏览器侧量不到的维度」去刷高自己。
func normGrade3(g string) int {
	n, ok := gradeOrder[g]
	if !ok {
		return 0
	}
	if n > 3 {
		n = 3
	}
	return n
}

// edgeDumpDOM 让 Edge 输出渲染后的 DOM（页面内采集脚本把结果写进 #__edge_probe）。
// `--virtual-time-budget` 是必需的：采集脚本挂在 setTimeout(collect, 1500) 上，
// 没有虚拟时间预算时无头模式会在定时器触发前就 dump 完，采集永远是空的。
func edgeDumpDOM(exe, pageURL string, vw, pageH int) (string, error) {
	cmd := exec.Command(exe, "--headless=new", "--disable-gpu", "--hide-scrollbars",
		fmt.Sprintf("--window-size=%d,%d", vw, pageH),
		"--virtual-time-budget=6000", "--dump-dom", pageURL)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// parseEdgeProbe 从 Edge 的 DOM 转储里取回页面采集结果（buildMatrix 注入的
// #__edge_probe，内容是 encodeURIComponent 后的 pageReport JSON）。
func parseEdgeProbe(dom string) ([]cellProbe, map[string]map[string]int, error) {
	const open = `<pre id="__edge_probe"`
	i := strings.Index(dom, open)
	if i < 0 {
		return nil, nil, fmt.Errorf("DOM 里没有 #__edge_probe（采集脚本未执行）")
	}
	gt := strings.Index(dom[i:], ">")
	if gt < 0 {
		return nil, nil, fmt.Errorf("#__edge_probe 标签不完整")
	}
	rest := dom[i+gt+1:]
	end := strings.Index(rest, "</pre>")
	if end < 0 {
		return nil, nil, fmt.Errorf("#__edge_probe 没有闭合标签")
	}
	raw := strings.TrimSpace(rest[:end])
	if raw == "" {
		return nil, nil, fmt.Errorf("采集结果为空（页面脚本未在 dump 之前完成采集）")
	}
	dec, err := url.QueryUnescape(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("解码采集结果失败: %w", err)
	}
	var rep pageReport
	if err := json.Unmarshal([]byte(dec), &rep); err != nil {
		return nil, nil, fmt.Errorf("解析采集结果失败: %w", err)
	}
	return rep.Cells, rep.Events, nil
}

// pngPixelsPremul 读 PNG 并按引擎侧采样路径的布局返回**预乘** RGBA 缓冲。
// Go 的 color.Color.RGBA() 返回的本就是 alpha 预乘的 16 位分量，故 >>8 即为预乘
// 8 位值——采样链路（pixelAt → unpremul）正是按这个布局写的，于是引擎与 Edge
// 两侧用同一套采样代码、同一套阈值。
func pngPixelsPremul(path string) ([]byte, int, int, error) {
	img, err := readPNGFile(path)
	if err != nil {
		return nil, 0, 0, err
	}
	bd := img.Bounds()
	w, h := bd.Dx(), bd.Dy()
	if w <= 0 || h <= 0 {
		return nil, 0, 0, fmt.Errorf("截图尺寸非法: %dx%d", w, h)
	}
	px := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, a := img.At(bd.Min.X+x, bd.Min.Y+y).RGBA()
			i := (y*w + x) * 4
			px[i], px[i+1], px[i+2], px[i+3] = byte(r>>8), byte(g>>8), byte(bl>>8), byte(a>>8)
		}
	}
	return px, w, h, nil
}

// judgeEdgeCells 用**同一个判定内核**（judgeCell）给 Edge 侧每格定级。
// 与引擎侧的差别只有输入：播放态传与静止态相同的缓冲（Edge 侧不做连拍），动画
// 帧传 nil ⇒ 动画/播放推进维度恒不成立，等级自然封顶 L3 —— 这正是「浏览器侧
// 量不到的维度不参与比较」的正确口径，而不是给浏览器放宽判据。
func judgeEdgeCells(cells []cell, man *manifestDoc, probes []cellProbe,
	events map[string]map[string]int, px []byte, vw int) map[string]cellResult {
	byID := make(map[string]cellProbe, len(probes))
	for _, p := range probes {
		byID[p.ID] = p
	}
	out := make(map[string]cellResult, len(cells))
	for _, c := range cells {
		p := byID[c.ID]
		r := judgeCell(c, man, p, p, events, px, px, nil, vw, 0, audioCellEvidence{}, "allow-all")
		out[c.ID] = r
	}
	return out
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
