//go:build !(windows && amd64)

package service

// 未内嵌组件产物的平台占位：置空后 ensureOShinDLib 会判定「当前平台无组件」，
// 主程序照常运行，仅下载功能不可用。
//
// 新增平台支持时需要两件事同时到位，缺一不可：
//  1. 按平台补一个 embed 文件（如 oshind_embed_darwin_arm64.go），内含 oshindlib/ 下的对应产物；
//  2. 把加载实现从 Windows 专属的 syscall.LoadLibrary 换成跨平台方案（purego 的 Dlopen/Dlsym）。
//
// 在此之前，本平台即使提供了库文件也无法被加载。
var oshindEmbedded []byte
