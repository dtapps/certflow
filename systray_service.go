package main

import (
	"context"
	_ "embed"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed image/icon2.png
var systrayIcon []byte

// SysTrayService 系统托盘服务
type SysTrayService struct {
	app        *application.App
	mainWindow application.Window
	systemTray *application.SystemTray
}

// NewSysTrayService 创建系统托盘服务
// https://v3.wails.io/features/menus/systray/
func NewSysTrayService() *SysTrayService {
	return &SysTrayService{}
}

// SetApp 设置 app 引用
func (s *SysTrayService) SetApp(app *application.App) {
	s.app = app
}

// SetMainWindow 设置主窗口引用
func (s *SysTrayService) SetMainWindow(window application.Window) {
	s.mainWindow = window
}

// Init 初始化系统托盘
func (s *SysTrayService) Init() {
	if s.app == nil || s.mainWindow == nil {
		return
	}

	// 创建系统托盘
	s.systemTray = s.app.SystemTray.New()

	// 设置图标
	s.systemTray.SetIcon(systrayIcon)

	// 设置标签（macOS 显示在菜单栏，Windows 作为 tooltip）
	// s.systemTray.SetLabel("CertFlow")

	// 创建右键菜单
	menu := s.app.Menu.New()
	menu.Add(i18n.T("systray.checkUpdate")).OnClick(func(ctx *application.Context) {
		s.ShowWindow()
		go func() {
			if err := s.app.Updater.CheckAndInstall(context.Background()); err != nil {
				logging.Warn("%s: %v", i18n.T("log.updater_check_failed"), err)
			}
		}()
	})
	menu.Add(i18n.T("systray.settings")).OnClick(func(ctx *application.Context) {
		s.ShowWindow()
		s.app.Event.Emit("navigate", "/settings")
	})
	menu.AddSeparator()
	menu.Add(i18n.T("systray.quit")).OnClick(func(ctx *application.Context) {
		s.app.Quit()
	})
	s.systemTray.SetMenu(menu)

	// 关联窗口 - 用于定位
	s.systemTray.AttachWindow(s.mainWindow).WindowOffset(5)

	// 左键点击托盘图标 - 总是显示窗口
	s.systemTray.OnClick(func() {
		s.ShowWindow()
	})

	// 窗口关闭时隐藏到托盘而不是退出
	s.mainWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		s.mainWindow.Hide()
		e.Cancel()
	})
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

// ServiceStartup 实现 Wails 服务接口
func (s *SysTrayService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown 实现 Wails 服务接口
func (s *SysTrayService) ServiceShutdown() error {
	return nil
}

// ServiceName 实现 Wails 服务接口
func (s *SysTrayService) ServiceName() string {
	return "SysTrayService"
}
