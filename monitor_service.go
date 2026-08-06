package main

import (
	"context"
	"fmt"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/monitor"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// MonitorServiceWrapper 包装 monitor.MonitorService
type MonitorServiceWrapper struct {
	app            *application.App
	monitorService *monitor.MonitorService
}

// NewMonitorServiceWrapper 创建新的监控服务包装器
func NewMonitorServiceWrapper(monitorService *monitor.MonitorService) *MonitorServiceWrapper {
	return &MonitorServiceWrapper{monitorService: monitorService}
}

// SetApp 设置 app 引用（用于获取应用生命周期）
func (w *MonitorServiceWrapper) SetApp(app *application.App) {
	w.app = app
}

// List 获取所有监控域名
func (s *MonitorServiceWrapper) List() ([]*monitor.MonitoredDomainItem, error) {
	items, err := s.monitorService.List(s.app.Context())
	if err != nil {
		logging.Error("%s", i18n.T("log.monitor_list_failed", "Error", err))
	}
	if err == nil {
		logging.Debug("%s", i18n.T("log.monitor_list_loaded", "Count", len(items), "Data", fmt.Sprintf("%+v", items)))
	}
	return items, err
}

// Create 创建监控域名
func (s *MonitorServiceWrapper) Create(input monitor.CreateInput) (*monitor.MonitoredDomainItem, error) {
	item, err := s.monitorService.Create(s.app.Context(), input)
	if err != nil {
		logging.Error("%s", i18n.T("log.monitor_create_failed", "Error", err))
	}
	return item, err
}

// Update 更新监控域名
func (s *MonitorServiceWrapper) Update(id int, input monitor.UpdateInput) (*monitor.MonitoredDomainItem, error) {
	item, err := s.monitorService.Update(s.app.Context(), id, input)
	if err != nil {
		logging.Error("%s", i18n.T("log.monitor_update_failed", "Error", err))
	}
	return item, err
}

// SetActive 设置监控域名的启用状态
func (s *MonitorServiceWrapper) SetActive(id int, active bool) (*monitor.MonitoredDomainItem, error) {
	item, err := s.monitorService.SetActive(s.app.Context(), id, active)
	if err != nil {
		logging.Error("%s", i18n.T("log.monitor_setactive_failed", "Error", err))
	}
	return item, err
}

// Delete 删除监控域名
func (s *MonitorServiceWrapper) Delete(id int) error {
	if err := s.monitorService.Delete(s.app.Context(), id); err != nil {
		logging.Error("%s", i18n.T("log.monitor_delete_failed", "Error", err))
		return err
	}
	return nil
}

// CheckNow 立即执行一次检查
func (s *MonitorServiceWrapper) CheckNow(id int) (*monitor.MonitoredDomainItem, error) {
	item, err := s.monitorService.CheckNow(s.app.Context(), id)
	if err != nil {
		logging.Error("%s", i18n.T("log.monitor_checknow_failed", "Error", err))
	}
	return item, err
}

// ListHistory 获取监控域名的检查历史记录（用于趋势图）。
// id: 监控域名ID；days: 查询最近多少天；返回检查历史列表。
func (s *MonitorServiceWrapper) ListHistory(id int, days int) ([]*monitor.MonitorCheckLogItem, error) {
	items, err := s.monitorService.ListHistory(s.app.Context(), id, days)
	if err != nil {
		logging.Error("%s", i18n.T("log.monitor_listhistory_failed", "Error", err))
	}
	return items, err
}

// ServiceStartup 实现 Wails 服务接口
func (s *MonitorServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown 实现 Wails 服务接口
func (s *MonitorServiceWrapper) ServiceShutdown() error {
	return nil
}

// ServiceName 实现 Wails 服务接口
func (s *MonitorServiceWrapper) ServiceName() string {
	return "MonitorService"
}
