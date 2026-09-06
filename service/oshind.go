package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"kinh-desktop/global"
)

// ==================== OShinD 下载组件（FFI 动态库加载与管理） ====================
//
// 组件形态：data/oshind.dll（OShinD FFI 产物，c-shared 动态库）
// 检测策略：文件存在即尝试加载，加载成功读取版本号；
//           加载失败（损坏/版本不兼容）视为未安装，不影响主程序运行。
// 集成接口：OShinD_Version / OShinD_Download / OShinD_GetTaskStatus 等（见 OShinD docs/ffi-api.md）
//
// 生命周期：进程启动后惰性加载（首次调用 GetOShinDVersion 时）；DLL 卸载有崩溃风险，
// 故加载后常驻内存不主动 FreeLibrary，与主程序同生命周期。

const (
	// oshindRepo OShinD 仓库坐标
	oshindRepo = "OshinTeam/OShinD"
	// oshindNotInstalled 组件未安装时的占位版本号
	oshindNotInstalled = "not_installed"
	// oshindLibName 组件动态库文件名（data 目录下）
	oshindLibName = "oshind.dll"
)

var (
	oshindMu         sync.Mutex
	oshindLib        *syscall.DLL  // 已加载的动态库（nil 为未加载）
	oshindLoadErr    error         // 加载失败原因（区别于未安装）
	oshindLoaded     bool          // 是否已尝试加载（含失败，避免重复尝试）
	oshindVersion    string        // 组件版本（加载成功后从 OShinD_Version 读取）
	oshindProcVer    *syscall.Proc // OShinD_Version 过程句柄
	oshindProcDl     *syscall.Proc // OShinD_Download 过程句柄
	oshindProcStat   *syscall.Proc // OShinD_GetTaskStatus 过程句柄
	oshindProcFree   *syscall.Proc // OShinD_FreeString 过程句柄
	oshindProcCancel *syscall.Proc // OShinD_CancelTask 过程句柄
	oshindProcPause  *syscall.Proc // OShinD_PauseTask 过程句柄
	oshindProcResume *syscall.Proc // OShinD_ResumeTask 过程句柄
	oshindProcRemove *syscall.Proc // OShinD_RemoveTask 过程句柄
)

// oshindLibPath 组件动态库完整路径
func oshindLibPath() string {
	return filepath.Join(baiduDataDir, oshindLibName)
}

// loadOShinD 惰性加载组件动态库（进程内仅尝试一次，失败不重试）
func loadOShinD() (loaded bool, version string, loadErr error) {
	oshindMu.Lock()
	defer oshindMu.Unlock()
	if oshindLoaded {
		return oshindLib != nil, oshindVersion, oshindLoadErr
	}
	oshindLoaded = true

	libPath := oshindLibPath()
	if _, err := os.Stat(libPath); err != nil {
		// 文件不存在属正常未安装场景，不记错误
		return false, oshindNotInstalled, nil
	}

	lib, err := syscall.LoadLibrary(libPath)
	if err != nil {
		oshindLoadErr = fmt.Errorf("加载动态库失败: %w", err)
		global.Log.Warnf("OShinD %v", oshindLoadErr)
		return false, oshindNotInstalled, oshindLoadErr
	}
	oshindLib = &syscall.DLL{Name: libPath, Handle: lib}

	// 逐一解析所需过程，缺失任一核心接口即判定不兼容
	required := map[string]**syscall.Proc{
		"OShinD_Version":       &oshindProcVer,
		"OShinD_Download":      &oshindProcDl,
		"OShinD_GetTaskStatus": &oshindProcStat,
		"OShinD_FreeString":    &oshindProcFree,
		"OShinD_CancelTask":    &oshindProcCancel,
		"OShinD_PauseTask":     &oshindProcPause,
		"OShinD_ResumeTask":    &oshindProcResume,
		"OShinD_RemoveTask":    &oshindProcRemove,
	}
	for name, slot := range required {
		proc, err := oshindLib.FindProc(name)
		if err != nil {
			oshindLoadErr = fmt.Errorf("缺少接口 %s（组件版本不兼容）", name)
			global.Log.Warnf("OShinD %v", oshindLoadErr)
			oshindLib = nil
			return false, oshindNotInstalled, oshindLoadErr
		}
		*slot = proc
	}

	ver, err := callOShinDVersion()
	if err != nil {
		oshindLoadErr = fmt.Errorf("读取版本失败: %w", err)
		global.Log.Warnf("OShinD %v", oshindLoadErr)
		oshindLib = nil
		return false, oshindNotInstalled, oshindLoadErr
	}
	oshindVersion = ver
	global.Log.Infof("OShinD 组件加载成功: %s (%s)", oshindLibName, ver)
	return true, oshindVersion, nil
}

// callOShinDVersion 调用 OShinD_Version（须持有 oshindMu 或组件已加载后调用）
func callOShinDVersion() (string, error) {
	ret, _, err := oshindProcVer.Call()
	if err != nil && isRealErr(err) {
		return "", err
	}
	s := cStringToGo(ret)
	return s, nil
}

// cStringToGo 将 FFI 返回的 char* 转 Go 字符串并释放组件内存
func cStringToGo(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	// 组件侧字符串为 NUL 结尾的 UTF-8。
	// uintptr→unsafe.Pointer 转换遵循 Go 规约：syscall.Call 返回后立即转换，
	// 后续仅基于 *byte 视图操作；vet 的 "possible misuse" 对本模式为已知误报
	//（syscall.Proc.Call 的返回值 uintptr 本就是合法的指针来源场景）。
	p := (*byte)(unsafe.Pointer(ptr))
	view := unsafe.Slice(p, 1)
	length := 0
	for view[length] != 0 {
		length++
		view = unsafe.Slice(p, length+1) // 逐步扩展只读视图
	}
	// string(...) 拷贝语义：必须在 FreeString 前复制内容。
	// 不可用 unsafe.String（零拷贝共享 C 内存），否则释放后字符串指向已回收内存，
	// 存入台账的 taskID 会被组件后续分配改写，导致状态查询/取消全部失效。
	s := string(unsafe.Slice(p, length))
	if oshindProcFree != nil {
		_, _, _ = oshindProcFree.Call(ptr)
	}
	return s
}

// OShinDInfo 组件信息（返回给前端）
type OShinDInfo struct {
	Installed bool   `json:"installed"`            // 是否已安装（文件存在且加载成功）
	Version   string `json:"version"`              // 已安装版本；未安装时为 not_installed
	RepoURL   string `json:"repo_url"`             // 仓库主页
	LoadError string `json:"load_error,omitempty"` // 加载失败原因（存在但损坏/不兼容时非空）
}

// GetOShinDVersion 返回下载组件当前信息
func (a *App) GetOShinDVersion() OShinDInfo {
	info := OShinDInfo{
		Version: oshindNotInstalled,
		// 未安装时引导跳转直接指向 Releases 页（下载组件入口）
		RepoURL: "https://github.com/" + oshindRepo + "/releases",
	}
	installed, version, loadErr := loadOShinD()
	info.Installed = installed
	if installed {
		info.Version = version
	}
	info.LoadError = ""
	if loadErr != nil {
		info.LoadError = loadErr.Error()
	}
	return info
}

// OShinDUpdateResult 检查 OShinD 更新结果
type OShinDUpdateResult struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	CurrentVer string `json:"current_version,omitempty"`
	LatestVer  string `json:"latest_version"`
	HasUpdate  bool   `json:"has_update"`
	Changelog  string `json:"changelog"`
	PageURL    string `json:"page_url"`
}

// CheckOShinDUpdate 检查组件更新：
//   - 已安装：compareVersion 真实对比
//   - 未安装：仅查询最新版本供展示（视为可安装）
func (a *App) CheckOShinDUpdate() OShinDUpdateResult {
	result := OShinDUpdateResult{}

	release, err := fetchLatestRelease(oshindRepo)
	if err != nil {
		result.Message = err.Error()
		global.Log.Warnf("检查 OShinD 更新失败: %v", err)
		return result
	}

	result.Success = true
	result.LatestVer = release.TagName
	result.Changelog = release.Body
	result.PageURL = release.HTMLURL

	installed, version, _ := loadOShinD()
	if installed {
		result.CurrentVer = version
		// GitHub releases/latest 返回的即最新 tag，直接一致性对比：
		// 与当前版本不一致即视为可更新，不做点分比较（规避预发布/非标版本号边界）
		result.HasUpdate = strings.Compare(release.TagName, "v"+version) > 0
		global.Log.Debugf("检查 OShinD 更新: 已安装=%v, 最新=%s, 当前=%s, 可更新=%v", installed, release.TagName, "v"+version, result.HasUpdate)
		if result.HasUpdate {
			result.Message = "发现新版本 " + release.TagName
		} else {
			result.Message = "当前已是最新版本"
		}
	} else {
		result.HasUpdate = true
		result.Message = "最新版本 " + release.TagName
	}
	global.Log.Infof("检查 OShinD 更新完成: 已安装=%v, 最新=%s", installed, release.TagName)
	return result
}

// UpdateOShinD 已移除：组件自更新无闭环价值——未安装时无下载通道，
// 已安装时下载产物无法落位为运行中的 oshind.dll 且不做自动替换（兼容性风险）。
// 更新路径统一为：CheckOShinDUpdate 提示新版本 → 跳 Releases 页 → 浏览器手动下载替换。

// fetchLatestRelease 查询仓库最新 release（update.go 与 oshind.go 共用）
func fetchLatestRelease(repo string) (*githubRelease, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("网络请求失败，请检查网络连接")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 返回异常状态: %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析更新信息失败")
	}
	return &release, nil
}

// oshindDownloadOptions 组装组件下载选项 JSON（UA/线程/目录/代理/分片 等）
// fileName 非空时透传 file_name 选项（同名去重后的最终文件名，组件按此落盘）
func oshindDownloadOptions(url, fileName, ua, outputDir, proxy string, connections, chunkKB int) string {
	opts := map[string]interface{}{
		"url":         url,
		"output_dir":  outputDir,
		"connections": connections,
	}
	if fileName != "" {
		opts["file_name"] = fileName
	}
	if chunkKB > 0 {
		opts["chunk_size"] = int64(chunkKB) * 1024
	}
	if ua != "" {
		opts["headers"] = map[string]string{"User-Agent": ua}
	}
	if proxy != "" {
		opts["proxy"] = proxy
	}
	data, err := json.Marshal(opts)
	if err != nil {
		return ""
	}
	return string(data)
}
