package bindings

// 动态伪类状态变化 → computedStyleFor 结果缓存失效回归测试。
// 缺陷：SetHovered/SetFocused/SetActive 只让 dom 层的 attrVersion++
// （style.Resolver 缓存失效），但 bindings 的 cssCache（computedStyleFor
// 结果）只按全局 styleVer + 属性/样式表变更失效——:hover/:focus/:active
// 状态变化后 getComputedStyle 仍返回旧样式：
//   - 鼠标移开后 .tb-close:hover 的 bg 不恢复（自身场景）
//   - .tree-head:hover .gdel 的 opacity 不恢复（兄弟分支场景，冒泡匹配）
// 修复：dom.DynamicPseudoStateChanged 回调（bindings init 注册）→
// BumpStyleVersion 全失效（动态伪类影响范围跨子树无法精确，切换低频）。
import (
	"testing"

	"wb-ui/engine/dom"
)

func TestDynamicPseudoInvalidatesCssCache(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("div")

	// 预置一条 computedStyleFor 结果缓存（模拟业务读过的样式）。
	cssCachePut(el, map[string]string{"backgroundColor": "rgb(1,1,1)"})
	if _, ok := cssCacheGet(el); !ok {
		t.Fatal("预置缓存未命中——测试前提不成立")
	}

	// :hover 状态变化必须失效（init 注册的回调 → BumpStyleVersion）。
	el.SetHovered(true)
	if _, ok := cssCacheGet(el); ok {
		t.Fatal("SetHovered 后 cssCache 未失效——「鼠标移开 :hover 样式不恢复」缺陷回归")
	}

	cssCachePut(el, map[string]string{"color": "y"})
	el.SetFocused(true)
	if _, ok := cssCacheGet(el); ok {
		t.Fatal("SetFocused 后 cssCache 未失效（:focus 样式陈旧）")
	}

	cssCachePut(el, map[string]string{"color": "z"})
	el.SetActive(true)
	if _, ok := cssCacheGet(el); ok {
		t.Fatal("SetActive 后 cssCache 未失效（:active 样式陈旧）")
	}
}
