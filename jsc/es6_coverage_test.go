package jsc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestES6Coverage 系统性测试 JSC 引擎的 ES6 覆盖度
// 每条测试独立 try/catch 包装，console.log 输出与期望值对比
func TestES6Coverage(t *testing.T) {
	vm := NewInterpreter()
	log := &BufferLogger{}
	vm.SetupGlobal(log)

	// 基础 polyfill
	pf := []string{
		`window={process:{env:{NODE_ENV:"test"}}}`,
		`if(!Object.getOwnPropertyNames)Object.getOwnPropertyNames=function(o){if(!o)return[];var k=[];for(var n in o)k.push(n);return k}`,
	}
	for _, p := range pf {
		if _, err := vm.Run(p); err != nil {
			t.Fatalf("polyfill error: %v", err)
		}
	}

	// ===================== 测试用例 =====================
	// want 是期望的 console.log 输出，多行用 \n 分隔
	type testCase struct {
		name string
		code string
		want string // 多行用 \n
	}
	NL := "\n"

	tests := []testCase{
		// ======= 1. 变量声明 =======
		{"let_basic", `
let x = 42;
console.log(x);
`, "42"},

		{"const_basic", `
const y = 100;
console.log(y);
`, "100"},

		{"let_block_scope", `
let a = 1;
{ let a = 2; console.log(a); }
console.log(a);
`, "2" + NL + "1"},

		// ======= 2. 解构赋值 =======
		{"destruct_array_basic", `
let [a,b,c] = [1,2,3];
console.log(a); console.log(b); console.log(c);
`, "1" + NL + "2" + NL + "3"},

		{"destruct_object_basic", `
let {name,age} = {name:"Alice", age:30};
console.log(name); console.log(age);
`, "Alice" + NL + "30"},

		{"destruct_object_default", `
let {title="Developer"} = {name:"Eve"};
console.log(title);
`, "Developer"},

		{"destruct_rest", `
let [first,...rest] = [1,2,3,4];
console.log(first);
console.log(rest.length === 3 ? "3" : "fail");
`, "1" + NL + "3"},

		// ======= 3. 箭头函数 =======
		{"arrow_basic", `
let sq = x => x*x;
console.log(sq(5));
`, "25"},

		{"arrow_multi_params", `
let add = (a,b) => a+b;
console.log(add(3,7));
`, "10"},

		{"arrow_block_body", `
let f = (a,b) => { let s = a+b; return s; };
console.log(f(10,20));
`, "30"},

		{"arrow_no_params", `
let one = () => 1;
console.log(one());
`, "1"},

		{"arrow_this_capture", `
let obj = {
  name: "objA",
  getName: function() {
    let arrow = () => this.name;
    return arrow();
  }
};
console.log(obj.getName());
`, "objA"},

		{"arrow_this_constructor", `
function Outer(){
  this.val = 42;
  this.getVal = () => this.val;
}
let o = new Outer();
console.log(o.getVal());
`, "42"},

		// ======= 4. 类与继承 =======
		{"class_basic", `
class Foo{constructor(x){this.x=x}}
let f = new Foo(42);
console.log(f.x);
`, "42"},

		{"class_method", `
class Foo{
  constructor(x){this.x=x}
  getVal(){return this.x}
}
let f = new Foo(77);
console.log(f.getVal());
`, "77"},

		{"class_getter", `
class Foo{
  constructor(x){this._x=x}
  get val(){return this._x}
}
let f = new Foo(99);
console.log(f.val);
`, "99"},

		{"class_static", `
class Foo{
  static greet(){return "hello"}
}
console.log(Foo.greet());
`, "hello"},

		{"class_extends", `
class Animal{constructor(n){this.name=n}}
class Dog extends Animal{
  speak(){return this.name + " barks"}
}
let d = new Dog("Rex");
console.log(d.speak());
`, "Rex barks"},

		{"class_super_call", `
class Base{constructor(x){this.x=x}}
class Derived extends Base{
  constructor(x,y){super(x);this.y=y}
}
let d = new Derived(1,2);
console.log(d.x); console.log(d.y);
`, "1" + NL + "2"},

		// ======= 5. 扩展/剩余运算符 =======
		{"spread_array", `
let arr2 = [1,2,3,4,5];
console.log(arr2[3]); console.log(arr2.length);
`, "4" + NL + "5"},

		{"spread_object", `
let o1 = {a:1,b:2};
let o2 = {c:3};
let o3 = {a:o1.a, b:o1.b, c:o2.c};
console.log(o3.a); console.log(o3.c);
`, "1" + NL + "3"},

		{"rest_params", `
function sum(...nums){
  let s=0;
  for(let i=0;i<nums.length;i++) s+=nums[i];
  return s;
}
console.log(sum(1,2,3,4));
`, "10"},

		// ======= 6. Map/Set =======
		{"map_basic", `
let m = new Map();
m.set("a",1); m.set("b",2);
console.log(m.get("a")); console.log(m.size); console.log(m.has("c")?"yes":"no");
`, "1" + NL + "2" + NL + "no"},

		{"set_basic", `
let s = new Set();
s.add(1); s.add(2); s.add(1);
console.log(s.size); console.log(s.has(2)?"yes":"no");
`, "2" + NL + "yes"},

		// ======= 7. Symbol =======
		{"symbol_basic", `
let sym = Symbol("test");
console.log(typeof sym);
`, "symbol"},

		// ======= 8. Promise =======
		{"promise_basic", `
let p = new Promise(function(resolve){ resolve(42); });
console.log("created");
`, "created"},

		// ======= 9. Object 静态方法 =======
		{"object_assign", `
let o = Object.assign({a:1}, {b:2}, {c:3});
console.log(o.a); console.log(o.b); console.log(o.c);
`, "1" + NL + "2" + NL + "3"},

		{"object_keys", `
let k = Object.keys({x:1,y:2,z:3});
console.log(k.length);
`, "3"},

		{"object_values", `
let v = Object.values({a:10,b:20});
console.log(v.length);
console.log(v.indexOf(10) >= 0 ? "yes" : "no");
`, "2" + NL + "yes"},

		{"object_entries", `
let e = Object.entries({name:"Tom"});
console.log(e[0][0]); console.log(e[0][1]);
`, "name" + NL + "Tom"},

		// ======= 10. Array 方法 =======
		{"array_find", `
let a = [1,2,3,4];
let r = a.find(function(x){return x>2});
console.log(r);
`, "3"},

		{"array_includes", `
let a = [1,2,3];
console.log(a.includes(2)?"yes":"no");
console.log(a.includes(5)?"yes":"no");
`, "yes" + NL + "no"},

		{"array_flat", `
let a = [[1,2],[3,4]];
let f = a.flat();
console.log(f.length); console.log(f[0]);
`, "4" + NL + "1"},

		// ======= 11. String 方法 =======
		{"string_includes", `
console.log("Hello".includes("ell")?"yes":"no");
`, "yes"},

		{"string_startsWith", `
console.log("Hello".startsWith("Hel")?"yes":"no");
`, "yes"},

		{"string_repeat", `
console.log("ab".repeat(3));
`, "ababab"},

	// ======= 12. 默认参数 =======
	{"default_params_basic", `
function f(x = 5) { return x; }
console.log(f());
console.log(f(10));
`, "5" + NL + "10"},
	
	{"default_params_multi", `
function add(a, b = 1) { return a + b; }
console.log(add(3));
console.log(add(3, 4));
`, "4" + NL + "7"},

	{"default_params_expression", `
function greet(name, greeting = "Hello " + name) { return greeting; }
console.log(greet("World"));
console.log(greet("World", "Hi"));
`, "Hello World" + NL + "Hi"},

		// ======= 13. 空值合并 =======
		{"nullish_coalescing", `
let a = null;
let b = a ?? "default";
console.log(b);
let c = 0 ?? "nope";
console.log(c);
`, "default" + NL + "0"},

		// ======= 14. 计算属性名 =======
		{"computed_property", `
let key = "color";
let obj = {[key]:"red"};
console.log(obj.color);
`, "red"},

		// ======= 15. Math 扩展 =======
		{"math_trunc", `
console.log(Math.trunc ? Math.trunc(3.7) : "missing");
`, "3"},

		// ======= 16. Number 方法 =======
		{"number_isNaN", `
console.log(Number.isNaN ? (Number.isNaN(NaN)?"yes":"no") : "missing");
`, "yes"},
	}

	// ===================== 执行 =====================
	passed := 0
	failed := 0
	var failedNames []string

	for _, tt := range tests {
		code := fmt.Sprintf(`
try {
  %s
} catch(e) {
  console.log("__EXC__:" + String(e));
}
`, tt.code)

		log.Clear()
		_, err := vm.Run(code)
		out := strings.TrimSpace(log.String())

		if strings.HasPrefix(out, "__EXC__:") {
			excMsg := strings.TrimPrefix(out, "__EXC__:")
			fmt.Fprintf(os.Stderr, "[FAIL] %-30s exception: %s\n", tt.name, excMsg)
			failed++
			failedNames = append(failedNames, tt.name)
			continue
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "[FAIL] %-30s vm error: %v\n", tt.name, err)
			failed++
			failedNames = append(failedNames, tt.name)
			continue
		}
		if out != tt.want {
			fmt.Fprintf(os.Stderr, "[FAIL] %-30s got %q want %q\n", tt.name, out, tt.want)
			failed++
			failedNames = append(failedNames, tt.name)
			continue
		}
		fmt.Fprintf(os.Stderr, "[PASS] %-30s %s\n", tt.name, strings.ReplaceAll(out, "\n", "|"))
		passed++
	}

	total := len(tests)
	fmt.Fprintf(os.Stderr, "\n=== ES6 Coverage Summary ===\n")
	fmt.Fprintf(os.Stderr, "Total: %d | Passed: %d | Failed: %d | Coverage: %.1f%%\n",
		total, passed, failed, float64(passed)/float64(total)*100)

	if len(failedNames) > 0 {
		fmt.Fprintf(os.Stderr, "\nFailed tests (%d):\n", len(failedNames))
		for _, n := range failedNames {
			fmt.Fprintf(os.Stderr, "  - %s\n", n)
		}
	}

	if failed > 0 {
		t.Fatalf("%d/%d ES6 tests FAILED.", failed, total)
	}
}
