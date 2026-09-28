package service

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// oshindAndroidLibName Android 侧组件库名：随 APK 的 jniLibs 打包后，
// 系统会把它解到应用私有库目录，dlopen 直接用库名即可
const oshindAndroidLibName = "liboshind.so"

// ensureOShinDLib 返回可供加载的组件库路径（或库名）。
//
// 桌面端：把内嵌的产物释放到 data 目录后返回其路径；
// Android：返回库名交给平台解析。
//
// 注意：Android 目前**不会走到这里** —— loadOShinD 已在更上层短路，
// 原因是同进程内两套 Go runtime（libwails + liboshind）会互相干扰导致概率性崩溃。
// 此分支保留，供将来改用 OShinD 的 Go 包做静态集成时参考。
//
// 桌面端调用方是 loadOShinD，需保证在加载之前调用：库文件被进程加载后会处于占用状态。
func ensureOShinDLib() (string, error) {
	if runtime.GOOS == "android" {
		return oshindAndroidLibName, nil
	}

	if len(oshindEmbedded) == 0 {
		return "", fmt.Errorf("当前平台未内嵌 OShinD 组件产物")
	}

	libPath := oshindLibPath()
	if existing, err := os.ReadFile(libPath); err == nil && bytes.Equal(existing, oshindEmbedded) {
		// 已是同一版本，跳过写入（避免每次启动都落盘 ~8MB）
		return libPath, nil
	}

	if err := os.MkdirAll(filepath.Dir(libPath), 0755); err != nil {
		return "", fmt.Errorf("创建组件目录失败: %w", err)
	}
	// 先写临时文件再改名：写入中断时不会留下半截动态库被后续启动误加载
	tmpPath := libPath + ".tmp"
	if err := os.WriteFile(tmpPath, oshindEmbedded, 0644); err != nil {
		return "", fmt.Errorf("释放组件失败: %w", err)
	}
	if err := os.Rename(tmpPath, libPath); err != nil {
		return "", fmt.Errorf("替换组件失败: %w", err)
	}
	return libPath, nil
}
