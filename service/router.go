package service

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"time"

	"kinh-desktop/global"

	"github.com/sirupsen/logrus"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// App 前端可调用的服务。
// v3 的 service 不再需要保存 context：窗口、对话框、事件都直接挂在 app 实例上，
// 因此这里持有 *application.App 引用，运行期由 main 通过构造函数注入。
type App struct {
	app *application.App
}

func NewApp(app *application.App) *App {
	return &App{app: app}
}

func (a *App) GetLangTextMap() map[string]string {
	return global.GetLangTextMap()
}

func (a *App) GetLangPack() *global.LanguagePack {
	langPack, err := global.GetLangPack()
	if err != nil {
		global.Log.Warnf("获取语言包失败: %v", err)
		return nil
	}
	return langPack
}

func (a *App) GetALLLang() []global.LanguageInfo {
	return global.GetLangInfoList()
}

func (a *App) SetLanguage(langCode string) bool {
	global.GlobalConfig.Language = langCode
	global.ClearLangCache()
	global.UpdateCurrentLangPath()
	return true
}

func (a *App) GetCurrentLang() string {
	return global.GlobalConfig.Language
}

func (a *App) GetLogFiles() []string {
	logDir := global.GlobalConfig.LogDir
	entries, err := os.ReadDir(logDir)
	if err != nil {
		global.Log.Warnf("读取日志目录失败: %v", err)
		return []string{}
	}

	var logFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".log") {
			logFiles = append(logFiles, entry.Name())
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(logFiles)))
	return logFiles
}

func (a *App) GetLogFileContent(filename string) string {
	// 安全检查：防止路径遍历
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		global.Log.Warnf("非法的日志文件名: %s", filename)
		return ""
	}

	logPath := filepath.Join(global.GlobalConfig.LogDir, filename)
	data, err := os.ReadFile(logPath)
	if err != nil {
		global.Log.Warnf("读取日志文件失败: %v", err)
		return ""
	}
	return string(data)
}

func (a *App) SetLogLevel(level string) bool {
	switch strings.ToLower(level) {
	case "debug":
		global.SetLogLevel(logrus.DebugLevel)
		global.Log.Debug("日志等级已切换为 Debug")
	case "info":
		global.SetLogLevel(logrus.InfoLevel)
		global.Log.Info("日志等级已切换为 Info")
	case "warn":
		global.SetLogLevel(logrus.WarnLevel)
		global.Log.Warn("日志等级已切换为 Warn")
	case "error":
		global.SetLogLevel(logrus.ErrorLevel)
		global.Log.Error("日志等级已切换为 Error")
	default:
		global.Log.Warnf("未知的日志等级: %s", level)
		return false
	}
	return true
}

func (a *App) GetLogLevel() string {
	if global.Log == nil {
		return "info"
	}
	return strings.ToUpper(global.Log.GetLevel().String())
}

// ==================== 窗口控制 ====================
// v3 用 window 对象的方法替代 v2 的 runtime.WindowXxx(ctx, ...)：
// 多窗口下每个窗口是独立对象，Current() 取当前活动窗口（本项目单窗口）

func (a *App) WindowMinimise() {
	a.app.Window.Current().Minimise()
}

func (a *App) WindowToggleMaximise() {
	a.app.Window.Current().ToggleMaximise()
}

func (a *App) WindowClose() {
	a.app.Quit()
}

func (a *App) GetSystemInfo() SystemInfo {
	hostname, _ := os.Hostname()

	return SystemInfo{
		OS:          goruntime.GOOS,
		Arch:        goruntime.GOARCH,
		NumCPU:      goruntime.NumCPU(),
		Hostname:    hostname,
		GoVer:       goruntime.Version(),
		Time:        time.Now().Format(time.DateTime),
		ProcessName: global.GetProcessName(),
	}
}

func (a *App) GetProcessName() string {
	return global.GetProcessName()
}

// ==================== 原生对话框 ====================
// v3 的对话框改为链式 Builder：配置后 PromptForSingleSelection() 弹出并返回选中路径

func (a *App) OpenFileSelect() string {
	file, err := a.app.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title: "选择文件",
		Filters: []application.FileFilter{
			{DisplayName: "所有文件", Pattern: "*.*"},
			{DisplayName: "文本文件", Pattern: "*.txt"},
			{DisplayName: "JSON 文件", Pattern: "*.json"},
		},
	}).PromptForSingleSelection()
	if err != nil {
		global.Log.Warnf("打开文件对话框失败: %v", err)
		return ""
	}
	return file
}

func (a *App) OpenFolderSelect() string {
	// v3 没有独立的目录选择对话框；用文件对话框并把可选目标切到目录。
	// CanChooseFiles 显式置 false 是声明意图：v3 在两者都为 false 时会兜底成选文件
	folder, err := a.app.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:                "选择目录",
		CanChooseDirectories: true,
		CanChooseFiles:       false,
	}).PromptForSingleSelection()
	if err != nil {
		global.Log.Warnf("打开目录对话框失败: %v", err)
		return ""
	}
	return folder
}

func (a *App) SaveFileSelect() string {
	file, err := a.app.Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
		Title: "保存文件",
		Filters: []application.FileFilter{
			{DisplayName: "文本文件", Pattern: "*.txt"},
			{DisplayName: "JSON 文件", Pattern: "*.json"},
		},
	}).PromptForSingleSelection()
	if err != nil {
		global.Log.Warnf("打开保存对话框失败: %v", err)
		return ""
	}
	return file
}

// allowedReadDir 允许读取的目录白名单（相对路径），防止前端任意路径读取本机文件
var allowedReadDirs = []string{"data", "logs"}

// resolveAllowedPath 校验并解析文件路径：仅允许白名单目录内的相对路径
// 返回空串表示路径非法
func resolveAllowedPath(filename string) string {
	if filename == "" || strings.Contains(filename, "..") || filepath.IsAbs(filename) {
		return ""
	}
	cleaned := filepath.Clean(filename)
	for _, dir := range allowedReadDirs {
		if strings.HasPrefix(cleaned, dir+string(filepath.Separator)) {
			return cleaned
		}
	}
	return ""
}

func (a *App) ReadFileContent(path string) string {
	resolved := resolveAllowedPath(path)
	if resolved == "" {
		global.Log.Warnf("非法的文件读取路径: %s", path)
		return ""
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		global.Log.Warnf("读取文件失败: %v", err)
		return ""
	}
	return string(data)
}

func (a *App) WriteFileContent(path string, content string) bool {
	// 写入仅限日志目录（日志诊断场景），data 目录不允许前端写入
	if !strings.HasPrefix(filepath.Clean(path), "logs"+string(filepath.Separator)) ||
		strings.Contains(path, "..") || filepath.IsAbs(path) {
		global.Log.Warnf("非法的文件写入路径: %s", path)
		return false
	}
	resolved := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(resolved), 0755); err != nil {
		global.Log.Warnf("创建写入目录失败: %v", err)
		return false
	}
	if err := os.WriteFile(resolved, []byte(content), 0644); err != nil {
		global.Log.Warnf("写入文件失败: %v", err)
		return false
	}
	global.Log.Infof("文件写入成功: %s", resolved)
	return true
}

func (a *App) Notify(title string, message string) {
	a.app.Event.Emit("notification", map[string]string{
		"title":   title,
		"message": message,
	})
}
