package main

import (
	"embed"
	"kinh-desktop/global"
	"kinh-desktop/service"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed lang/*
var langFS embed.FS

// 托盘图标：Windows 托盘要求 ICO 格式，直接复用构建产物的应用图标
//
//go:embed build/windows/icon.ico
var trayIcon []byte

// singleInstanceID 单实例锁标识，与 build/config.yml 的 productIdentifier 保持一致
const singleInstanceID = "com.oshinteam.kinhdesktop"

// mainWindow 主窗口引用。托盘唤回与二次启动唤起都要用到，
// 而回调在闭包与包级函数中触发，故提升为包级变量（生命周期与进程一致）
var mainWindow *application.WebviewWindow

func main() {
	global.LangFS = langFS

	// 启动顺序：初始化日志 → 加载设置（语言，依赖日志输出）→ 目录/语言系统 → 按设置应用日志等级
	// 设置加载过程需要日志通道，日志系统必须最先就绪；
	// 语言必须在 InitLang 前就位，否则重启后按默认 zh-CN 显示
	global.InitLogger()
	global.Log.Info("日志系统初始化完成")
	service.InitSettings()
	global.Init()
	service.ApplyLogSetting()

	appName := global.GetProcessName()

	// v3 把「应用创建 / 窗口创建 / 运行」拆成独立阶段：
	// 应用先建好，再注册需要 app 引用的服务，最后创建窗口并运行
	// 窗口尺寸、最小尺寸、无边框、背景色仍沿用 v2 时的取值
	app := application.New(application.Options{
		Name:        appName,
		Description: "KinhWeb 桌面客户端",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		// 单实例：窗口收进托盘后再次启动 exe，不再开新进程，而是唤起已有窗口
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: singleInstanceID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				focusMainWindow()
			},
		},
	})

	// 服务的方法需要操作窗口/对话框/事件，依赖 app 实例，故在应用创建后注册
	app.RegisterService(application.NewService(service.NewApp(app)))

	mainWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  appName,
		Width:  1024,
		Height: 768,
		// 最小窗口尺寸：防止用户缩得过小导致布局挤压不可用
		MinWidth:  800,
		MinHeight: 600,
		// 无边框窗口：标题栏由前端自绘
		Frameless: true,
		// 与 v2 的 RGBA{27,38,54,1} 等价，v3 用 0-255 分量表示不透明
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              "/",
	})

	setupTray(app, appName)

	// 关闭行为按设置分流：直接退出 / 隐藏到托盘 / 询问用户
	service.RegisterCloseHandler(app, mainWindow)

	// 排队调度器常驻后台：递补不能只依赖前端轮询，
	// 否则用户离开下载页后剩余排队任务会永远停住
	service.StartDownloadScheduler()

	if err := app.Run(); err != nil {
		println("Error:", err.Error())
	}
}

// focusMainWindow 把主窗口从隐藏或最小化状态唤回前台。
// 托盘菜单、托盘单击、二次启动三处共用，保证唤起行为一致。
// 二次启动可能在窗口创建前被触发，故做 nil 保护
func focusMainWindow() {
	if mainWindow == nil {
		return
	}
	mainWindow.Restore()
	mainWindow.Show()
	mainWindow.Focus()
}

// setupTray 注册系统托盘，提供「显示主窗口 / 退出」入口。
// 关闭行为选择「最小化到托盘」时窗口只是隐藏（Hide 不等同于关闭），需由此唤回
func setupTray(app *application.App, appName string) {
	textMap := global.GetLangTextMap()
	label := func(key, fallback string) string {
		if v := textMap[key]; v != "" {
			return v
		}
		return fallback
	}

	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip(appName)

	menu := app.NewMenu()
	menu.Add(label("tray_show_window", "显示主窗口")).OnClick(func(*application.Context) {
		focusMainWindow()
	})
	menu.Add(label("tray_exit", "退出")).OnClick(func(*application.Context) {
		app.Quit()
	})
	tray.SetMenu(menu)

	// 单击托盘图标直接唤回主窗口（Windows 下的习惯交互）
	tray.OnClick(focusMainWindow)
}
