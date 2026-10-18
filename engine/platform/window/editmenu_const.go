// Package window — 系统右键编辑菜单命令 ID。
//
// 常量本身与平台无关（PopupEditMenu 的返回值，引擎层按此执行剪切/复制/
// 粘贴/全选），而 Windows 之外的平台 PopupEditMenu 是 no-op —— 但 app/host.go
// 跨平台引用这四个常量，所以必须放在无 build tag 的文件里（此前定义在
// editmenu_windows.go 中，导致 linux/darwin GUI 构建报 undefined）。
package window

// 编辑菜单命令 ID（PopupEditMenu 返回值；引擎层按此执行编辑操作）。
const (
	EditMenuCut       = 1
	EditMenuCopy      = 2
	EditMenuPaste     = 3
	EditMenuSelectAll = 4
)
