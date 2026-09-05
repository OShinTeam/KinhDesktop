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

// AppVersion 应用版本号，发布时更新（前端顶栏与设置页均显示此值）
const AppVersion = "v0.1.0"

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

// normalizeVersion 统一版本号格式便于比较（去 v 前缀，转小写）
func normalizeVersion(ver string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ver), "v")))
}

// compareVersion 比较点分版本号：latest > current 返回 1，相等 0，小于 -1
// 非数字段按字符串比较，段数不足视为 0
func compareVersion(latest, current string) int {
	ls := strings.Split(latest, ".")
	cs := strings.Split(current, ".")
	n := len(ls)
	if len(cs) > n {
		n = len(cs)
	}
	for i := 0; i < n; i++ {
		lv, cv := "0", "0"
		if i < len(ls) {
			lv = ls[i]
		}
		if i < len(cs) {
			cv = cs[i]
		}
		var ln, cn int
		if _, err := fmt.Sscanf(lv, "%d", &ln); err != nil {
			ln = -1
		}
		if _, err := fmt.Sscanf(cv, "%d", &cn); err != nil {
			cn = -1
		}
		if ln != cn {
			if ln > cn {
				return 1
			}
			return -1
		}
	}
	return 0
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
	result.HasUpdate = compareVersion(normalizeVersion(release.TagName), normalizeVersion(AppVersion)) > 0
	if result.HasUpdate {
		result.Message = "发现新版本 " + release.TagName
	} else {
		result.Message = "当前已是最新版本"
	}
	global.Log.Infof("检查更新完成: 当前 %s, 最新 %s, 有更新: %v", AppVersion, release.TagName, result.HasUpdate)
	return result
}
