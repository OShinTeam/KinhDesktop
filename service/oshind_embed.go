package service

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// ensureOShinDLib 把内嵌的组件产物释放到 data 目录，返回可供加载的库文件路径。
//
// 组件版本与主程序绑定发版，不提供外部替换通道：data 目录中既有的同名文件在内容
// 不一致时会被直接覆盖，避免用户放入的旧版本或异构库与当前主程序产生兼容性问题。
//
// 调用方是 loadOShinD，需保证在 LoadLibrary 之前调用：
// 库文件被进程加载后会处于占用状态，此处只在加载前落盘。
func ensureOShinDLib() (string, error) {
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
