//go:build windows

package main

// Approval notifications replace the former modal MessageBox: while approval
// prompts wait and the desktop app is not focused, a transient toast appears
// bottom-right and the taskbar button carries a count badge. Neither channel
// grants a decision; every choice is made in the in-app approval card.

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"opscopilot/pkg/assistantcontrol"
)

type assistantPrompt = assistantcontrol.Prompt

var (
	modShell32Notify = windows.NewLazySystemDLL("shell32.dll")
	modGdi32Badge    = windows.NewLazySystemDLL("gdi32.dll")
	modUser32Badge   = windows.NewLazySystemDLL("user32.dll")

	procShellNotifyIcon    = modShell32Notify.NewProc("Shell_NotifyIconW")
	procExtractIconExW     = modShell32Notify.NewProc("ExtractIconExW")
	procCreateSolidBrush   = modGdi32Badge.NewProc("CreateSolidBrush")
	procCreatePen          = modGdi32Badge.NewProc("CreatePen")
	procSelectObject       = modGdi32Badge.NewProc("SelectObject")
	procDeleteObject       = modGdi32Badge.NewProc("DeleteObject")
	procEllipse            = modGdi32Badge.NewProc("Ellipse")
	procSetBkMode          = modGdi32Badge.NewProc("SetBkMode")
	procSetTextColor       = modGdi32Badge.NewProc("SetTextColor")
	procDrawTextW          = modUser32Badge.NewProc("DrawTextW")
	procCreateDIBSection   = modGdi32Badge.NewProc("CreateDIBSection")
	procCreateBitmap       = modGdi32Badge.NewProc("CreateBitmap")
	procCreateCompatibleDC = modGdi32Badge.NewProc("CreateCompatibleDC")
	procDeleteDC           = modGdi32Badge.NewProc("DeleteDC")
	procCreateFontW        = modGdi32Badge.NewProc("CreateFontW")
	procGdiFlush           = modGdi32Badge.NewProc("GdiFlush")
	procCoCreateTaskbar    = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")
	procCreateIconIndirect = modUser32Badge.NewProc("CreateIconIndirect")
	procGetDC              = modUser32Badge.NewProc("GetDC")
	procReleaseDC          = modUser32Badge.NewProc("ReleaseDC")
	procGetDeviceCaps      = modGdi32Badge.NewProc("GetDeviceCaps")
	procDestroyIcon        = modUser32Badge.NewProc("DestroyIcon")
	procLoadIconW          = modUser32Badge.NewProc("LoadIconW")
	procEnumWindowsBadge   = modUser32Badge.NewProc("EnumWindows")
	procIsWindowVisibleB   = modUser32Badge.NewProc("IsWindowVisible")
)

const (
	nimAdd, nimDelete            = 0x0, 0x2
	nifMessage, nifIcon, nifInfo = 0x1, 0x2, 0x10
	niifInfo, niifUser           = 0x1, 0x4
	idiApplication               = 32512
	notifyIconUID                = 0x4150 // "AP"
	toastLifetime                = 12 * time.Second
)

type notifyIconData struct {
	CbSize           uint32
	HWnd             windows.HWND
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            windows.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     windows.Handle
}

type badgeIconInfo struct {
	FIcon              uint32
	XHotspot, YHotspot uint32
	HbmMask            windows.Handle
	HbmColor           windows.Handle
}

type badgeBitmapInfo struct {
	Size        uint32
	Width       int32
	Height      int32
	Planes      uint16
	BitCount    uint16
	Compression uint32
	SizeImage   uint32
	_           [16]byte
}

var notifyWindowSearch struct {
	sync.Mutex
	result windows.HWND
}

// EnumWindows calls this synchronously while notifyWindowSearch is locked.
// Reuse the callback so repeated lookups do not consume Windows callback slots.
var notifyWindowCallback = windows.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
	var pid uint32
	windows.GetWindowThreadProcessId(hwnd, &pid)
	if pid != uint32(os.Getpid()) {
		return 1
	}
	visible, _, _ := procIsWindowVisibleB.Call(uintptr(hwnd))
	if visible == 0 {
		return 1
	}
	notifyWindowSearch.result = hwnd
	return 0
})

func notifyMainWindow() windows.HWND {
	notifyWindowSearch.Lock()
	defer notifyWindowSearch.Unlock()
	notifyWindowSearch.result = 0
	procEnumWindowsBadge.Call(notifyWindowCallback, 0)
	return notifyWindowSearch.result
}

// approvalAppIcon extracts the small icon from the packaged application.
// The caller owns the returned handle.
func approvalAppIcon() windows.Handle {
	exe, err := os.Executable()
	if err != nil {
		return 0
	}
	path, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0
	}
	var icon windows.Handle
	procExtractIconExW.Call(uintptr(unsafe.Pointer(path)), 0, 0, uintptr(unsafe.Pointer(&icon)), 1)
	return icon
}

// showApprovalToast raises one transient bottom-right notification. Failures
// are silent: the in-app card remains the authoritative surface.
func showApprovalToast(prompt assistantPrompt) {
	if prompt.Title == "" {
		return
	}
	hwnd := notifyMainWindow()
	if hwnd == 0 {
		return
	}
	message := prompt.Title
	if prompt.Command != "" {
		message = prompt.Title + "\n" + firstLine(prompt.Command)
	}
	titleUTF16, err := windows.UTF16FromString("OpsCopilot 审批等待确认")
	if err != nil || len(titleUTF16) > 64 {
		return
	}
	messageUTF16, err := windows.UTF16FromString(message)
	if err != nil || len(messageUTF16) > 256 {
		messageUTF16, _ = windows.UTF16FromString(prompt.Title)
		if len(messageUTF16) > 256 {
			return
		}
	}
	var data notifyIconData
	data.CbSize = uint32(unsafe.Sizeof(data))
	data.HWnd = hwnd
	data.UID = notifyIconUID
	data.UFlags = nifMessage | nifInfo
	data.UCallbackMessage = 0x8000 // WM_APP: inert, no handler is installed
	appIcon := approvalAppIcon()
	if appIcon != 0 {
		data.HIcon = appIcon
		data.HBalloonIcon = appIcon
		data.DwInfoFlags = niifUser
	} else {
		handle, _, _ := procLoadIconW.Call(0, idiApplication)
		data.HIcon = windows.Handle(handle)
		data.DwInfoFlags = niifInfo
	}
	if data.HIcon != 0 {
		data.UFlags |= nifIcon
	}
	copy(data.SzInfoTitle[:], titleUTF16)
	copy(data.SzInfo[:], messageUTF16)
	added, _, _ := procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&data)))
	if added == 0 {
		if appIcon != 0 {
			procDestroyIcon.Call(uintptr(appIcon))
		}
		return
	}
	// The balloon disappears on its own; remove the hidden tray entry after a
	// bounded delay so repeated prompts never accumulate tray icons.
	go func(snapshot notifyIconData) {
		time.Sleep(toastLifetime)
		procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&snapshot)))
		if appIcon != 0 {
			procDestroyIcon.Call(uintptr(appIcon))
		}
	}(data)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

type taskbarList3 struct {
	vtable *struct {
		_              [2]uintptr // QueryInterface, AddRef
		release        uintptr
		hrInit         uintptr
		_              [14]uintptr // AddTab through ThumbBarSetImageList
		setOverlayIcon uintptr     // slot 18, per the Windows SDK
	}
}

// updateApprovalBadge overlays the pending count on the app's taskbar button.
// A zero count clears the overlay. A failed update can be retried by the watcher.
func updateApprovalBadge(count int) bool {
	hwnd := notifyMainWindow()
	if hwnd == 0 {
		return false
	}
	// COM initialization and the interface lifetime belong to one OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	switch err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); err {
	case nil, syscall.Errno(1): // S_OK or S_FALSE
		defer windows.CoUninitialize()
	case syscall.Errno(windows.RPC_E_CHANGED_MODE):
		// Use the apartment already initialized on this thread.
	default:
		return false
	}
	clsid := windows.GUID{Data1: 0x56FDF344, Data2: 0xFD6D, Data3: 0x11D0, Data4: [8]byte{0x95, 0x8A, 0x00, 0x60, 0x97, 0xC9, 0xA0, 0x90}}
	iid := windows.GUID{Data1: 0xEA1AFB91, Data2: 0x9E28, Data3: 0x4B86, Data4: [8]byte{0x90, 0xE9, 0x9E, 0x9F, 0x8A, 0x5E, 0xEF, 0xAF}}
	var taskbar *taskbarList3
	hr, _, _ := procCoCreateTaskbar.Call(uintptr(unsafe.Pointer(&clsid)), 0, 1, // CLSCTX_INPROC_SERVER
		uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&taskbar)))
	if hr != 0 || taskbar == nil {
		return false
	}
	defer syscall.SyscallN(taskbar.vtable.release, uintptr(unsafe.Pointer(taskbar)))
	hr, _, _ = syscall.SyscallN(taskbar.vtable.hrInit, uintptr(unsafe.Pointer(taskbar)))
	if hr != 0 {
		return false
	}

	var icon windows.Handle
	var description *uint16
	if count > 0 {
		icon = renderBadgeIcon(count)
		if icon == 0 {
			return false
		}
		defer procDestroyIcon.Call(uintptr(icon)) // The taskbar copies the icon.
		description, _ = windows.UTF16PtrFromString(fmt.Sprintf("%d 项审批等待确认", count))
	}
	hr, _, _ = syscall.SyscallN(taskbar.vtable.setOverlayIcon, uintptr(unsafe.Pointer(taskbar)), uintptr(hwnd), uintptr(icon), uintptr(unsafe.Pointer(description)))
	return hr == 0
}

// renderBadgeIcon draws count as a filled disc sized for the taskbar DPI.
func renderBadgeIcon(count int) windows.Handle {
	dc, _, _ := procGetDC.Call(0)
	if dc == 0 {
		return 0
	}
	defer procReleaseDC.Call(0, dc)
	dpi, _, _ := procGetDeviceCaps.Call(dc, 88) // LOGPIXELSX
	if int(dpi) <= 0 {
		dpi = 96
	}
	size := 16 * int(dpi) / 96
	if size < 12 {
		size = 12
	}
	label := fmt.Sprintf("%d", count)
	if count > 9 {
		label = "9+"
	}
	return drawBadgeDisc(dc, size, label)
}

// drawBadgeDisc wraps the rendered count in a 32-bit alpha icon.
func drawBadgeDisc(dc uintptr, size int, label string) windows.Handle {
	bi := badgeBitmapInfo{Width: int32(size), Height: int32(size), Planes: 1, BitCount: 32}
	bi.Size = uint32(unsafe.Sizeof(bi))
	var bits unsafe.Pointer
	bitmap, _, _ := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)), 0 /*DIB_RGB_COLORS*/, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 {
		return 0
	}
	defer procDeleteObject.Call(bitmap)
	if bits == nil {
		return 0
	}
	pixels := unsafe.Slice((*uint32)(bits), size*size)
	clear(pixels)

	memDC, _, _ := procCreateCompatibleDC.Call(dc)
	if memDC == 0 {
		return 0
	}
	defer procDeleteDC.Call(memDC)
	oldBitmap, _, _ := procSelectObject.Call(memDC, bitmap)
	if oldBitmap == 0 {
		return 0
	}
	accent := uint32(0x001C65C1) // COLORREF 0x00BBGGRR: the approval badge color.
	brush, _, _ := procCreateSolidBrush.Call(uintptr(accent))
	defer procDeleteObject.Call(brush)
	pen, _, _ := procCreatePen.Call(0 /*PS_SOLID*/, 1, uintptr(accent))
	defer procDeleteObject.Call(pen)
	oldBrush, _, _ := procSelectObject.Call(memDC, brush)
	oldPen, _, _ := procSelectObject.Call(memDC, pen)
	procEllipse.Call(memDC, 0, 0, uintptr(size), uintptr(size))
	procSelectObject.Call(memDC, oldBrush)
	procSelectObject.Call(memDC, oldPen)

	procSetBkMode.Call(memDC, 1 /*TRANSPARENT*/)
	procSetTextColor.Call(memDC, 0x00FFFFFF)
	fontHeight := -int32(size * 10 / 16)
	font, _, _ := procCreateFontW.Call(uintptr(fontHeight), 0, 0, 0, 700, 0, 0, 0, 1, 0, 0, 4, 0, 0)
	if font != 0 {
		defer procDeleteObject.Call(font)
		oldFont, _, _ := procSelectObject.Call(memDC, font)
		defer procSelectObject.Call(memDC, oldFont)
	}
	text, _ := windows.UTF16FromString(label)
	draw := rect{Right: int32(size), Bottom: int32(size)}
	procDrawTextW.Call(memDC, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)-1),
		uintptr(unsafe.Pointer(&draw)), 0x00000025 /*DT_CENTER|DT_VCENTER|DT_SINGLELINE*/)
	procGdiFlush.Call()
	procSelectObject.Call(memDC, oldBitmap) // CreateIconIndirect needs unselected bitmaps.

	// GDI writes RGB only. Make the painted pixels opaque and keep the corners clear.
	for i, pixel := range pixels {
		if pixel&0x00FFFFFF != 0 {
			pixels[i] = pixel | 0xFF000000
		}
	}
	// The color icon uses its alpha channel. Supply a separate monochrome AND mask.
	maskBits := make([]byte, ((size+15)/16)*2*size)
	mask, _, _ := procCreateBitmap.Call(uintptr(size), uintptr(size), 1, 1, uintptr(unsafe.Pointer(&maskBits[0])))
	if mask == 0 {
		return 0
	}
	defer procDeleteObject.Call(mask)
	info := badgeIconInfo{FIcon: 1, HbmMask: windows.Handle(mask), HbmColor: windows.Handle(bitmap)}
	icon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&info)))
	return windows.Handle(icon)
}

type rect struct{ Left, Top, Right, Bottom int32 }
