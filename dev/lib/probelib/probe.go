// Package probelib 提供 CSS 夹具（dev/suites/cssprobe/fixtures）的公共原语：
// checks.json 读取、纯色连通域检测、期望比对，以及「真实浏览器参照」
// 所需的浏览器探测与截图封装。
//
// 为什么需要真实浏览器参照：夹具的期望值（checks.json）是**人写的**，它
// 可能是错的——legacy-center 的 "centered table resets legacy alignment
// internally" 一项就曾被怀疑是期望错误，而 Chrome headless 的
// getComputedStyle/截图证实期望正确（`<table>` 上的 -webkit-center 归一
// 为 start）。没有参照时，修 bug 与改期望无法区分；有了参照，每个失败
// 项都能先判定「渲染器错」还是「期望错」。本机 Edge headless 不可用，
// Chrome headless 可用，故默认顺序为 Chrome → Edge。
package probelib

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// 参考视口与容差沿用 obscura render-repros 的 check.py 约定（900x1000、
// ±1px、面积 <20px 的连通域视为噪点），使夹具期望可与参照渲染逐字比较。
const (
	ViewportW = 900
	ViewportH = 1000
	MinArea   = 20
	Tolerance = 1
	// NearTolerance 是"接近但不等"的每通道窗口，用于区分"画成近似色"
	// （绘制差异）与"完全没画"（实现缺失）。
	NearTolerance = 8
)

// Check 是一条期望的纯色连通域（checks.json 的条目）。坐标与尺寸是
// **渲染像素**空间；未给出的字段不参与判定（只用给定项计数）。
type Check struct {
	Name   string `json:"name"`
	Color  string `json:"color"`
	X      *int   `json:"x,omitempty"`
	Y      *int   `json:"y,omitempty"`
	Width  *int   `json:"width,omitempty"`
	Height *int   `json:"height,omitempty"`
	Count  int    `json:"count,omitempty"`
}

// Describe 渲染一条期望的紧凑描述，用于报告。
func (c Check) Describe() string {
	parts := []string{c.Color}
	if c.X != nil {
		parts = append(parts, fmt.Sprintf("x=%d", *c.X))
	}
	if c.Y != nil {
		parts = append(parts, fmt.Sprintf("y=%d", *c.Y))
	}
	if c.Width != nil {
		parts = append(parts, fmt.Sprintf("w=%d", *c.Width))
	}
	if c.Height != nil {
		parts = append(parts, fmt.Sprintf("h=%d", *c.Height))
	}
	if c.Count > 0 {
		parts = append(parts, fmt.Sprintf("count>=%d", c.Count))
	}
	return strings.Join(parts, " ")
}

// Component 是渲染结果中一块指定纯色的连通区域（4 邻接，语义等同
// scipy.ndimage.label 的连通域）。
type Component struct {
	X, Y, W, H, Area int
}

func (c Component) String() string {
	return fmt.Sprintf("%dx%d at (%d,%d) area=%d", c.W, c.H, c.X, c.Y, c.Area)
}

// NearColor 汇总"最接近期望色"的像素，用于解释未命中：是画成了近似色，
// 还是根本没有绘制。
type NearColor struct {
	WithinTolerance int    `json:"within_tolerance"`
	Closest         string `json:"closest"`
	ClosestDelta    int    `json:"closest_delta"`
	ClosestX        int    `json:"closest_x"`
	ClosestY        int    `json:"closest_y"`
}

// Mask 复用一块布尔缓冲做连通域标记，避免每个 check 重新分配。
type Mask struct {
	w, h    int
	mask    []bool
	touched []int32
}

// NewMask 创建 w×h 的连通域缓冲。
func NewMask(w, h int) *Mask {
	return &Mask{w: w, h: h, mask: make([]bool, w*h)}
}

// Components 返回与 want 颜色**完全相等**的所有连通域（面积 >= minArea）。
func (m *Mask) Components(img *image.RGBA, want color.RGBA, minArea int) []Component {
	for _, i := range m.touched {
		m.mask[i] = false
	}
	m.touched = m.touched[:0]

	pix := img.Pix
	for y := 0; y < m.h && y < img.Rect.Dy(); y++ {
		row := y * img.Stride
		for x := 0; x < m.w && x < img.Rect.Dx(); x++ {
			o := row + x*4
			if pix[o] == want.R && pix[o+1] == want.G && pix[o+2] == want.B {
				i := int32(y*m.w + x)
				m.mask[i] = true
				m.touched = append(m.touched, i)
			}
		}
	}

	var out []Component
	for _, start := range m.touched {
		if !m.mask[start] {
			continue
		}
		comp := m.flood(start)
		if comp.Area >= minArea {
			out = append(out, comp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Y != out[j].Y {
			return out[i].Y < out[j].Y
		}
		return out[i].X < out[j].X
	})
	return out
}

// flood 从 start（必须已标记）起吞掉一个 4 连通域，返回包围盒与面积。
func (m *Mask) flood(start int32) Component {
	stack := []int32{start}
	m.mask[start] = false
	minX, minY := m.w, m.h
	maxX, maxY := -1, -1
	area := 0
	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := int(i)%m.w, int(i)/m.w
		area++
		if x < minX {
			minX = x
		}
		if y < minY {
			minY = y
		}
		if x > maxX {
			maxX = x
		}
		if y > maxY {
			maxY = y
		}
		if x > 0 {
			if j := i - 1; m.mask[j] {
				m.mask[j] = false
				stack = append(stack, j)
			}
		}
		if x < m.w-1 {
			if j := i + 1; m.mask[j] {
				m.mask[j] = false
				stack = append(stack, j)
			}
		}
		if y > 0 {
			if j := i - int32(m.w); m.mask[j] {
				m.mask[j] = false
				stack = append(stack, j)
			}
		}
		if y < m.h-1 {
			if j := i + int32(m.w); m.mask[j] {
				m.mask[j] = false
				stack = append(stack, j)
			}
		}
	}
	return Component{X: minX, Y: minY, W: maxX - minX + 1, H: maxY - minY + 1, Area: area}
}

// NearestColor 报告页面上最接近 want 的像素（用于解释未命中）。
func NearestColor(img *image.RGBA, want color.RGBA) *NearColor {
	best := NearColor{ClosestDelta: 1 << 30, Closest: "#000000"}
	w := img.Rect.Dx()
	h := img.Rect.Dy()
	for y := 0; y < h; y++ {
		row := y * img.Stride
		for x := 0; x < w; x++ {
			o := row + x*4
			r, g, b := img.Pix[o], img.Pix[o+1], img.Pix[o+2]
			d := max3(abs(int(r)-int(want.R)), abs(int(g)-int(want.G)), abs(int(b)-int(want.B)))
			if d <= NearTolerance {
				best.WithinTolerance++
			}
			if d < best.ClosestDelta {
				best.ClosestDelta = d
				best.Closest = fmt.Sprintf("#%02x%02x%02x", r, g, b)
				best.ClosestX, best.ClosestY = x, y
			}
		}
	}
	return &best
}

// MatchesCheck 判定一个连通域是否满足期望（给出的字段逐一比对，
// 容差 ±Tolerance 像素）。
func MatchesCheck(comp Component, chk Check) bool {
	if chk.X != nil && abs(comp.X-*chk.X) > Tolerance {
		return false
	}
	if chk.Y != nil && abs(comp.Y-*chk.Y) > Tolerance {
		return false
	}
	if chk.Width != nil && abs(comp.W-*chk.Width) > Tolerance {
		return false
	}
	if chk.Height != nil && abs(comp.H-*chk.Height) > Tolerance {
		return false
	}
	return true
}

// ParseHex 解析 "#rgb"/"#rrggbb"（也接受无 # 前缀）。
func ParseHex(value string) (color.RGBA, bool) {
	s := strings.TrimSpace(value)
	s = strings.TrimPrefix(s, "#")
	switch len(s) {
	case 3:
		r, err1 := strconv.ParseUint(s[0:1], 16, 8)
		g, err2 := strconv.ParseUint(s[1:2], 16, 8)
		b, err3 := strconv.ParseUint(s[2:3], 16, 8)
		if err1 != nil || err2 != nil || err3 != nil {
			return color.RGBA{}, false
		}
		return color.RGBA{R: uint8(r * 17), G: uint8(g * 17), B: uint8(b * 17), A: 255}, true
	case 6, 8:
		v, err := strconv.ParseUint(s[:6], 16, 32)
		if err != nil {
			return color.RGBA{}, false
		}
		return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8 & 0xff), B: uint8(v & 0xff), A: 255}, true
	}
	return color.RGBA{}, false
}

// LoadChecks 读取夹具目录下的 checks.json：fixture 名 → 期望列表。
func LoadChecks(dir string) (map[string][]Check, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "checks.json"))
	if err != nil {
		return nil, err
	}
	var out map[string][]Check
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("checks.json: %w", err)
	}
	return out, nil
}

// CountOf 统计同一 fixture 中同名 check 的条数（颜色相同、位置不同的
// 多块共用同一个 name 时用于提示）。
func CountOf(checks []Check, name string) int {
	n := 0
	for _, c := range checks {
		if c.Name == name {
			n++
		}
	}
	return n
}

// FormatComponents 紧凑渲染一组连通域。
func FormatComponents(comps []Component) string {
	parts := make([]string, 0, len(comps))
	for _, c := range comps {
		parts = append(parts, c.String())
	}
	return strings.Join(parts, ", ")
}

// ReadPNG 读 PNG 并统一为 *image.RGBA（Alpha 已预乘到不透明，兼容截图
// 自带的 alpha 通道）。
func ReadPNG(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	if rgba, ok := src.(*image.RGBA); ok {
		return rgba, nil
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst, nil
}

// BrowserCandidates 返回按优先级排列的本机浏览器可执行文件路径。
// 本机 Edge headless 会静默失败（截图/dump-dom 均无输出），Chrome 可用，
// 故 Chrome 优先；可用环境变量 WBUI_BROWSER 覆盖。
func BrowserCandidates() []string {
	if env := os.Getenv("WBUI_BROWSER"); env != "" {
		return []string{env}
	}
	return []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
	}
}

// FindBrowser 返回第一个存在且能跑通一次空截图的浏览器路径。
func FindBrowser(probeDir string) (string, error) {
	var tried []string
	for _, path := range BrowserCandidates() {
		if _, err := os.Stat(path); err != nil {
			tried = append(tried, path+"(缺失)")
			continue
		}
		out := filepath.Join(probeDir, "browser-probe.png")
		if err := Screenshot(path, "about:blank", out, 64, 64, probeDir); err == nil {
			return path, nil
		}
		tried = append(tried, path+"(不可用)")
	}
	return "", fmt.Errorf("没有可用的真实浏览器参照：%s", strings.Join(tried, ", "))
}

// Screenshot 用 headless 浏览器把 htmlPath 渲染为 PNG（视口 w×h，
// 关闭滚动条以匹配 wb-ui 的离屏渲染约定）。
func Screenshot(browser, htmlPath, outPNG string, w, h int, profileDir string) error {
	url := htmlPath
	if !strings.Contains(url, "://") {
		abs, err := filepath.Abs(htmlPath)
		if err != nil {
			return err
		}
		url = "file:///" + strings.ReplaceAll(abs, `\`, "/")
	}
	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--no-sandbox",
		"--hide-scrollbars",
		"--force-device-scale-factor=1",
		"--window-size=" + strconv.Itoa(w) + "," + strconv.Itoa(h),
		"--screenshot=" + strings.ReplaceAll(outPNG, `\`, "/"),
	}
	if profileDir != "" {
		args = append(args, "--user-data-dir="+strings.ReplaceAll(profileDir, `\`, "/"))
	}
	args = append(args, url)
	cmd := exec.Command(browser, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", filepath.Base(browser), err, strings.TrimSpace(string(out)))
	}
	if _, err := os.Stat(outPNG); err != nil {
		return fmt.Errorf("%s 未产出截图（headless 静默失败）：%s", filepath.Base(browser), strings.TrimSpace(string(out)))
	}
	return nil
}

// WithScript 报告 HTML 是否含 <script>。cssprobe 不执行脚本，参照浏览器
// 会执行——含脚本的夹具差异属"探测能力差异"而非渲染回归。
func WithScript(htmlText string) bool {
	return strings.Contains(strings.ToLower(htmlText), "<script")
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func max3(a, b, c int) int {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}
