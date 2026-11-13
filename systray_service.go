package main

import (
	"context"
	_ "embed"

	"cnb.cool/dtapp/certflow/internal/events"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/settings"
	"github.com/wailsapp/wails/v3/pkg/application"
	wailsEvents "github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

//go:embed build/trayicon.png
var systrayIcon []byte

// SysTrayService 系统托盘服务
type SysTrayService struct {
	app        *application.App
	mainWindow application.Window
	systemTray *application.SystemTray
	// settings 供托盘「预发布版更新」开关读写配置
	settings *settings.Service
	// autostart 供托盘「开机自启」开关读写系统自启状态
	autostart *AutostartServiceWrapper
}

// NewSysTrayService 创建系统托盘服务
// https://v3.wails.io/features/menus/systray/
func NewSysTrayService() *SysTrayService {
	return &SysTrayService{}
}

// SetApp 设置 app 引用（用于获取应用生命周期）
func (s *SysTrayService) SetApp(app *application.App) {
	s.app = app
}

// setMainWindow 设置主窗口引用（内部初始化用，不暴露给前端 RPC）
func (s *SysTrayService) setMainWindow(window application.Window) {
	s.mainWindow = window
}

// setSettingsService 注入设置服务（内部初始化用），供托盘「预发布版更新」开关读写配置。
func (s *SysTrayService) setSettingsService(svc *settings.Service) {
	s.settings = svc
}

// setAutostartService 注入开机自启服务（内部初始化用），供托盘「开机自启」开关使用。
func (s *SysTrayService) setAutostartService(svc *AutostartServiceWrapper) {
	s.autostart = svc
}

// ShowWindow 显示窗口
func (s *SysTrayService) ShowWindow() {
	if s.mainWindow == nil {
		return
	}
	s.mainWindow.Show()
	s.mainWindow.Focus()
}

// HideWindow 隐藏窗口
func (s *SysTrayService) HideWindow() {
	if s.mainWindow == nil {
		return
	}
	s.mainWindow.Hide()
}

// ServiceStartup 实现 Wails 服务接口：按 v3 生命周期最佳实践，将托盘初始化
// （创建托盘、图标、右键菜单、窗口关闭隐藏到托盘的钩子）放进启动阶段，
// 而不再于 main.go 手动调用 Init()。app 与 mainWindow 在 Run 前已注入，此处必然就绪。
func (s *SysTrayService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	if s.app == nil || s.mainWindow == nil {
		return nil
	}

	// 创建系统托盘
	s.systemTray = s.app.SystemTray.New()

	// 设置图标
	s.systemTray.SetIcon(systrayIcon)

	// 设置标签（macOS 显示在菜单栏，Windows 作为 tooltip）
	// s.systemTray.SetLabel("CertFlow")

	// 创建右键菜单（更新就绪后由 RefreshMenu 重建，插入手动重启入口）
	s.buildMenu()

	// 关联窗口 - 用于定位
	s.systemTray.AttachWindow(s.mainWindow).WindowOffset(5)

	// 左键点击托盘图标 - 总是显示窗口
	s.systemTray.OnClick(func() {
		s.ShowWindow()
	})

	// 窗口关闭时隐藏到托盘而不是退出；但应用正在退出（含自更新重启）时必须放行，
	// 否则 Cancel 会拦住窗口关闭，拖慢甚至卡死退出流程。
	s.mainWindow.RegisterHook(wailsEvents.Common.WindowClosing, func(e *application.WindowEvent) {
		if appQuitting.Load() {
			return
		}
		s.mainWindow.Hide()
		e.Cancel()
	})
	return nil
}

// buildMenu 构建（或重建）托盘右键菜单。
// 「更新已下载待安装」（updater.StateReady）时插入「重启并安装更新」入口：
// 设置项「更新完自动重启」关闭时，用户只能靠这一项完成安装，故需要动态出现。
func (s *SysTrayService) buildMenu() {
	if s.app == nil || s.systemTray == nil {
		return
	}

	menu := s.app.Menu.New()

	// 顶部：版本号（只读展示，禁用避免误点）
	menu.Add(i18n.T("systray.version", "Version", displayVersion())).SetEnabled(false)
	menu.AddSeparator()

	// 一级：常用证书操作
	menu.Add(i18n.T("systray.applyCert")).
		OnClick(func(ctx *application.Context) {
			// 显示窗口
			s.ShowWindow()
			if ok := s.app.Event.Emit(events.EventNavigate, events.NavigatePayload{
				Path: "/certificates/apply",
			}); !ok {
				logging.Warn("%s", i18n.T("error.navigate_failed"))
			}
		})
	menu.Add(i18n.T("systray.scan")).
		OnClick(func(ctx *application.Context) {
			// 显示窗口
			s.ShowWindow()
			// 通知前端导航到证书扫描页面
			if ok := s.app.Event.Emit(events.EventNavigate, events.NavigatePayload{
				Path: "/scan",
			}); !ok {
				logging.Warn("%s", i18n.T("error.navigate_failed"))
			}
		})
	menu.AddSeparator()

	// 二级：开机自启（复选框，状态读系统真实状态）+ 设置
	if s.autostart != nil {
		menu.AddCheckbox(i18n.T("systray.autostart"), s.autostart.IsEnabled()).
			OnClick(func(ctx *application.Context) {
				enabled := ctx.IsChecked()
				go func() {
					if enabled {
						if err := s.autostart.Enable(); err != nil {
							logging.Error("%s", i18n.T("log.autostart_enable_failed"))
						}
					} else {
						if err := s.autostart.Disable(); err != nil {
							logging.Error("%s", i18n.T("log.autostart_disable_failed"))
						}
					}
					s.buildMenu() // 与系统真实状态同步勾选（Enable/Disable 失败时回滚）
				}()
			})
	}
	menu.Add(i18n.T("systray.settings")).
		OnClick(func(ctx *application.Context) {
			// 显示窗口
			s.ShowWindow()
			// 通知前端导航到设置页面
			if ok := s.app.Event.Emit(events.EventNavigate, events.NavigatePayload{
				Path: "/settings",
			}); !ok {
				logging.Warn("%s", i18n.T("error.navigate_failed"))
			}
		})
	menu.AddSeparator()

	// 三级：更新相关
	menu.Add(i18n.T("systray.checkUpdate")).
		OnClick(func(ctx *application.Context) {
			// 显示窗口
			s.ShowWindow()
			go func() {
				defer func() {
					if r := recover(); r != nil {
						logging.Error("%s: %v", i18n.T("log.updater_check_panic"), r)
					}
				}()
				if err := s.app.Updater.CheckAndInstall(s.app.Context()); err != nil {
					logging.Warn("%s: %v", i18n.T("log.updater_check_failed"), err)
				} else {
					logging.Info("%s", i18n.T("log.updater_check_done"))
				}
			}()
		})

	// 预发布通道开关：勾选状态由 Wails 自动翻转，这里只负责落盘。
	// 该值在 Updater 初始化时读取一次，改动后需重启应用生效。
	if s.settings != nil {
		menu.AddCheckbox(i18n.T("systray.prerelease"), s.settings.Get().Prerelease).
			OnClick(func(ctx *application.Context) {
				cur := s.settings.Get()
				cur.Prerelease = ctx.IsChecked()
				if err := s.settings.Save(cur); err != nil {
					logging.Error("%s: %v", i18n.T("log.settings_save_failed"), err)
					s.buildMenu() // 落盘失败：按当前配置重建，回滚勾选状态
				}
			})
	}

	// 更新已就绪（下载 + 校验完成，等待替换二进制）：提供手动重启入口。
	if s.app.Updater != nil && s.app.Updater.State() == updater.StateReady {
		menu.Add(i18n.T("systray.restartAndUpdate")).
			OnClick(func(ctx *application.Context) {
				go func() {
					defer func() {
						if r := recover(); r != nil {
							logging.Error("%s: %v", i18n.T("log.updater_restart_panic"), r)
						}
					}()
					if err := s.app.Updater.Restart(context.Background()); err != nil {
						logging.Warn("%s: %v", i18n.T("log.updater_restart_failed"), err)
					}
				}()
			})
	}

	menu.AddSeparator()
	menu.Add(i18n.T("systray.quit")).
		OnClick(func(ctx *application.Context) {
			s.app.Quit()
		})
	s.systemTray.SetMenu(menu)
}

// RefreshMenu 重建托盘菜单，使「重启并安装更新」入口随更新状态即时出现/消失。
// 由 main.go 的 EventUpdateReady 回调调用。
func (s *SysTrayService) RefreshMenu() {
	s.buildMenu()
}

// ServiceShutdown 实现 Wails 服务接口
func (s *SysTrayService) ServiceShutdown() error {
	return nil
}

// ServiceName 实现 Wails 服务接口
func (s *SysTrayService) ServiceName() string {
	return "SysTrayService"
}
