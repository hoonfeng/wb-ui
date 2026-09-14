// 组件注册表：同一套界面里，一部分组件由 Go 构建（基础方式 = native），
// 一部分由 HTML 片段提供（web 方式），宿主在注册表里声明来源，挂载时按
// 来源分派到对应实现——这就是「某些作为基础、某些作为 web 方式提供」的
// 统一接线点。
//
// 两种来源的差别是真实的，不只是元数据：
//   - SourceNative：Go 逐节点构建（dom.CreateElement + 属性/样式/事件），
//     无 HTML 解析、无字符串拼接，类型安全、可组合。
//   - SourceWeb：组件提供 HTML 片段，由引擎的片段解析器解析后接入文档，
//     适合复用现成的 HTML/CSS 资产、Markdown 渲染结果、页面脚本产出。
//
// 两者产出的节点在同一棵文档树里，共用 CSS/布局/渲染/事件管线。

package ui

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Source 表示组件的提供方式。
type Source int

const (
	// SourceNative：基础方式——Go 构建（Component.Native）。
	SourceNative Source = iota
	// SourceWeb：web 方式——HTML 片段（Component.Web）。
	SourceWeb
)

// String 返回来源的稳定名称（"native"/"web"）。
func (s Source) String() string {
	switch s {
	case SourceNative:
		return "native"
	case SourceWeb:
		return "web"
	}
	return fmt.Sprintf("Source(%d)", int(s))
}

// Props 是组件属性（宿主传入、组件读取）。
type Props map[string]any

// String 读取字符串属性（缺失/类型不符时返回 fallback 的零值 ""）。
func (p Props) String(key string) string {
	if p == nil {
		return ""
	}
	v, ok := p[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// Int 读取整数属性（兼容 int/int32/int64/float64/字符串数字）。
func (p Props) Int(key string) int {
	if p == nil {
		return 0
	}
	switch v := p[key].(type) {
	case nil:
		return 0
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float32:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	}
	n, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprint(p[key])))
	return n
}

// Bool 读取布尔属性（兼容 "true"/"1"/"yes" 等字符串写法）。
func (p Props) Bool(key string) bool {
	if p == nil {
		return false
	}
	switch v := p[key].(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		return s == "1" || s == "true" || s == "yes" || s == "on"
	}
	return false
}

// Has 报告属性是否存在（值为 nil 也算存在）。
func (p Props) Has(key string) bool {
	if p == nil {
		return false
	}
	_, ok := p[key]
	return ok
}

// Component 是一个组件定义：Native 与 Web 恰好设置一个。
type Component struct {
	// Native 是基础方式的构建函数（SourceNative）。
	Native func(v *View, props Props) *Node
	// Web 返回 HTML 片段（SourceWeb）。
	Web func(props Props) string
	// Description 可选：用途说明（内省/文档）。
	Description string
}

// Source 返回该组件的提供方式。
func (c Component) Source() Source {
	if c.Native != nil {
		return SourceNative
	}
	return SourceWeb
}

var (
	// ErrComponentName 表示组件名为空。
	ErrComponentName = errors.New("ui: component name must not be empty")
	// ErrComponentDef 表示定义非法（Native/Web 必须恰好设置一个）。
	ErrComponentDef = errors.New("ui: Component must set exactly one of Native or Web")
	// ErrComponentNotFound 表示组件未注册。
	ErrComponentNotFound = errors.New("ui: component not registered")
)

// Registry 是组件注册表（并发安全；同一视图一个，也可多视图共享）。
type Registry struct {
	mu      sync.RWMutex
	entries map[string]Component
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry {
	return &Registry{entries: map[string]Component{}}
}

// Register 注册（或覆盖）一个组件。
func (r *Registry) Register(name string, c Component) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrComponentName
	}
	if (c.Native == nil) == (c.Web == nil) {
		return fmt.Errorf("%w: %q", ErrComponentDef, name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries == nil {
		r.entries = map[string]Component{}
	}
	r.entries[name] = c
	return nil
}

// RegisterNative 便捷注册基础方式组件。
func (r *Registry) RegisterNative(name string, build func(v *View, props Props) *Node) error {
	return r.Register(name, Component{Native: build})
}

// RegisterWeb 便捷注册 web 方式组件（html 返回 HTML 片段工厂）。
func (r *Registry) RegisterWeb(name string, html func(props Props) string) error {
	return r.Register(name, Component{Web: html})
}

// Unregister 移除一个组件（不存在时不做任何事）。
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, name)
}

// Lookup 查找组件。
func (r *Registry) Lookup(name string) (Component, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.entries[strings.TrimSpace(name)]
	return c, ok
}

// Source 报告组件的提供方式。
func (r *Registry) Source(name string) (Source, bool) {
	c, ok := r.Lookup(name)
	if !ok {
		return SourceNative, false
	}
	return c.Source(), true
}

// Names 返回已注册组件名（字典序，便于内省/测试）。
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.entries))
	for name := range r.entries {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Mount 在根节点下挂载一个已注册组件。
func (v *View) Mount(name string, props Props) (*Node, error) {
	if v == nil || v.root == nil {
		return nil, ErrNoDocument
	}
	return v.root.Mount(name, props)
}

// Mount 在本节点下挂载一个已注册组件：按组件声明的来源分派——
//   - SourceNative：调用 Native 构建函数（Go 逐节点），挂到本节点下
//   - SourceWeb：取 HTML 片段，交给引擎解析后接入本节点下
//
// 返回挂载出的节点（web 方式返回承载片段的容器节点）。
func (n *Node) Mount(name string, props Props) (*Node, error) {
	if n == nil || n.view == nil || n.view.reg == nil {
		return nil, ErrNoDocument
	}
	comp, ok := n.view.reg.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrComponentNotFound, name)
	}
	switch comp.Source() {
	case SourceWeb:
		fragment := comp.Web(props)
		holder := n.Web(fragment)
		if holder == nil {
			return nil, fmt.Errorf("ui: component %q: HTML fragment failed to parse", name)
		}
		return holder, nil
	default:
		child := comp.Native(n.view, props)
		if child == nil {
			return nil, fmt.Errorf("ui: component %q: native factory returned nil", name)
		}
		// 返回组件实例本身（Append 返回的是父节点，链式用）。
		n.Append(child)
		return child, nil
	}
}
