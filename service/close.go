package service

import (
	"kinh-desktop/global"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// closeActionCancel 用户在前端询问弹窗中选择「取消」，仅作用于本次关闭，不落盘
const closeActionCancel = "cancel"

// closeRequestEvent 询问事件名：关闭行为未设置时，由后端通知前端弹窗
const closeRequestEvent = "close-action-request"

// RegisterCloseHandler 注册窗口关闭拦截。
// v3 的 RegisterHook 先于默认关闭行为执行，调用 event.Cancel() 即可阻止窗口关闭。
//
// 分流依据设置项 close_action：
//   - exit：不拦截，窗口正常关闭，应用随后退出
//   - tray：拦截并隐藏窗口，应用继续驻留托盘
//   - ask（默认）：拦截并通知前端弹窗询问，用户选择经 ResolveCloseAction 回传
func RegisterCloseHandler(app *application.App, window *application.WebviewWindow) {
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		switch getSettings().CloseAction {
		case CloseActionExit:
			// 放行：交由 wails 完成关闭流程
		case CloseActionTray:
			e.Cancel()
			window.Hide()
		default:
			// 未设置（含设置文件中的非法值）：拦截并转交前端询问
			e.Cancel()
			app.Event.Emit(closeRequestEvent)
		}
	})
}

// ResolveCloseAction 前端询问完成后回传用户选择。
// remember 为 true 时把选择写入设置，此后关闭不再询问。
// action 取值：tray（隐藏到托盘）/ exit（退出）/ cancel（取消本次关闭）
func (a *App) ResolveCloseAction(action string, remember bool) {
	// 仅 exit / tray 是合法的落盘值，cancel 只影响本次关闭
	if remember && (action == CloseActionExit || action == CloseActionTray) {
		settings := *getSettings()
		settings.CloseAction = action
		if err := saveSettings(&settings); err != nil {
			global.Log.Warnf("保存关闭行为设置失败: %v", err)
		} else {
			global.Log.Infof("关闭行为已设置为: %s", action)
		}
	}

	switch action {
	case CloseActionTray:
		a.app.Window.Current().Hide()
	case CloseActionExit:
		a.app.Quit()
	default:
		// cancel：用户取消关闭，窗口维持现状
	}
}
