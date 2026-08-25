//go:build !windows

package window

// 非 Windows 平台：无系统编辑菜单（no-op）。引擎侧在 menuFn 未注入且
// 平台不支持时跳过默认右键菜单（前端仍可通过 contextmenu 事件自建）。
func (w *Window) PopupEditMenu(x, y int, canCut, canCopy, canPaste, canSelectAll bool) int {
	return 0
}

// EditMenuAt 非 Windows 平台 no-op。
func EditMenuAt(hwnd uintptr, x, y int, canCut, canCopy, canPaste, canSelectAll bool) int {
	return 0
}
