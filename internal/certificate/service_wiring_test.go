package certificate

import (
	"testing"

	"cnb.cool/dtapp/certflow/internal/notification"
	"cnb.cool/dtapp/certflow/internal/settings"
)

// TestSetNotificationService 覆盖通知服务注入（nil 与非 nil）。
func TestSetNotificationService(t *testing.T) {
	svc, _ := newTestService(t)

	svc.SetNotificationService(nil)
	if svc.notifService != nil {
		t.Fatal("注入 nil 后字段应为 nil")
	}

	// 直接构造零值：避免触发 Wails 通知服务的初始化逻辑
	ns := &notification.NotificationService{}
	svc.SetNotificationService(ns)
	if svc.notifService != ns {
		t.Fatal("通知服务未注入")
	}
}

// TestSetSettingsProvider 覆盖设置提供者注入，并确认申请证书时会调用它。
func TestSetSettingsProvider(t *testing.T) {
	svc, _ := newTestService(t)

	called := false
	svc.SetSettingsProvider(func() settings.Settings {
		called = true
		return settings.Settings{}
	})
	if svc.settingsProvider == nil {
		t.Fatal("设置提供者未注入")
	}
	got := svc.settingsProvider()
	if !called {
		t.Fatal("设置提供者应被调用")
	}
	if got.AutoRenewEnabled {
		t.Fatal("返回值应来自注入的函数")
	}
}
