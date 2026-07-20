// +build ignore

package main

import (
	"fmt"
	"os"
	"wb-ui/jsc"
)

var failedTests []string

func test(name, code string) {
	rt := jsc.NewInterpreter()
	_, err := rt.Run(code)
	if err != nil {
		fmt.Printf("  ✗ %s\n", name)
		failedTests = append(failedTests, name)
	} else {
		fmt.Printf("  ✓ %s\n", name)
	}
}

func main() {
	fmt.Println("=== JSC 引擎测试 ===")

	// 基础语法测试
	fmt.Println("\n--- 变量声明 ---")
	test("var声明", "var x = 42;")
	test("let声明", "let y = 'hello';")
	test("const声明", "const z = true;")
	test("多变量声明", "var a = 1, b = 2;")

	fmt.Println("\n--- 函数 ---")
	test("函数定义", "function add(a,b) { return a + b; }")
	test("箭头函数", "var fn = (x) => x * 2;")
	test("无参函数", "function hello() { return 'hi'; }")

	fmt.Println("\n--- 控制流 ---")
	test("if语句", "var x = 1; if (x > 0) { x = 2; }")
	test("for循环", "var sum = 0; for (var i = 0; i < 10; i++) { sum += i; }")
	test("while循环", "var n = 0; while (n < 5) { n++; }")
	test("switch语句", "var x = 2; var r = ''; switch(x) { case 1: r='a'; break; case 2: r='b'; break; }")

	fmt.Println("\n--- 对象与数组 ---")
	test("对象字面量", "var obj = {a: 1, b: 2};")
	test("数组", "var arr = [1,2,3,4,5];")
	test("嵌套对象", "var obj = {a: {b: {c: 1}}};")

	fmt.Println("\n--- 运算符 ---")
	test("算术运算", "var x = 1 + 2 * 3;")
	test("比较运算", "var x = 1 < 2 && 3 > 1;")
	test("三元运算符", "var x = true ? 1 : 0;")

	fmt.Println("\n--- ES6 特性 ---")
	test("模板字符串", "var x = `hello ${name}`;") // 需要先定义 name
	test("解构赋值", "var [a,b] = [1,2];")
	test("扩展运算符", "var arr = [...[1,2,3]];")
	test("箭头函数this", "var obj = {fn: () => 42};")

	fmt.Println("\n--- 错误处理 ---")
	test("try-catch", "try { throw new Error('test'); } catch(e) {}")

	fmt.Println("\n=== 测试结果 ===")
	if len(failedTests) > 0 {
		fmt.Printf("通过: %d, 失败: %d\n", 22-len(failedTests), len(failedTests))
		for _, n := range failedTests {
			fmt.Printf("  失败: %s\n", n)
		}
	} else {
		fmt.Println("全部通过 (22/22) ✓")
	}
	_ = os.Args
}
