// engine/js/bindings/lazytext.go — Text 节点包装器的惰性属性（同 lazyelement.go 模式）。
// CM6 的 DOM 里文本节点与元素数量相当（每行 5+ 个），wrapText 此前每节点
// 安装 ~16 个自有属性，同样拖慢文件打开/编辑器重绘。
package bindings

import (
	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
)

type lazyTextProps struct {
	t       *dom.Text
	interp  *jsc.Interpreter
	cached  map[string]jsc.JSValue
	expando map[string]jsc.JSValue
}

// textAccessorProps：Text 包装器的 accessor（活值）属性名。
var textAccessorProps = map[string]bool{
	"data": true, "textContent": true, "nodeValue": true, "length": true,
	"nodeName": true, "nodeType": true,
	"parentNode": true, "nextSibling": true, "previousSibling": true,
	"firstChild": true, "lastChild": true, "childNodes": true,
}

// Live 实现 jsc.LazyLiveProps：accessor 属性每次读取重新求值。
func (p *lazyTextProps) Live(key string) bool { return textAccessorProps[key] }

func (p *lazyTextProps) Get(key string) jsc.JSValue {
	if key == "" {
		return jsc.Undefined()
	}
	if v, ok := p.expando[key]; ok {
		return v
	}
	if v, ok := p.cached[key]; ok {
		return v
	}
	val, acc, ok := installTextProperty(p.interp, p.t, key)
	if !ok {
		return jsc.Undefined()
	}
	if acc != nil {
		if acc.get != nil {
			return acc.get()
		}
		return jsc.Undefined()
	}
	p.cached[key] = val
	return val
}

func (p *lazyTextProps) Set(key string, v jsc.JSValue) bool {
	if key == "" {
		return false
	}
	if _, acc, ok := installTextProperty(p.interp, p.t, key); ok && acc != nil {
		if acc.set != nil {
			acc.set(v)
		}
		return true
	}
	delete(p.cached, key)
	p.expando[key] = v
	return true
}

func (p *lazyTextProps) Has(key string) bool {
	if _, ok := textKnownProps[key]; ok {
		return true
	}
	_, ok := p.expando[key]
	return ok
}

func (p *lazyTextProps) Delete(key string) bool {
	delete(p.expando, key)
	delete(p.cached, key)
	return true
}

func (p *lazyTextProps) Keys() []string {
	out := make([]string, 0, len(textKnownPropNames)+len(p.expando))
	for _, k := range textKnownPropNames {
		out = append(out, k)
	}
	for k := range p.expando {
		out = append(out, k)
	}
	return out
}

var (
	textKnownProps     = map[string]bool{}
	textKnownPropNames = []string{
		"constructor",
		"remove", "splitText", "getRootNode", "compareDocumentPosition",
		"data", "textContent", "nodeValue", "length", "nodeName", "nodeType",
		"parentNode", "nextSibling", "previousSibling", "firstChild", "lastChild", "childNodes",
	}
)

func init() {
	for _, k := range textKnownPropNames {
		textKnownProps[k] = true
	}
}

// installTextProperty 按属性名物化 Text 包装器的一个属性（实现与 wrapText
// 原属性定义逐字一致）。
func installTextProperty(rt *jsc.Interpreter, t *dom.Text, key string) (jsc.JSValue, *elemAccessor, bool) {
	switch key {
	case "constructor":
		o := jsc.NewObject(rt.ObjectPrototype())
		o.Set("name", jsc.StringValue("Text"))
		return jsc.ObjectValue(o), nil, true
	case "remove":
		return jsc.FunctionValue(jsc.NewNativeFunction("remove",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				if p := t.ParentNode(); p != nil {
					p.RemoveChild(t)
				}
				return jsc.Undefined()
			}, 0)), nil, true
	case "splitText":
		// 把文本节点在 offset 处拆成两个，返回后半部分节点（CM6 DOMObserver 用）。
		return jsc.FunctionValue(jsc.NewNativeFunction("splitText",
			func(in *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
				off := 0
				if len(a) >= 1 {
					off = int(a[0].ToNumber())
				}
				tail, err := t.SplitText(off)
				if err != nil {
					return jsc.Null()
				}
				return nodeToJS(in, tail)
			}, 1)), nil, true
	case "getRootNode":
		return jsc.FunctionValue(jsc.NewNativeFunction("getRootNode",
			func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				doc := t.OwnerDocument()
				if doc == nil {
					if cached, ok := nodeWrapperCache[t]; ok {
						return jsc.ObjectValue(cached)
					}
					return jsc.ObjectValue(wrapText(in, t))
				}
				return jsc.ObjectValue(wrapDocument(in, doc))
			}, 0)), nil, true
	case "compareDocumentPosition":
		return jsc.FunctionValue(jsc.NewNativeFunction("compareDocumentPosition",
			func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
				if len(a) == 0 {
					return jsc.NumberValue(0)
				}
				other := unwrapNode(a[0])
				return jsc.NumberValue(float64(compareDocPosition(t, other)))
			}, 1)), nil, true
	case "data":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(t.Data()) },
			set: func(v jsc.JSValue) { t.SetData(v.ToString()) }}, true
	case "textContent":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(t.Data()) },
			set: func(v jsc.JSValue) { t.SetData(v.ToString()) }}, true
	case "nodeValue":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(t.Data()) },
			set: func(v jsc.JSValue) { t.SetData(v.ToString()) }}, true
	case "length":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return jsc.NumberValue(float64(t.Length()))
		}}, true
	case "nodeName":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return strAcc(t.NodeName())(rt, jsc.JSValue{})
		}}, true
	case "nodeType":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return jsc.NumberValue(float64(t.NodeType()))
		}}, true
	case "parentNode":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return nodeAccFn(rt, func() dom.Node { return t.ParentNode() })(rt, jsc.JSValue{})
		}}, true
	case "nextSibling":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return nodeAccFn(rt, func() dom.Node { return t.NextSibling() })(rt, jsc.JSValue{})
		}}, true
	case "previousSibling":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return nodeAccFn(rt, func() dom.Node { return t.PreviousSibling() })(rt, jsc.JSValue{})
		}}, true
	case "firstChild":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return nodeAccFn(rt, func() dom.Node { return t.FirstChild() })(rt, jsc.JSValue{})
		}}, true
	case "lastChild":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return nodeAccFn(rt, func() dom.Node { return t.LastChild() })(rt, jsc.JSValue{})
		}}, true
	case "childNodes":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return getter(func(in *jsc.Interpreter) jsc.JSValue {
				return arrNode(in, t.ChildNodes())
			})(rt, jsc.JSValue{})
		}}, true
	}
	return jsc.JSValue{}, nil, false
}
