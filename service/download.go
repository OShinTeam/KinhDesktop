package service

import (
	"encoding/json"
	"strings"
	"sync"
	"time"
	"unsafe"

	"kinh-desktop/global"
)

// ==================== 下载任务管理（组件存在时通过 OShinD FFI 执行） ====================
//
// 组件存在：解析直链后调用 OShinD_Download 推送任务，任务状态由组件维护，
//           前端轮询 GetDownloadTasks（透传组件状态 JSON）。
// 组件不存在：无任务能力，下载管理页展示安装引导。
// 代理设置：download_proxy 非空时透传给组件 options.proxy。

type DownloadTaskInfo struct {
	TaskID   string `json:"task_id"`
	URL      string `json:"url"`
	FileName string `json:"file_name"`
	Status   string `json:"status"`
}

type DownloadSubmitResult struct {
	Success bool   `json:"success"`
	TaskID  string `json:"task_id"`
	Message string `json:"message,omitempty"`
}

// downloadTaskStore 本地任务台账（task_id → 元数据），组件侧保存完整状态
type downloadTaskEntry struct {
	TaskID   string    `json:"task_id"`
	FileName string    `json:"file_name"`
	URL      string    `json:"url"`
	Created  time.Time `json:"created_at"`
}

var (
	downloadMu    sync.Mutex
	downloadTasks []downloadTaskEntry // 提交顺序保留（新任务追加尾部）
)

// SubmitDownload 解析并提交下载任务（组件存在时可用）
// fsID >= 0 表示网盘文件（先解析直链），url 非空表示自定义任务直接下载
func (a *App) SubmitDownload(url, fileName string, fsID int64) *DownloadSubmitResult {
	result := &DownloadSubmitResult{}

	installed, _, loadErr := loadOShinD()
	if !installed {
		result.Message = "OShinD 组件未安装"
		if loadErr != nil {
			result.Message += "（" + loadErr.Error() + "）"
		}
		return result
	}

	// 网盘文件：先解析直链
	if fsID > 0 {
		linkResult := a.GetBaiduDownloadLink(fsID)
		if !linkResult.Success {
			result.Message = linkResult.Message
			return result
		}
		url = linkResult.Dlink
		if fileName == "" {
			fileName = linkResult.Filename
		}
	}
	if url == "" {
		result.Message = "下载地址为空"
		return result
	}

	settings := getSettings()
	opts := oshindDownloadOptions(url, effectiveDownloadUA(), settings.DownloadDir, settings.DownloadProxy, settings.DownloadThreads)

	ret, _, err := oshindProcDl.Call(strPtr(url), strPtr(opts))
	if err != nil && isRealErr(err) {
		global.Log.Errorf("OShinD 提交下载任务失败: %v", err)
		result.Message = "提交下载任务失败"
		return result
	}
	taskID := cStringToGo(ret)
	if taskID == "" {
		result.Message = "提交下载任务失败（组件返回空任务 ID）"
		return result
	}

	downloadMu.Lock()
	downloadTasks = append(downloadTasks, downloadTaskEntry{
		TaskID:   taskID,
		FileName: fileName,
		URL:      url,
		Created:  time.Now(),
	})
	downloadMu.Unlock()

	result.Success = true
	result.TaskID = taskID
	global.Log.Infof("下载任务已提交: %s (%s)", fileName, taskID)
	return result
}

// GetDownloadTasks 返回任务列表（台账元数据 + 组件实时状态合并）
func (a *App) GetDownloadTasks() []map[string]interface{} {
	installed, _, _ := loadOShinD()
	if !installed {
		return []map[string]interface{}{}
	}

	downloadMu.Lock()
	tasks := append([]downloadTaskEntry(nil), downloadTasks...)
	downloadMu.Unlock()

	list := make([]map[string]interface{}, 0, len(tasks))
	for idx, entry := range tasks {
		item := map[string]interface{}{
			"task_id":    entry.TaskID,
			"file_name":  entry.FileName,
			"url":        entry.URL,
			"created_at": entry.Created.Format(time.RFC3339),
		}
		// 透传组件状态：失败/组件侧任务丢失时保留台账元数据
		if statusJSON := oshindTaskStatus(entry.TaskID); statusJSON != "" {
			var status map[string]interface{}
			if json.Unmarshal([]byte(statusJSON), &status) == nil {
				for _, k := range []string{"status", "progress", "speed", "downloaded", "total", "error", "file_name"} {
					if v, ok := status[k]; ok {
						item[k] = v
					}
				}
				// probe 后组件返回真实文件名（含 Content-Disposition 名称），
				// 台账为空时回填；用户显式指定的名称不覆盖
				if name, _ := status["file_name"].(string); name != "" {
					downloadMu.Lock()
					if downloadTasks[idx].TaskID == entry.TaskID && downloadTasks[idx].FileName == "" {
						downloadTasks[idx].FileName = name
					}
					downloadMu.Unlock()
				}
			}
		}
		list = append(list, item)
	}
	return list
}

// CancelDownloadTask 取消任务（保留已下载内容）
func (a *App) CancelDownloadTask(taskID string) bool {
	installed, _, _ := loadOShinD()
	if !installed || oshindProcCancel == nil {
		return false
	}
	ret, _, err := oshindProcCancel.Call(strPtr(taskID))
	if err != nil && isRealErr(err) {
		global.Log.Warnf("OShinD 取消任务失败: %v", err)
		return false
	}
	return ret == 1
}

// PauseDownloadTask 暂停任务（组件保存断点状态，可恢复）
func (a *App) PauseDownloadTask(taskID string) bool {
	installed, _, _ := loadOShinD()
	if !installed || oshindProcPause == nil {
		return false
	}
	_, _, err := oshindProcPause.Call(strPtr(taskID))
	if err != nil && isRealErr(err) {
		global.Log.Warnf("OShinD 暂停任务失败: %v", err)
		return false
	}
	return true
}

// ResumeDownloadTask 恢复暂停/失败的任务
// 组件侧移除旧任务并重新提交（自动检测 .oshin 断点状态），返回新任务 ID，
// 台账条目需同步替换 ID，否则后续轮询查不到状态
func (a *App) ResumeDownloadTask(taskID string) bool {
	installed, _, _ := loadOShinD()
	if !installed || oshindProcResume == nil {
		return false
	}
	ret, _, err := oshindProcResume.Call(strPtr(taskID))
	if err != nil && isRealErr(err) {
		global.Log.Warnf("OShinD 恢复任务失败: %v", err)
		return false
	}
	resp := cStringToGo(ret)
	var result struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(resp), &result) != nil || result.ID == "" {
		global.Log.Warnf("OShinD 恢复任务返回异常: %s", resp)
		return false
	}
	if result.Error != "" {
		global.Log.Warnf("OShinD 恢复任务失败: %s", result.Error)
		return false
	}

	// 台账换 ID（保持原位置与创建时间、文件名元数据）
	downloadMu.Lock()
	for i := range downloadTasks {
		if downloadTasks[i].TaskID == taskID {
			downloadTasks[i].TaskID = result.ID
			break
		}
	}
	downloadMu.Unlock()

	global.Log.Infof("下载任务已恢复: %s -> %s", taskID, result.ID)
	return true
}

// DownloadTaskOptions 自定义下载任务选项（前端新建任务弹窗提交）
type DownloadTaskOptions struct {
	URL           string            `json:"url"`
	FileName      string            `json:"file_name"`
	OutputDir     string            `json:"output_dir"`
	Connections   int               `json:"connections"`
	UserAgent     string            `json:"user_agent"`
	Proxy         string            `json:"proxy"`
	Headers       map[string]string `json:"headers"`
	ChecksumType  string            `json:"checksum_type"`
	ChecksumValue string            `json:"checksum_value"`
	SkipTLSVerify bool              `json:"skip_tls_verify"`
}

// SubmitDownloadWithOptions 提交自定义下载任务（组件存在时可用，选项覆盖设置默认值）
func (a *App) SubmitDownloadWithOptions(opts DownloadTaskOptions) *DownloadSubmitResult {
	result := &DownloadSubmitResult{}

	installed, _, loadErr := loadOShinD()
	if !installed {
		result.Message = "OShinD 组件未安装"
		if loadErr != nil {
			result.Message += "（" + loadErr.Error() + "）"
		}
		return result
	}
	if opts.URL == "" {
		result.Message = "下载地址为空"
		return result
	}

	settings := getSettings()
	// 未填写的选项回退设置默认值
	outputDir := opts.OutputDir
	if outputDir == "" {
		outputDir = settings.DownloadDir
	}
	ua := opts.UserAgent
	if strings.TrimSpace(ua) == "" {
		ua = effectiveDownloadUA()
	}
	proxy := opts.Proxy
	if proxy == "" {
		proxy = settings.DownloadProxy
	}
	connections := opts.Connections
	if connections <= 0 {
		connections = settings.DownloadThreads
	}

	options := map[string]interface{}{
		"output_dir":  outputDir,
		"connections": connections,
	}
	if ua != "" {
		options["headers"] = map[string]string{"User-Agent": ua}
	}
	if len(opts.Headers) > 0 {
		// 自定义 headers 与 UA 合并（显式传入的 UA 优先）
		merged := make(map[string]string, len(opts.Headers)+1)
		for k, v := range opts.Headers {
			merged[k] = v
		}
		if ua != "" {
			merged["User-Agent"] = ua
		}
		options["headers"] = merged
	}
	if proxy != "" {
		options["proxy"] = proxy
	}
	if opts.ChecksumType != "" && opts.ChecksumValue != "" {
		options["checksum_type"] = opts.ChecksumType
		options["checksum_value"] = opts.ChecksumValue
	}
	if opts.SkipTLSVerify {
		options["skip_tls_verify"] = true
	}

	optsJSON, err := json.Marshal(options)
	if err != nil {
		result.Message = "组装下载选项失败"
		return result
	}

	ret, _, callErr := oshindProcDl.Call(strPtr(opts.URL), strPtr(string(optsJSON)))
	if callErr != nil && isRealErr(callErr) {
		global.Log.Errorf("OShinD 提交自定义任务失败: %v", callErr)
		result.Message = "提交下载任务失败"
		return result
	}
	taskID := cStringToGo(ret)
	if taskID == "" {
		result.Message = "提交下载任务失败（组件返回空任务 ID）"
		return result
	}

	downloadMu.Lock()
	downloadTasks = append(downloadTasks, downloadTaskEntry{
		TaskID:   taskID,
		FileName: opts.FileName,
		URL:      opts.URL,
		Created:  time.Now(),
	})
	downloadMu.Unlock()

	result.Success = true
	result.TaskID = taskID
	global.Log.Infof("自定义下载任务已提交: %s (%s)", opts.FileName, taskID)
	return result
}

// oshindTaskStatus 查询单个任务状态 JSON（组件不可用返回空串）
func oshindTaskStatus(taskID string) string {
	if oshindProcStat == nil {
		return ""
	}
	ret, _, err := oshindProcStat.Call(strPtr(taskID))
	if err != nil && isRealErr(err) {
		return ""
	}
	return cStringToGo(ret)
}

// strPtr Go 字符串转 C 兼容指针（NUL 结尾由 Go 字符串字面量保证？否——需显式追加）
func strPtr(s string) uintptr {
	b := append([]byte(s), 0)
	return uintptr(unsafe.Pointer(&b[0]))
}

// isRealErr 区分 syscall 返回的 "operation completed successfully" 噪音错误
func isRealErr(err error) bool {
	return err != nil && err.Error() != "The operation completed successfully."
}
