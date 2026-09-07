package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"kinh-desktop/global"

	"github.com/sirupsen/logrus"
)

// ==================== 应用设置持久化（data/settings.json，明文 JSON） ====================
//
// 与凭证不同，设置不含敏感信息，无需加密：直接 JSON 落盘，可读可手改。
// 结构分组：程序（语言/关闭行为）、下载（UA/线程/目录）、日志等级。
// 启动时加载，保存立即生效（语言、日志等级热切换）。

const settingsFile = "settings.json"

// 关闭行为常量（close_action 字段取值）
const (
	CloseActionExit = "exit" // 直接退出
	CloseActionTray = "tray" // 最小化到托盘（托盘功能预留，暂同退出）
)

// AppSettings 应用设置结构，JSON 字段即落盘字段
type AppSettings struct {
	// 程序设置
	Language      string `json:"language"`       // 语言代码，如 zh-CN
	CloseAction   string `json:"close_action"`   // 点击关闭后的操作：exit / tray（预留）
	DownloadProxy string `json:"download_proxy"` // 下载代理（留空不启用），作用于 OShinD 组件下载与直链请求
	// 下载设置
	DownloadUserAgent  string `json:"download_user_agent"`  // 默认 UA
	DownloadThreads    int    `json:"download_threads"`     // 默认线程数
	DownloadChunkKB    int    `json:"download_chunk_kb"`    // 分片大小（KB，0 表示未设置用默认值）
	DownloadDir        string `json:"download_dir"`         // 默认下载目录
	DownloadAccLink    string `json:"download_acc_link"`    // 远程解析加速链接（留空仅本地解析）
	DownloadMaxRetries int    `json:"download_max_retries"` // 下载失败自动重试次数（0 表示不重试）
	// 其他
	LogLevel string `json:"log_level"` // 日志等级：debug/info/warn/error
}

// DefaultUserAgent 内置默认下载 UA（DefaultSettings / 保存兜底 / 运行时回退三处共用）
const DefaultUserAgent = "netdisk;DL"

// DefaultChunkKB 默认分片大小（KB）
const DefaultChunkKB = 500

// DefaultMaxRetries 下载失败自动重试次数默认值
const DefaultMaxRetries = 3

// DefaultSettings 返回默认设置（线程数、UA、下载目录取常见默认值）
func DefaultSettings() *AppSettings {
	return &AppSettings{
		Language:           "zh-CN",
		CloseAction:        CloseActionExit,
		DownloadUserAgent:  DefaultUserAgent,
		DownloadThreads:    4,
		DownloadChunkKB:    DefaultChunkKB,
		DownloadDir:        defaultDownloadDir(),
		DownloadMaxRetries: DefaultMaxRetries,
		LogLevel:           "info",
	}
}

// effectiveDownloadUA 返回实际生效的下载 UA：设置值为空时回退内置默认值。
// 直链解析与实际下载共用同一 UA（百度直链绑定 UA，解析与下载必须一致）；
// 旧版设置文件可能存在空 UA，不兜底会导致空 UA 请求与组件默认 UA 不一致。
func effectiveDownloadUA() string {
	if ua := strings.TrimSpace(getSettings().DownloadUserAgent); ua != "" {
		return ua
	}
	return DefaultUserAgent
}

// settingsFilePath 设置文件路径（与凭证同目录）
func settingsFilePath() string {
	return filepath.Join(baiduDataDir, settingsFile)
}

// defaultDownloadDir 默认下载目录：用户下载文件夹，取不到回退到当前目录
func defaultDownloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "."
	}
	dir := filepath.Join(home, "Downloads")
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir
	}
	return home
}

var (
	settingsMu  sync.RWMutex
	appSettings *AppSettings
)

// InitSettings 启动时加载设置文件，把语言同步到全局配置。
// 必须在 global.Init() 的 InitLang() 之前调用，否则语言系统会按默认值 zh-CN 固化，
// 导致「修改语言后重启不生效」（设置文件里的语言只在设置页打开时才被读取）。
//
// 日志等级不在本函数应用：启动顺序为 InitLogger（默认 info）→ InitSettings → global.Init
// → ApplyLogSetting，设置加载前日志已就绪，加载过程的问题有输出通道。
func InitSettings() {
	settings := loadSettings()
	appSettings = settings

	global.GlobalConfig.Language = settings.Language
}

// ApplyLogSetting 将设置中的日志等级应用到日志系统（InitLogger 之后调用）
func ApplyLogSetting() {
	if level, err := logrus.ParseLevel(normalizeLogLevel(getSettings().LogLevel)); err == nil {
		global.SetLogLevel(level)
	}
}

// loadSettings 启动时读取设置文件，不存在或损坏时回退默认值
// 启动顺序保证：main 先 InitLogger 再 InitSettings，日志通道已就绪
func loadSettings() *AppSettings {
	data, err := os.ReadFile(settingsFilePath())
	if err != nil {
		global.Log.Info("未找到设置文件，使用默认设置")
		return DefaultSettings()
	}

	settings := DefaultSettings()
	if err := json.Unmarshal(data, settings); err != nil {
		global.Log.Warnf("解析设置文件失败，使用默认设置: %v", err)
		return DefaultSettings()
	}
	return settings
}

// getSettings 获取当前设置（首次调用时从磁盘加载）
func getSettings() *AppSettings {
	settingsMu.RLock()
	if appSettings != nil {
		s := appSettings
		settingsMu.RUnlock()
		return s
	}
	settingsMu.RUnlock()

	settingsMu.Lock()
	defer settingsMu.Unlock()
	if appSettings == nil {
		appSettings = loadSettings()
	}
	return appSettings
}

// saveSettings 保存设置到磁盘（带锁，前端并发调用安全）
func saveSettings(settings *AppSettings) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	if err := os.MkdirAll(baiduDataDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	appSettings = settings
	return os.WriteFile(settingsFilePath(), data, 0644)
}

// normalizeLogLevel 校验日志等级，非法值回退 info
func normalizeLogLevel(level string) string {
	switch strings.ToLower(level) {
	case "debug", "info", "warn", "error":
		return strings.ToLower(level)
	default:
		return "info"
	}
}

// GetSettings 返回当前设置副本（前端读取入口）
func (a *App) GetSettings() AppSettings {
	return *getSettings()
}

// SaveSettings 保存设置并立即生效，返回错误信息（空串为成功）
//
// TODO(托盘功能实现时)：close_action 的 tray 选项当前仅存值未生效——
//  1. main.go 需注册系统托盘（wails v2 无内置托盘，需引入第三方 systray 类库）
//  2. WindowClose 按设置分流：exit 直接退出，tray 隐藏窗口到托盘
//  3. 托盘菜单：显示主窗口 / 退出，并处理二次启动唤起
func (a *App) SaveSettings(settings AppSettings) string {
	// 校验与规范化
	settings.Language = strings.TrimSpace(settings.Language)
	settings.DownloadThreads = clampInt(settings.DownloadThreads, 1, 64)
	// 分片大小（KB）：OShinD 组件限制 64KB ~ 1GB，夹取后落库
	settings.DownloadChunkKB = clampInt(settings.DownloadChunkKB, 64, 1024*1024)
	// 失败自动重试次数：0 ~ 10 次
	settings.DownloadMaxRetries = clampInt(settings.DownloadMaxRetries, 0, 10)
	if strings.TrimSpace(settings.DownloadDir) == "" {
		settings.DownloadDir = defaultDownloadDir()
	}
	settings.DownloadAccLink = strings.TrimSpace(settings.DownloadAccLink)
	settings.DownloadProxy = strings.TrimSpace(settings.DownloadProxy)
	// UA 兜底：留空时落默认值，保证设置文件里永远有有效 UA
	if strings.TrimSpace(settings.DownloadUserAgent) == "" {
		settings.DownloadUserAgent = DefaultUserAgent
	}
	settings.LogLevel = normalizeLogLevel(settings.LogLevel)
	if settings.CloseAction != CloseActionTray {
		settings.CloseAction = CloseActionExit
	}

	// 语言热切换
	if settings.Language != global.GlobalConfig.Language {
		old := global.GlobalConfig.Language
		global.GlobalConfig.Language = settings.Language
		global.ClearLangCache()
		global.UpdateCurrentLangPath()
		global.Log.Infof("语言切换: %s -> %s", old, settings.Language)
	}

	// 日志等级热切换
	if normalizeLogLevel(settings.LogLevel) != normalizeLogLevel(global.Log.GetLevel().String()) {
		level, err := logrus.ParseLevel(settings.LogLevel)
		if err == nil {
			global.SetLogLevel(level)
			global.Log.Infof("日志等级已切换为 %s", strings.ToUpper(settings.LogLevel))
		}
	}

	if err := saveSettings(&settings); err != nil {
		global.Log.Errorf("保存设置失败: %v", err)
		return err.Error()
	}
	global.Log.Info("设置已保存")
	return ""
}

// clampInt 数值夹取
func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
