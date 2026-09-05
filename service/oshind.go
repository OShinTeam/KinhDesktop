package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"kinh-desktop/global"
)

// ==================== OShinD 下载组件（预留接口） ====================
//
// 下载功能尚未实现，本文件先预留组件版本管理与检查更新接口：
//   - 组件来源：https://github.com/OshinTeam/OShinD（Releases 提供 FFI 动态库）
//   - 未来集成方式：加载对应平台动态库（windows: oshind-windows-amd64.dll）
//   - 当前阶段：组件未安装，GetOShinDVersion 返回默认内容；
//     CheckOShinDUpdate 仅查询最新 release 供展示，不做安装动作

const (
	// oshindRepo OShinD 仓库坐标
	oshindRepo = "OshinTeam/OShinD"
	// oshindNotInstalled 组件未安装时的占位版本号
	oshindNotInstalled = "not_installed"
)

// OShinDInfo 组件信息（返回给前端）
type OShinDInfo struct {
	Installed bool   `json:"installed"` // 是否已安装（当前恒为 false，预留）
	Version   string `json:"version"`   // 已安装版本；未安装时为 not_installed
	RepoURL   string `json:"repo_url"`  // 仓库主页
}

// OShinDUpdateResult 检查 OShinD 更新结果
type OShinDUpdateResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	LatestVer string `json:"latest_version"`
	HasUpdate bool   `json:"has_update"` // 未安装时也标记 true，提示可安装（预留）
	Changelog string `json:"changelog"`
	PageURL   string `json:"page_url"`
}

// GetOShinDVersion 返回下载组件当前信息（未实现下载功能前返回默认内容）
func (a *App) GetOShinDVersion() OShinDInfo {
	return OShinDInfo{
		Installed: false,
		Version:   oshindNotInstalled,
		RepoURL:   "https://github.com/" + oshindRepo,
	}
}

// CheckOShinDUpdate 查询 OShinD 最新 release（仅展示信息）
//
// TODO(下载功能实现时)：接入组件安装/更新流程——
//  1. 已安装版本从组件目录的元数据读取（替换 oshindNotInstalled 占位）
//  2. has_update 改为 compareVersion(最新, 已安装) 真实对比，而非恒 true
//  3. 下载对应平台 asset（如 oshind-windows-amd64.dll）并替换本地动态库
func (a *App) CheckOShinDUpdate() OShinDUpdateResult {
	result := OShinDUpdateResult{}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", oshindRepo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		result.Message = "构造请求失败: " + err.Error()
		global.Log.Warnf("检查 OShinD 更新失败: %v", err)
		return result
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		result.Message = "网络请求失败，请检查网络连接"
		global.Log.Warnf("检查 OShinD 更新请求失败: %v", err)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.Message = fmt.Sprintf("GitHub API 返回异常状态: %d", resp.StatusCode)
		global.Log.Warnf("检查 OShinD 更新失败: %s", result.Message)
		return result
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		result.Message = "解析更新信息失败"
		global.Log.Warnf("解析 OShinD Releases 响应失败: %v", err)
		return result
	}

	result.Success = true
	result.LatestVer = release.TagName
	result.Changelog = release.Body
	result.PageURL = release.HTMLURL
	// 组件未安装，任何最新版本都视为「可安装」；安装逻辑实现后改为 compareVersion 对比
	result.HasUpdate = true
	result.Message = "最新版本 " + release.TagName
	global.Log.Infof("检查 OShinD 更新完成: 最新 %s", release.TagName)
	return result
}
