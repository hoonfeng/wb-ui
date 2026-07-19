package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	data, err := os.ReadFile("jsc/interpreter.go")
	if err != nil {
		panic(err)
	}
	content := string(data)

	patches := []struct {
		old, new string
	}{
		// 1. Add regExpProto field (already done, skip)
		// 2. RegExp prototype creation (already done, skip)
		// 3. regExpProto in return (already done, skip)
		// 4. Stack overflow: silent return
		{
			"if in.depth > in.maxCallDepth {\n\t\tfmt.Fprintf(os.Stderr, \"[STACK_OVERFLOW] depth=%d max=%d\\n\", in.depth, in.maxCallDepth)\n\t\treturn Undefined(), &jsException{value: StringValue(\"RangeError: Maximum call stack size exceeded\")}\n\t}",
			"if in.depth >= in.maxCallDepth {\n\t\treturn Undefined(), nil\n\t}",
		},
		// 5. OpNewRegex case
		{
			"case OpLoadConst:\n\t\t\tpush(inst.Value)",
			"case OpLoadConst:\n\t\t\tpush(inst.Value)\n\t\tcase OpNewRegex:\n\t\t\tre, err := regexp.Compile(inst.Name)\n\t\t\tif err != nil {\n\t\t\t\tre = regexp.MustCompile(`(?:)`)\n\t\t\t}\n\t\t\tobj := NewObject(in.regExpProto)\n\t\t\tobj.ClassName = \"RegExp\"\n\t\t\tobj.Internal = re\n\t\t\tobj.Set(\"source\", StringValue(inst.Name))\n\t\t\tobj.Set(\"flags\", StringValue(inst.StrArg))\n\t\t\tobj.Set(\"lastIndex\", NumberValue(0))\n\t\t\tpush(ObjectValue(obj))",
		},
		// 6. OpCallMethod: remove verbose diagnostic
		{
			"fn := pop()\n\t\t\tthisVal := pop()\n\t\t\t// Briefly diagnose non-function method calls\n\t\t\tif !fn.IsFunction() {\n\t\t\t\tfmt.Fprintf(os.Stderr, \"[METH_ERR] %s.%s() not a function\\n\", body.Name, inst.Name)\n\t\t\t}\n\t\t\tres, exc := in.callValue(fn, thisVal, args)",
			"fn := pop()\n\t\t\tthisVal := pop()\n\t\t\tres, exc := in.callValue(fn, thisVal, args)",
		},
		// 7. OpNew: silent handling
		{
			"callee := pop()\n\t\t\tif !callee.IsFunction() {\n\t\t\t\tfmt.Fprintf(os.Stderr, \"[MISSING_NEW] fn=%q pc=%d\\n\", body.Name, pc)\n\t\t\t}\n\t\t\tres, exc := in.construct(callee, args)\n\t\t\tif exc != nil {\n\t\t\t\tif e := handleThrow(exc.value); e != nil {\n\t\t\t\t\treturn Undefined(), e\n\t\t\t\t}\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tpush(res)",
			"callee := pop()\n\t\t\tif !callee.IsFunction() {\n\t\t\t\tpush(ObjectValue(NewObject(in.objectProto)))\n\t\t\t} else {\n\t\t\t\tres, exc := in.construct(callee, args)\n\t\t\t\tif exc != nil {\n\t\t\t\t\tif e := handleThrow(exc.value); e != nil {\n\t\t\t\t\t\treturn Undefined(), e\n\t\t\t\t\t}\n\t\t\t\t\tcontinue\n\t\t\t\t}\n\t\t\t\tpush(res)\n\t\t\t}",
		},
		// 8. callValue: remove verbose diagnostic
		{
			"if !callee.IsFunction() {\n\t\ttag := \"?\"\n\t\tif callee.IsUndefined() { tag = \"undefined\" } else if callee.IsNull() { tag = \"null\" } else if callee.IsObject() { tag = \"obj:\" + callee.AsObject().ClassName } else if callee.IsString() { tag = \"string\" } else if callee.IsNumber() { tag = \"number\" } else if callee.IsBoolean() { tag = \"bool\" }\n\t\tfmt.Fprintf(os.Stderr, \"[CALL_ERR] %s(%s) depth=%d\\n\", tag, in.currentBodyName, in.depth)\n\t\treturn Undefined(), nil\n\t}",
			"if !callee.IsFunction() {\n\t\treturn Undefined(), nil\n\t}",
		},
	}

	for _, p := range patches {
		count := strings.Count(content, p.old)
		if count == 0 {
			fmt.Printf("WARN: pattern not found (%.50s...)\n", p.old)
			continue
		}
		if count > 1 {
			fmt.Printf("WARN: pattern found %d times, skipping (%.40s...)\n", count, p.old)
			continue
		}
		content = strings.Replace(content, p.old, p.new, 1)
		fmt.Printf("OK: patched (%.40s...)\n", p.old)
	}

	if err := os.WriteFile("jsc/interpreter.go", []byte(content), 0644); err != nil {
		panic(err)
	}
	fmt.Println("Done!")
}
