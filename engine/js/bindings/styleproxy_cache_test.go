package bindings

import "testing"

// C-P4 行为断言：句柄缓存与解析缓存都不能改变可观察语义。

// TestStyleHandleIdentity 钉死「el.style 是稳定实例」——浏览器里
// `el.style === el.style` 为 true；宿主侧此前每次取句柄都新建对象（C-P4-3
// 之前的实测值 ~159ns/次）。缓存后必须仍然返回同一对象。
func TestStyleHandleIdentity(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := doc.CreateElement("div")
	el.SetId("style-id")
	doc.AppendChild(el)
	mustRun(t, rt, `
		var el = document.getElementById("style-id");
		var a = el.style, b = el.style;
		if (a !== b) throw new Error("el.style 句柄不稳定");
		a.width = "12px";
		if (b.getPropertyValue("width") !== "12px") throw new Error("同句柄没共享状态: " + b.getPropertyValue("width"));
	`)
}

// TestStyleDeclsCacheSurvivesWrites 钉死解析缓存的正确性：读（填缓存）→ 写
// （改声明）→ 再读必须看到新值。setStyleDecl 命中同名声明时是**原位改**共享
// 数组，写路径若不克隆就会把缓存写脏（属性文本没变、缓存内容已变）。
func TestStyleDeclsCacheSurvivesWrites(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := doc.CreateElement("div")
	el.SetId("style-cache")
	el.SetAttribute("style", "width:10px;height:20px")
	doc.AppendChild(el)
	mustRun(t, rt, `
		var el = document.getElementById("style-cache");
		var s = el.style;
		if (s.getPropertyValue("width") !== "10px") throw new Error("初值: " + s.getPropertyValue("width"));
		s.width = "20px";                       // 写：命中已有声明，原位替换
		if (s.getPropertyValue("width") !== "20px") throw new Error("写后读: " + s.getPropertyValue("width"));
		if (el.getAttribute("style") !== "width:20px;height:20px") throw new Error("属性文本: " + el.getAttribute("style"));
		s.setProperty("width", "20px");         // 同值：快速路径不应改动任何东西
		if (s.getPropertyValue("width") !== "20px") throw new Error("同值后: " + s.getPropertyValue("width"));
		s.setProperty("width", "30px");         // setProperty 路径写
		if (s.getPropertyValue("width") !== "30px") throw new Error("setProperty 后: " + s.getPropertyValue("width"));
		if (s.length !== 2) throw new Error("声明条数: " + s.length);
		s.removeProperty("width");
		if (s.getPropertyValue("width") !== "") throw new Error("移除后仍在: " + s.getPropertyValue("width"));
		if (s.length !== 1) throw new Error("移除后条数: " + s.length);
		delete s.height;                        // Delete 路径
		if (el.getAttribute("style") !== "") throw new Error("delete 后属性: " + el.getAttribute("style"));
	`)
}

// TestStyleNoopWriteKeepsOrder 保序语义不能被快速路径破坏：写同值不得改变
// 声明顺序（cssText 按序输出），写不同值保持首次出现位置。
func TestStyleNoopWriteKeepsOrder(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := doc.CreateElement("div")
	el.SetId("style-order")
	el.SetAttribute("style", "color:red;width:10px;height:20px")
	doc.AppendChild(el)
	mustRun(t, rt, `
		var el = document.getElementById("style-order");
		var s = el.style;
		s.width = "10px";   // 同值
		s.width = "11px";   // 改值
		var text = el.getAttribute("style");
		if (text !== "color:red;width:11px;height:20px") throw new Error("顺序: " + text);
	`)
}
