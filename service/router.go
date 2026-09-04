package service

import (
	"context"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"time"

	"kinh-desktop/global"

	"github.com/sirupsen/logrus"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
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

func (a *App) WindowMinimise() {
	runtime.WindowMinimise(a.ctx)
}

func (a *App) WindowToggleMaximise() {
	runtime.WindowToggleMaximise(a.ctx)
}

func (a *App) WindowClose() {
	runtime.Quit(a.ctx)
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

func (a *App) OpenFileSelect() string {
	file, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "所有文件", Pattern: "*.*"},
			{DisplayName: "文本文件", Pattern: "*.txt"},
			{DisplayName: "JSON 文件", Pattern: "*.json"},
		},
	})
	if err != nil {
		global.Log.Warnf("打开文件对话框失败: %v", err)
		return ""
	}
	return file
}

func (a *App) OpenFolderSelect() string {
	folder, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择目录",
	})
	if err != nil {
		global.Log.Warnf("打开目录对话框失败: %v", err)
		return ""
	}
	return folder
}

func (a *App) SaveFileSelect() string {
	file, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title: "保存文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "文本文件", Pattern: "*.txt"},
			{DisplayName: "JSON 文件", Pattern: "*.json"},
		},
	})
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
	runtime.EventsEmit(a.ctx, "notification", map[string]string{
		"title":   title,
		"message": message,
	})
}
