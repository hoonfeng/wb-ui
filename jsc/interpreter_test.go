package jsc

import (
	"strings"
	"testing"
)

// runScript parses and executes src with a BufferLogger, returning the console output.
func runScript(t *testing.T, src string) string {
	t.Helper()
	in := NewInterpreter()
	log := &BufferLogger{}
	in.SetupGlobal(log)
	_, err := in.Run(src)
	if err != nil {
		t.Fatalf("Run(%q) error: %v", src, err)
	}
	return log.String()
}

func TestInterpreterArithmeticConsoleLog(t *testing.T) {
	// The canonical example from the task: let x = 1+2; console.log(x); -> 3
	out := runScript(t, "let x = 1 + 2; console.log(x);")
	if got := strings.TrimSpace(out); got != "3" {
		t.Fatalf("got %q, want 3", out)
	}
}

func TestInterpreterVariablesAndArithmetic(t *testing.T) {
	out := runScript(t, `
		let a = 10;
		let b = 3;
		console.log(a + b);
		console.log(a - b);
		console.log(a * b);
		console.log(a / b);
		console.log(a % b);
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{"13", "7", "30", "3.3333333333333335", "1"}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines: %v", len(lines), lines)
	}
	for i, w := range want {
		if strings.TrimSpace(lines[i]) != w {
			t.Fatalf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestInterpreterFunctionCall(t *testing.T) {
	out := runScript(t, `
		function add(a, b) {
			return a + b;
		}
		console.log(add(3, 4));
	`)
	if got := strings.TrimSpace(out); got != "7" {
		t.Fatalf("got %q, want 7", out)
	}
}

func TestInterpreterClosure(t *testing.T) {
	out := runScript(t, `
		function makeCounter() {
			let count = 0;
			return function() {
				count = count + 1;
				return count;
			};
		}
		let next = makeCounter();
		console.log(next());
		console.log(next());
		console.log(next());
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{"1", "2", "3"}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines: %v", len(lines), lines)
	}
	for i, w := range want {
		if strings.TrimSpace(lines[i]) != w {
			t.Fatalf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestInterpreterArrowFunction(t *testing.T) {
	out := runScript(t, `
		const square = (x) => x * x;
		const add = (a, b) => a + b;
		console.log(square(5));
		console.log(add(2, 3));
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if strings.TrimSpace(lines[0]) != "25" {
		t.Fatalf("square(5) = %q, want 25", lines[0])
	}
	if strings.TrimSpace(lines[1]) != "5" {
		t.Fatalf("add(2,3) = %q, want 5", lines[1])
	}
}

func TestInterpreterArrayLiteral(t *testing.T) {
	out := runScript(t, `
		let arr = [1, 2, 3];
		console.log(arr[0]);
		console.log(arr[1]);
		console.log(arr.length);
		console.log(arr.join("-"));
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{"1", "2", "3", "1-2-3"}
	for i, w := range want {
		if strings.TrimSpace(lines[i]) != w {
			t.Fatalf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestInterpreterObjectLiteral(t *testing.T) {
	out := runScript(t, `
		let obj = { name: "Alice", age: 30 };
		console.log(obj.name);
		console.log(obj.age);
		obj.age = 31;
		console.log(obj.age);
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{"Alice", "30", "31"}
	for i, w := range want {
		if strings.TrimSpace(lines[i]) != w {
			t.Fatalf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestInterpreterIfElse(t *testing.T) {
	out := runScript(t, `
		let x = 5;
		if (x > 3) {
			console.log("big");
		} else {
			console.log("small");
		}
	`)
	if got := strings.TrimSpace(out); got != "big" {
		t.Fatalf("got %q, want big", out)
	}
}

func TestInterpreterForLoop(t *testing.T) {
	out := runScript(t, `
		let sum = 0;
		for (let i = 1; i <= 5; i++) {
			sum = sum + i;
		}
		console.log(sum);
	`)
	if got := strings.TrimSpace(out); got != "15" {
		t.Fatalf("sum = %q, want 15", out)
	}
}

func TestInterpreterWhileLoop(t *testing.T) {
	out := runScript(t, `
		let n = 5;
		let fact = 1;
		while (n > 0) {
			fact = fact * n;
			n = n - 1;
		}
		console.log(fact);
	`)
	if got := strings.TrimSpace(out); got != "120" {
		t.Fatalf("fact = %q, want 120", out)
	}
}

func TestInterpreterStringConcatenation(t *testing.T) {
	out := runScript(t, `
		let greeting = "Hello";
		let name = "World";
		console.log(greeting + ", " + name + "!");
	`)
	if got := strings.TrimSpace(out); got != "Hello, World!" {
		t.Fatalf("got %q, want 'Hello, World!'", out)
	}
}

func TestInterpreterMathObject(t *testing.T) {
	out := runScript(t, `
		console.log(Math.floor(3.7));
		console.log(Math.ceil(3.2));
		console.log(Math.abs(-5));
		console.log(Math.max(1, 2, 3));
		console.log(Math.min(1, 2, 3));
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{"3", "4", "5", "3", "1"}
	for i, w := range want {
		if strings.TrimSpace(lines[i]) != w {
			t.Fatalf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestInterpreterJSON(t *testing.T) {
	out := runScript(t, `
		let obj = JSON.parse('{"a":1,"b":[2,3]}');
		console.log(obj.a);
		console.log(obj.b[1]);
		let s = JSON.stringify({x: 1, y: 2});
		console.log(s);
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if strings.TrimSpace(lines[0]) != "1" {
		t.Fatalf("obj.a = %q, want 1", lines[0])
	}
	if strings.TrimSpace(lines[1]) != "3" {
		t.Fatalf("obj.b[1] = %q, want 3", lines[1])
	}
	s := strings.TrimSpace(lines[2])
	if !strings.Contains(s, `"x":1`) || !strings.Contains(s, `"y":2`) {
		t.Fatalf("stringify = %q, want x:1 and y:2", s)
	}
}

func TestInterpreterTryCatch(t *testing.T) {
	out := runScript(t, `
		try {
			throw "boom";
		} catch (e) {
			console.log("caught: " + e);
		}
	`)
	if got := strings.TrimSpace(out); got != "caught: boom" {
		t.Fatalf("got %q, want 'caught: boom'", out)
	}
}

func TestInterpreterComparisonOperators(t *testing.T) {
	out := runScript(t, `
		console.log(1 == 1);
		console.log(1 === "1");
		console.log(1 != 2);
		console.log(2 < 3);
		console.log(3 >= 3);
		console.log(1 == "1");
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{"true", "false", "true", "true", "true", "true"}
	for i, w := range want {
		if strings.TrimSpace(lines[i]) != w {
			t.Fatalf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestInterpreterLogicalOperators(t *testing.T) {
	out := runScript(t, `
		console.log(true && "yes");
		console.log(false || "fallback");
		console.log(null ?? "default");
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{"yes", "fallback", "default"}
	for i, w := range want {
		if strings.TrimSpace(lines[i]) != w {
			t.Fatalf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestInterpreterRecursionFibonacci(t *testing.T) {
	out := runScript(t, `
		function fib(n) {
			if (n < 2) return n;
			return fib(n - 1) + fib(n - 2);
		}
		console.log(fib(10));
	`)
	if got := strings.TrimSpace(out); got != "55" {
		t.Fatalf("fib(10) = %q, want 55", out)
	}
}

func TestInterpreterObjectMethodThis(t *testing.T) {
	out := runScript(t, `
		let counter = {
			value: 0,
			increment: function() {
				this.value = this.value + 1;
				return this.value;
			}
		};
		console.log(counter.increment());
		console.log(counter.increment());
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if strings.TrimSpace(lines[0]) != "1" {
		t.Fatalf("increment() = %q, want 1", lines[0])
	}
	if strings.TrimSpace(lines[1]) != "2" {
		t.Fatalf("increment() = %q, want 2", lines[1])
	}
}

func TestInterpreterNestedClosures(t *testing.T) {
	out := runScript(t, `
		function outer() {
			let x = 10;
			function inner() {
				return x;
			}
			return inner();
		}
		console.log(outer());
	`)
	if got := strings.TrimSpace(out); got != "10" {
		t.Fatalf("got %q, want 10", out)
	}
}

func TestInterpreterForInLoop(t *testing.T) {
	out := runScript(t, `
		let obj = { a: 1, b: 2, c: 3 };
		let keys = [];
		for (let k in obj) {
			keys.push(k);
		}
		console.log(keys.join(","));
	`)
	// Keys order is map-iteration order; just check all three are present.
	out = strings.TrimSpace(out)
	for _, want := range []string{"a", "b", "c"} {
		if !strings.Contains(out, want) {
			t.Fatalf("for-in missing key %q in %q", want, out)
		}
	}
}

func TestInterpreterConstReassignment(t *testing.T) {
	// const reassignment is silently ignored (subset simplification); value stays 5.
	out := runScript(t, `
		const x = 5;
		x = 10;
		console.log(x);
	`)
	if got := strings.TrimSpace(out); got != "5" {
		t.Fatalf("got %q, want 5 (const reassignment ignored)", out)
	}
}

func TestInterpreterStringMethods(t *testing.T) {
	out := runScript(t, `
		let s = "Hello, World";
		console.log(s.length);
		console.log(s.toUpperCase());
		console.log(s.slice(0, 5));
		console.log(s.indexOf("World"));
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{"12", "HELLO, WORLD", "Hello", "7"}
	for i, w := range want {
		if strings.TrimSpace(lines[i]) != w {
			t.Fatalf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestInterpreterArrayMethods(t *testing.T) {
	out := runScript(t, `
		let arr = [1, 2, 3, 4];
		console.log(arr.indexOf(3));
		console.log(arr.slice(1, 3).join(","));
		console.log(arr.length);
		arr.push(5);
		console.log(arr.length);
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{"2", "2,3", "4", "5"}
	for i, w := range want {
		if strings.TrimSpace(lines[i]) != w {
			t.Fatalf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestInterpreterConditionalExpression(t *testing.T) {
	out := runScript(t, `
		let x = 7;
		console.log(x > 5 ? "big" : "small");
	`)
	if got := strings.TrimSpace(out); got != "big" {
		t.Fatalf("got %q, want big", out)
	}
}

func TestInterpreterCompoundAssignment(t *testing.T) {
	out := runScript(t, `
		let x = 10;
		x += 5;
		console.log(x);
		x *= 2;
		console.log(x);
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if strings.TrimSpace(lines[0]) != "15" {
		t.Fatalf("+=5 = %q, want 15", lines[0])
	}
	if strings.TrimSpace(lines[1]) != "30" {
		t.Fatalf("*=2 = %q, want 30", lines[1])
	}
}
