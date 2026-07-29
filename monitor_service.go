package main

import (
	"context"

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

// ListHistory 获取监控域名的检查历史记录（用于趋势图）。
// id: 监控域名ID；days: 查询最近多少天；返回检查历史列表。
func (s *MonitorServiceWrapper) ListHistory(id int, days int) ([]*monitor.MonitorCheckLogItem, error) {
	return s.monitorService.ListHistory(context.Background(), id, days)
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
