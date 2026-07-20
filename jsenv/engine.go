// 包 jsenv 提供基于 goja 的 JavaScript 执行环境
package jsenv

import (
	"fmt"
	"strings"

	"wb-ui.com/goja"
)

// Engine JS 引擎封装
type Engine struct {
	vm *goja.Runtime
}

// NewEngine 创建新的 JS 引擎
func NewEngine() *Engine {
	e := &Engine{
		vm: goja.New(),
	}
	e.initBuiltins()
	return e
}

// initBuiltins 初始化内置对象和函数
func (e *Engine) initBuiltins() {
	// 注册 console.log/error/warn/info
	consoleObj := e.vm.NewObject()
	consoleObj.Set("log", func(call goja.FunctionCall) goja.Value {
		var parts []string
		for _, arg := range call.Arguments {
			parts = append(parts, fmt.Sprintf("%v", arg.Export()))
		}
		fmt.Println(strings.Join(parts, " "))
		return goja.Undefined()
	})
	consoleObj.Set("error", func(call goja.FunctionCall) goja.Value {
		var parts []string
		for _, arg := range call.Arguments {
			parts = append(parts, fmt.Sprintf("%v", arg.Export()))
		}
		fmt.Fprintln(nil, strings.Join(parts, " "))
		return goja.Undefined()
	})
	consoleObj.Set("warn", func(call goja.FunctionCall) goja.Value {
		var parts []string
		for _, arg := range call.Arguments {
			parts = append(parts, fmt.Sprintf("%v", arg.Export()))
		}
		fmt.Println("[warn]", strings.Join(parts, " "))
		return goja.Undefined()
	})
	consoleObj.Set("info", func(call goja.FunctionCall) goja.Value {
		var parts []string
		for _, arg := range call.Arguments {
			parts = append(parts, fmt.Sprintf("%v", arg.Export()))
		}
		fmt.Println("[info]", strings.Join(parts, " "))
		return goja.Undefined()
	})
	e.vm.Set("console", consoleObj)
}

// Run 执行 JS 代码，返回结果字符串
func (e *Engine) Run(code string) (string, error) {
	val, err := e.vm.RunString(code)
	if err != nil {
		// 尝试提取异常信息
		if ex, ok := err.(*goja.Exception); ok {
			return "", fmt.Errorf("JS 异常: %s", ex.String())
		}
		return "", fmt.Errorf("JS 执行错误: %w", err)
	}
	if val == nil {
		return "", nil
	}
	return fmt.Sprintf("%v", val.Export()), nil
}

// RunScript 执行 JS 脚本（无返回值）
func (e *Engine) RunScript(code string) error {
	_, err := e.vm.RunString(code)
	if err != nil {
		if ex, ok := err.(*goja.Exception); ok {
			return fmt.Errorf("JS 异常: %s", ex.String())
		}
		return fmt.Errorf("JS 执行错误: %w", err)
	}
	return nil
}

// Get 获取 JS 全局变量
func (e *Engine) Get(name string) interface{} {
	return e.vm.Get(name).Export()
}

// Set 设置 JS 全局变量
func (e *Engine) Set(name string, val interface{}) {
	e.vm.Set(name, val)
}

// VM 返回底层的 goja Runtime
func (e *Engine) VM() *goja.Runtime {
	return e.vm
}

// ScriptEngine 返回 page.Frame 可用的 ScriptEngine 回调函数
// 每次调用创建一个新的独立引擎实例
func ScriptEngine() func(code string) error {
	return func(code string) error {
		eng := NewEngine()
		return eng.RunScript(code)
	}
}

// NewScriptEngineWithConsole 创建一个 ScriptEngine 回调，使用指定输出
func NewScriptEngineWithConsole() func(code string) error {
	eng := NewEngine()
	return func(code string) error {
		return eng.RunScript(code)
	}
}

// Evaluate 便捷函数：执行 JS 返回字符串
func Evaluate(code string) (string, error) {
	e := NewEngine()
	return e.Run(code)
}
