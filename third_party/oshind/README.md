# OShinD 源码快照

本目录是 [OShinD](https://github.com/OshinTeam/OShinD) 下载引擎的源码快照，
由 KinhDesktop 以 Go 包形式直接引入 —— 项目根的 `go.mod` 里有：

```
replace github.com/mogumc/oshind => ./third_party/oshind
```

## 为什么用源码快照，而不是模块依赖

OShinD 仓库的 module 布局不符合 Go 规范：`go.mod` 位于 `src/` 子目录，
而 git tag（如 `v0.0.5-beta`）打在仓库根。Go 只能通过 `src/v*` 形式的
tag 解析子目录 module，因此它既不能被 `go get` 直接拉取，也无法用
`replace` 指向 GitHub 上的子目录。

快照放在项目内，好处是 CI 只 clone 本仓库即可构建，不依赖任何外部目录。

## 来源

| 项 | 值 |
|---|---|
| 仓库 | https://github.com/OshinTeam/OShinD |
| module | `github.com/mogumc/oshind`（与仓库名不一致，属上游既有情况） |
| 分支 | `dev` |
| 提交 | `650bda9` — fix(downloader): 取消任务应置 CANCELLED 而非落入 FAILED（2026-10-05） |
| 取用范围 | `src/go.mod`、`src/go.sum`、`src/pkg/`、`src/types/` |

`src/cmd/`（CLI 与 FFI 入口）**未取用**：KinhDesktop 直接调用引擎包，
不需要命令行入口，顺带避免引入其 TUI 依赖（bubbletea / lipgloss）。

## 如何更新到新版

1. 在 OShinD 仓库切到目标提交
2. 用 `src/go.mod`、`src/go.sum`、`src/pkg/`、`src/types/` 覆盖本目录对应内容，
   保持目录结构与 `src/` 一致（勿改文件内部相对结构）
3. 回到项目根执行 `go mod tidy`，再跑 `go vet ./...`
4. 留意 `service/oshind.go` 依赖的 API 是否有签名变化：
   `NewEngine` / `SubmitDownload` / `GetTask` / `CancelTask` / `PauseTask` /
   `ResumeTask` / `RemoveTask`，以及 `types.DownloadConfig` 的字段
5. 同步更新本文件顶部的「来源」表

## 注意

- **不要在本目录内改代码**。这是外部快照，任何本地修改都会在下次同步时丢失；
  需要适配请写在 `service/oshind.go` 这层。
- 本目录是**独立 Go module**（自带 go.mod），因此 `go vet ./...`
  不会扫描它，项目根的 tidy 也只会按需拉取真正用到的依赖。
- **任务状态语义（2026-10-05 起）**：引擎有 9 个状态，其中两个「用户主动中断」态
  不可恢复，务必区分，否则会出现「取消后被自动重试复活」：
  - `PAUSED` — 暂停，保留 `.oshin` 断点，可 `ResumeTask`
  - `CANCELLED` — 取消，保留 `.oshin` 断点，但 `ResumeTask` 会拒绝
  - 两者调用 `types.DownloadTask.Fail()` 时都是 no-op（不被记成 FAILED）
  - 前端轮询只在 `status == "FAILED"` 时触发自动重试，故 `CANCELLED` 天然免疫
- 许可证：AGPL-3.0（见同目录 `LICENSE`），版权归 OshinTeam。
  本项目为 GPL-2.0，两者的兼容性需由项目负责人确认。
