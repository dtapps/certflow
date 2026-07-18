package main

import (
	"context"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/monitor"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// MonitorServiceWrapper 包装 monitor.MonitorService
type MonitorServiceWrapper struct {
	monitorService *monitor.MonitorService
}

// NewMonitorServiceWrapper 创建新的监控服务包装器
func NewMonitorServiceWrapper(monitorService *monitor.MonitorService) *MonitorServiceWrapper {
	return &MonitorServiceWrapper{monitorService: monitorService}
}

// List 获取所有监控域名
func (s *MonitorServiceWrapper) List() ([]*monitor.MonitoredDomainItem, error) {
	return s.monitorService.List(context.Background())
}

// Create 创建监控域名
func (s *MonitorServiceWrapper) Create(input monitor.CreateInput) (*monitor.MonitoredDomainItem, error) {
	return s.monitorService.Create(context.Background(), input)
}

// Update 更新监控域名
func (s *MonitorServiceWrapper) Update(id int, input monitor.UpdateInput) (*monitor.MonitoredDomainItem, error) {
	return s.monitorService.Update(context.Background(), id, input)
}

// SetActive 设置监控域名的启用状态
func (s *MonitorServiceWrapper) SetActive(id int, active bool) (*monitor.MonitoredDomainItem, error) {
	return s.monitorService.SetActive(context.Background(), id, active)
}

// Delete 删除监控域名
func (s *MonitorServiceWrapper) Delete(id int) error {
	return s.monitorService.Delete(context.Background(), id)
}

// CheckNow 立即执行一次检查
func (s *MonitorServiceWrapper) CheckNow(id int) (*monitor.MonitoredDomainItem, error) {
	return s.monitorService.CheckNow(context.Background(), id)
}

// SetUserAgent 设置 User-Agent
func (s *MonitorServiceWrapper) SetUserAgent(ua string) {
	logging.Info(i18n.T("log.monitor.set_user_agent", "UA", ua))
	s.monitorService.SetUserAgent(ua)
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
