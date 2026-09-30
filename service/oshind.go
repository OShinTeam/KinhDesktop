package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mogumc/oshind/pkg/downloader"
	"github.com/mogumc/oshind/types"

	"kinh-desktop/global"
)

// ==================== OShinD 下载引擎（包引入·静态集成） ====================
//
// 历史形态：OShinD 曾以 c-shared 动态库（FFI）方式引入 —— 编译期把按平台产物 embed
// 进主程序，运行期释放到 data/ 后 dlopen，再逐个解析符号并据此判定版本兼容。
// 该方案有两个硬伤：
//
//  1. libwails 与 liboshind 同为 Go 编译出的 c-shared 库，同一进程内存在两套 Go runtime
//     （各自注册 signal handler、各自维护 cgocallback 的 goroutine 记录），彼此干扰，
//     表现为概率性崩溃。跨 .so 的 Go panic 会直接 abort 进程，调用侧 recover 拦不住，
//     因此 Android 一度被整体禁用。
//  2. 需要长期维护「按平台拉产物 → embed → 释放 → 符号解析 → 兼容判定」一整条链路，
//     连带 data/ 目录落盘 ~8MB、产物文件名约定、jniLibs 打包等一串约束。
//
// 现直接引入 OShinD 的 Go 包，随主程序一同编译：单 runtime、单进程，上述问题不复存在，
// 动态库产物与各平台加载器全部不再需要，Android 也因此解除了原有禁用。
//
// 引擎以进程级单例常驻（Engine 内部自带任务表与读写锁），与主程序同生命周期。
// 任务状态对外仍是 JSON 形态（前端按字段渲染），不因引入方式改变而变动。

// oshindRepo OShinD 仓库坐标（设置页展示与跳转用）
const oshindRepo = "OshinTeam/OShinD"

// oshindVersion 集成的 OShinD 版本。
// OShinD 未导出自己的版本常量（cmd/cli 与 cmd/ffi 各自硬编码 "1.0.0"），
// 这里与上游保持一致，升级引入的组件版本时同步更新。
const oshindVersion = "1.0.0"

var (
	oshindEngineOnce sync.Once
	oshindEngine     *downloader.Engine
)

// downloadEngine 返回下载引擎单例
func downloadEngine() *downloader.Engine {
	oshindEngineOnce.Do(func() {
		oshindEngine = downloader.NewEngine(nil)
		global.Log.Infof("OShinD 下载引擎已就绪 (v%s)", oshindVersion)
	})
	return oshindEngine
}

// OShinDInfo 下载引擎信息（返回给前端）
type OShinDInfo struct {
	Version string `json:"version"`
	RepoURL string `json:"repo_url"`
}

// GetOShinDVersion 返回下载引擎信息。
// 引擎已编译进主程序，不存在「未安装」「加载失败」这一类状态。
func (a *App) GetOShinDVersion() OShinDInfo {
	return OShinDInfo{
		Version: oshindVersion,
		RepoURL: "https://github.com/" + oshindRepo,
	}
}

// buildDownloadConfig 组装引擎下载配置。
// outputDir / ua 为空表示不覆盖默认值；connections、chunkKB <= 0 表示沿用引擎默认。
//
// 注意：引擎当前不提供代理配置项（types.DownloadConfig 无对应字段，
// 内部 http.Transport 也未接 ProxyFromEnvironment），因此下载代理设置实际不生效。
func buildDownloadConfig(outputDir, ua string, connections, chunkKB int) *types.DownloadConfig {
	config := types.DefaultConfig()
	if outputDir != "" {
		config.OutputDir = outputDir
	}
	if connections > 0 {
		config.MaxConnections = connections
	}
	if chunkKB > 0 {
		config.ChunkSize = int64(chunkKB) * 1024
	}
	if ua != "" && config.Headers != nil {
		config.Headers["User-Agent"] = ua
	}
	return config
}

// ==================== 任务状态（对外 JSON 形态，与切换前保持一致） ====================

// oshindChunkState 分片状态
type oshindChunkState struct {
	Index      int               `json:"index"`
	Start      int64             `json:"start"`
	End        int64             `json:"end"`
	Status     string            `json:"status"`
	Downloaded int64             `json:"downloaded"`
	Speed      float64           `json:"speed"`
	Headers    map[string]string `json:"headers,omitempty"`
	RetryCount int               `json:"retry_count"`
	Error      string            `json:"error,omitempty"`
}

// oshindTaskState 任务状态
type oshindTaskState struct {
	ID              string             `json:"id"`
	URL             string             `json:"url"`
	FileName        string             `json:"file_name"`
	Status          string             `json:"status"`
	Progress        float64            `json:"progress"`
	Speed           float64            `json:"speed"`
	Downloaded      int64              `json:"downloaded"`
	Total           int64              `json:"total"`
	Chunks          []oshindChunkState `json:"chunks"`
	Protocol        string             `json:"protocol"`
	MultiSource     bool               `json:"multi_source"`
	ActiveThreads   int32              `json:"active_threads"`
	RemainingChunks int32              `json:"remaining_chunks"`
	FailedChunks    int32              `json:"failed_chunks"`
	MaxConnections  int                `json:"max_connections"`
	ChunkSize       int64              `json:"chunk_size"`
	TempSize        int64              `json:"temp_size"`
	CreatedAt       string             `json:"created_at"`
	UpdatedAt       string             `json:"updated_at"`
	Error           string             `json:"error,omitempty"`
}

// buildOShindTaskState 把引擎侧任务转换为对外状态结构（读取均为线程安全快照）
func buildOShindTaskState(task *types.DownloadTask) oshindTaskState {
	snapshots := task.GetChunkSnapshots()
	chunks := make([]oshindChunkState, len(snapshots))
	for i, snap := range snapshots {
		chunks[i] = oshindChunkState{
			Index:      snap.Index,
			Start:      snap.Start,
			End:        snap.End,
			Status:     snap.Status.String(),
			Downloaded: snap.Downloaded,
			Headers:    snap.Headers,
			RetryCount: snap.RetryCount,
		}
		if snap.Error != nil {
			chunks[i].Error = snap.Error.Error()
		}
	}

	// 进度基于已知总大小；probe 未完成时 Size 为 -1 或 0，此时保持 0 不显示负数
	progressPct := 0.0
	if task.Metadata != nil && task.Metadata.Size > 0 {
		progressPct = float64(task.Progress.GetDownloaded()) / float64(task.Metadata.Size) * 100
	}

	state := oshindTaskState{
		ID:              task.ID,
		URL:             task.URL,
		FileName:        task.FileName,
		Status:          task.GetStatus().String(),
		Progress:        progressPct,
		Speed:           task.Progress.CalculateSpeed(),
		Downloaded:      task.Progress.GetDownloaded(),
		Chunks:          chunks,
		Protocol:        task.Protocol.String(),
		MultiSource:     task.MultiSource,
		ActiveThreads:   task.Progress.GetActiveThreads(),
		RemainingChunks: task.Progress.GetRemainingChunks(),
		FailedChunks:    task.Progress.GetFailedChunks(),
		CreatedAt:       task.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       task.UpdatedAt.Format(time.RFC3339),
	}
	if task.Metadata != nil {
		state.Total = task.Metadata.Size
	}
	if task.Config != nil {
		state.MaxConnections = task.Config.MaxConnections
		state.ChunkSize = task.Config.ChunkSize
		state.TempSize = oshindTempSize(task)
	}
	if taskErr := task.GetError(); taskErr != nil {
		state.Error = taskErr.Error()
	}
	return state
}

// oshindTempSize 输出文件对应的临时分片文件已写入字节数
func oshindTempSize(task *types.DownloadTask) int64 {
	outputPath := task.OutputPath
	if outputPath == "" && task.Config != nil {
		outputPath = filepath.Join(task.Config.OutputDir, task.FileName)
	}
	if outputPath == "" {
		return 0
	}
	fi, err := os.Stat(downloader.GetTempPath(outputPath))
	if err != nil {
		return 0
	}
	return fi.Size()
}

// oshindTaskStatus 查询单个任务状态 JSON（任务不存在或序列化失败返回空串）
func oshindTaskStatus(taskID string) string {
	task, ok := downloadEngine().GetTask(taskID)
	if !ok {
		return ""
	}
	data, err := json.Marshal(buildOShindTaskState(task))
	if err != nil {
		global.Log.Warnf("序列化下载任务状态失败: %v", err)
		return ""
	}
	return string(data)
}
