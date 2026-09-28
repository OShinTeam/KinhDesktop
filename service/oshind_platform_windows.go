//go:build windows

package service

import "syscall"

// 平台层类型别名。
//
// 目的是让 oshind.go / download.go 里不出现 syscall 字样，从而能在非 Windows 平台
// 用 oshind_platform_other.go 中的占位类型替换掉。
// 这里用别名（=）而非新类型：底层仍是 syscall.DLL / syscall.Proc，
// 因此 *syscall.DLL 的 FindProc、*syscall.Proc 的 Call 都照常可用，
// download.go 里既有的 oshindProcXxx.Call(...) 调用点无需任何改动。
type oshindLibHandle = syscall.DLL
type oshindProc = syscall.Proc

// oshindOpenLib 加载动态库并构造句柄
func oshindOpenLib(path string) (*oshindLibHandle, error) {
	handle, err := syscall.LoadLibrary(path)
	if err != nil {
		return nil, err
	}
	return &syscall.DLL{Name: path, Handle: handle}, nil
}
