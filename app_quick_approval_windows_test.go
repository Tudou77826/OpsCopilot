//go:build windows

package main

import (
	"os"
	"runtime"
	"testing"
	"unsafe"

	"opscopilot/pkg/assistantcontrol"
)

func TestQuickToastTextFallsBackToTitle(t *testing.T) {
	if firstLine("df -h\nsecond") != "df -h" || firstLine("df -h") != "df -h" {
		t.Fatal("firstLine is not the first line")
	}
	prompt := assistantcontrol.Prompt{Title: "执行远程命令", Command: "df -h\nmore"}
	message := prompt.Title + "\n" + firstLine(prompt.Command)
	if message != "执行远程命令\ndf -h" {
		t.Fatalf("unexpected toast body %q", message)
	}
}

func TestQuickBadgeHasVisibleNumberAndTransparentCorners(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	dc, _, _ := procGetDC.Call(0)
	if dc == 0 {
		t.Skip("Windows display DC is unavailable")
	}
	defer procReleaseDC.Call(0, dc)

	const size = 16
	icon := drawBadgeDisc(dc, size, "1")
	if icon == 0 {
		t.Fatal("failed to create a Windows badge icon")
	}
	defer procDestroyIcon.Call(uintptr(icon))
	var info badgeIconInfo
	if ok, _, err := modUser32Badge.NewProc("GetIconInfo").Call(uintptr(icon), uintptr(unsafe.Pointer(&info))); ok == 0 {
		t.Fatalf("read badge icon: %v", err)
	}
	defer procDeleteObject.Call(uintptr(info.HbmMask))
	defer procDeleteObject.Call(uintptr(info.HbmColor))

	var pixels [size * size]uint32
	bitmap := badgeBitmapInfo{Width: size, Height: size, Planes: 1, BitCount: 32}
	bitmap.Size = uint32(unsafe.Sizeof(bitmap))
	lines, _, err := modGdi32Badge.NewProc("GetDIBits").Call(dc, uintptr(info.HbmColor), 0, size,
		uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&bitmap)), 0)
	if lines != size {
		t.Fatalf("read badge bitmap: %d rows, %v", lines, err)
	}
	if pixels[0] != 0 || pixels[size-1] != 0 || pixels[len(pixels)-size] != 0 || pixels[len(pixels)-1] != 0 {
		t.Fatal("badge corners should be transparent")
	}
	var disc, digit bool
	for _, pixel := range pixels {
		disc = disc || pixel == 0xFFC1651C
		digit = digit || pixel == 0xFFFFFFFF
	}
	if !disc || !digit {
		t.Fatalf("badge needs an opaque disc and readable digit: disc=%t digit=%t", disc, digit)
	}
}

func TestQuickToastRendersOnDesktop(t *testing.T) {
	if os.Getenv("OPSCOPILOT_TEST_NATIVE_APPROVAL") != "1" {
		t.Skip("opt-in native desktop resources")
	}
	showApprovalToast(assistantcontrol.Prompt{Title: "测试审批通知", Command: "echo test\ntail"})
	updateApprovalBadge(2)
	updateApprovalBadge(0)
}
