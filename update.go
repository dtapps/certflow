package main

import (
	"context"
	"crypto/rand"
	"math/big"
	"time"

	"cnb.cool/dtapp/certflow/internal/events"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// checkUpdateOnStart 启动后异步检查一次更新，有新版本时发桌面通知。
//
// 周期性检查由 updater.Config.CheckInterval（设置项 update_check_interval，
// 单位小时，0 表示关闭）负责：框架每 tick 调 CheckAndInstall，弹内置更新窗口
// 并自动下载安装，下载完成后由 EventUpdateReady 回调按「更新完自动重启」
// 决定是否立即重启（见 main.go）。
//
// 这里保留启动时的这次单次检查，是因为框架 ticker 要等满一个间隔才首次触发：
// 间隔为 0（关闭）时，只有这次检查会提示用户有新版本。
func checkUpdateOnStart(app *application.App) {
	go func() {
		// 首次检查加 1~3 分钟随机延迟，避免所有客户端同一时刻打更新源
		time.Sleep(randomUpdaterDelay())
		safeCheckUpdate(app)
	}()
}

// safeCheckUpdate 带 panic 兜底的检查：Updater 原生层（macOS Sparkle 等）panic
// 时只记录日志，不拖死主进程。
func safeCheckUpdate(app *application.App) {
	defer func() {
		if r := recover(); r != nil {
			logging.Error("%s: %v", i18n.T("log.updater_check_panic"), r)
		}
	}()
	checkUpdateOnce(app)
}

// checkUpdateOnce 执行一次检查；有新版本时发桌面通知。
func checkUpdateOnce(app *application.App) {
	rel, err := app.Updater.Check(context.Background())
	if err != nil {
		logging.Warn("%s: %v", i18n.T("log.updater_check_failed"), err)
		return
	}
	if rel == nil {
		return // 已是最新版本
	}
	emitNotification(app, events.NotificationPayload{
		Title:    i18n.T("notification.update_available_title"),
		Subtitle: i18n.T("notification.update_available_subtitle", "version", rel.Version),
		Category: "system",
		Level:    "info",
	})
}

// emitNotification 发送桌面通知事件；未送达时只告警，不影响更新主流程。
// 注意 Wails v3 的 Event.Emit 返回值是「事件是否被取消」，true 表示未送达。
func emitNotification(app *application.App, payload events.NotificationPayload) {
	if ok := app.Event.Emit(events.EventNotification, payload); ok {
		logging.Warn("%s", i18n.T("error.notification_failed"))
	}
}

// randomUpdaterDelay 返回 1~3 分钟之间的随机延迟。
func randomUpdaterDelay() time.Duration {
	const minDelay = time.Minute
	const maxDelay = 3 * time.Minute
	n, err := rand.Int(rand.Reader, big.NewInt(int64(maxDelay-minDelay)))
	if err != nil {
		return minDelay
	}
	return minDelay + time.Duration(n.Int64())
}
