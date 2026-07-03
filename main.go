package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"cnb.cool/dtapp/certflow/internal/auth"
	"cnb.cool/dtapp/certflow/internal/ca"
	"cnb.cool/dtapp/certflow/internal/certificate"
	"cnb.cool/dtapp/certflow/internal/db"
	"cnb.cool/dtapp/certflow/internal/dnsprovider"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/monitor"
	"cnb.cool/dtapp/certflow/internal/notification"
	"cnb.cool/dtapp/certflow/internal/scheduler"
	"cnb.cool/dtapp/certflow/internal/settings"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

//go:embed all:frontend/dist
var assets embed.FS

// 当前版本号（构建时通过 -ldflags 覆盖）
var currentVersion = "dev"

// 构建信息（构建时通过 -ldflags 覆盖）
var buildTime = ""
var gitCommit = ""

func init() {
	application.RegisterEvent[string]("time")
	application.RegisterEvent[map[string]string]("notification")
}

func main() {
	// 确定数据目录
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get home dir: %v", err)
	}
	dataDir := filepath.Join(homeDir, ".certflow")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data dir: %v", err)
	}

	// 初始化设置服务（尽早加载，i18n 语言设置依赖它）
	settingsService, err := settings.NewService(dataDir)
	if err != nil {
		log.Fatalf("Failed to load settings: %v", err)
	}
	i18n.SetLocale(settingsService.Get().Language)

	// 初始化数据库
	if err := db.Init(dataDir); err != nil {
		log.Fatalf(i18n.T("error.open_db_failed")+": %v", err)
	}
	defer db.Close()

	// 创建内部服务
	caService := ca.NewCAService(db.Client, dataDir)
	dnsService := dnsprovider.NewDNSProviderService(db.Client)
	certService := certificate.NewCertificateService(db.Client, dataDir)
	notifService := notification.NewNotificationService()
	certService.SetNotificationService(notifService)
	certService.SetSettingsProvider(settingsService.Get)
	schedulerService := scheduler.NewScheduler(db.Client, certService, notifService, settingsService, dataDir)
	monitorService := monitor.NewMonitorService(db.Client)
	monitorService.SetSettingsProvider(settingsService.Get)
	monitorService.SetNotificationService(notifService)
	authService, err := auth.NewAuthService(dataDir)
	if err != nil {
		log.Fatalf(i18n.T("error.load_auth_failed")+": %v", err)
	}

	// 初始化日志系统
	settings := settingsService.Get()
	logDir := filepath.Join(dataDir, "logs")
	if err := logging.InitGlobalLogger(logDir, settings.Log.Level, settings.Log.MaxMB, settings.Log.MaxBackups); err != nil {
		log.Fatalf(i18n.T("error.create_log_dir_failed")+": %v", err)
	}
	defer logging.Global().Close()
	logging.Info(i18n.T("log.app_starting"))

	// 设置通知服务的数据库客户端
	notifService.SetDB(db.Client)

	// 预置默认 CA（仅在首次启动、数据库无 CA 记录时）
	if err := caService.SeedDefaults(context.Background()); err != nil {
		logging.Error(i18n.T("log.seed_ca_failed"), err)
		log.Fatalf(i18n.T("error.create_schema_failed")+": %v", err)
	}

	// 创建剪贴板服务和浏览器服务（需要在 app 创建后设置 app 引用）
	clipboardSvc := NewClipboardServiceWrapper(nil)
	browserSvc := NewBrowserServiceWrapper(nil)
	systemSvc := NewSystemServiceWrapper()
	fileSvc := NewFileServiceWrapper()
	windowSvc := NewWindowServiceWrapper()

	// 创建 Wails 应用
	app := application.New(application.Options{
		Name:        "CertFlow",
		Description: i18n.T("app.description"),
		Services: []application.Service{
			application.NewService(NewCAServiceWrapper(caService)),
			application.NewService(NewDNSProviderServiceWrapper(dnsService)),
			application.NewService(NewCertificateServiceWrapper(certService)),
			application.NewService(NewSchedulerServiceWrapper(schedulerService)),
			application.NewService(NewNotificationServiceWrapper(notifService)),
			application.NewService(NewSettingsServiceWrapper(settingsService)),
			application.NewService(NewLoggingServiceWrapper()),
			application.NewService(NewAuthServiceWrapper(authService)),
			application.NewService(NewMonitorServiceWrapper(monitorService)),
			application.NewService(clipboardSvc),
			application.NewService(browserSvc),
			application.NewService(systemSvc),
			application.NewService(fileSvc),
			application.NewService(windowSvc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// 设置剪贴板服务和浏览器服务的 app 引用
	clipboardSvc.SetApp(app)
	browserSvc.SetApp(app)
	notifService.SetApp(app)
	systemSvc.SetApp(app)
	fileSvc.SetApp(app)
	windowSvc.SetApp(app)

	// 配置自更新功能
	gh, err := github.New(github.Config{
		Repository: "dtapps/certflow",
	})
	if err != nil {
		logging.Error(i18n.T("log.updater_init_failed"), err)
	} else {
		if err := app.Updater.Init(updater.Config{
			CurrentVersion: currentVersion,
			Providers:      []updater.Provider{gh},
			CheckInterval:  6 * time.Hour,
		}); err != nil {
			logging.Error(i18n.T("log.updater_init_failed"), err)
		}
	}

	// 创建主窗口
	mainWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  i18n.T("app.title"),
		Width:  1280,
		Height: 800,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 0,
			Backdrop:                application.MacBackdropNormal,
			TitleBar: application.MacTitleBar{
				AppearsTransparent: false,
			},
		},
		BackgroundType: application.BackgroundTypeTransparent,
		URL:            "/",
		Frameless:      false,
	})
	mainWindow.OnWindowEvent(events.Mac.WebViewDidFinishNavigation, func(event *application.WindowEvent) {
		mainWindow.Show()
		mainWindow.Focus()
	})

	// 创建应用菜单（含检查更新）
	appMenu := app.Menu.New()
	appMenu.AddSubmenu(i18n.T("menu.app")).
		Add(i18n.T("menu.checkUpdate")).OnClick(func(ctx *application.Context) {
		go func() {
			if err := app.Updater.CheckAndInstall(context.Background()); err != nil {
				logging.Error(i18n.T("log.updater_check_failed"), err)
			}
		}()
	})
	appMenu.AddSubmenu(i18n.T("menu.help")).
		Add(i18n.T("menu.about")).OnClick(func(ctx *application.Context) {})
	app.Menu.SetApplicationMenu(appMenu)

	// 启动域名监控后台任务
	monitorService.Start()
	defer monitorService.Stop()

	// 监听系统主题变化，通知前端
	app.Event.OnApplicationEvent(events.Common.ThemeChanged, func(event *application.ApplicationEvent) {
		isDark := app.Env.IsDarkMode()
		app.Event.Emit("theme_changed", map[string]bool{"dark": isDark})
	})

	// 启动定时任务调度器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-ctx.Done()
		schedulerService.Stop()
	}()

	// 运行应用
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}

	fmt.Println(i18n.T("app.exited"))
}
