package page

// ESM 转译器单元测试。
//
// 两组断言：
//  1. 转译形态（关键片段是否出现）；
//  2. **语法可编译**——把转译结果包进模块函数，用引擎后端（goja）实际编译一遍。
//     这一条是防回归的关键：goja parser 不认识 import/export，任何漏转的语句
//     都会在这里暴露成编译错误，而不是等到运行时。

import (
	"strings"
	"testing"

	"wb-ui/engine/js/goja"
)

// testResolver 把相对说明符解析到 http://test/ 下。
func testResolver(spec string) (string, error) {
	if strings.HasPrefix(spec, ".") {
		return "http://test/" + strings.TrimPrefix(strings.TrimPrefix(spec, "./"), "/"), nil
	}
	return spec, nil
}

func translate(t *testing.T, src string) (string, []string) {
	t.Helper()
	tr, err := TranslateModule(src, "http://test/m.js", testResolver)
	if err != nil {
		t.Fatalf("TranslateModule(%q) 失败: %v", src, err)
	}
	return tr.Body, tr.Deps
}

// assertCompiles 用 goja 编译「模块包装 + 转译体」，确保没有残留 ESM 语法。
func assertCompiles(t *testing.T, body string) {
	t.Helper()
	wrapped := `(function (__exports, __module, __require, __importMeta, __importDynamic, __export) { "use strict"; ` + body + `
})`
	if _, err := goja.Compile("esm-test.js", wrapped, false); err != nil {
		t.Fatalf("转译结果无法编译：%v\n--- body ---\n%s", err, body)
	}
}

func TestTranslateSideEffectImport(t *testing.T) {
	body, deps := translate(t, `import "./side.js";`)
	if !strings.Contains(body, `__require("http://test/side.js");`) {
		t.Fatalf("副作用导入未转译：%s", body)
	}
	if len(deps) != 1 || deps[0] != "http://test/side.js" {
		t.Fatalf("依赖列表不对：%v", deps)
	}
	assertCompiles(t, body)
}

func TestTranslateDefaultImport(t *testing.T) {
	body, _ := translate(t, `import React from "react"; React.x;`)
	if !strings.Contains(body, `__require("react")`) || !strings.Contains(body, `React =`) {
		t.Fatalf("默认导入未转译：%s", body)
	}
	assertCompiles(t, body)
}

func TestTranslateNamedAndNamespaceImports(t *testing.T) {
	cases := []string{
		`import { a, b as c } from "./m.js"; a(); c();`,
		`import * as ns from "./m.js"; ns.a();`,
		`import d, { a } from "./m.js"; d(); a();`,
		`import d, * as ns from "./m.js"; d(); ns.a();`,
		`import { "x-y" as z } from "./m.js"; z();`,
	}
	for _, src := range cases {
		body, _ := translate(t, src)
		if !strings.Contains(body, "__require(") {
			t.Fatalf("导入未转译：%s\n%s", src, body)
		}
		assertCompiles(t, body)
	}
}

func TestTranslateDefaultExportForms(t *testing.T) {
	cases := []string{
		`export default 42;`,
		`export default function foo() { return 1; }`,
		`export default function () { return 1; }`,
		`export default class Bar { m() { return 1; } }`,
		`export default class { m() { return 1; } }`,
		`export default { a: 1, b: [1, 2] };`,
		`export default async function load() { return 1; }`,
	}
	for _, src := range cases {
		body, _ := translate(t, src)
		if !strings.Contains(body, `__export("default"`) {
			t.Fatalf("默认导出未转译：%s\n%s", src, body)
		}
		assertCompiles(t, body)
	}
}

func TestTranslateExportDeclarations(t *testing.T) {
	cases := []string{
		`export const a = 1, b = 2;`,
		`export let x = 1;`,
		`export var { a, b: c } = obj;`,
		`export var [p, q] = arr;`,
		`export function f() {}`,
		`export async function g() {}`,
		`export class C {}`,
		`export { a, b as c };`,
		`export { a as default };`,
		`export { a, b as c } from "./m.js";`,
		`export { default } from "./m.js";`,
		`export * from "./m.js";`,
		`export * as ns from "./m.js";`,
	}
	for _, src := range cases {
		body, _ := translate(t, src)
		if !strings.Contains(body, "__export(") {
			t.Fatalf("导出未转译：%s\n%s", src, body)
		}
		assertCompiles(t, body)
	}
}

func TestTranslateDynamicImportAndMeta(t *testing.T) {
	body, _ := translate(t, `const p = import("./lazy.js"); const u = import.meta.url;`)
	if !strings.Contains(body, `__importDynamic("./lazy.js")`) {
		t.Fatalf("动态导入未转译：%s", body)
	}
	if !strings.Contains(body, `__importMeta.url`) {
		t.Fatalf("import.meta 未转译：%s", body)
	}
	assertCompiles(t, body)
}

// TestTranslateDoesNotTouchStringsAndComments 是误转译防护：
// 字符串 / 注释 / 模板字符串 / 正则里出现 import/export 必须原样保留。
func TestTranslateDoesNotTouchStringsAndComments(t *testing.T) {
	src := "// import x from './fake.js'\n" +
		"/* export const y = 1; */\n" +
		"var s = \"import a from './a.js'\";\n" +
		"var t = 'export const z = 2';\n" +
		"var u = `import ${'x'} from './tpl.js'`;\n" +
		"var re = /import\\s+/;\n" +
		"var o = { import: 1, export: 2 };\n" +
		"var fn = obj.import;\n"
	body, _ := translate(t, src)
	for _, want := range []string{"import x from './fake.js'", "export const y = 1;", `var s = "import a from './a.js'";`, "var o = { import: 1, export: 2 };", "obj.import"} {
		if !strings.Contains(body, want) {
			t.Fatalf("误转译包含了 %q 的文本：\n%s", want, body)
		}
	}
	assertCompiles(t, body)
}

// TestTranslateMultilineImport 覆盖构建产物里常见的多行导入。
func TestTranslateMultilineImport(t *testing.T) {
	src := "import {\n  a,\n  b as c,\n} from \"./m.js\";\nimport d,\n  { e } from \"./n.js\";\n"
	body, deps := translate(t, src)
	if len(deps) != 2 {
		t.Fatalf("多行导入依赖数不对：%v\n%s", deps, body)
	}
	assertCompiles(t, body)
}

// TestTranslateNestedExportInFunction 覆盖 export 语句出现在函数体内（合法：
// 模块顶层才允许，但转译器不应因此崩掉或吞代码）。
func TestTranslateKeepsSurroundingCode(t *testing.T) {
	src := "function boot() {\n  return 1;\n}\nimport { a } from './m.js';\nboot();\nexport const done = a;\nconsole.log('tail');\n"
	body, _ := translate(t, src)
	for _, want := range []string{"function boot()", "boot();", "console.log('tail');"} {
		if !strings.Contains(body, want) {
			t.Fatalf("转译丢失了代码 %q：\n%s", want, body)
		}
	}
	assertCompiles(t, body)
}
