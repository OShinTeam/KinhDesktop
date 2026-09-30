package service

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kinh-desktop/global"
)

// ==================== 下载任务管理（经 OShinD 引擎执行） ====================
//
// 解析直链后交给 OShinD 引擎提交任务，任务状态由引擎维护，
// 前端轮询 GetDownloadTasks（透传引擎状态 JSON）。
// 代理设置：download_proxy 当前不生效 —— 引擎的 DownloadConfig 没有代理字段，
//           内部 http.Transport 也未接 ProxyFromEnvironment（详见 oshind.go）。
// 同名去重：提交前检查磁盘（成品/.tmp/.oshin）与活动任务，重名自动重命名 name(n).ext；
//           文件名未知时若存在同 URL 活动任务则显性报错，避免多任务同时写同一文件。
//           ⚠️ 去重得到的文件名传不进引擎（引擎按 URL 推导、probe 后用 Content-Disposition 覆盖），
//           目前仅用于台账展示与产物删除定位。

type DownloadSubmitResult struct {
	Success bool   `json:"success"`
	TaskID  string `json:"task_id"`
	Message string `json:"message,omitempty"`
	// Queued 任务因并发上限进入排队（TaskID 是 queued-<seq> 占位符，
	// 引擎侧真实 taskID 在调度器递补时才生成）
	Queued bool `json:"queued,omitempty"`
}

// downloadTaskStore 本地任务台账（task_id → 元数据），组件侧保存完整状态
type downloadTaskEntry struct {
	TaskID   string    `json:"task_id"`
	Seq      int64     `json:"seq"` // 程序内部自增编号（跨 resume 稳定，组件侧 task_id 会换新）
	FileName string    `json:"file_name"`
	URL      string    `json:"url"`
	Created  time.Time `json:"created_at"`
	// Queued 任务因「同时下载文件数」达到上限而排队：尚未提交给引擎（TaskID 为空），
	// 由调度器在有空位时按入队顺序自动提交
	Queued bool `json:"queued"`
	// LastError 排队任务递补提交失败的原因（仅此类失败有值，走前端自动重试）
	LastError string `json:"last_error,omitempty"`
	// Sources 多源地址（实验性多地址下载）：排队任务递补时按原样重建引擎 config
	Sources []string `json:"sources,omitempty"`
}

var (
	downloadMu    sync.Mutex
	downloadTasks []downloadTaskEntry // 提交顺序保留（新任务追加尾部）
	downloadSeq   int64               // 任务序号计数器（downloadMu 保护）

	// downloadSubmitMu 提交串行锁：查重（磁盘+台账）与台账追加需原子完成，
	// 避免并发提交同名文件时双双通过检查
	downloadSubmitMu sync.Mutex

	// downloadListRequested 是否已记录过「下载列表首次被请求」的日志（只打一次）
	downloadListRequested bool
)

// nextDownloadSeq 分配任务序号（须持有 downloadMu）
func nextDownloadSeq() int64 {
	downloadSeq++
	return downloadSeq
}

// activeStatuses 组件侧活动状态（占用输出文件，提交同名任务时需避让）
var activeStatuses = map[string]bool{
	"PENDING": true, "PROBING": true, "DOWNLOADING": true,
	"RESUMING": true, "VERIFYING": true, "PAUSED": true,
}

// slotOccupyingStatuses 占用并发名额的状态。与 activeStatuses 分开：
// PAUSED 暂停任务不耗带宽却占着文件，文件避让要算它，但并发名额应该让出来 ——
// 否则两个任务都暂停后队列永久饥饿。状态未知时保守计入（引擎不可用不抢名额）
var slotOccupyingStatuses = map[string]bool{
	"PENDING": true, "PROBING": true, "DOWNLOADING": true,
	"RESUMING": true, "VERIFYING": true,
}

// taskStatusOf 引擎侧任务状态（空串表示引擎不可用/任务丢失）
func taskStatusOf(taskID string) string {
	status := oshindTaskStatus(taskID)
	if status == "" {
		return ""
	}
	var s struct {
		Status string `json:"status"`
	}
	if json.Unmarshal([]byte(status), &s) != nil {
		return ""
	}
	return s.Status
}

// downloadTaskActive 判断任务是否处于占用输出文件的活动状态（组件不可用/状态未知时保守视为活动）
func downloadTaskActive(taskID string) bool {
	status := taskStatusOf(taskID)
	if status == "" {
		return true
	}
	return activeStatuses[status]
}

// fileNameTaken 指定文件名在输出目录是否被占用：
// 磁盘已有成品 / .tmp / .oshin 任一，或活动任务台账同名（Windows 不区分大小写）
func fileNameTaken(outputDir, name string, activeNames map[string]bool) bool {
	base := filepath.Join(outputDir, name)
	for _, p := range []string{base, base + ".tmp", base + ".oshin"} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return activeNames[strings.ToLower(name)]
}

// dedupeFileName 冲突时自动重命名为 name(1).ext、name(2).ext … 直至可用
func dedupeFileName(outputDir, name string, activeNames map[string]bool) string {
	if !fileNameTaken(outputDir, name, activeNames) {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s(%d)%s", base, i, ext)
		if !fileNameTaken(outputDir, candidate, activeNames) {
			return candidate
		}
	}
}

// maxActiveDownloads 同时下载文件数上限（设置 download_max_active，已规范化 ≥1）
func maxActiveDownloads() int {
	return getSettings().DownloadMaxActive
}

// slotOccupyingCount 占用并发名额的任务数（PAUSED 不算 —— 暂停就是为了让出带宽；
// 引擎不可用/状态未知时保守计入，避免引擎故障期间无限放行新任务）
func slotOccupyingCount() int {
	n := 0
	downloadMu.Lock()
	entries := append([]downloadTaskEntry(nil), downloadTasks...)
	downloadMu.Unlock()
	for _, e := range entries {
		if e.Queued {
			continue
		}
		status := taskStatusOf(e.TaskID)
		if status == "" || slotOccupyingStatuses[status] {
			n++
		}
	}
	return n
}

// enqueueDownload 把任务加入排队台账（须持有 downloadSubmitMu）
func enqueueDownload(fileName, url string) *downloadTaskEntry {
	return enqueueDownloadWithOptions(DownloadTaskOptions{URL: url, FileName: fileName})
}

// enqueueDownloadWithOptions 带完整选项入队（多源地址等需在递补时重建）
func enqueueDownloadWithOptions(opts DownloadTaskOptions) *downloadTaskEntry {
	downloadMu.Lock()
	defer downloadMu.Unlock()
	entry := downloadTaskEntry{
		Seq:      nextDownloadSeq(),
		FileName: opts.FileName,
		URL:      opts.URL,
		Created:  time.Now(),
		Queued:   true,
		Sources:  opts.MultiSources,
	}
	downloadTasks = append(downloadTasks, entry)
	return &downloadTasks[len(downloadTasks)-1]
}

// resolveUniqueDownloadName 提交前文件名去重（须持有 downloadSubmitMu）：
//   - 文件名已知：与磁盘文件或活动任务重名时自动重命名 name(1).ext
//   - 文件名未知（组件 probe 后才知晓）：存在同 URL 活动任务时显性报错，拒绝创建
//
// 返回最终文件名（未知时为空串）与冲突提示（无冲突为空）。
// 排队中的任务不占用输出文件，不参与「活动任务」查重。
func resolveUniqueDownloadName(outputDir, name, rawURL string) (finalName string, conflict string) {
	downloadMu.Lock()
	defer downloadMu.Unlock()

	if strings.TrimSpace(name) == "" {
		for _, entry := range downloadTasks {
			if !entry.Queued && entry.URL == rawURL && downloadTaskActive(entry.TaskID) {
				return "", "已存在相同文件的下载任务（文件名待识别，无法自动重命名），请等待其完成或先移除原任务"
			}
		}
		return "", ""
	}

	taken := make(map[string]bool)
	for _, entry := range downloadTasks {
		if !entry.Queued && entry.FileName != "" && downloadTaskActive(entry.TaskID) {
			taken[strings.ToLower(entry.FileName)] = true
		}
	}
	return dedupeFileName(outputDir, name, taken), ""
}

// DispatchQueuedDownloads 调度器：按入队顺序把排队任务递补到引擎，直到填满并发名额。
// 调用时机：新任务入队后、以及每次轮询发现某任务离开活动态时（完成/失败/被移除）。
// 成功递补的条目会换成引擎返回的真实 taskID。
func DispatchQueuedDownloads() {
	for {
		downloadSubmitMu.Lock()

		if slotOccupyingCount() >= maxActiveDownloads() {
			downloadSubmitMu.Unlock()
			return
		}

		// 取最早入队的排队任务（快照 Seq 定位条目——切片可能在提交期间被增删，
		// 绝不能跨锁复用切片下标）
		downloadMu.Lock()
		var entry *downloadTaskEntry
		for i := range downloadTasks {
			if downloadTasks[i].Queued {
				entry = &downloadTasks[i]
				break
			}
		}
		var snap downloadTaskEntry
		if entry != nil {
			snap = *entry
		}
		downloadMu.Unlock()

		if entry == nil {
			// 没有排队任务了
			downloadSubmitMu.Unlock()
			return
		}

		config := buildDownloadConfig(
			getSettings().DownloadDir, effectiveDownloadUA(),
			getSettings().DownloadThreads, getSettings().DownloadChunkKB)
		// 排队时保存的多源地址原样带回（多地址下载任务递补不降级为单源）
		if len(snap.Sources) > 0 {
			config.MultiSources = snap.Sources
		}

		taskID, err := downloadEngine().SubmitDownload(snap.URL, config, nil)

		// 提交返回后按 Seq 重新定位条目（排队期间它可能已被移除/发生变化）
		downloadMu.Lock()
		found := -1
		for i := range downloadTasks {
			if downloadTasks[i].Seq == snap.Seq {
				found = i
				break
			}
		}

		if err != nil && taskID == "" {
			// 提交失败（协议不支持等）：落为 FAILED 态并记录错误，交给前端自动重试
			// 机制处理 —— 不保留排队态（会每轮轮询白提交一次），也不能悬空
			// （既非排队又无引擎状态，界面无从展示、无从操作）
			if found >= 0 && downloadTasks[found].Queued {
				downloadTasks[found].Queued = false
				downloadTasks[found].TaskID = fmt.Sprintf("failed-%d", snap.Seq)
				downloadTasks[found].LastError = err.Error()
			}
			downloadMu.Unlock()
			global.Log.Errorf("排队任务提交失败: %s (%s): %v", snap.FileName, snap.URL, err)
			downloadSubmitMu.Unlock()
			continue
		}

		if found >= 0 && downloadTasks[found].Queued {
			// 提交成功：台账条目从排队态转为引擎态
			downloadTasks[found].TaskID = taskID
			downloadTasks[found].Queued = false
			downloadMu.Unlock()
			downloadSubmitMu.Unlock()
			global.Log.Infof("排队任务已开始下载: %s (%s)", snap.FileName, taskID)
		} else {
			// 条目在排队期间被移除：撤回引擎侧任务，防孤儿下载
			downloadMu.Unlock()
			downloadSubmitMu.Unlock()
			downloadEngine().CancelTask(taskID)
			downloadEngine().RemoveTask(taskID)
		}
	}
}

// SubmitDownload 解析并提交下载任务（组件存在时可用）
// fsID >= 0 表示网盘文件（先解析直链），url 非空表示自定义任务直接下载
func (a *App) SubmitDownload(url, fileName string, fsID int64) *DownloadSubmitResult {
	result := &DownloadSubmitResult{}

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

	// 同名文件去重：磁盘/活动任务占用时自动重命名，文件名未知且同 URL 活动任务存在时显性报错
	downloadSubmitMu.Lock()
	finalName, conflict := resolveUniqueDownloadName(settings.DownloadDir, fileName, url)
	if conflict != "" {
		downloadSubmitMu.Unlock()
		result.Message = conflict
		return result
	}
	fileName = finalName

	config := buildDownloadConfig(settings.DownloadDir, effectiveDownloadUA(), settings.DownloadThreads, settings.DownloadChunkKB)

	// 并发上限：占用名额的任务已满时转入排队（不占引擎名额），由调度器递补
	if slotOccupyingCount() >= maxActiveDownloads() {
		entry := enqueueDownload(fileName, url)
		downloadSubmitMu.Unlock()
		result.Success = true
		result.Queued = true
		result.TaskID = fmt.Sprintf("queued-%d", entry.Seq)
		global.Log.Infof("并发已满(%d)，任务排队: %s (seq=%d)", maxActiveDownloads(), fileName, entry.Seq)
		return result
	}

	// fileName 无法透传给引擎：types.DownloadConfig 没有文件名字段，Engine.SubmitDownload
	// 一律按 URL 推导、probe 完成后再用 Content-Disposition 覆盖。因此上面经过
	// resolveUniqueDownloadName 去重得到的文件名，实际只用于台账记录与产物删除定位。
	taskID, err := downloadEngine().SubmitDownload(url, config, nil)
	if err != nil && taskID == "" {
		// 提交阶段未产生任务（协议不支持等），无状态可查询
		downloadSubmitMu.Unlock()
		global.Log.Errorf("提交下载任务失败: %v", err)
		result.Message = "提交下载任务失败: " + err.Error()
		return result
	}

	downloadMu.Lock()
	downloadTasks = append(downloadTasks, downloadTaskEntry{
		TaskID:   taskID,
		Seq:      nextDownloadSeq(),
		FileName: fileName,
		URL:      url,
		Created:  time.Now(),
	})
	downloadMu.Unlock()
	downloadSubmitMu.Unlock()

	result.Success = true
	result.TaskID = taskID
	global.Log.Infof("下载任务已提交: %s (%s)", fileName, taskID)
	return result
}

// GetDownloadTasks 返回任务列表（台账元数据 + 引擎实时状态合并）
func (a *App) GetDownloadTasks() []map[string]interface{} {
	downloadMu.Lock()
	tasks := append([]downloadTaskEntry(nil), downloadTasks...)
	downloadMu.Unlock()

	// 首次被请求时留一行日志：用于区分「前端没在轮询」与「轮询了但拿不到数据」，
	// 之后静默以免每秒刷屏
	if !downloadListRequested {
		downloadListRequested = true
		global.Log.Infof("下载列表首次被请求，当前台账 %d 条", len(tasks))
	}

	maxRetries := getSettings().DownloadMaxRetries
	list := make([]map[string]interface{}, 0, len(tasks))
	for idx, entry := range tasks {
		item := map[string]interface{}{
			"task_id":     entry.TaskID,
			"seq":         entry.Seq,
			"file_name":   entry.FileName,
			"url":         entry.URL,
			"created_at":  entry.Created.Format(time.RFC3339),
			"max_retries": maxRetries,
			"queued":      entry.Queued,
		}
		if entry.Queued {
			// 排队任务：无引擎状态，补一个固定状态字段供前端展示
			item["status"] = "QUEUED"
			list = append(list, item)
			continue
		}
		// 递补提交失败的任务（failed-<seq> 占位）：引擎侧无状态，落 FAILED + 原因，
		// 前端自动重试（ResumeDownloadTask）会在有空位后把它送回引擎
		if entry.LastError != "" {
			item["status"] = "FAILED"
			item["error"] = entry.LastError
			list = append(list, item)
			continue
		}
		// 透传组件状态：失败/组件侧任务丢失时保留台账元数据
		if statusJSON := oshindTaskStatus(entry.TaskID); statusJSON != "" {
			var status map[string]interface{}
			if json.Unmarshal([]byte(statusJSON), &status) == nil {
				// active_threads 为组件实时活跃线程数（非提交上限）
				for _, k := range []string{"status", "progress", "speed", "downloaded", "total", "error", "file_name", "active_threads"} {
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

	// 有任务离开活动态（完成/失败）且存在排队任务时，递补下一个。
	// 放在列表读取路径上避免引入额外 goroutine/定时器；slotOccupyingCount 的
	// 判断很轻（缓存台账 + 状态查引擎），空转开销可忽略
	if len(downloadTasks) > 0 {
		hasQueued := false
		downloadMu.Lock()
		for _, e := range downloadTasks {
			if e.Queued {
				hasQueued = true
				break
			}
		}
		downloadMu.Unlock()
		if hasQueued {
			go DispatchQueuedDownloads()
		}
	}
	return list
}

// CancelDownloadTask 取消任务（保留已下载内容）。
// 排队任务（queued-<seq> 占位 ID）引擎侧无句柄，前端不对其发 Cancel，
// 移除走 RemoveDownloadTask
func (a *App) CancelDownloadTask(taskID string) bool {
	if err := downloadEngine().CancelTask(taskID); err != nil {
		global.Log.Warnf("取消下载任务失败: %v", err)
		return false
	}
	return true
}

// PauseDownloadTask 暂停任务（引擎保存断点状态，可恢复）
func (a *App) PauseDownloadTask(taskID string) bool {
	if err := downloadEngine().PauseTask(taskID); err != nil {
		global.Log.Warnf("暂停下载任务失败: %v", err)
		return false
	}
	return true
}

// ResumeDownloadTask 恢复暂停/失败的任务
// 组件侧移除旧任务并重新提交（自动检测 .oshin 断点状态），返回新任务 ID，
// 台账条目需同步替换 ID，否则后续轮询查不到状态。
// 并发已满时拒绝恢复（false）—— 前端自动重试会在下一轮轮询再试，防止击穿上限
func (a *App) ResumeDownloadTask(taskID string) bool {
	if slotOccupyingCount() >= maxActiveDownloads() {
		global.Log.Infof("并发已满(%d)，暂缓恢复任务: %s", maxActiveDownloads(), taskID)
		return false
	}

	newID, err := downloadEngine().ResumeTask(taskID, nil)
	if err != nil {
		global.Log.Warnf("恢复下载任务失败: %v", err)
		return false
	}

	// 台账换 ID（保持原位置与创建时间、文件名元数据），并清掉递补失败留下的旧错误
	downloadMu.Lock()
	for i := range downloadTasks {
		if downloadTasks[i].TaskID == taskID {
			downloadTasks[i].TaskID = newID
			downloadTasks[i].LastError = ""
			break
		}
	}
	downloadMu.Unlock()

	global.Log.Infof("下载任务已恢复: %s -> %s", taskID, newID)
	return true
}

// RemoveDownloadTask 移除任务并删除台账条目
// deleteFiles 为 true 时同时删除下载产物（成品文件 + .tmp 临时文件 + .oshin 断点状态）。
// 无论是否删除文件，都先取消并移除组件侧任务（CancelTask + RemoveTask 双重保证），
// 防止后台继续空跑下载。
func (a *App) RemoveDownloadTask(taskID string, deleteFiles bool) map[string]interface{} {
	result := map[string]interface{}{"success": false, "task_removed": false, "files_deleted": false}

	// 读取台账元数据（文件名/URL 用于删除产物），随后从台账移除
	downloadMu.Lock()
	var entry *downloadTaskEntry
	for i := range downloadTasks {
		if downloadTasks[i].TaskID == taskID {
			entry = &downloadTasks[i]
			downloadTasks = append(downloadTasks[:i], downloadTasks[i+1:]...)
			break
		}
	}
	downloadMu.Unlock()
	if entry == nil {
		result["message"] = "任务不存在"
		return result
	}
	result["task_removed"] = true

	// 排队任务未提交给引擎，台账移除即完成；有排队任务时顺便触发一次调度
	if entry.Queued {
		global.Log.Infof("排队任务已移除: %s (seq=%d)", entry.FileName, entry.Seq)
		go DispatchQueuedDownloads()
		if deleteFiles {
			// 排队任务从未落盘，无产物可删
			result["files_deleted"] = false
		}
		result["success"] = true
		return result
	}

	// 引擎侧取消 + 移除（RemoveTask 内部亦会 cancel，此处 CancelTask 先行确保运行中任务停止）。
	// ⚠️ 删除产物前必须先从引擎取实际输出路径（RemoveTask 会清理 task 引用，之后拿不到）：
	// 台账里的文件名（用户指定 / 去重重命名）传不进引擎，引擎按 URL 推导 +
	// Content-Disposition 覆盖自行写盘，实际落盘名可能与台账名完全不同 ——
	// 用台账名拼路径会删到不存在的文件，这正是「勾选删除但 tmp/oshin 还在」的根因
	engine := downloadEngine()
	engineBase := "" // 引擎侧实际输出路径（不含 .tmp/.oshin 后缀）
	if task, ok := engine.GetTask(taskID); ok {
		switch {
		case task.OutputPath != "":
			engineBase = task.OutputPath
		case task.FileName != "" && task.Config != nil && task.Config.OutputDir != "":
			engineBase = filepath.Join(task.Config.OutputDir, task.FileName)
		}
	}
	cancelled := engine.CancelTask(taskID) == nil
	if engine.RemoveTask(taskID) {
		cancelled = true
	}
	global.Log.Infof("下载任务已移除: %s (%s) 引擎侧停止=%v", entry.FileName, taskID, cancelled)

	// 删除下载产物（成品 / .tmp / .oshin）。
	// 候选 = 引擎实际路径三件套 + 台账名三件套（都尝试，幂等）。
	// Windows 上引擎取消后写 goroutine 释放文件句柄有延迟，失败做短暂重试
	if deleteFiles {
		candidates := make(map[string]bool)
		for _, base := range []string{engineBase} {
			if base != "" {
				candidates[base] = true
			}
		}
		if entry.URL != "" {
			name := entry.FileName
			if name == "" {
				name = fileNameFromURL(entry.URL)
			}
			if name != "" {
				legacyBase := filepath.Join(downloadDirForTask(entry), name)
				candidates[legacyBase] = true
			}
		}

		deleted := false
		var lastErr error
		for base := range candidates {
			for _, p := range []string{base, base + ".tmp", base + ".oshin"} {
				for attempt := 0; attempt < 3; attempt++ {
					if err := os.Remove(p); err == nil {
						deleted = true
						break
					} else if attempt == 2 {
						lastErr = err
					} else if _, statErr := os.Stat(p); statErr != nil {
						break // 文件本就不存在，无需重试
					} else {
						time.Sleep(150 * time.Millisecond) // 句柄未及时释放，稍后重试
					}
				}
			}
		}
		if !deleted && lastErr != nil {
			global.Log.Warnf("删除下载产物失败（已尝试 %d 个候选路径）: %v", len(candidates), lastErr)
		}
		result["files_deleted"] = deleted
	}

	result["success"] = true
	return result
}

// downloadDirForTask 任务下载目录（当前统一取设置下载目录；台账未存每任务目录）
func downloadDirForTask(entry *downloadTaskEntry) string {
	return getSettings().DownloadDir
}

// fileNameFromURL 从 URL 提取文件名（与组件 ExtractFileInfo 行为一致的轻量版）
func fileNameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(u.Path, "/")
	name := parts[len(parts)-1]
	if decoded, err := url.QueryUnescape(name); err == nil && decoded != "" {
		name = decoded
	}
	return name
}

// DownloadTaskOptions 自定义下载任务选项（前端新建任务弹窗提交）
type DownloadTaskOptions struct {
	URL           string            `json:"url"`
	FileName      string            `json:"file_name"`
	OutputDir     string            `json:"output_dir"`
	Connections   int               `json:"connections"`
	ChunkKB       int               `json:"chunk_kb"`
	UserAgent     string            `json:"user_agent"`
	Proxy         string            `json:"proxy"`
	Headers       map[string]string `json:"headers"`
	ChecksumType  string            `json:"checksum_type"`
	ChecksumValue string            `json:"checksum_value"`
	SkipTLSVerify bool              `json:"skip_tls_verify"`
	// MultiSources 多源地址（实验性多地址下载）：引擎会从这些地址与主 URL
	// 加权轮询拉取同一文件；nil/空表示单源
	MultiSources []string `json:"multi_sources,omitempty"`
}

// SubmitDownloadWithOptions 提交自定义下载任务（组件存在时可用，选项覆盖设置默认值）
func (a *App) SubmitDownloadWithOptions(opts DownloadTaskOptions) *DownloadSubmitResult {
	result := &DownloadSubmitResult{}

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
	connections := opts.Connections
	if connections <= 0 {
		connections = settings.DownloadThreads
	}
	// 分片大小：前端高级选项未指定（0）时回退设置默认值
	chunkKB := opts.ChunkKB
	if chunkKB <= 0 {
		chunkKB = settings.DownloadChunkKB
	}

	// 同名文件去重：磁盘/活动任务占用时自动重命名，文件名未知且同 URL 活动任务存在时显性报错
	downloadSubmitMu.Lock()
	finalName, conflict := resolveUniqueDownloadName(outputDir, opts.FileName, opts.URL)
	if conflict != "" {
		downloadSubmitMu.Unlock()
		result.Message = conflict
		return result
	}
	opts.FileName = finalName

	config := buildDownloadConfig(outputDir, ua, connections, chunkKB)
	if len(opts.Headers) > 0 {
		// 自定义 headers 与 UA 合并（显式传入的 UA 优先）
		merged := make(map[string]string, len(opts.Headers)+1)
		for k, v := range opts.Headers {
			merged[k] = v
		}
		if ua != "" {
			merged["User-Agent"] = ua
		}
		config.Headers = merged
	}
	if opts.ChecksumType != "" && opts.ChecksumValue != "" {
		config.ChecksumType = opts.ChecksumType
		config.ChecksumValue = opts.ChecksumValue
	}
	if opts.SkipTLSVerify && config.TLSConfig != nil {
		config.TLSConfig.InsecureSkipVerify = true
	}
	// 多源地址（实验性多地址下载）：引擎会从主 URL + 这些地址加权拉取同一文件
	if len(opts.MultiSources) > 0 {
		config.MultiSources = opts.MultiSources
	}

	// 并发上限：占用名额的任务已满时转入排队（不占引擎名额），由调度器递补。
	// 排队任务入队时保存多源信息，递补时按原样重建 config
	if slotOccupyingCount() >= maxActiveDownloads() {
		entry := enqueueDownloadWithOptions(opts)
		downloadSubmitMu.Unlock()
		result.Success = true
		result.Queued = true
		result.TaskID = fmt.Sprintf("queued-%d", entry.Seq)
		global.Log.Infof("并发已满(%d)，任务排队: %s (seq=%d)", maxActiveDownloads(), opts.FileName, entry.Seq)
		return result
	}

	// opts.FileName 同样无法透传给引擎，原因见 SubmitDownload 中的说明
	taskID, err := downloadEngine().SubmitDownload(opts.URL, config, nil)
	if err != nil && taskID == "" {
		downloadSubmitMu.Unlock()
		global.Log.Errorf("提交自定义下载任务失败: %v", err)
		result.Message = "提交下载任务失败: " + err.Error()
		return result
	}

	downloadMu.Lock()
	downloadTasks = append(downloadTasks, downloadTaskEntry{
		TaskID:   taskID,
		Seq:      nextDownloadSeq(),
		FileName: opts.FileName,
		URL:      opts.URL,
		Created:  time.Now(),
		Sources:  opts.MultiSources,
	})
	downloadMu.Unlock()
	downloadSubmitMu.Unlock()

	result.Success = true
	result.TaskID = taskID
	global.Log.Infof("自定义下载任务已提交: %s (%s)", opts.FileName, taskID)
	return result
}
