# OShinD 组件产物目录

本目录存放 OShinD 下载组件的按平台产物，由 Go 的 `//go:embed` 在编译期内嵌进主程序。

## 为什么目录是空的

动态库属于编译产物，**不入版本库**（根 `.gitignore` 已排除 `*.dll` / `*.so` / `*.dylib`）。
编译前需要先拉取对应平台的产物：

```
wails3 task common:fetch:oshind
```

该任务会按当前 `GOOS` / `GOARCH` 拼出文件名，从 OShinD Releases 下载到本目录并校验 sha256。
CI 构建走的是同一个任务，不需要重复实现下载逻辑。

## 文件命名

文件名与 OShinD Releases 的 asset 命名保持一致，便于按目标平台直接拼接下载地址：

| 平台 | 文件名 |
|---|---|
| windows/amd64 | `oshind-windows-amd64.dll` |
| darwin/amd64 | `liboshind-darwin-amd64.dylib` |
| darwin/arm64 | `liboshind-darwin-arm64.dylib` |
| linux/amd64 | `liboshind-linux-amd64.so` |
| android/amd64 | `liboshind-android-amd64.so` |
| android/arm64 | `liboshind-android-arm64.so` |

## 当前支持范围

仅 **windows/amd64** 已打通（内嵌 + 加载）。

其余平台需要两件事同时到位，缺一不可（详见 `../oshind_embed_unsupported.go` 的说明）：

1. 补对应的 embed 文件（如 `../oshind_embed_darwin_arm64.go`）；
2. 把加载实现从 Windows 专属的 `syscall.LoadLibrary` 换成跨平台方案（purego 的 `Dlopen` / `Dlsym`）。

此外 Android 不能从可写目录 `dlopen` 运行时释放的 `.so`（受 SELinux 与 W^X 限制），
必须打包进 APK 的 `jniLibs/<abi>/`，集成路径与桌面端不同。

## 组件可否自行替换

**不可以，这是刻意的设计。** OShinD 仍在演进，主程序与组件之间存在 FFI 接口约定，
放开外部替换会让"接口不匹配"变成难以排查的运行时故障。内嵌产物随主程序发版，
`data/` 目录中的旧文件会在版本不一致时被覆盖。
