package service

import (
	"kinh-desktop/global"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// App 前端可调用的服务。
// v3 的 service 不再需要保存 context：窗口、对话框、事件都直接挂在 app 实例上，
// 因此这里持有 *application.App 引用，运行期由 main 通过构造函数注入。
type App struct {
	app *application.App
}

func NewApp(app *application.App) *App {
	appRef = &App{app: app}
	return appRef
}

// ==================== 语言 ====================
// 语言包按「文案表」下发（GetLangTextMap），前端 useI18n 全局持有；
// 切换语言走 SaveSettings（写 settings.json + 重载语言包），因此无需单独的
// SetLanguage / GetCurrentLang / GetLangPack 接口

func (a *App) GetLangTextMap() map[string]string {
	return global.GetLangTextMap()
}

func (a *App) GetALLLang() []global.LanguageInfo {
	return global.GetLangInfoList()
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

// WindowClose 请求关闭窗口。
// 不直接退出，而是走窗口关闭流程交由 close.go 的 hook 按设置分流
// （直接退出 / 隐藏到托盘 / 询问用户），保证前端按钮与系统关闭走同一条路径
func (a *App) WindowClose() {
	a.app.Window.Current().Close()
}

// ==================== 原生对话框 ====================
// v3 的对话框改为链式 Builder：配置后 PromptForSingleSelection() 弹出并返回选中路径

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
