//go:build windows && amd64

package service

import _ "embed"

// OShinD 组件按目标平台内嵌：embed 的路径必须位于本包目录内，故统一放在 oshindlib/。
// 文件名与 OShinD Releases 的 asset 命名保持一致，便于 Taskfile 按 GOOS/GOARCH 直接拼出下载地址。
//
// 动态库不入库（.gitignore 已排除 *.dll/*.so/*.dylib），
// 编译前需先执行 `wails3 task common:fetch:oshind` 把对应平台产物拉取到本目录。
//
//go:embed oshindlib/oshind-windows-amd64.dll
var oshindEmbedded []byte
