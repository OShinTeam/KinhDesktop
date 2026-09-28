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
	})

	// 服务的方法需要操作窗口/对话框/事件，依赖 app 实例，故在应用创建后注册
	app.RegisterService(application.NewService(service.NewApp(app)))

	app.Window.NewWithOptions(application.WebviewWindowOptions{
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

	if err := app.Run(); err != nil {
		println("Error:", err.Error())
	}
}
