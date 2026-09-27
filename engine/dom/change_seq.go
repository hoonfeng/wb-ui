// Package dom — DOM 变更序号（派生数据的失效判据）
//
// 背景：querySelector 族的结构索引（engine/js/bindings 的 tag/class/id 索引）
// 按「DOM 变更序号」失效——序号未变 ⇒ 期间没有任何结构或属性变更 ⇒ 索引
// 仍然有效；序号变了 ⇒ 丢弃索引重建。序号是一次 O(1) 读取，而「按元素
// 逐一校验」不可能便宜地做到。
package dom

import "sync/atomic"

// domChangeSeq 是 DOM 结构/属性变更的全局单调序号（atomic，只增不减）。
//
// ★ 失效方向必须安全：序号只增不减，宁可多失效（后果只是重建一次派生数据
//   = 性能），绝不返回陈旧结果（= 功能错误）。因此挂点只需保证「所有写入
//   路径都 bump」——即使某次变更实际不影响索引也无害：
//
//	结构：nodeBase.{AppendChild,InsertBefore,RemoveChild,ReplaceChild}
//	属性：notifyAttributeChanged（SetAttribute/RemoveAttribute 的无条件钩子）
//
// 审计依据（为什么这两类就够）：
//   - 索引只保存 tag / class / id 三者的分布；
//   - tag 不可变；class / id 都是普通属性 → 必然经过 SetAttribute /
//     RemoveAttribute → 必然经过 notifyAttributeChanged（无条件调用，
//     与「只在有观察者时才走」的 NotifyAttributes 不同）；
//   - 元素增删必然经过上述 4 个 mutation 方法（appendChild(documentFragment)、
//     innerHTML、SetTextContent、adoptNode 等上层写法最终都落到它们）；
//   - cloneShallow / 未挂载子树内的变更不需要 bump（不影响文档树索引），
//     但它们也都走同样的 mutation 方法，属多 bump 的保守方向。
var domChangeSeq atomic.Uint64

// DOMChangeSeq 返回当前 DOM 变更序号（单调递增）。任何结构或属性变更都会
// 使它变化；用它做「按 DOM 内容派生的缓存/索引」的失效判据。
func DOMChangeSeq() uint64 { return domChangeSeq.Load() }

// bumpDOMChangeSeq 递增 DOM 变更序号（仅由 dom 包内的写入路径调用）。
func bumpDOMChangeSeq() { domChangeSeq.Add(1) }
