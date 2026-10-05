package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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
	CloseActionAsk  = "ask"  // 未设置时问用户：每次关闭都弹窗确认
	CloseActionExit = "exit" // 直接退出
	CloseActionTray = "tray" // 最小化到系统托盘
)

// AppSettings 应用设置结构，JSON 字段即落盘字段
type AppSettings struct {
	// 程序设置
	Language      string `json:"language"`       // 语言代码，如 zh-CN
	CloseAction   string `json:"close_action"`   // 关闭行为：ask（询问）/ exit（退出）/ tray（最小化到托盘）
	DownloadProxy string `json:"download_proxy"` // 下载代理（留空不启用）。⚠️ 当前未生效：OShinD 引擎不支持代理配置
	// 下载设置
	DownloadUserAgent  string `json:"download_user_agent"`  // 默认 UA
	DownloadThreads    int    `json:"download_threads"`     // 默认线程数
	DownloadChunkKB    int    `json:"download_chunk_kb"`    // 分片大小（KB，0 表示未设置用默认值）
	DownloadDir        string `json:"download_dir"`         // 默认下载目录
	DownloadAccLink    string `json:"download_acc_link"`    // 远程解析加速链接（留空仅本地解析）
	DownloadForceTLS   bool   `json:"download_force_tls"`   // 强制启用 TLS：网盘解析出的非 https 地址升级为 https（不影响手动新建任务）
	DownloadMaxRetries int    `json:"download_max_retries"` // 下载失败自动重试次数（0 表示不重试）
	DownloadMaxActive  int    `json:"download_max_active"`  // 同时下载文件数上限（超出部分排队）
	// ExperimentalMultiLink 实验性下载：对选定渠道并发发起多次地址解析。
	// ⚠️ 百度 API 每次请求都会重新生成下载地址，因此对同一渠道多次请求
	// 即可获得多个不同的地址 —— 这是多地址的来源
	ExperimentalMultiLink bool   `json:"experimental_multi_link"` // 启用实验性下载（默认关）
	ExperimentalChannel   string `json:"experimental_channel"`    // 地址渠道：auto（程序决定）/ remote（加速链接）/ local（本地解析），默认 auto
	ExperimentalCount     int    `json:"experimental_count"`      // 获取地址数量（2~6，默认 4）
	// 其他
	LogLevel string `json:"log_level"` // 日志等级：debug/info/warn/error
}

// DefaultUserAgent 内置默认下载 UA（DefaultSettings / 保存兜底 / 运行时回退三处共用）
const DefaultUserAgent = "netdisk;DL"

// DefaultChunkKB 默认分片大小（KB）
const DefaultChunkKB = 500

// DefaultMaxRetries 下载失败自动重试次数默认值
const DefaultMaxRetries = 3

// DefaultMaxActive 同时下载文件数默认上限
const DefaultMaxActive = 2

// DefaultMultiLinkCount 实验性下载默认获取地址数
const DefaultMultiLinkCount = 4

// normalizeChannel 校验地址渠道取值（auto / remote / local），非法值回落 auto
func normalizeChannel(ch string) string {
	switch ch {
	case "remote", "local":
		return ch
	default:
		return "auto"
	}
}

// DefaultSettings 返回默认设置（线程数、UA、下载目录取常见默认值）
func DefaultSettings() *AppSettings {
	return &AppSettings{
		Language:           "zh-CN",
		CloseAction:        CloseActionAsk,
		DownloadUserAgent:  DefaultUserAgent,
		DownloadThreads:    4,
		DownloadChunkKB:    DefaultChunkKB,
		DownloadDir:        defaultDownloadDir(),
		DownloadMaxRetries: DefaultMaxRetries,
		DownloadMaxActive:  DefaultMaxActive,
		ExperimentalCount:  DefaultMultiLinkCount,
		DownloadForceTLS:   true, // 默认启用：https 优先是更安全的默认选择
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

// androidDownloadDir Android 外部存储的公共下载目录。
// 之所以硬编码：Go 侧拿不到 Android 的 Environment API，主存储固定挂载在
// /storage/emulated/0；多用户或外置 SD 卡场景需另行适配。
const androidDownloadDir = "/storage/emulated/0/Download"

// defaultDownloadDir 默认下载目录：用户下载文件夹，取不到回退到当前目录
func defaultDownloadDir() string {
	// Android 的文件系统布局与桌面不同，没有「用户主目录/Downloads」这一层
	// （os.UserHomeDir 在 Android 上取不到桌面语义的目录），直接用公共下载目录。
	// 写入该路径需要「所有文件访问」权限，见 MainActivity.requestStoragePermission
	if runtime.GOOS == "android" {
		return androidDownloadDir
	}

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

// isValidCloseAction 校验关闭行为取值是否合法（ask / exit / tray）
func isValidCloseAction(action string) bool {
	switch action {
	case CloseActionAsk, CloseActionExit, CloseActionTray:
		return true
	default:
		return false
	}
}

// GetSettings 返回当前设置副本（前端读取入口）
func (a *App) GetSettings() AppSettings {
	return *getSettings()
}

// SaveSettings 保存设置并立即生效，返回错误信息（空串为成功）
func (a *App) SaveSettings(settings AppSettings) string {
	// 校验与规范化
	settings.Language = strings.TrimSpace(settings.Language)
	settings.DownloadThreads = clampInt(settings.DownloadThreads, 1, 64)
	// 分片大小（KB）：OShinD 引擎限制 64KB ~ 1GB，夹取后落库
	settings.DownloadChunkKB = clampInt(settings.DownloadChunkKB, 64, 1024*1024)
	// 失败自动重试次数：0 ~ 10 次
	settings.DownloadMaxRetries = clampInt(settings.DownloadMaxRetries, 0, 10)
	// 同时下载文件数：1 ~ 10 个（0 视为未设置，回落默认值 2）
	if settings.DownloadMaxActive <= 0 {
		settings.DownloadMaxActive = DefaultMaxActive
	}
	settings.DownloadMaxActive = clampInt(settings.DownloadMaxActive, 1, 10)
	if strings.TrimSpace(settings.DownloadDir) == "" {
		settings.DownloadDir = defaultDownloadDir()
	}
	settings.DownloadAccLink = strings.TrimSpace(settings.DownloadAccLink)
	settings.DownloadProxy = strings.TrimSpace(settings.DownloadProxy)
	// 实验性多地址下载：未配置加速链接时「加速渠道」不可用，渠道强制回落
	// auto（程序决定会自动走本地）；渠道取值非法也回落 auto
	if settings.ExperimentalMultiLink && !stringHasValue(settings.DownloadAccLink) {
		if settings.ExperimentalChannel == "remote" {
			settings.ExperimentalChannel = "auto"
		}
	}
	settings.ExperimentalChannel = normalizeChannel(settings.ExperimentalChannel)
	// 获取地址数量：2 ~ 6 个（1 无意义——单地址等价于普通下载；0 视为未设置，回落默认值 4）
	if settings.ExperimentalCount <= 0 {
		settings.ExperimentalCount = DefaultMultiLinkCount
	}
	settings.ExperimentalCount = clampInt(settings.ExperimentalCount, 2, MaxMultiLinkSources)
	// UA 兜底：留空时落默认值，保证设置文件里永远有有效 UA
	if strings.TrimSpace(settings.DownloadUserAgent) == "" {
		settings.DownloadUserAgent = DefaultUserAgent
	}
	settings.LogLevel = normalizeLogLevel(settings.LogLevel)
	// 关闭行为：仅认 ask / exit / tray，非法值回落到「询问」
	if !isValidCloseAction(settings.CloseAction) {
		settings.CloseAction = CloseActionAsk
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
