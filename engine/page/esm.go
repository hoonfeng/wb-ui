package page

// ES Module 支持（宿主侧转译 + 模块加载）。
//
// 背景：本引擎的 JS 后端（vendored goja）的解析器**不支持** ES Module 语法——
// `import` / `export` 在 parser 里是 reserved word，直接抛
// 「Unexpected reserved word」（见 engine/js/goja/parser/parser_test.go）。
// 而现代前端项目的构建产物（Vite / Rollup / esbuild / Webpack 5 ESM 输出）
// 几乎全是 `<script type="module">` + import/export，因此「任意前端项目
// 直接加载」的第一道门槛就是它。
//
// 本文件做的是**语法层转译**：把 ESM 的 import/export 改写成 CommonJS 形态的
// 运行时调用（__require / __exports），模块体整体包进一个函数
// `(function (__exports, __module, __require, __importMeta) { ... })`。
// 真正的依赖加载、循环依赖处理、缓存与执行顺序在 webkit/esm.go 里完成。
//
// 为什么不用 AST：仓库内的 goja parser 不支持 ESM，引入第三方解析器成本更高；
// 这里用**词法扫描**（跳过字符串/模板/注释/正则），只在语句起始位置识别
// import/export 关键字，足以覆盖构建产物使用到的语法子集：
//
//	import "m";                       // 副作用
//	import d from "m";                // 默认导入
//	import * as ns from "m";          // 命名空间
//	import { a, b as c } from "m";    // 命名导入
//	import d, { a } from "m";
//	import d, * as ns from "m";
//	export default <expr>;            // 默认导出（含 function/class 声明）
//	export const/let/var ...;         // 声明导出
//	export function/class ...;
//	export { a, b as c } [from "m"];
//	export * from "m";
//	export * as ns from "m";
//	const p = import("m");            // 动态导入 → __importDynamic("m")
//	console.log(import.meta.url);     // → __importMeta.url
//
// 不支持（构建产物里罕见，遇到时按普通代码执行 → 运行期报错，不会静默错译）：
// import 断言（assert/with）、`export * as "string"`、顶层 TLA（await 在模块顶层
// 需要 async 包装——本引擎按同步执行，TLA 模块会报语法错）。

import (
	"fmt"
	"strings"
)

// TranslatedModule 是转译结果。
type TranslatedModule struct {
	// Body 是模块函数体（不含外层包装）；执行方按
	// `(function (__exports, __module, __require, __importMeta) { "use strict"; <Body> })`
	// 求值。
	Body string
	// Deps 是**静态**依赖的绝对 URL（按出现顺序、已去重）。动态 import 不在其中
	// （它在运行时经 __importDynamic 解析）。
	Deps []string
}

// TranslateModule 把一段 ESM 源码转译成模块函数体。
//
//	resolve 把模块说明符（"react"、"./a.js"、"/assets/x.js"）解析成绝对 URL；
//	返回错误表示该说明符无法解析（模块图构建失败，加载方应报错）。
//
// moduleURL 仅用于诊断信息（错误里指明是哪个模块出的问题）。
func TranslateModule(code, moduleURL string, resolve func(spec string) (string, error)) (*TranslatedModule, error) {
	t := &esmTranslator{src: code, moduleURL: moduleURL, resolve: resolve}
	if err := t.run(); err != nil {
		return nil, err
	}
	return &TranslatedModule{Body: t.out.String(), Deps: t.deps}, nil
}

type esmTranslator struct {
	src       string
	i         int
	out       strings.Builder
	deps      []string
	depIndex  int
	moduleURL string
	resolve   func(string) (string, error)
}

func (t *esmTranslator) run() error {
	src := t.src
	for t.i < len(src) {
		c := src[t.i]
		switch {
		case c == '/' && t.i+1 < len(src) && src[t.i+1] == '/':
			end := skipLineComment(src, t.i)
			t.out.WriteString(src[t.i:end])
			t.i = end
		case c == '/' && t.i+1 < len(src) && src[t.i+1] == '*':
			end := skipBlockComment(src, t.i)
			t.out.WriteString(src[t.i:end])
			t.i = end
		case c == '"' || c == '\'':
			t.copyString(c)
		case c == '`':
			t.copyTemplate()
		case isIdentStart(c):
			start := t.i
			ident := t.readIdent()
			if ident == "import" {
				// 动态 import(...) 与 import.meta 可以出现在任意表达式位置；
				// 静态 import 声明只出现在语句起始。
				next := t.peekNonSpace()
				if next == '(' || next == '.' || t.atStatementStart(start) {
					if err := t.handleKeyword(ident, start); err != nil {
						return err
					}
					continue
				}
			} else if ident == "export" && t.atStatementStart(start) {
				if err := t.handleKeyword(ident, start); err != nil {
					return err
				}
				continue
			}
			t.out.WriteString(ident)
		default:
			t.out.WriteByte(c)
			t.i++
		}
	}
	return nil
}

// atStatementStart 判断 pos 处的标识符是否处于语句起始位置：向前跳过空白，
// 前一个非空白字符为文件开头、';'、'{'、'}'、')'、']' 或换行（ASI 边界）。
// 这样 `obj.import`、`foo(import)` 之类不会被误判为声明。
func (t *esmTranslator) atStatementStart(pos int) bool {
	if pos > 0 && t.src[pos-1] == '.' {
		return false
	}
	j := pos - 1
	for j >= 0 {
		c := t.src[j]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			j--
			continue
		}
		switch c {
		case ';', '{', '}', ')', ']', '(', ',':
			return true
		}
		return false
	}
	return true
}

// peekNonSpace 返回当前位置之后的第一个非空白字符（没有则返回 0）。
func (t *esmTranslator) peekNonSpace() byte {
	for i := t.i; i < len(t.src); i++ {
		if !isSpace(t.src[i]) {
			return t.src[i]
		}
	}
	return 0
}

func (t *esmTranslator) handleKeyword(ident string, start int) error {
	// 关键字后跳过空白判断形态。
	j := t.i
	for j < len(t.src) && isSpace(t.src[j]) {
		j++
	}
	// 对象字面量的键名（`{ import: 1 }` / `{ export: 2 }`）不是声明：
	// 关键字后紧跟 ':' 或 ',' '=' '}' 时按普通标识符输出。
	if j < len(t.src) {
		switch t.src[j] {
		case ':', ',', '=', '}', ']':
			t.out.WriteString(ident)
			return nil
		}
	}
	if ident == "import" {
		if j < len(t.src) && t.src[j] == '(' {
			// 动态导入：import(...) → __importDynamic(...)
			t.out.WriteString("__importDynamic")
			t.i = j
			return nil
		}
		if j < len(t.src) && t.src[j] == '.' {
			// import.meta → __importMeta（消费掉 ".meta"，后续 ".url" 原样保留）
			k := j + 1
			if t.readIdentAt(k) == "meta" {
				t.out.WriteString("__importMeta")
				t.i = k + len("meta")
				return nil
			}
		}
		return t.translateImport(j)
	}
	return t.translateExport(j)
}

// ─── import ────────────────────────────────────────────────────────────────

func (t *esmTranslator) translateImport(pos int) error {
	src := t.src
	clauseStart := pos
	fromPos, specStart, specEnd, err := t.scanImportClause(pos)
	if err != nil {
		return err
	}
	if fromPos < 0 {
		// 副作用导入：import "m";
		spec, qErr := unquoteJSString(src[specStart:specEnd])
		if qErr != nil {
			return qErr
		}
		abs, rErr := t.resolveSpec(spec)
		if rErr != nil {
			return rErr
		}
		t.out.WriteString("__require(")
		t.writeJSString(abs)
		t.out.WriteString(");")
		t.i = t.skipSemicolon(specEnd + 1) // specEnd 指向结束引号，+1 越过它
		return nil
	}

	clause := strings.TrimSpace(src[clauseStart:fromPos])
	spec, qErr := unquoteJSString(src[specStart:specEnd])
	if qErr != nil {
		return qErr
	}
	abs, rErr := t.resolveSpec(spec)
	if rErr != nil {
		return rErr
	}
	modVar := fmt.Sprintf("__esm_dep_%d", t.depIndex)
	t.depIndex++
	t.out.WriteString("var ")
	t.out.WriteString(modVar)
	t.out.WriteString(" = __require(")
	t.writeJSString(abs)
	t.out.WriteString(");")
	if err := t.emitImportBindings(clause, modVar); err != nil {
		return err
	}
	t.i = t.skipSemicolon(specEnd + 1)
	return nil
}

// emitImportBindings 把 import clause 转成对模块命名空间的取值语句。
func (t *esmTranslator) emitImportBindings(clause, modVar string) error {
	clause = strings.TrimSpace(clause)
	if clause == "" {
		return nil
	}
	// 默认导入（可能后接 , { … } 或 , * as ns）
	if clause[0] != '{' && clause[0] != '*' {
		name, rest, err := readIdentPrefix(clause)
		if err != nil {
			return err
		}
		t.out.WriteString("var ")
		t.out.WriteString(name)
		t.out.WriteString(` = `)
		t.out.WriteString(modVar)
		t.out.WriteString(`["default"];`)
		clause = rest
	}
	if clause == "" {
		return nil
	}
	if strings.HasPrefix(clause, ",") {
		clause = strings.TrimSpace(clause[1:])
	}
	if clause == "" {
		return nil
	}
	if strings.HasPrefix(clause, "*") {
		rest := strings.TrimSpace(clause[1:])
		if !strings.HasPrefix(rest, "as") {
			return fmt.Errorf("esm: unsupported namespace import %q in %s", clause, t.moduleURL)
		}
		ns, _, err := readIdentPrefix(strings.TrimSpace(rest[2:]))
		if err != nil {
			return err
		}
		t.out.WriteString("var ")
		t.out.WriteString(ns)
		t.out.WriteString(" = ")
		t.out.WriteString(modVar)
		t.out.WriteString(";")
		return nil
	}
	if strings.HasPrefix(clause, "{") {
		items, err := parseNamedList(clause)
		if err != nil {
			return err
		}
		for _, it := range items {
			local := it.local
			if local == "" {
				local = it.imported
			}
			t.out.WriteString("var ")
			t.out.WriteString(local)
			t.out.WriteString(" = ")
			t.out.WriteString(modVar)
			t.out.WriteString("[")
			t.writeJSString(it.imported)
			t.out.WriteString("];")
		}
		return nil
	}
	return fmt.Errorf("esm: unsupported import clause %q in %s", clause, t.moduleURL)
}

// scanImportClause 在 pos（import 关键字之后）扫描到 `from` 关键字或首个字符串字面量。
// 返回值：
//
//	fromPos  —— `from` 的起始下标；副作用导入时为 -1
//	specStart/specEnd —— 引号内的说明符区间（不含引号）
func (t *esmTranslator) scanImportClause(pos int) (fromPos, specStart, specEnd int, err error) {
	src := t.src
	fromPos = -1
	i := pos
	for i < len(src) {
		c := src[i]
		switch {
		case c == '"' || c == '\'':
			// 首个字符串：副作用导入（没有 from）
			ss, ee, _, e := scanJSString(src, i)
			if e != nil {
				return 0, 0, 0, e
			}
			return -1, ss, ee, nil
		case c == '{':
			// 命名导入子句 `{ a, "x-y" as z }`：整段跳过——里面的字符串不是
			// 模块说明符（否则会被误判成副作用导入）。
			close := findMatching(src, i, '{', '}')
			if close < 0 {
				return 0, 0, 0, fmt.Errorf("esm: unterminated import clause in %s", t.moduleURL)
			}
			i = close + 1
		case isIdentStart(c):
			st := i
			word := t.readIdentAt(i)
			i += len(word)
			if word == "from" && st > pos {
				// from 之后必须是字符串
				for i < len(src) && isSpace(src[i]) {
					i++
				}
				if i >= len(src) || (src[i] != '"' && src[i] != '\'') {
					return 0, 0, 0, fmt.Errorf("esm: expected module specifier after 'from' in %s", t.moduleURL)
				}
				ss, ee, _, e := scanJSString(src, i)
				if e != nil {
					return 0, 0, 0, e
				}
				return st, ss, ee, nil
			}
		case c == '/':
			if i+1 < len(src) && src[i+1] == '/' {
				i = skipLineComment(src, i)
				continue
			}
			if i+1 < len(src) && src[i+1] == '*' {
				i = skipBlockComment(src, i)
				continue
			}
			i++
		default:
			i++
		}
	}
	return 0, 0, 0, fmt.Errorf("esm: unterminated import statement in %s", t.moduleURL)
}

// ─── export ────────────────────────────────────────────────────────────────

func (t *esmTranslator) translateExport(pos int) error {
	src := t.src
	j := pos
	for j < len(src) && isSpace(src[j]) {
		j++
	}
	if j >= len(src) {
		return fmt.Errorf("esm: unterminated export statement in %s", t.moduleURL)
	}
	// export default
	if word := t.readIdentAt(j); word == "default" {
		return t.translateExportDefault(j + len("default"))
	}
	// export * [as ns] from "m"
	if src[j] == '*' {
		return t.translateExportStar(j)
	}
	// export { … } [from "m"]
	if src[j] == '{' {
		return t.translateExportList(j)
	}
	// export var|let|const / function / class / async function
	word := t.readIdentAt(j)
	if word == "async" {
		k := j + len("async")
		for k < len(src) && isSpace(src[k]) {
			k++
		}
		if t.readIdentAt(k) == "function" {
			return t.translateExportDeclaration(j, k)
		}
	}
	switch word {
	case "var", "let", "const":
		return t.translateExportVariable(j, word)
	case "function", "class":
		return t.translateExportDeclaration(j, j)
	}
	return fmt.Errorf("esm: unsupported export form %q in %s", word, t.moduleURL)
}

// translateExportDefault 处理 export default …
func (t *esmTranslator) translateExportDefault(pos int) error {
	src := t.src
	// 跳到 default 之后的有效内容
	j := pos
	for j < len(src) && isSpace(src[j]) {
		j++
	}
	// export default function/class（可能有名字）
	kw := t.readIdentAt(j)
	declStart := j // async function 时 j 会移到 function，声明文本仍要从 async 开始
	isAsync := false
	if kw == "async" {
		k := j + len("async")
		for k < len(src) && isSpace(src[k]) {
			k++
		}
		if t.readIdentAt(k) == "function" {
			isAsync = true
			kw = "function"
			j = k
		}
	}
	if kw == "function" || kw == "class" {
		namePos := j + len(kw)
		for namePos < len(src) && isSpace(src[namePos]) {
			namePos++
		}
		name := ""
		if namePos < len(src) && isIdentStart(src[namePos]) {
			name = t.readIdentAt(namePos)
		}
		// 匿名 function/class：必须变成表达式才能赋值
		if name == "" {
			end := t.scanFunctionOrClassEnd(j)
			expr := strings.TrimSpace(src[j:end])
			t.out.WriteString(`__export("default", function () { return (`)
			t.out.WriteString(expr)
			t.out.WriteString(`); });`)
			t.i = t.skipSemicolon(end)
			return nil
		}
		_ = isAsync
		end := t.scanStatementEnd(declStart)
		decl := strings.TrimSpace(src[declStart:end])
		t.out.WriteString(decl)
		t.out.WriteString(";")
		t.out.WriteString(`__export("default", function () { return `)
		t.out.WriteString(name)
		t.out.WriteString(`; });`)
		t.i = t.skipSemicolon(end)
		return nil
	}
	// 表达式形式：export default <expr>;
	end := t.scanStatementEnd(j)
	expr := strings.TrimSpace(src[j:end])
	t.out.WriteString(`__export("default", function () { return (`)
	t.out.WriteString(expr)
	t.out.WriteString(`); });`)
	t.i = t.skipSemicolon(end)
	return nil
}

// translateExportStar 处理 export * [as ns] from "m"。
func (t *esmTranslator) translateExportStar(pos int) error {
	src := t.src
	i := pos + 1 // skip '*'
	for i < len(src) && isSpace(src[i]) {
		i++
	}
	asName := ""
	if t.readIdentAt(i) == "as" {
		i += len("as")
		for i < len(src) && isSpace(src[i]) {
			i++
		}
		name, _, err := readIdentPrefix(src[i:])
		if err != nil {
			return fmt.Errorf("esm: %v in %s", err, t.moduleURL)
		}
		asName = name
		i += len(name)
		for i < len(src) && isSpace(src[i]) {
			i++
		}
	}
	if t.readIdentAt(i) != "from" {
		return fmt.Errorf("esm: expected 'from' in export * statement in %s", t.moduleURL)
	}
	i += len("from")
	for i < len(src) && isSpace(src[i]) {
		i++
	}
	if i >= len(src) || (src[i] != '"' && src[i] != '\'') {
		return fmt.Errorf("esm: expected module specifier in export * in %s", t.moduleURL)
	}
	ss, ee, next, err := scanJSString(src, i)
	if err != nil {
		return err
	}
	spec, err := unquoteJSString(src[ss:ee])
	if err != nil {
		return err
	}
	abs, err := t.resolveSpec(spec)
	if err != nil {
		return err
	}
	tmp := fmt.Sprintf("__esm_re_%d", t.depIndex)
	t.depIndex++
	t.out.WriteString("var ")
	t.out.WriteString(tmp)
	t.out.WriteString(" = __require(")
	t.writeJSString(abs)
	t.out.WriteString(");")
	if asName != "" {
		t.out.WriteString(`__export(`)
		t.writeJSString(asName)
		t.out.WriteString(`, function () { return `)
		t.out.WriteString(tmp)
		t.out.WriteString(`; });`)
	} else {
		// 星号再导出：复制除 default 外的全部可枚举键（不覆盖本地已有导出）。
		t.out.WriteString(`Object.keys(`)
		t.out.WriteString(tmp)
		t.out.WriteString(`).forEach(function (__k) { if (__k !== "default" && !(__k in __exports)) { __export(__k, function () { return `)
		t.out.WriteString(tmp)
		t.out.WriteString(`[__k]; }); } });`)
	}
	t.i = t.skipSemicolon(next)
	return nil
}

// translateExportList 处理 export { a, b as c } [from "m"]。
func (t *esmTranslator) translateExportList(pos int) error {
	src := t.src
	closeIdx := findMatching(src, pos, '{', '}')
	if closeIdx < 0 {
		return fmt.Errorf("esm: unterminated export list in %s", t.moduleURL)
	}
	listText := src[pos : closeIdx+1]
	items, err := parseNamedList(listText)
	if err != nil {
		return fmt.Errorf("esm: %v in %s", err, t.moduleURL)
	}
	i := closeIdx + 1
	for i < len(src) && isSpace(src[i]) {
		i++
	}
	fromMod := ""
	if t.readIdentAt(i) == "from" {
		i += len("from")
		for i < len(src) && isSpace(src[i]) {
			i++
		}
		if i >= len(src) || (src[i] != '"' && src[i] != '\'') {
			return fmt.Errorf("esm: expected module specifier in export {…} from in %s", t.moduleURL)
		}
		ss, ee, next, err := scanJSString(src, i)
		if err != nil {
			return err
		}
		spec, err := unquoteJSString(src[ss:ee])
		if err != nil {
			return err
		}
		abs, err := t.resolveSpec(spec)
		if err != nil {
			return err
		}
		tmp := fmt.Sprintf("__esm_re_%d", t.depIndex)
		t.depIndex++
		t.out.WriteString("var ")
		t.out.WriteString(tmp)
		t.out.WriteString(" = __require(")
		t.writeJSString(abs)
		t.out.WriteString(");")
		fromMod = tmp
		i = next
	}
	for _, it := range items {
		// `export { local as exported }`：as 前是本地名，as 后是导出名
		// （与 import 的方向相反——import 是 `{ imported as local }`）。
		localName := it.imported
		exportedName := it.local
		if exportedName == "" {
			exportedName = it.imported
		}
		expr := localName
		if fromMod != "" {
			expr = fromMod + "[" + jsQuote(localName) + "]"
		}
		t.out.WriteString("__export(")
		t.writeJSString(exportedName)
		t.out.WriteString(", function () { return ")
		t.out.WriteString(expr)
		t.out.WriteString("; });")
	}
	t.i = t.skipSemicolon(i)
	return nil
}

// translateExportDeclaration 处理 export function/class/async function。
func (t *esmTranslator) translateExportDeclaration(j, declStart int) error {
	src := t.src
	end := t.scanStatementEnd(declStart)
	decl := strings.TrimSpace(src[declStart:end])
	t.out.WriteString(decl)
	t.out.WriteString(";")
	// 取声明名：function/class 关键字后的标识符
	name := ""
	kw := t.readIdentAt(declStart)
	k := declStart + len(kw)
	if kw == "async" {
		for k < len(src) && isSpace(src[k]) {
			k++
		}
		k += len("function")
	}
	for k < len(src) && isSpace(src[k]) {
		k++
	}
	if name == "" && k < len(src) && isIdentStart(src[k]) {
		name = t.readIdentAt(k)
	}
	if name == "" {
		return fmt.Errorf("esm: cannot determine exported declaration name in %s", t.moduleURL)
	}
	t.out.WriteString("__export(")
	t.writeJSString(name)
	t.out.WriteString(", function () { return ")
	t.out.WriteString(name)
	t.out.WriteString("; });")
	t.i = t.skipSemicolon(end)
	return nil
}

// translateExportVariable 处理 export var/let/const 声明。
func (t *esmTranslator) translateExportVariable(j int, kw string) error {
	src := t.src
	end := t.scanStatementEnd(j)
	decl := strings.TrimSpace(src[j:end])
	t.out.WriteString(decl)
	t.out.WriteString(";")
	names, err := parseDeclaredNames(decl, kw)
	if err != nil {
		return fmt.Errorf("esm: %v in %s", err, t.moduleURL)
	}
	for _, n := range names {
		t.out.WriteString("__export(")
		t.writeJSString(n)
		t.out.WriteString(", function () { return ")
		t.out.WriteString(n)
		t.out.WriteString("; });")
	}
	t.i = t.skipSemicolon(end)
	return nil
}

// ─── 扫描辅助 ──────────────────────────────────────────────────────────────

func (t *esmTranslator) resolveSpec(spec string) (string, error) {
	if t.resolve == nil {
		return spec, nil
	}
	abs, err := t.resolve(spec)
	if err != nil {
		return "", fmt.Errorf("esm: cannot resolve %q in %s: %w", spec, t.moduleURL, err)
	}
	for _, d := range t.deps {
		if d == abs {
			return abs, nil
		}
	}
	t.deps = append(t.deps, abs)
	return abs, nil
}

func (t *esmTranslator) writeJSString(s string) {
	t.out.WriteString(jsQuote(s))
}

func (t *esmTranslator) readIdent() string {
	s := t.readIdentAt(t.i)
	t.i += len(s)
	return s
}

func (t *esmTranslator) readIdentAt(pos int) string {
	i := pos
	for i < len(t.src) && isIdentPart(t.src[i]) {
		i++
	}
	return t.src[pos:i]
}

// scanStatementEnd 从 pos 起扫描到顶层语句结束（depth 0 的 ';' 或换行/块尾）。
// 返回结束下标（不含）。
func (t *esmTranslator) scanStatementEnd(pos int) int {
	src := t.src
	depth := 0
	for i := pos; i < len(src); i++ {
		c := src[i]
		switch c {
		case '"', '\'':
			_, _, next, err := scanJSString(src, i)
			if err != nil {
				return len(src)
			}
			i = next - 1
		case '`':
			next := skipTemplate(src, i)
			i = next - 1
		case '/':
			if i+1 < len(src) && src[i+1] == '/' {
				i = skipLineComment(src, i) - 1
			} else if i+1 < len(src) && src[i+1] == '*' {
				i = skipBlockComment(src, i) - 1
			}
		case '{', '(', '[':
			depth++
		case '}', ')', ']':
			if depth == 0 {
				if c == '}' {
					return i
				}
				depth--
			} else {
				depth--
			}
		case ';':
			if depth == 0 {
				return i
			}
		}
	}
	return len(src)
}

func (t *esmTranslator) skipSemicolon(pos int) int {
	i := pos
	for i < len(t.src) && (t.src[i] == ' ' || t.src[i] == '\t' || t.src[i] == '\r' || t.src[i] == '\n') {
		i++
	}
	if i < len(t.src) && t.src[i] == ';' {
		i++
	}
	return i
}

// scanFunctionOrClassEnd 从 function/class 声明起始扫描到其主体结束（匹配的
// '}' 之后）。用于 `export default function () { … }` 这类匿名默认导出：
// 不能依赖 scanStatementEnd（函数表达式后面可能没有分号，会吞掉下一条语句）。
func (t *esmTranslator) scanFunctionOrClassEnd(pos int) int {
	src := t.src
	paren := 0
	for i := pos; i < len(src); {
		switch src[i] {
		case '"', '\'':
			_, _, next, err := scanJSString(src, i)
			if err != nil {
				return len(src)
			}
			i = next
		case '`':
			i = skipTemplate(src, i)
		case '/':
			if i+1 < len(src) && src[i+1] == '/' {
				i = skipLineComment(src, i)
			} else if i+1 < len(src) && src[i+1] == '*' {
				i = skipBlockComment(src, i)
			} else {
				i++
			}
		case '(':
			paren++
			i++
		case ')':
			if paren > 0 {
				paren--
			}
			i++
		case '{':
			if paren == 0 {
				close := findMatching(src, i, '{', '}')
				if close < 0 {
					return len(src)
				}
				return close + 1
			}
			i++
		default:
			i++
		}
	}
	return len(src)
}

func (t *esmTranslator) copyString(quote byte) {
	start := t.i
	ss, ee, next, err := scanJSString(t.src, t.i)
	if err != nil {
		t.out.WriteString(t.src[start:])
		t.i = len(t.src)
		return
	}
	_ = ss
	_ = ee
	t.out.WriteString(t.src[start:next])
	t.i = next
}

func (t *esmTranslator) copyTemplate() {
	next := skipTemplate(t.src, t.i)
	t.out.WriteString(t.src[t.i:next])
	t.i = next
}

// ─── 词法工具 ──────────────────────────────────────────────────────────────

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isIdentPart(c byte) bool { return isIdentStart(c) || (c >= '0' && c <= '9') }

// scanJSString 从 quote 位置扫描一个单/双引号字符串，返回内容区间与结束后下标。
func scanJSString(src string, start int) (contentStart, contentEnd, next int, err error) {
	quote := src[start]
	i := start + 1
	for i < len(src) {
		if src[i] == '\\' {
			i += 2
			continue
		}
		if src[i] == quote {
			return start + 1, i, i + 1, nil
		}
		i++
	}
	return 0, 0, 0, fmt.Errorf("esm: unterminated string literal")
}

// skipTemplate 扫描到模板字符串结尾（处理 ${ … } 嵌套）。
func skipTemplate(src string, start int) int {
	i := start + 1
	for i < len(src) {
		switch src[i] {
		case '\\':
			i += 2
		case '`':
			return i + 1
		case '$':
			if i+1 < len(src) && src[i+1] == '{' {
				i = skipTemplateExpr(src, i+2)
				continue
			}
			i++
		default:
			i++
		}
	}
	return len(src)
}

// skipTemplateExpr 跳过模板表达式 ${ … }（从 '{' 之后开始），返回 '}' 之后下标。
func skipTemplateExpr(src string, start int) int {
	depth := 1
	i := start
	for i < len(src) {
		switch src[i] {
		case '{':
			depth++
			i++
		case '}':
			depth--
			i++
			if depth == 0 {
				return i
			}
		case '"', '\'':
			_, _, next, err := scanJSString(src, i)
			if err != nil {
				return len(src)
			}
			i = next
		case '`':
			i = skipTemplate(src, i)
		default:
			i++
		}
	}
	return len(src)
}

func skipLineComment(src string, start int) int {
	i := start
	for i < len(src) && src[i] != '\n' {
		i++
	}
	return i
}

func skipBlockComment(src string, start int) int {
	i := start + 2
	for i+1 < len(src) {
		if src[i] == '*' && src[i+1] == '/' {
			return i + 2
		}
		i++
	}
	return len(src)
}

// findMatching 从 start（必须是 open 字符）找到匹配的 close。
func findMatching(src string, start int, open, close byte) int {
	depth := 0
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '"', '\'':
			_, _, next, err := scanJSString(src, i)
			if err != nil {
				return -1
			}
			i = next - 1
		case '`':
			i = skipTemplate(src, i) - 1
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// namedItem 是一项「导入名 [as 本地名]」。
type namedItem struct{ imported, local string }

// parseNamedList 解析 `{ a, b as c, "d" as e }`（大括号已含在 text 内）。
func parseNamedList(text string) ([]namedItem, error) {
	text = strings.TrimSpace(text)
	if len(text) < 2 || text[0] != '{' {
		return nil, fmt.Errorf("bad named list %q", text)
	}
	inner := strings.TrimSpace(text[1 : len(text)-1])
	if inner == "" {
		return nil, nil
	}
	var out []namedItem
	for _, part := range splitTopLevel(inner, ',') {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, rest, err := readIdentOrString(part)
		if err != nil {
			return nil, err
		}
		item := namedItem{imported: name}
		rest = strings.TrimSpace(rest)
		if rest != "" {
			if !strings.HasPrefix(rest, "as ") {
				return nil, fmt.Errorf("unsupported named import %q", part)
			}
			local, after, err := readIdentOrString(strings.TrimSpace(rest[3:]))
			if err != nil {
				return nil, err
			}
			_ = after
			item.local = local
		}
		out = append(out, item)
	}
	return out, nil
}

// readIdentOrString 读取一个标识符或字符串字面量，返回名字与剩余文本。
func readIdentOrString(s string) (name, rest string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", fmt.Errorf("empty name")
	}
	if s[0] == '"' || s[0] == '\'' {
		ss, ee, next, e := scanJSString(s, 0)
		if e != nil {
			return "", "", e
		}
		n, e2 := unquoteJSString(s[ss:ee])
		if e2 != nil {
			return "", "", e2
		}
		return n, s[next:], nil
	}
	if !isIdentStart(s[0]) {
		return "", "", fmt.Errorf("expected identifier, got %q", s)
	}
	i := 0
	for i < len(s) && isIdentPart(s[i]) {
		i++
	}
	return s[:i], s[i:], nil
}

func readIdentPrefix(s string) (name, rest string, err error) {
	s = strings.TrimSpace(s)
	if s == "" || !isIdentStart(s[0]) {
		return "", "", fmt.Errorf("expected identifier, got %q", s)
	}
	i := 0
	for i < len(s) && isIdentPart(s[i]) {
		i++
	}
	return s[:i], strings.TrimSpace(s[i:]), nil
}

// splitTopLevel 按顶层分隔符切分（跳过括号/字符串/模板）。
func splitTopLevel(s string, sep byte) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"', '\'':
			_, _, next, err := scanJSString(s, i)
			if err != nil {
				return append(out, s[start:])
			}
			i = next - 1
		case '`':
			i = skipTemplate(s, i) - 1
		case '{', '(', '[':
			depth++
		case '}', ')', ']':
			depth--
		default:
			if s[i] == sep && depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, s[start:])
	return out
}

// parseDeclaredNames 从 `var/let/const <decls>` 中提取被声明的绑定名。
func parseDeclaredNames(decl, kw string) ([]string, error) {
	s := strings.TrimSpace(decl)
	s = strings.TrimSpace(strings.TrimPrefix(s, kw))
	var names []string
	for _, part := range splitTopLevel(s, ',') {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// 去掉初始化部分（顶层 '=' 之前）
		lhs := part
		if eq := topLevelIndex(part, '='); eq >= 0 {
			lhs = strings.TrimSpace(part[:eq])
		}
		ns, err := bindingNames(lhs)
		if err != nil {
			return nil, err
		}
		names = append(names, ns...)
	}
	return names, nil
}

// bindingNames 解析一个绑定模式（标识符 / 对象解构 / 数组解构）里的名字。
func bindingNames(pattern string) ([]string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, nil
	}
	switch pattern[0] {
	case '{':
		close := findMatching(pattern, 0, '{', '}')
		if close < 0 {
			return nil, fmt.Errorf("unterminated object pattern %q", pattern)
		}
		var names []string
		for _, part := range splitTopLevel(pattern[1:close], ',') {
			part = strings.TrimSpace(part)
			if part == "" || strings.HasPrefix(part, "...") {
				// 剩余元素：取后面的标识符
				if strings.HasPrefix(part, "...") {
					if n, _, err := readIdentOrString(strings.TrimSpace(part[3:])); err == nil {
						names = append(names, n)
					}
				}
				continue
			}
			// key: value / key（简写）/ key = default
			if colon := topLevelIndex(part, ':'); colon >= 0 {
				val := strings.TrimSpace(part[colon+1:])
				if eq := topLevelIndex(val, '='); eq >= 0 {
					val = strings.TrimSpace(val[:eq])
				}
				ns, err := bindingNames(val)
				if err != nil {
					return nil, err
				}
				names = append(names, ns...)
				continue
			}
			// 简写 / 带默认值
			if eq := topLevelIndex(part, '='); eq >= 0 {
				part = strings.TrimSpace(part[:eq])
			}
			ns, err := bindingNames(part)
			if err != nil {
				return nil, err
			}
			names = append(names, ns...)
		}
		return names, nil
	case '[':
		close := findMatching(pattern, 0, '[', ']')
		if close < 0 {
			return nil, fmt.Errorf("unterminated array pattern %q", pattern)
		}
		var names []string
		for _, part := range splitTopLevel(pattern[1:close], ',') {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if strings.HasPrefix(part, "...") {
				part = strings.TrimSpace(part[3:])
			}
			if eq := topLevelIndex(part, '='); eq >= 0 {
				part = strings.TrimSpace(part[:eq])
			}
			ns, err := bindingNames(part)
			if err != nil {
				return nil, err
			}
			names = append(names, ns...)
		}
		return names, nil
	}
	n, _, err := readIdentOrString(pattern)
	if err != nil {
		return nil, err
	}
	return []string{n}, nil
}

// topLevelIndex 返回顶层（跳过括号/字符串/模板）字符 ch 的下标，找不到返回 -1。
func topLevelIndex(s string, ch byte) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"', '\'':
			_, _, next, err := scanJSString(s, i)
			if err != nil {
				return -1
			}
			i = next - 1
		case '`':
			i = skipTemplate(s, i) - 1
		case '{', '(', '[':
			depth++
		case '}', ')', ']':
			depth--
		default:
			if s[i] == ch && depth == 0 {
				return i
			}
		}
	}
	return -1
}

// unquoteJSString 还原 JS 字符串字面量内容（处理常见转义）。
func unquoteJSString(s string) (string, error) {
	if len(s) < 2 || (s[0] != '"' && s[0] != '\'') {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(s) {
			break
		}
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'v':
			b.WriteByte('\v')
		case '0':
			b.WriteByte(0)
		case 'x':
			if i+2 < len(s) {
				var v int
				fmt.Sscanf(s[i+1:i+3], "%x", &v)
				b.WriteByte(byte(v))
				i += 2
			}
		case 'u':
			if i+4 < len(s) {
				var v int
				fmt.Sscanf(s[i+1:i+5], "%x", &v)
				b.WriteRune(rune(v))
				i += 4
			}
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String(), nil
}

// jsQuote 生成 JS 双引号字符串字面量。
func jsQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '<':
			// 防止 "<!--" 触发 HTML 解析器的脚本内容转义（模块里可能出现）
			b.WriteString(`\u003c`)
		case '>':
			b.WriteString(`\u003e`)
		case '&':
			b.WriteString(`\u0026`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}
