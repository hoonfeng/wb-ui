// Command cssoracle 用**真实浏览器**（Chrome/Edge headless）渲染
// dev/suites/cssprobe 的同一批夹具，并与 checks.json 的期望逐条比对。
//
// 用途：把"夹具期望错了"与"wb-ui 渲染错了"分开。cssprobe 只能告诉我们
// 「渲染结果 ≠ 期望」；cssoracle 给出第三个事实来源——期望本身是否等于
// 真实浏览器。修复夹具失败项时的顺序因此固定为：
//
//	go run ./dev/probes/cssoracle -filter <fixture>   # 期望是否与浏览器一致？
//	go run ./dev/suites/cssprobe  -filter <fixture> -v # wb-ui 与同一期望的差距
//
// 判定：
//
//	MATCH     期望与浏览器一致 → 失败项是 wb-ui 的渲染 bug
//	MISMATCH  期望与浏览器不一致 → 先校准期望（报告给出实际几何）
//	SKIP      夹具含 <script> 且差异只可能来自 JS（cssprobe 不执行脚本）
//
// 用法：
//
//	go run ./dev/probes/cssoracle                      # 全部夹具
//	go run ./dev/probes/cssoracle -filter legacy       # 名字匹配正则
//	go run ./dev/probes/cssoracle -v                   # 同时列出 MATCH 项
//	go run ./dev/probes/cssoracle -json oracle.json    # 机器可读结果
//	go run ./dev/probes/cssoracle -dump /tmp/oracle    # 保留浏览器截图
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"wb-ui/dev/lib/probelib"
)

type checkResult struct {
	Name     string               `json:"name"`
	Color    string               `json:"color"`
	Expected string               `json:"expected"`
	Verdict  string               `json:"verdict"` // MATCH / MISMATCH / SKIP
	Role     string               `json:"role"`
	Found    int                  `json:"found"`
	Actual   []probelib.Component `json:"actual,omitempty"`
	Near     *probelib.NearColor  `json:"near,omitempty"`
}

type fixtureResult struct {
	Name       string        `json:"name"`
	HasScript  bool          `json:"has_script,omitempty"`
	Error      string        `json:"error,omitempty"`
	Checks     []checkResult `json:"checks"`
	Mismatches int           `json:"mismatches"`
}

func defaultFixtureDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "dev/suites/cssprobe/fixtures"
	}
	// dev/probes/cssoracle → ../../suites/cssprobe/fixtures
	return filepath.Join(filepath.Dir(file), "..", "..", "suites", "cssprobe", "fixtures")
}

func main() {
	filter := flag.String("filter", "", "regexp: 只跑名字匹配的夹具")
	fixturesDir := flag.String("fixtures", defaultFixtureDir(), "夹具目录（HTML + checks.json）")
	browser := flag.String("browser", "", "浏览器可执行文件（缺省自动探测 Chrome→Edge）")
	dump := flag.String("dump", "", "保存浏览器截图的目录")
	verbose := flag.Bool("v", false, "同时列出 MATCH 项")
	jsonOut := flag.String("json", "", "把机器可读结果写入该文件")
	flag.Parse()

	checks, err := probelib.LoadChecks(*fixturesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取 checks.json 失败: %v\n", err)
		os.Exit(2)
	}

	names := make([]string, 0, len(checks))
	for name := range checks {
		if *filter != "" {
			re, err := regexp.Compile(*filter)
			if err != nil {
				fmt.Fprintf(os.Stderr, "无效 -filter 正则: %v\n", err)
				os.Exit(2)
			}
			if !re.MatchString(name) {
				continue
			}
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		fmt.Println("没有匹配的夹具")
		return
	}

	workDir, err := os.MkdirTemp("", "cssoracle")
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建临时目录失败: %v\n", err)
		os.Exit(2)
	}
	defer os.RemoveAll(workDir)
	profileDir := filepath.Join(workDir, "profile")

	chrome := *browser
	if chrome == "" {
		chrome, err = probelib.FindBrowser(workDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	fmt.Printf("参照浏览器: %s\n", chrome)

	results := make([]fixtureResult, 0, len(names))
	mask := probelib.NewMask(probelib.ViewportW, probelib.ViewportH)

	totalChecks, totalMismatch, fixturesClean, fixturesWithMismatch := 0, 0, 0, 0
	for _, name := range names {
		htmlPath := filepath.Join(*fixturesDir, name+".html")
		htmlText, err := os.ReadFile(htmlPath)
		if err != nil {
			fmt.Printf("%-34s SKIP (无 HTML: %v)\n", name, err)
			continue
		}
		res := fixtureResult{Name: name, HasScript: probelib.WithScript(string(htmlText))}

		pngPath := filepath.Join(workDir, name+".png")
		if err := probelib.Screenshot(chrome, htmlPath, pngPath, probelib.ViewportW, probelib.ViewportH, profileDir); err != nil {
			res.Error = err.Error()
			fmt.Printf("%-34s ERROR %v\n", name, err)
			results = append(results, res)
			continue
		}
		if *dump != "" {
			if err := os.MkdirAll(*dump, 0o755); err == nil {
				_ = copyFile(pngPath, filepath.Join(*dump, name+".png"))
			}
		}
		img, err := probelib.ReadPNG(pngPath)
		if err != nil {
			res.Error = err.Error()
			fmt.Printf("%-34s ERROR %v\n", name, err)
			results = append(results, res)
			continue
		}
		img = normalizeSize(img)

		for _, chk := range checks[name] {
			totalChecks++
			cr := checkResult{Name: chk.Name, Color: chk.Color, Expected: chk.Describe()}
			want, ok := probelib.ParseHex(chk.Color)
			if !ok {
				cr.Verdict = "SKIP"
				cr.Role = "无法解析的期望颜色"
				res.Checks = append(res.Checks, cr)
				continue
			}
			comps := mask.Components(img, want, probelib.MinArea)
			cr.Found = len(comps)
			cr.Actual = comps
			pass := false
			for _, c := range comps {
				if probelib.MatchesCheck(c, chk) {
					pass = true
					break
				}
			}
			if pass {
				cr.Verdict = "MATCH"
				cr.Role = "期望 = 浏览器"
			} else {
				cr.Verdict = "MISMATCH"
				cr.Role = "期望 ≠ 浏览器（以实际几何校准期望）"
				if len(comps) == 0 {
					cr.Near = probelib.NearestColor(img, want)
				}
				res.Mismatches++
			}
			res.Checks = append(res.Checks, cr)
		}

		totalMismatch += res.Mismatches
		if res.Mismatches == 0 && res.Error == "" {
			fixturesClean++
		} else {
			fixturesWithMismatch++
		}
		results = append(results, res)

		status := "MATCH"
		if res.Mismatches > 0 {
			status = "MISMATCH"
		}
		scriptNote := ""
		if res.HasScript {
			scriptNote = " [含<script>]"
		}
		fmt.Printf("%-34s %s (%d/%d)%s\n", name, status, len(res.Checks)-res.Mismatches, len(res.Checks), scriptNote)
		for _, cr := range res.Checks {
			if cr.Verdict == "MISMATCH" || (*verbose && cr.Verdict != "SKIP") {
				line := fmt.Sprintf("    %-4s %-52s want %s", cr.Verdict, truncate(cr.Name, 52), cr.Expected)
				if cr.Found > 0 {
					line += " got " + probelib.FormatComponents(cr.Actual)
				} else if cr.Near != nil {
					line += fmt.Sprintf(" got none: closest=%s Δ=%d @(%d,%d) within±%d=%dpx",
						cr.Near.Closest, cr.Near.ClosestDelta, cr.Near.ClosestX, cr.Near.ClosestY,
						probelib.NearTolerance, cr.Near.WithinTolerance)
				}
				fmt.Println(line)
			}
		}
	}

	fmt.Printf("\n%d/%d fixtures 期望与浏览器一致, %d/%d checks 一致\n",
		fixturesClean, fixturesClean+fixturesWithMismatch, totalChecks-totalMismatch, totalChecks)
	if fixturesWithMismatch > 0 {
		bad := make([]string, 0, fixturesWithMismatch)
		for _, r := range results {
			if r.Mismatches > 0 || r.Error != "" {
				bad = append(bad, r.Name)
			}
		}
		fmt.Printf("期望需校准的夹具: %s\n", strings.Join(bad, " "))
	}

	if *jsonOut != "" {
		raw, _ := json.MarshalIndent(results, "", "  ")
		if err := os.WriteFile(*jsonOut, raw, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "写 json 失败: %v\n", err)
		}
	}
}

// normalizeSize 把截图裁到参考视口（浏览器可能把 window-size 解释成含
// 浏览器 UI，或截图比视口略大）。
func normalizeSize(img *image.RGBA) *image.RGBA {
	if img.Rect.Dx() == probelib.ViewportW && img.Rect.Dy() == probelib.ViewportH {
		return img
	}
	h := probelib.ViewportH
	if img.Rect.Dy() < h {
		h = img.Rect.Dy()
	}
	w := probelib.ViewportW
	if img.Rect.Dx() < w {
		w = img.Rect.Dx()
	}
	dst := image.NewRGBA(image.Rect(0, 0, probelib.ViewportW, probelib.ViewportH))
	// 用截图左上角像素铺底（夹具底色通常为白），再拷贝重叠区域。
	draw.Draw(dst, dst.Bounds(), image.NewUniform(img.At(0, 0)), image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(0, 0, w, h), img, image.Point{}, draw.Src)
	return dst
}

func copyFile(src, dst string) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, raw, 0o644)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
