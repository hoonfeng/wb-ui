//go:build windows

package window

// 系统右键编辑菜单（剪切/复制/粘贴/全选），Windows 原生 TrackPopupMenu。
// 引擎默认上下文菜单：编辑框右键弹出——wb-ui 无自有菜单 UI，下沉到平台层
// 使用系统菜单（Windows 标准行为，菜单文字用中文与主场景一致）。
import (
	"syscall"
	"unsafe"
)

// 编辑菜单命令 ID（PopupEditMenu 返回值；引擎层按此执行编辑操作）。
const (
	EditMenuCut       = 1
	EditMenuCopy      = 2
	EditMenuPaste     = 3
	EditMenuSelectAll = 4
)

var (
	user32E                 = syscall.NewLazyDLL("user32.dll")
	procCreatePopupMenu     = user32E.NewProc("CreatePopupMenu")
	procAppendMenuW         = user32E.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32E.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32E.NewProc("DestroyMenu")
	procClientToScreen      = user32E.NewProc("ClientToScreen")
	procSetForegroundWindow = user32E.NewProc("SetForegroundWindow")
)

type winPOINT struct{ X, Y int32 }

// PopupEditMenu 在 (x,y)（窗口客户区坐标，逻辑像素）弹出系统编辑菜单并
// 阻塞等待用户选择。canCut/canCopy/canPaste/canSelectAll 控制菜单项使能
// （灰色）。返回 EditMenu* 命令 ID；0 = 取消。
func (w *Window) PopupEditMenu(x, y int, canCut, canCopy, canPaste, canSelectAll bool) int {
	if w == nil || w.win == nil {
		return 0
	}
	return EditMenuAt(platformHWND(w.win), x, y, canCut, canCopy, canPaste, canSelectAll)
}

// EditMenuAt 在 (x,y)（客户区坐标）弹出系统编辑菜单并阻塞等待选择。
// hwnd 为任意 Win32 窗口句柄（不依赖 glfw——裸 WebView 宿主（configwin
// 等自管理窗口）复用同一菜单实现）。返回 EditMenu* 命令 ID；0 = 取消。
func EditMenuAt(hwnd uintptr, x, y int, canCut, canCopy, canPaste, canSelectAll bool) int {
	if hwnd == 0 {
		return 0
	}
	const (
		MF_STRING    = 0x0000
		MF_GRAYED    = 0x0001
		MF_SEPARATOR = 0x0800
	)
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return 0
	}
	defer procDestroyMenu.Call(menu)

	add := func(id int, text string, enabled bool) {
		flags := uintptr(MF_STRING)
		if !enabled {
			flags |= MF_GRAYED
		}
		ptext, err := syscall.UTF16PtrFromString(text)
		if err != nil {
			return
		}
		procAppendMenuW.Call(menu, flags, uintptr(id), uintptr(unsafe.Pointer(ptext)))
	}
	add(EditMenuCut, "剪切(&T)", canCut)
	add(EditMenuCopy, "复制(&C)", canCopy)
	add(EditMenuPaste, "粘贴(&P)", canPaste)
	procAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0)
	add(EditMenuSelectAll, "全选(&A)", canSelectAll)

	// 客户区坐标 → 屏幕坐标（TrackPopupMenu 需要屏幕坐标）
	var pt winPOINT
	pt.X, pt.Y = int32(x), int32(y)
	procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&pt)))

	// TrackPopupMenu 要求窗口在前台（否则菜单立即消失或无法键盘导航）
	procSetForegroundWindow.Call(hwnd)

	const (
		TPM_RETURNCMD = 0x0100
		TPM_LEFTALIGN = 0x0000
		TPM_TOPALIGN  = 0x0000
		TPM_NONOTIFY  = 0x0080
	)
	cmd, _, _ := procTrackPopupMenu.Call(menu,
		uintptr(TPM_RETURNCMD|TPM_LEFTALIGN|TPM_TOPALIGN|TPM_NONOTIFY),
		0, uintptr(int32(pt.X)), uintptr(int32(pt.Y)), 0, hwnd, 0)
	return int(cmd)
}
