//go:build !windows && !android

package service

import "errors"

// 非 Windows 平台的占位实现。
//
// OShinD 组件目前只有 Windows 的加载实现（syscall.LoadLibrary），
// 而 syscall.DLL / syscall.Proc 并不存在于其他平台。这里提供形状一致的
// 类型与函数，使 oshind.go / download.go 无需写平台分支：
//   - 编译期：类型齐全，可正常构建出 darwin / linux / android 产物；
//   - 运行期：所有调用返回「不支持」，组件被判定为不可用，
//     主程序照常运行，仅下载相关功能不可用。
//
// 新增平台支持时替换本文件即可（另需补对应的 embed 产物）。
// 注意 Android 还有额外约束：不能从可写目录 dlopen 运行时释放的 .so，
// 必须走 APK 的 jniLibs，集成路径与桌面端不同。

// errOShinDUnsupported 非 Windows 平台的统一错误
var errOShinDUnsupported = errors.New("当前平台不支持 OShinD 组件")

type oshindLibHandle struct{}
type oshindProc struct{}

// oshindOpenLib 非 Windows 平台直接失败
func oshindOpenLib(path string) (*oshindLibHandle, error) {
	return nil, errOShinDUnsupported
}

// FindProc 与 *syscall.DLL 保持同签名
func (l *oshindLibHandle) FindProc(name string) (*oshindProc, error) {
	return nil, errOShinDUnsupported
}

// Call 与 *syscall.Proc 保持同签名
func (p *oshindProc) Call(args ...uintptr) (uintptr, uintptr, error) {
	return 0, 0, errOShinDUnsupported
}
