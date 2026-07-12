package main

import (
	"context"
	"crypto/rand"
	"embed"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cnb.cool/dtapp/certflow/internal/auth"
	"cnb.cool/dtapp/certflow/internal/ca"
	"cnb.cool/dtapp/certflow/internal/certificate"
	"cnb.cool/dtapp/certflow/internal/db"
	"cnb.cool/dtapp/certflow/internal/deploy"
	"cnb.cool/dtapp/certflow/internal/deploycredential"
	"cnb.cool/dtapp/certflow/internal/dnsprovider"
	"cnb.cool/dtapp/certflow/internal/events"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/monitor"
	"cnb.cool/dtapp/certflow/internal/notification"
	"cnb.cool/dtapp/certflow/internal/scanner"
	"cnb.cool/dtapp/certflow/internal/scheduler"
	"cnb.cool/dtapp/certflow/internal/settings"
	"github.com/wailsapp/wails/v3/pkg/application"
	wailsEvents "github.com/wailsapp/wails/v3/pkg/events"
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
var githubToken = ""

func init() {
	application.RegisterEvent[events.TimePayload](events.EventTime)
	application.RegisterEvent[events.NotificationPayload](events.EventNotification)
	application.RegisterEvent[struct{}](events.EventAuthVerified)
	application.RegisterEvent[events.ThemeChangedPayload](events.EventThemeChanged)
	application.RegisterEvent[events.LocaleChangedPayload](events.EventLocaleChanged)
	application.RegisterEvent[events.NavigatePayload](events.EventNavigate)
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

	// 运行时语言切换：前端保存设置（含语言）后，文件监控触发 OnChange，
	// 同步后端 i18n 语言环境，使后端返回的错误/日志文案跟随界面语言变化。
	settingsService.OnChange(func(s settings.Settings) {
		i18n.SetLocale(s.Language)
	})

	// 初始化日志系统（必须在数据库和调度器之前）
	settings := settingsService.Get()
	logDir := filepath.Join(dataDir, "logs")
	if err := logging.InitGlobalLogger(logDir, settings.Log.Level, settings.Log.MaxMB, settings.Log.MaxBackups); err != nil {
		log.Fatalf(i18n.T("error.create_log_dir_failed")+": %v", err)
	}
	defer logging.Global().Close()
	logging.Info(i18n.T("log.app_starting"))

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
	deployService := deploy.NewDeployService(db.Client)
	deployService.SetNotificationService(notifService)
	deployCredentialService := deploycredential.NewService(db.Client)
	schedulerService := scheduler.NewScheduler(db.Client, certService, notifService, settingsService, dataDir)
	monitorService := monitor.NewMonitorService(db.Client)
	monitorService.SetSettingsProvider(settingsService.Get)
	monitorService.SetNotificationService(notifService)
	scannerService := scanner.NewScannerService(db.Client)
	scannerService.SetSettingsProvider(settingsService.Get)
	authService := auth.NewAuthService(db.Client)

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
	systraySvc := NewSysTrayService()
	dockSvc := NewDockServiceWrapper()
	autostartSvc := NewAutostartServiceWrapper()

	// 创建 Wails 应用
	app := application.New(application.Options{
		Name:        "CertFlow",
		Description: i18n.T("app.description"),
		Services: []application.Service{
			application.NewService(NewCAServiceWrapper(caService)),
			application.NewService(NewDNSProviderServiceWrapper(dnsService)),
			application.NewService(NewCertificateServiceWrapper(certService)),
			application.NewService(NewDeployServiceWrapper(deployService)),
			application.NewService(NewDeployCredentialServiceWrapper(deployCredentialService)),
			application.NewService(NewSchedulerServiceWrapper(schedulerService)),
			application.NewService(NewNotificationServiceWrapper(notifService)),
			application.NewService(NewSettingsServiceWrapper(settingsService)),
			application.NewService(NewLoggingServiceWrapper()),
			application.NewService(NewAuthServiceWrapper(authService)),
			application.NewService(NewMonitorServiceWrapper(monitorService)),
			application.NewService(NewScannerServiceWrapper(scannerService)),
			application.NewService(clipboardSvc),
			application.NewService(browserSvc),
			application.NewService(systemSvc),
			application.NewService(fileSvc),
			application.NewService(windowSvc),
			application.NewService(systraySvc),
			application.NewService(dockSvc),
			application.NewService(autostartSvc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	// 设置剪贴板服务和浏览器服务的 app 引用
	clipboardSvc.SetApp(app)
	browserSvc.SetApp(app)
	notifService.SetApp(app)
	systemSvc.SetApp(app)
	fileSvc.SetApp(app)
	windowSvc.SetApp(app)
	systraySvc.SetApp(app)
	dockSvc.SetApp(app)
	autostartSvc.SetApp(app)

	// 配置自更新功能
	// https://v3.wails.io/guides/updater/
	gh, err := github.New(github.Config{
		Repository:    "dtapps/certflow",
		Token:         githubToken,
		Prerelease:    settingsService.Get().Prerelease,
		ChecksumAsset: "SHA256SUMS",
		// 自定义资源匹配：仅匹配「升级专用文件」（文件名以 updater- 开头）。
		// 打包/安装文件（Windows -install.exe、macOS .app.zip、Linux
		// AppImage/deb/rpm/pkg.tar.zst）只用于首次安装，不用于自更新；
		// 升级文件统一压缩（Windows/macOS 为 .zip，Linux 为 .tar.gz），内
		// 含单一二进制，由 updater 下载、校验（SHA256SUMS）后替换自身。
		AssetMatcher: func(req updater.CheckRequest, assets []github.ReleaseAsset) int {
			plat := strings.ToLower(req.Platform)
			arch := strings.ToLower(req.Arch)
			logging.Debug(i18n.T("log.updater_matcher_start", "Plat", plat, "Arch", arch, "Count", len(assets)))
			for i, a := range assets {
				name := strings.ToLower(a.Name)
				logging.Debug(i18n.T("log.updater_matcher_check", "Index", i, "Name", a.Name))
				// 仅升级专用文件（updater- 前缀）参与自更新
				if !strings.HasPrefix(name, "updater-") {
					logging.Debug(i18n.T("log.updater_matcher_skip_not_updater"))
					continue
				}
				if strings.HasSuffix(name, ".sig") || strings.HasSuffix(name, ".asc") || strings.HasSuffix(name, ".zsync") {
					logging.Debug(i18n.T("log.updater_matcher_skip_sig"))
					continue
				}
				// 升级文件必须是压缩归档（.zip / .tar.gz / .tgz）
				if !strings.HasSuffix(name, ".zip") &&
					!strings.HasSuffix(name, ".tar.gz") &&
					!strings.HasSuffix(name, ".tgz") {
					logging.Debug(i18n.T("log.updater_matcher_skip_format"))
					continue
				}
				if plat != "" && !strings.Contains(name, plat) {
					logging.Debug(i18n.T("log.updater_matcher_skip_plat", "Plat", plat))
					continue
				}
				if arch != "" && !strings.Contains(name, arch) {
					logging.Debug(i18n.T("log.updater_matcher_skip_arch", "Arch", arch))
					continue
				}
				logging.Debug(i18n.T("log.updater_matcher_hit", "Index", i, "Name", a.Name))
				return i
			}
			logging.Debug(i18n.T("log.updater_matcher_none"))
			return -1
		},
	})
	if err != nil {
		logging.Error(i18n.T("log.updater_init_failed"), err)
	} else {
		if err := app.Updater.Init(updater.Config{
			CurrentVersion: currentVersion,
			Providers:      []updater.Provider{gh},
			Window: &updater.BuiltinWindow{
				Options: updater.WindowOptions{
					Title:       i18n.T("updater.title"),
					AlwaysOnTop: true,
				},
			},
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
				AppearsTransparent: true,
			},
		},
		BackgroundType: application.BackgroundTypeTransparent,
		URL:            "/",
		Frameless:      true,
	})
	mainWindow.OnWindowEvent(wailsEvents.Mac.WebViewDidFinishNavigation, func(event *application.WindowEvent) {
		mainWindow.Show()
		mainWindow.Focus()
	})

	// 初始化系统托盘
	systraySvc.SetMainWindow(mainWindow)
	systraySvc.Init()

	// 设置系统服务的主窗口引用
	systemSvc.SetMainWindow(mainWindow)

	// 创建应用菜单（含检查更新和设置）
	appMenu := app.Menu.New()
	appSubmenu := appMenu.AddSubmenu(i18n.T("menu.app"))
	appSubmenu.Add(i18n.T("menu.settings")).
		OnClick(func(ctx *application.Context) {
			// 通知前端导航到设置页面
			if ok := app.Event.Emit(events.EventNavigate, events.NavigatePayload{
				Path: "/settings",
			}); !ok {
				logging.Warn("%s", i18n.T("error.navigate_failed"))
			}
		})
	appSubmenu.Add(i18n.T("menu.checkUpdate")).
		OnClick(func(ctx *application.Context) {
			go func() {
				if err := app.Updater.CheckAndInstall(context.Background()); err != nil {
					logging.Warn("%s: %v", i18n.T("log.updater_check_failed"), err)
				} else {
					logging.Info("%s", i18n.T("log.updater_check_done"))
				}
			}()
		})
	appSubmenu.AddSeparator()
	appSubmenu.Add(i18n.T("systray.applyCert")).
		OnClick(func(ctx *application.Context) {
			// 通知前端导航到申请证书页面
			if ok := app.Event.Emit(events.EventNavigate, events.NavigatePayload{
				Path: "/certificates/apply",
			}); !ok {
				logging.Warn("%s", i18n.T("error.navigate_failed"))
			}
		})
	appSubmenu.Add(i18n.T("systray.scan")).
		OnClick(func(ctx *application.Context) {
			// 通知前端导航到证书扫描页面
			if ok := app.Event.Emit(events.EventNavigate, events.NavigatePayload{Path: "/scan"}); !ok {
				logging.Warn("%s", i18n.T("error.navigate_failed"))
			}
		})
	appSubmenu.AddSeparator()
	appSubmenu.Add(i18n.T("systray.quit")).
		OnClick(func(ctx *application.Context) {
			app.Quit()
		})

	helpSubmenu := appMenu.AddSubmenu(i18n.T("menu.help"))
	helpSubmenu.Add(i18n.T("menu.about")).
		OnClick(func(ctx *application.Context) {
			aboutMsg := fmt.Sprintf("CertFlow\n\n%s: %s", i18n.T("settings.about.version"), currentVersion)
			if buildTime != "" {
				aboutMsg += fmt.Sprintf("\n%s: %s", i18n.T("settings.about.buildTime"), buildTime)
			}
			if gitCommit != "" {
				aboutMsg += fmt.Sprintf("\n%s: %s", i18n.T("settings.about.gitCommit"), gitCommit)
			}
			app.Dialog.Info().
				SetTitle(i18n.T("menu.about")).
				SetMessage(aboutMsg).
				Show()
		})
	app.Menu.SetApplicationMenu(appMenu)

	// 启动域名监控后台任务
	monitorService.Start()
	defer monitorService.Stop()

	// 启动定时任务调度器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-ctx.Done()
		schedulerService.Stop()
	}()

	// updater 重启前主动释放资源，确保进程快速退出
	// Windows 上 helper 需要替换 exe 文件，如果父进程退不掉会导致文件占用
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			schedulerService.Stop()
			monitorService.Stop()
			cancel()
			db.Close()
		})
	}

	app.Event.On(updater.EventUpdateReady, func(e *application.CustomEvent) {
		logging.Info(i18n.T("log.updater_ready"))
		cleanup()
	})

	// 监听系统主题变化，通知前端
	app.Event.OnApplicationEvent(wailsEvents.Common.ThemeChanged, func(event *application.ApplicationEvent) {
		if ok := app.Event.Emit(events.EventThemeChanged, events.ThemeChangedPayload{
			Dark: app.Env.IsDarkMode(),
		}); !ok {
			logging.Warn("%s", i18n.T("error.theme_notify_failed"))
		}
	})

	// 启动后异步检查更新
	checkUpdateOnStart(app)

	// 运行应用
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}

	fmt.Println(i18n.T("app.exited"))
}

// checkUpdateOnStart 启动后异步检查更新，有更新时发通知
func checkUpdateOnStart(app *application.App) {
	go func() {
		// 生成 1 到 3 分钟之间的随机持续时间
		minDuration := 1 * time.Minute
		maxDuration := 3 * time.Minute
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(maxDuration-minDuration)))
		randomDuration := minDuration + time.Duration(n.Int64())
		time.Sleep(randomDuration)
		rel, err := app.Updater.Check(context.Background())
		if err != nil {
			logging.Warn("%s: %v", i18n.T("log.updater_check_failed"), err)
			return
		}
		if rel == nil {
			return // 没有更新
		}
		// 发送桌面通知
		if ok := app.Event.Emit(events.EventNotification, events.NotificationPayload{
			Title:    i18n.T("notification.update_available_title"),
			Subtitle: i18n.T("notification.update_available_subtitle", "version", rel.Version),
			Category: "system",
		}); !ok {
			logging.Warn("%s", i18n.T("error.notification_failed"))
		}
	}()
}
