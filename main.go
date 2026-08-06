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
	"cnb.cool/dtapp/certflow/internal/data"
	"cnb.cool/dtapp/certflow/internal/db"
	"cnb.cool/dtapp/certflow/internal/deploy"
	"cnb.cool/dtapp/certflow/internal/deploycredential"
	"cnb.cool/dtapp/certflow/internal/dnsprovider"
	"cnb.cool/dtapp/certflow/internal/events"
	"cnb.cool/dtapp/certflow/internal/httplog"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/monitor"
	"cnb.cool/dtapp/certflow/internal/network"
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

//go:embed updater_window.html
var updaterWindowHTML string

// 当前版本号（构建时通过 -ldflags 覆盖）不带 v
var currentVersion = "dev"

// 构建信息（构建时通过 -ldflags 覆盖）
var buildTime = ""
var gitCommit = ""
var githubToken = ""
var cnbToken = ""

func init() {
	application.RegisterEvent[events.TimePayload](events.EventTime)
	application.RegisterEvent[events.NotificationPayload](events.EventNotification)
	application.RegisterEvent[struct{}](events.EventAuthVerified)
	application.RegisterEvent[events.ThemeChangedPayload](events.EventThemeChanged)
	application.RegisterEvent[events.LocaleChangedPayload](events.EventLocaleChanged)
	application.RegisterEvent[events.NavigatePayload](events.EventNavigate)
	application.RegisterEvent[events.WindowResizedPayload](events.EventWindowResized)
}

func main() {
	// 确定数据目录
	// 开发构建（currentVersion == "dev"，即 `wails3 dev` / 未注入 VERSION）使用独立的
	// .certflow.dev 目录，与正式版 .certflow 隔离，避免开发调试污染正式数据。
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get home dir: %v", err)
	}
	dataDirName := ".certflow"
	if currentVersion == "dev" {
		dataDirName = ".certflow.dev"
	}
	dataDir := filepath.Join(homeDir, dataDirName)
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
	// 前端错误日志写入独立的 frontend.log（不混入 certflow.log），供前端运行时错误上报落盘。
	if err := logging.InitFrontendLogger(logDir, settings.Log.Level, settings.Log.MaxMB, settings.Log.MaxBackups); err != nil {
		logging.Error(i18n.T("error.create_log_dir_failed")+": %v", err)
	}
	// 全局兜底：捕获主流程及任何 goroutine 中未 recover 的 panic，
	// 先写 ERROR 日志（再 defer Close 会落盘）再退出，避免进程静默消失、无任何痕迹。
	// 必须在 InitGlobalLogger 之后、defer Close 之前注册，以确保 LIFO 时最后关闭日志。
	defer func() {
		if r := recover(); r != nil {
			logging.Error("%s: %v", i18n.T("log.global_panic"), r)
			log.Printf("FATAL panic (recovered, see log): %v", r)
		}
	}()
	defer logging.Global().Close()
	logging.Info(i18n.T("log.app_starting", "Version", currentVersion, "BuildTime", buildTime))
	logging.Info(i18n.T("log.data_dir", "Dir", dataDir))

	// 初始化 HTTP 请求日志（独立日志库 httplog.db，仅 DEBUG 级别记录）
	if err := httplog.Init(dataDir); err != nil {
		logging.Error(i18n.T("error.httplog_init_failed", "Error", err))
	} else {
		defer httplog.Close()
		// 启动 HTTP 请求日志定期清理（保留天数取设置，默认 30 天）
		go func() {
			cleanupHttpLog := func() {
				days := settingsService.Get().HttpLogRetentionDays
				if n, err := httplog.Cleanup(days); err != nil {
					logging.Error(i18n.T("log.httplog_cleanup_failed", "Error", err))
				} else if n > 0 {
					logging.Info(i18n.T("log.httplog_cleanup", "Count", n))
				}
			}
			cleanupHttpLog()
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				cleanupHttpLog()
			}
		}()
	}

	// 初始化数据库
	if err := db.Init(dataDir); err != nil {
		log.Fatalf(i18n.T("error.open_db_failed")+": %v", err)
	}
	defer db.Close()

	// 创建内部服务
	caService := ca.NewCAService(db.Client, dataDir)
	caService.SetSettingsProvider(settingsService.Get)
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
	// 业务服务包装器（需在 app 创建后调用 SetApp 注入生命周期）
	caSvc := NewCAServiceWrapper(caService)
	dnsSvc := NewDNSProviderServiceWrapper(dnsService)
	certSvc := NewCertificateServiceWrapper(certService)
	deploySvc := NewDeployServiceWrapper(deployService)
	deployCredSvc := NewDeployCredentialServiceWrapper(deployCredentialService)
	schedulerSvc := NewSchedulerServiceWrapper(schedulerService)
	monitorSvc := NewMonitorServiceWrapper(monitorService)
	scannerSvc := NewScannerServiceWrapper(scannerService)
	notifSvc := NewNotificationServiceWrapper(notifService)
	fileSvc := NewFileServiceWrapper()
	windowSvc := NewWindowServiceWrapper()
	systraySvc := NewSysTrayService()
	dockSvc := NewDockServiceWrapper()
	autostartSvc := NewAutostartServiceWrapper()
	// 数据管理服务（使用原生文件对话框，app 在下方 SetApp 注入；
	// 因 Wails 绑定解析器要求 Service 在 Options 之前声明，而 app 在 Options 之后才可用）
	dataSvc := data.NewService(dataDir)

	// 主窗口引用（提前声明，供单实例回调闭包引用）
	var mainWindow *application.WebviewWindow

	// 创建 Wails 应用
	app := application.New(application.Options{
		Name:        "CertFlow",
		Description: i18n.T("app.description"),
		// 单实例限制：应用只允许运行一个实例。当用户重复点击图标启动第二个
		// 实例时，第二个实例会通知第一个实例并自行退出，第一个实例负责将已有
		// 主窗口恢复并置于最前。
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "net.dtapp.certflow",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				if mainWindow == nil {
					return
				}
				mainWindow.Restore()
				mainWindow.Show()
				mainWindow.Focus()
			},
		},
		Services: []application.Service{
			application.NewService(caSvc),
			application.NewService(dnsSvc),
			application.NewService(certSvc),
			application.NewService(deploySvc),
			application.NewService(deployCredSvc),
			application.NewService(schedulerSvc),
			application.NewService(notifSvc),
			application.NewService(NewSettingsServiceWrapper(settingsService)),
			application.NewService(NewLoggingServiceWrapper()),
			application.NewService(NewAuthServiceWrapper(authService)),
			application.NewService(monitorSvc),
			application.NewService(scannerSvc),
			application.NewService(clipboardSvc),
			application.NewService(browserSvc),
			application.NewService(systemSvc),
			application.NewService(fileSvc),
			application.NewService(windowSvc),
			application.NewService(systraySvc),
			application.NewService(dockSvc),
			application.NewService(autostartSvc),
			application.NewService(dataSvc),
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
	systemSvc.SetApp(app)
	fileSvc.SetApp(app)
	windowSvc.SetApp(app)
	systraySvc.SetApp(app)
	dockSvc.SetApp(app)
	dataSvc.SetApp(app)
	dockSvc.SetApp(app)
	autostartSvc.SetApp(app)
	// 业务服务包装器：注入 app 以使用应用生命周期 context（替代 context.Background()）
	caSvc.SetApp(app)
	dnsSvc.SetApp(app)
	certSvc.SetApp(app)
	deploySvc.SetApp(app)
	deployCredSvc.SetApp(app)
	schedulerSvc.SetApp(app)
	monitorSvc.SetApp(app)
	scannerSvc.SetApp(app)
	notifSvc.SetApp(app)

	// 配置自更新功能
	// https://v3.wails.io/guides/updater/
	// 更新器复用全局 HTTP 客户端（含 UA 注入、代理、自定义 DNS），
	// 而非自建裸 client，避免丢失全局注入与可观测性。
	updaterClient := network.BuildHTTPClient(settingsService.Get())
	gh, err := newMirrorProvider(github.Config{
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
	}, updaterClient, buildTime, cnbToken)
	if err != nil {
		logging.Error(i18n.T("log.updater_init_failed"), err)
	} else {
		if err := app.Updater.Init(updater.Config{
			CurrentVersion: currentVersion,
			Providers:      []updater.Provider{gh},
			Window: &updater.BuiltinWindow{
				// 使用定制模板：版本号副标题不拼 "v" 前缀（nightly 显示 nightly 而非 vnightly）。
				HTML: updaterWindowHTML,
				Options: updater.WindowOptions{
					Title: i18n.T("updater.title"),
				},
			},
		}); err != nil {
			logging.Error(i18n.T("log.updater_init_failed"), err)
		}
	}

	// 创建主窗口
	mainWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
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

	// 将主窗口引用交给窗口服务，供其调用 window.Size() 获取尺寸
	windowSvc.setMainWindow(mainWindow)

	// 监听窗口尺寸变化（Go 端 WindowDidResize 事件），向 frontend 广播最新尺寸
	// https://v3.wails.io/zh-cn/reference/window/#size
	mainWindow.OnWindowEvent(wailsEvents.Common.WindowDidResize, func(event *application.WindowEvent) {
		width, height := mainWindow.Size()
		app.Event.Emit(events.EventWindowResized, events.WindowResizedPayload{
			Width:  width,
			Height: height,
		})
	})

	// 初始化系统托盘
	systraySvc.setMainWindow(mainWindow)
	systraySvc.Init()

	// 设置系统服务的主窗口引用
	systemSvc.setMainWindow(mainWindow)

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
				// 兜底：避免 Wails Updater 原生层（macOS Sparkle 等）崩溃
				// 通过未 recover 的 panic 拖死整个主进程（表现为点击后进程静默消失）。
				defer func() {
					if r := recover(); r != nil {
						logging.Error("%s: %v", i18n.T("log.updater_check_panic"), r)
					}
				}()
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

	// 标准编辑菜单（撤销/重做/剪切/复制/粘贴/全选）。macOS 上系统快捷键
	//（Cmd+C/V/X/A/Z 等）是通过菜单项的 accelerator 路由到 WebView 的，
	// 缺失该菜单会导致应用内复制、粘贴等失效。
	appMenu.AddRole(application.EditMenu)

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

	// updater 下载安装完成（EventUpdateReady）后主动停掉后台任务，
	// 避免它们在二进制替换 / 重启前继续写库或占用资源。
	// 注意：这里【不能】关闭数据库。此时进程并未退出（要等用户点击
	// Restart 触发 Quit 才退出），前端仍在运行并周期性调用后端绑定
	// （如未读通知数量 CountUnread），过早关闭 DB 会导致
	// "sql: database is closed" 错误。数据库由 main 的 defer db.Close()
	// 在进程真正退出时统一关闭。
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			schedulerService.Stop()
			monitorService.Stop()
			cancel()
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
	if currentVersion != "dev" {
		checkUpdateOnStart(app)
	}

	// 运行应用
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}

	fmt.Println(i18n.T("app.exited"))
}

// checkUpdateOnStart 启动后异步检查更新，有更新时发通知
func checkUpdateOnStart(app *application.App) {
	go func() {
		// 兜底：同点击检查更新，避免 Updater 原生层 panic 拖死主进程。
		defer func() {
			if r := recover(); r != nil {
				logging.Error("%s: %v", i18n.T("log.updater_check_panic"), r)
			}
		}()
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
			Level:    "info",
		}); !ok {
			logging.Warn("%s", i18n.T("error.notification_failed"))
		}
	}()
}
