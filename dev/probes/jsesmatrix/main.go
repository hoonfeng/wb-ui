// Command jsesmatrix —— JS 引擎特性矩阵探针（实现路径主线 C-P0）。
//
// 目的：把「引擎对浏览器标准的对标程度」变成**可回归比对的快照**。清单不手抄：
// 运行时读取 engine/js/goja/tc39_test.go 的 featuresBlackList（上游 goja 自己
// 标注的「已知未实现」特性集合），逐项在**引擎真实 JS 环境**（engine/js/jsc 适配
// 层，而不是裸 goja）里探测，输出 JSON 快照。
//
// 三点约定（对应 docs/implementation-path.md §4.5 / §4.6 的验收）：
//  1. **不允许「未知」**：清单里任何一项若没有对应探测表达式，探针报错退出并列出
//     ——清单与探测表必须同步演进。
//  2. **三态**：supported / missing / exempt（exempt = 明确豁免：规范已弃用的
//     tail-call-optimization，以及 tc39 测试框架的内部标记 __getter__/__setter__）。
//  3. **planned 标记**：缺失但在 §4.5 C-P1 排队补齐的特性（async-iteration、
//     Array.fromAsync / iterator-helpers、regexp v/d 标志）单独标出。
//
// 用法（仓库根目录）：
//
//	go run ./dev/probes/jsesmatrix                    # 摘要 + 写 dev/output/jsesmatrix.json
//	go run ./dev/probes/jsesmatrix -out x.json -v     # 指定输出并逐项打印
//
// 读法：探针跑的是**引擎环境**——引擎在 jsc/webapi.go 里自行补齐的能力（如
// structuredClone、WeakRef/FinalizationRegistry 的降级实现）会显示 supported，
// 这正是「与裸 goja 的差别」所在，也是判断「换引擎能解决多少缺口」的依据。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"wb-ui/engine/js/jsc"
)

// blacklistPath 是权威清单的来源（相对仓库根目录）。
const blacklistPath = "engine/js/goja/tc39_test.go"

// probeSpec 是一项特性的探测定义。
type probeSpec struct {
	Category string
	Expr     string // 返回 boolean 的 JS 表达式（在引擎环境里求值）
	Why      string
	Exempt   bool // 明确豁免：不探测，状态恒为 exempt
	Planned  bool // 缺失时标记为「已排期」（C-P1 优先级）
}

// featureSpecs 是特性名 → 探测定义。★ 与 featuresBlackList 一一对应（缺项时探针
// 报错退出，见 main）。
var featureSpecs = map[string]probeSpec{
	// ── 模块系统（最影响工程可用性的缺口）──
	"dynamic-import": {
		Category: "模块", Planned: true,
		Expr: `(function(){ try { new Function('import("x")'); return true } catch (e) { return false } })()`,
		Why:  "动态 import()：无模块系统 ⇒ 代码分割/懒加载不可用",
	},
	"import.meta": {
		Category: "模块",
		Expr:     `(function(){ try { new Function('return import.meta'); return true } catch (e) { return false } })()`,
		Why:      "模块上下文元信息（import.meta.url 等）",
	},
	"import-assertions": {
		Category: "模块",
		Expr:     `(function(){ try { new Function('import x from "y" assert { type: "json" }'); return true } catch (e) { return false } })()`,
		Why:      "导入断言（旧提案写法）",
	},
	"import-attributes": {
		Category: "模块",
		Expr:     `(function(){ try { new Function('import x from "y" with { type: "json" }'); return true } catch (e) { return false } })()`,
		Why:      "导入属性（新写法，JSON/CSS 模块依赖它）",
	},
	"source-phase-imports": {
		Category: "模块",
		Expr:     `(function(){ try { new Function('import source x from "y"'); return true } catch (e) { return false } })()`,
		Why:      "源阶段导入（Wasm 源码模块）",
	},
	"import-defer": {
		Category: "模块",
		Expr:     `(function(){ try { new Function('import defer * as ns from "y"'); return true } catch (e) { return false } })()`,
		Why:      "延迟导入",
	},

	// ── 异步迭代 ──
	"async-iteration": {
		Category: "异步", Planned: true,
		Expr: `typeof Symbol.asyncIterator !== 'undefined' && (function(){ try { new Function('async function* g(){ yield 1 }'); return true } catch (e) { return false } })()`,
		Why:  "Symbol.asyncIterator + async generator + for await",
	},
	"Symbol.asyncIterator": {
		Category: "异步",
		Expr:     `typeof Symbol.asyncIterator !== 'undefined'`,
		Why:      "异步迭代器符号（流式 API 的基础）",
	},

	// ── ArrayBuffer 系 ──
	"resizable-arraybuffer": {
		Category: "二进制",
		Expr:     `(function(){ try { var b = new ArrayBuffer(8, { maxByteLength: 16 }); return b.resizable === true } catch (e) { return false } })()`,
		Why:      "可变长 ArrayBuffer（Wasm 内存增长依赖）",
	},
	"immutable-arraybuffer": {
		Category: "二进制",
		Expr:     `typeof ArrayBuffer.prototype.transferToFixedLength === 'function'`,
		Why:      "不可变 ArrayBuffer（transferToFixedLength）",
	},
	"arraybuffer-transfer": {
		Category: "二进制",
		Expr:     `typeof ArrayBuffer.prototype.transfer === 'function'`,
		Why:      "零拷贝转移（transfer / transferToFixedLength）",
	},
	"uint8array-base64": {
		Category: "二进制",
		Expr:     `typeof Uint8Array.prototype.toBase64 === 'function' && typeof Uint8Array.fromBase64 === 'function'`,
		Why:      "Uint8Array 的 base64/hex 编解码",
	},
	"Float16Array": {
		Category: "二进制",
		Expr:     `typeof Float16Array === 'function'`,
		Why:      "半精度浮点数组",
	},

	// ── 正则 ──
	"regexp-v-flag": {
		Category: "正则", Planned: true,
		Expr: `(function(){ try { new RegExp('[\p{Script=Greek}]', 'v'); return true } catch (e) { return false } })()`,
		Why:  "v 标志（集合运算、更严的字符类）",
	},
	"regexp-match-indices": {
		Category: "正则", Planned: true,
		Expr: `(function(){ try { var r = new RegExp('a', 'd'); return typeof r.indices !== 'undefined' || r.hasIndices === true } catch (e) { return false } })()`,
		Why:  "d 标志（匹配位置索引，解析器/高亮库常用）",
	},
	"regexp-unicode-property-escapes": {
		Category: "正则",
		Expr:     `(function(){ try { new RegExp('\p{Script=Greek}', 'u'); return true } catch (e) { return false } })()`,
		Why:      `\p{...}` + " Unicode 属性转义",
	},
	"regexp-duplicate-named-groups": {
		Category: "正则",
		Expr:     `(function(){ try { new RegExp('(?<a>x)(?<a>y)', 'u'); return true } catch (e) { return false } })()`,
		Why:      "重复命名捕获组",
	},
	"regexp-modifiers": {
		Category: "正则",
		Expr:     `(function(){ try { new RegExp('(?i:a)'); return true } catch (e) { return false } })()`,
		Why:      "内联修饰符 (?i:…)",
	},
	"RegExp.escape": {
		Category: "正则",
		Expr:     `typeof RegExp.escape === 'function'`,
		Why:      "RegExp.escape（把字面串安全变正则）",
	},
	"legacy-regexp": {
		Category: "正则",
		Expr:     `(function(){ try { var re = new RegExp(/a/g); return re.source === 'a' && re.flags === 'g' } catch (e) { return false } })()`,
		Why:      "Annex B 的 legacy RegExp 行为（构造器接受正则对象）",
	},

	// ── 并发 / 内存 ──
	"Atomics": {
		Category: "并发",
		Expr:     `typeof Atomics === 'object' && Atomics !== null`,
		Why:      "Atomics（多线程/SharedArrayBuffer 配套）",
	},
	"Atomics.waitAsync": {
		Category: "并发",
		Expr:     `typeof Atomics === 'object' && typeof Atomics.waitAsync === 'function'`,
		Why:      "Atomics.waitAsync（非阻塞等待）",
	},
	"Atomics.pause": {
		Category: "并发",
		Expr:     `typeof Atomics === 'object' && typeof Atomics.pause === 'function'`,
		Why:      "Atomics.pause（自旋提示）",
	},
	"SharedArrayBuffer": {
		Category: "并发",
		Expr:     `typeof SharedArrayBuffer === 'function'`,
		Why:      "共享内存（Worker 间零拷贝）",
	},
	"WeakRef": {
		Category: "内存",
		Expr:     `typeof WeakRef === 'function'`,
		Why:      "弱引用（引擎已在 jsc/webapi.go 提供降级实现）",
	},
	"FinalizationRegistry": {
		Category: "内存",
		Expr:     `typeof FinalizationRegistry === 'function'`,
		Why:      "终结回调（引擎已在 jsc/webapi.go 提供降级实现）",
	},
	"symbols-as-weakmap-keys": {
		Category: "内存",
		Expr:     `(function(){ try { var m = new WeakMap(); m.set(Symbol('s'), 1); return true } catch (e) { return false } })()`,
		Why:      "Symbol 作为 WeakMap 键",
	},

	// ── 新标准 ──
	"Temporal": {
		Category: "新标准",
		Expr:     `typeof Temporal !== 'undefined'`,
		Why:      "Temporal（取代 Date 的新日期时间 API，成本极高）",
	},
	"ShadowRealm": {
		Category: "新标准",
		Expr:     `typeof ShadowRealm === 'function'`,
		Why:      "ShadowRealm（隔离 realm）",
	},
	"decorators": {
		Category: "新标准",
		Expr:     `(function(){ try { new Function('function dec(v, c) {}; @dec class A {}'); return true } catch (e) { return false } })()`,
		Why:      "装饰器语法（TS/Angular 生态）",
	},
	"explicit-resource-management": {
		Category: "新标准",
		Expr:     `(function(){ try { new Function('using x = null'); return true } catch (e) { return false } })()`,
		Why:      "using 声明（确定性释放资源）",
	},
	"set-methods": {
		Category: "新标准",
		Expr:     `typeof Set.prototype.union === 'function' && typeof Set.prototype.intersection === 'function'`,
		Why:      "Set 集合运算（union/intersection/difference…）",
	},
	"iterator-helpers": {
		Category: "新标准", Planned: true,
		Expr: `typeof Iterator === 'function' && typeof Iterator.prototype.map === 'function'`,
		Why:  "迭代器助手（map/filter/take…，链式处理惰性序列）",
	},
	"joint-iteration": {
		Category: "新标准",
		Expr:     `typeof Iterator === 'function' && typeof Iterator.zip === 'function'`,
		Why:      "Iterator.zip / zipKeyed（并发迭代）",
	},
	"iterator-sequencing": {
		Category: "新标准",
		Expr:     `typeof Iterator === 'function' && typeof Iterator.concat === 'function'`,
		Why:      "Iterator.concat（顺序拼接迭代器）",
	},
	"promise-try": {
		Category: "新标准",
		Expr:     `typeof Promise.try === 'function'`,
		Why:      "Promise.try",
	},
	"promise-with-resolvers": {
		Category: "新标准",
		Expr:     `typeof Promise.withResolvers === 'function'`,
		Why:      "Promise.withResolvers",
	},
	"array-grouping": {
		Category: "新标准",
		Expr:     `typeof Object.groupBy === 'function' && typeof Map.groupBy === 'function'`,
		Why:      "Object.groupBy / Map.groupBy",
	},
	"Array.fromAsync": {
		Category: "新标准", Planned: true,
		Expr: `typeof Array.fromAsync === 'function'`,
		Why:  "Array.fromAsync（从异步可迭代构造数组）",
	},
	"Math.sumPrecise": {
		Category: "新标准",
		Expr:     `typeof Math.sumPrecise === 'function'`,
		Why:      "Math.sumPrecise（精确求和）",
	},
	"String.prototype.toWellFormed": {
		Category: "新标准",
		Expr:     `typeof String.prototype.toWellFormed === 'function'`,
		Why:      "toWellFormed（替换孤立代理项，网络 API 前置）",
	},
	"String.prototype.isWellFormed": {
		Category: "新标准",
		Expr:     `typeof String.prototype.isWellFormed === 'function'`,
		Why:      "isWellFormed",
	},

	// ── 明确豁免 ──
	"tail-call-optimization": {
		Category: "豁免", Exempt: true,
		Why: "规范已弃用（V8/JSC 均已移除实现），不作为对标项",
	},
	"__getter__": {
		Category: "豁免", Exempt: true,
		Why: "tc39 测试框架的内部标记（不是 ES 特性）",
	},
	"__setter__": {
		Category: "豁免", Exempt: true,
		Why: "tc39 测试框架的内部标记（不是 ES 特性）",
	},
}

// featureResult 是一项探测结果。
type featureResult struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Status   string `json:"status"` // supported / missing / exempt
	Planned  bool   `json:"planned,omitempty"`
	Why      string `json:"why,omitempty"`
	Expr     string `json:"expr,omitempty"`
	Error    string `json:"error,omitempty"`
}

// snapshot 是 JSON 快照（可入库做回归比对）。
type snapshot struct {
	GeneratedAt string          `json:"generatedAt"`
	Engine      string          `json:"engine"`
	Source      string          `json:"source"`
	Summary     map[string]int  `json:"summary"`
	Features    []featureResult `json:"features"`
	// Note 说明判定方式，避免把「粗粒度探测通过」误读成「tc39 逐用例全过」。
	Note string `json:"note"`
}

func main() {
	out := flag.String("out", filepath.Join("dev", "output", "jsesmatrix.json"), "JSON 快照输出路径")
	verbose := flag.Bool("v", false, "逐项打印探测结果")
	flag.Parse()

	names, err := loadBlacklist(blacklistPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jsesmatrix: 读取权威清单失败: %v\n", err)
		os.Exit(1)
	}

	// ① 完整性：清单每项都必须有探测定义（不允许「未知」——这正是 C-P0 的验收）。
	var unknown []string
	for _, n := range names {
		if _, ok := featureSpecs[n]; !ok {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		fmt.Fprintf(os.Stderr, "jsesmatrix: 权威清单里有 %d 项没有探测定义（不允许「未知」，请补 featureSpecs）：\n  %s\n",
			len(unknown), strings.Join(unknown, "\n  "))
		os.Exit(1)
	}

	// ② 探测：在**引擎真实 JS 环境**里求值——jsc 适配层 + RegisterWebAPIs()
	// （webkit.ensureJSRuntime 也这么做：引擎在 jsc/webapi.go 自行补齐了
	// structuredClone、WeakRef/FinalizationRegistry 降级实现等）。只跑裸 goja
	// 会把「引擎已补的能力」误报成缺失。
	in := jsc.NewInterpreter()
	in.RegisterWebAPIs()
	results := make([]featureResult, 0, len(names))
	summary := map[string]int{"supported": 0, "missing": 0, "exempt": 0, "planned": 0}
	for _, name := range names {
		spec := featureSpecs[name]
		r := featureResult{Name: name, Category: spec.Category, Why: spec.Why}
		if spec.Exempt {
			r.Status = "exempt"
		} else {
			r.Expr = spec.Expr
			v, err := in.RunJS("!!(" + spec.Expr + ")")
			switch {
			case err != nil:
				r.Status = "missing"
				r.Error = err.Error()
			case v.ToBoolean():
				r.Status = "supported"
			default:
				r.Status = "missing"
			}
			if r.Status == "missing" && spec.Planned {
				r.Planned = true
				summary["planned"]++
			}
		}
		summary[r.Status]++
		results = append(results, r)
		if *verbose {
			mark := map[string]string{"supported": "✓", "missing": "✗", "exempt": "—"}[r.Status]
			extra := ""
			if r.Planned {
				extra = "（已排期 C-P1）"
			}
			fmt.Printf("  %s %-34s %-6s %s%s\n", mark, r.Name, r.Category, r.Status, extra)
		}
	}

	// ③ 摘要 + 快照。
	fmt.Printf("jsesmatrix: 清单 %d 项（supported %d / missing %d / exempt %d；其中 missing 已排期 %d）\n",
		len(results), summary["supported"], summary["missing"], summary["exempt"], summary["planned"])
	printNames("supported", results)
	printNames("missing", results)

	snap := snapshot{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Engine:      "wb-ui engine/js/jsc（goja 适配层 + jsc.RegisterWebAPIs 自补 Web）。",
		Source:      blacklistPath + " # featuresBlackList",
		Summary:     summary,
		Features:    results,
		Note: "判定方式 = 粗粒度能力探测（API 存在性 / 语法是否可编译），**不是** tc39 逐用例一致性：" +
			"某项显示 supported 只说明该能力可用，不排除语义细节仍有差异；细粒度缺口以 " +
			blacklistPath + "（本清单的来源）为准。",
	}
	raw, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "jsesmatrix: 序列化失败: %v\n", err)
		os.Exit(1)
	}
	if dir := filepath.Dir(*out); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "jsesmatrix: 建目录失败: %v\n", err)
			os.Exit(1)
		}
	}
	if err := os.WriteFile(*out, append(raw, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "jsesmatrix: 写快照失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("快照已写入 %s\n", *out)
}

// printNames 打印某状态的特性名（摘要用）。
func printNames(status string, results []featureResult) {
	var names []string
	for _, r := range results {
		if r.Status == status {
			names = append(names, r.Name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return
	}
	prefix := map[string]string{"supported": "已支持", "missing": "仍缺失"}[status]
	fmt.Printf("  %s（%d）：%s\n", prefix, len(names), strings.Join(names, ", "))
}

// loadBlacklist 从 tc39_test.go 解析 featuresBlackList（去重保序）——清单的单一
// 事实源就是上游测试文件，不在这里手抄一份（手抄必然漂移）。
func loadBlacklist(path string) ([]string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block := regexp.MustCompile(`(?s)featuresBlackList\s*=\s*\[\]string\{(.*?)\}`).FindSubmatch(src)
	if block == nil {
		return nil, fmt.Errorf("%s 里找不到 featuresBlackList", path)
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(string(block[1]), -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s 的 featuresBlackList 解析结果为空", path)
	}
	return out, nil
}
