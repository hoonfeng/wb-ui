// Package dom — MutationObserver 实现（DOM 变更异步通知）
//
// 对标浏览器 MutationObserver API：
//   - 构造：new MutationObserver(callback)
//   - observe(target, options)：监听 childList/attributes/characterData/subtree
//   - disconnect()：停止监听
//   - takeRecords()：同步获取并清空记录队列
//
// 实现方式：在 DOM 变更方法中调用全局通知钩子 NotifyMutation，
// 由 bindings 层设置钩子并管理 JS 回调的异步投递。

package dom

// MutationRecordType 镜像 MutationRecord::type。
type MutationRecordType string

const (
	MutationChildList     MutationRecordType = "childList"
	MutationAttributes    MutationRecordType = "attributes"
	MutationCharacterData MutationRecordType = "characterData"
)

// MutationRecord 镜像 WebCore::MutationRecord。
type MutationRecord struct {
	Type             MutationRecordType
	Target           Node
	AddedNodes       []Node
	RemovedNodes     []Node
	PreviousSibling  Node
	NextSibling      Node
	AttributeName    string
	AttributeValue   string
	OldValue         string
}

// MutationObserverOptions 镜像 MutationObserverInit。
type MutationObserverOptions struct {
	ChildList             bool
	Attributes            bool
	CharacterData         bool
	Subtree               bool
	AttributeOldValue     bool
	CharacterDataOldValue bool
	AttributeFilter       []string
}

// MutationObserverCallback 是 MutationObserver 的回调类型。
type MutationObserverCallback func(records []*MutationRecord, observer *MutationObserver)

// MutationObserver 镜像 WebCore::MutationObserver。
type MutationObserver struct {
	callback MutationObserverCallback
	targets  map[Node]*MutationObserverOptions
	records  []*MutationRecord
}

// NewMutationObserver 创建一个新的 MutationObserver。
func NewMutationObserver(callback MutationObserverCallback) *MutationObserver {
	return &MutationObserver{
		callback: callback,
		targets:  make(map[Node]*MutationObserverOptions),
	}
}

// Observe 开始监听目标节点的变更。
func (mo *MutationObserver) Observe(target Node, options *MutationObserverOptions) {
	mo.targets[target] = options
	registerObserver(target, mo)
}

// Disconnect 停止所有监听并清空记录。
func (mo *MutationObserver) Disconnect() {
	for target := range mo.targets {
		unregisterObserver(target, mo)
	}
	mo.targets = make(map[Node]*MutationObserverOptions)
	mo.records = nil
}

// TakeRecords 同步返回并清空记录队列。
func (mo *MutationObserver) TakeRecords() []*MutationRecord {
	records := mo.records
	mo.records = nil
	return records
}

// QueueRecord 添加一条变更记录（由 DOM 层调用）。
func (mo *MutationObserver) QueueRecord(record *MutationRecord) {
	mo.records = append(mo.records, record)
}

// Deliver 投递所有记录到回调（由事件循环微任务队列调用）。
func (mo *MutationObserver) Deliver() {
	if len(mo.records) == 0 {
		return
	}
	records := mo.TakeRecords()
	mo.callback(records, mo)
}

// ─── 全局观察者注册 ─────────────────────────────────

var (
	// observerRegistry 按目标节点索引活跃的观察者。
	observerRegistry = make(map[Node][]*MutationObserver)
)

// ResetObserverRegistry 清空全局观察者注册表。文档导航/重建（LoadHTML
// 新文档）时调用，否则旧文档节点被 Go map 强引用、连同其观察者回调
// 一起永不被 GC（内存探针实测：每次导航累积大量对象）。
func ResetObserverRegistry() {
	observerRegistry = make(map[Node][]*MutationObserver)
}

func registerObserver(target Node, mo *MutationObserver) {
	observerRegistry[target] = append(observerRegistry[target], mo)
}

func unregisterObserver(target Node, mo *MutationObserver) {
	obs := observerRegistry[target]
	for i, o := range obs {
		if o == mo {
			observerRegistry[target] = append(obs[:i], obs[i+1:]...)
			return
		}
	}
}

// observersFor 返回关注指定节点的所有观察者。
func observersFor(target Node) []*MutationObserver {
	return observerRegistry[target]
}

// NotifyChildList 通知子节点列表变更。
// 由 AppendChild/RemoveChild/InsertBefore 调用。
func NotifyChildList(target Node, added, removed []Node, prevSibling, nextSibling Node) {
	obs := observersFor(target)
	if len(obs) == 0 {
		return
	}
	record := &MutationRecord{
		Type:            MutationChildList,
		Target:          target,
		AddedNodes:      added,
		RemovedNodes:    removed,
		PreviousSibling: prevSibling,
		NextSibling:     nextSibling,
	}
	for _, mo := range obs {
		if opts, ok := mo.targets[target]; ok && opts.ChildList {
			mo.QueueRecord(record)
		}
	}
}

// NotifyAttributes 通知属性变更。
// 由 Element.SetAttribute/RemoveAttribute 调用。
func NotifyAttributes(target *Element, name, oldValue string) {
	obs := observersFor(target)
	if len(obs) == 0 {
		return
	}
	newValue := target.GetAttribute(name)
	record := &MutationRecord{
		Type:           MutationAttributes,
		Target:         target,
		AttributeName:  name,
		AttributeValue: newValue,
	}
	for _, mo := range obs {
		if opts, ok := mo.targets[Node(target)]; ok && opts.Attributes {
			if len(opts.AttributeFilter) > 0 {
				found := false
				for _, f := range opts.AttributeFilter {
					if f == name {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			if opts.AttributeOldValue {
				record.OldValue = oldValue
			}
			mo.QueueRecord(record)
		}
	}
}

// NotifyCharacterData 通知文本内容变更。
// 由 SetTextContent/Text.SetData 调用。
func NotifyCharacterData(target Node, oldValue string) {
	obs := observersFor(target)
	if len(obs) == 0 {
		return
	}
	record := &MutationRecord{
		Type:   MutationCharacterData,
		Target: target,
	}
	for _, mo := range obs {
		if opts, ok := mo.targets[target]; ok && opts.CharacterData {
			if opts.CharacterDataOldValue {
				record.OldValue = oldValue
			}
			mo.QueueRecord(record)
		}
	}
}

// FlushMutationObservers 投递所有待处理的 MutationObserver 记录。
// 应由事件循环在每个宏任务之后调用。
func FlushMutationObservers() {
	// Snapshot observers to avoid mutation during delivery
	var toDeliver []*MutationObserver
	for _, obs := range observerRegistry {
		for _, mo := range obs {
			if len(mo.records) > 0 {
				toDeliver = append(toDeliver, mo)
			}
		}
	}
	for _, mo := range toDeliver {
		mo.Deliver()
	}
}
