package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"kinh-desktop/global"
)

// ==================== 版本与检查更新（GitHub Releases） ====================

// AppVersion 应用版本号（前端顶栏与设置页均显示此值）。
//
// 声明为 var 而非 const：发布构建由 CI 通过
//
//	-ldflags "-X kinh-desktop/service.AppVersion=<version>"
//
// 注入 tag 里的版本号，而 -X 只能覆盖变量，无法覆盖常量。
// 本机构建没有注入通道，显示下面的默认值。
var AppVersion = "0.0.0-dev"

// updateCheckRepo GitHub 仓库坐标，Releases 页面为更新源
const updateCheckRepo = "OshinTeam/KinhDesktop"

// githubRelease GitHub Releases API 返回所需字段
type githubRelease struct {
	TagName     string `json:"tag_name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

// UpdateCheckResult 检查更新结果（返回给前端）
type UpdateCheckResult struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	CurrentVer string `json:"current_version"`
	LatestVer  string `json:"latest_version"`
	HasUpdate  bool   `json:"has_update"`
	Changelog  string `json:"changelog"`
	PageURL    string `json:"page_url"`
}

// GetAppVersion 返回当前版本号
func (a *App) GetAppVersion() string {
	return AppVersion
}

// CheckUpdate 请求 GitHub Releases API 获取最新版本并对比
func (a *App) CheckUpdate() UpdateCheckResult {
	result := UpdateCheckResult{CurrentVer: AppVersion}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", updateCheckRepo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		result.Message = "构造请求失败: " + err.Error()
		global.Log.Warnf("检查更新失败: %v", err)
		return result
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		result.Message = "网络请求失败，请检查网络连接"
		global.Log.Warnf("检查更新请求失败: %v", err)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.Message = fmt.Sprintf("GitHub API 返回异常状态: %d", resp.StatusCode)
		global.Log.Warnf("检查更新失败: %s", result.Message)
		return result
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		result.Message = "解析更新信息失败"
		global.Log.Warnf("解析 Releases 响应失败: %v", err)
		return result
	}

	result.Success = true
	result.LatestVer = release.TagName
	result.Changelog = release.Body
	result.PageURL = release.HTMLURL
	// Releases 的 latest 本来就是当前最新发布版，所以只要 tag 与本地版本不一致
	// 就说明不是最新版，无需逐段比较版本号大小。
	// tag 带 "v" 前缀、本地 AppVersion 不带，比对前统一剥掉。
	// ⚠️ 别改回 strings.Compare：那是字典序，会把 "v0.10.0" 判为小于 "v0.9.0"
	result.HasUpdate = strings.TrimPrefix(release.TagName, "v") != AppVersion
	global.Log.Debugf("检查 KinhDesktop 更新: 已安装=%v, 最新=%s, 当前=%s, 可更新=%v", result.HasUpdate, release.TagName, AppVersion, result.HasUpdate)
	if result.HasUpdate {
		result.Message = "发现新版本 " + release.TagName
	} else {
		result.Message = "当前已是最新版本"
	}
	global.Log.Infof("检查更新完成: 当前 %s, 最新 %s, 有更新: %v", AppVersion, release.TagName, result.HasUpdate)
	return result
}
