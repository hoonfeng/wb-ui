package jsc

import (
	"strings"
	"testing"
)

// runModuleScript runs src as a module with the given moduleName, storing exports
// in the interpreter's moduleRegistry. It returns the console output.
func runModuleScript(t *testing.T, src, moduleName string) string {
	t.Helper()
	in := NewInterpreter()
	log := &BufferLogger{}
	in.SetupGlobal(log)
	// Set module context so OpExport populates the registry.
	in.currentModuleName = moduleName
	in.moduleRegistry[moduleName] = make(map[string]JSValue)

	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", src, err)
	}
	body := CompileProgram(prog)
	_, err = in.RunBody(body, in.globalEnv, Undefined())
	if err != nil {
		t.Fatalf("Run(%q) error: %v", src, err)
	}
	return log.String()
}

// runImportScript runs src that uses import statements. The moduleRegistry of the
// interpreter is pre-populated with the given exports map.
func runImportScript(t *testing.T, src string, exports map[string]JSValue) string {
	t.Helper()
	in := NewInterpreter()
	log := &BufferLogger{}
	in.SetupGlobal(log)
	// Pre-populate the module registry with exported values.
	in.moduleRegistry["testmod"] = exports

	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", src, err)
	}
	body := CompileProgram(prog)
	_, err = in.RunBody(body, in.globalEnv, Undefined())
	if err != nil {
		t.Fatalf("Run(%q) error: %v", src, err)
	}
	return log.String()
}

// TestInterpreterModuleExportImport simulates a module exporting a value and then
// importing it in another context.
func TestInterpreterModuleExportImport(t *testing.T) {
	// Export a value from a module.
	out := runModuleScript(t, `
		let a = 42;
		let b = "hello";
		export { a, b };
	`, "testmod")
	_ = out

	// Now import those exports in a new script.
	out2 := runImportScript(t, `
		import { a, b } from "testmod";
		console.log(a);
		console.log(b);
	`, map[string]JSValue{
		"a": IntValue(42),
		"b": StringValue("hello"),
	})
	lines := strings.Split(strings.TrimSpace(out2), "\n")
	if len(lines) != 2 || lines[0] != "42" || lines[1] != "hello" {
		t.Fatalf("got %v, want [42 hello]", lines)
	}
}

// TestInterpreterModuleDefaultExport tests export default and import default.
func TestInterpreterModuleDefaultExport(t *testing.T) {
	// Import a default export.
	out := runImportScript(t, `
		import val from "testmod";
		console.log(val);
	`, map[string]JSValue{
		"default": IntValue(99),
	})
	if strings.TrimSpace(out) != "99" {
		t.Fatalf("got %q, want 99", out)
	}
}

// TestInterpreterModuleNamedExports tests multiple named exports.
func TestInterpreterModuleNamedExports(t *testing.T) {
	out := runImportScript(t, `
		import { x, y } from "testmod";
		console.log(x);
		console.log(y);
	`, map[string]JSValue{
		"x": StringValue("first"),
		"y": StringValue("second"),
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || lines[0] != "first" || lines[1] != "second" {
		t.Fatalf("got %v, want [first second]", lines)
	}
}

// TestInterpreterModuleNotFound verifies that importing from a non-existent module
// produces undefined values.
func TestInterpreterModuleNotFound(t *testing.T) {
	out := runImportScript(t, `
		import { missing } from "nonexistent";
		console.log(missing);
	`, nil) // no exports at all
	if strings.TrimSpace(out) != "undefined" {
		t.Fatalf("got %q, want undefined", out)
	}
}
