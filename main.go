package main

import (
	"embed"
	"kinh-desktop/global"
	"kinh-desktop/service"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
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
	App := service.NewApp()

	err := wails.Run(&options.App{
		Title:  appName,
		Width:  1024,
		Height: 768,
		// 最小窗口尺寸：防止用户缩得过小导致布局挤压不可用
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        App.Startup,
		Frameless:        true,
		Bind: []interface{}{
			App,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
