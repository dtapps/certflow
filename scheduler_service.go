package main

import (
	"context"
	"time"

	"cnb.cool/dtapp/certflow/internal/scheduler"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// SchedulerServiceWrapper 包装 scheduler.Scheduler 以适配 Wails v3 服务接口
type SchedulerServiceWrapper struct {
	scheduler *scheduler.Scheduler
}

// NewSchedulerServiceWrapper 创建新的调度器服务包装器
func NewSchedulerServiceWrapper(scheduler *scheduler.Scheduler) *SchedulerServiceWrapper {
	return &SchedulerServiceWrapper{scheduler: scheduler}
}

// RenewalLogItem 续期日志项（前端展示用）
type RenewalLogItem struct {
	ID            int    `json:"id"`             // 日志 ID
	CertificateID int    `json:"certificate_id"` // 关联证书 ID
	Domain        string `json:"domain"`         // 域名
	Status        string `json:"status"`         // 续期状态
	ErrorMessage  string `json:"error_message"`  // 错误信息
	CertContent   string `json:"cert_content"`   // 新证书 PEM 内容
	KeyContent    string `json:"key_content"`    // 新私钥 PEM 内容
	AttemptAt     string `json:"attempt_at"`     // 尝试时间
	CompletedAt   string `json:"completed_at"`   // 完成时间
	CreatedAt     string `json:"created_at"`     // 创建时间
}

// GetRecentRenewalLogs 获取最近的续期日志
func (s *SchedulerServiceWrapper) GetRecentRenewalLogs(limit int) ([]RenewalLogItem, error) {
	ctx := context.Background()
	logs, err := s.scheduler.GetRecentRenewalLogs(ctx, limit)
	if err != nil {
		return nil, err
	}

	items := make([]RenewalLogItem, len(logs))
	for i, l := range logs {
		completedAt := ""
		if !l.CompletedAt.IsZero() {
			completedAt = l.CompletedAt.Format(time.DateTime)
		}
		certID := 0
		domain := ""
		if l.Edges.Certificate != nil {
			certID = l.Edges.Certificate.ID
			domain = l.Edges.Certificate.Domain
		}
		items[i] = RenewalLogItem{
			ID:            l.ID,
			CertificateID: certID,
			Domain:        domain,
			Status:        l.Status.String(),
			ErrorMessage:  l.ErrorMessage,
			CertContent:   l.CertContent,
			KeyContent:    l.KeyContent,
			AttemptAt:     l.AttemptAt.Format(time.DateTime),
			CompletedAt:   completedAt,
			CreatedAt:     l.CreatedAt.Format(time.DateTime),
		}
	}
	return items, nil
}

// GetRenewalLogs 获取指定证书的续期日志
func (s *SchedulerServiceWrapper) GetRenewalLogs(certID int) ([]RenewalLogItem, error) {
	ctx := context.Background()
	logs, err := s.scheduler.GetRenewalLogs(ctx, certID)
	if err != nil {
		return nil, err
	}

	items := make([]RenewalLogItem, len(logs))
	for i, l := range logs {
		completedAt := ""
		if !l.CompletedAt.IsZero() {
			completedAt = l.CompletedAt.Format(time.DateTime)
		}
		certID := 0
		if l.Edges.Certificate != nil {
			certID = l.Edges.Certificate.ID
		}
		items[i] = RenewalLogItem{
			ID:            l.ID,
			CertificateID: certID,
			Status:        l.Status.String(),
			ErrorMessage:  l.ErrorMessage,
			CertContent:   l.CertContent,
			KeyContent:    l.KeyContent,
			AttemptAt:     l.AttemptAt.Format(time.DateTime),
			CompletedAt:   completedAt,
			CreatedAt:     l.CreatedAt.Format(time.DateTime),
		}
	}
	return items, nil
}

// RunRenewalNow 立即执行续期任务
func (s *SchedulerServiceWrapper) RunRenewalNow() {
	s.scheduler.RunRenewalNow()
}

// RunExpiryCheckNow 立即执行过期检查
func (s *SchedulerServiceWrapper) RunExpiryCheckNow() {
	s.scheduler.RunExpiryCheckNow()
}

// ServiceStartup 实现 Wails 服务接口
func (s *SchedulerServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return s.scheduler.Start(ctx)
}

// ServiceShutdown 实现 Wails 服务接口
func (s *SchedulerServiceWrapper) ServiceShutdown() error {
	return s.scheduler.Stop()
}

// ServiceName 实现 Wails 服务接口
func (s *SchedulerServiceWrapper) ServiceName() string {
	return "SchedulerService"
}
