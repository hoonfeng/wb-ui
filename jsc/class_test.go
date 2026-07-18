package jsc

import (
	"strings"
	"testing"
)

func TestClassJustConstructor(t *testing.T) {
	out := runScript(t, `class Foo{constructor(x){this.x=x}} var f=new Foo(42); console.log(f.x);`)
	got := strings.TrimSpace(out)
	if got != "42" {
		t.Fatalf("ctor+new: got %q, want 42", out)
	}
}

func TestClassMethodDeclaredOnly(t *testing.T) {
	// Declare class with method but don't call it
	out := runScript(t, `class Foo{constructor(x){this.x=x}getVal(){return this.x}} var f=new Foo(42); console.log(f.x);`)
	got := strings.TrimSpace(out)
	if got != "42" {
		t.Fatalf("declare+new: got %q, want 42", out)
	}
}

func TestClassCallMethod(t *testing.T) {
	// Declare class with method AND call it
	out := runScript(t, `class Foo{constructor(x){this.x=x}getVal(){return this.x}} var f=new Foo(42); console.log(f.getVal());`)
	got := strings.TrimSpace(out)
	if got != "42" {
		t.Fatalf("call method: got %q, want 42", out)
	}
}

func TestClassGetterAccess(t *testing.T) {
	// Access a getter on an instance
	out := runScript(t, `class Foo{constructor(x){this.x=x}get val(){return this.x}} var f=new Foo(42); console.log(f.val);`)
	got := strings.TrimSpace(out)
	if got != "42" {
		t.Fatalf("getter: got %q, want 42", out)
	}
}
