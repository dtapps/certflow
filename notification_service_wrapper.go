package main

import (
	"context"
	"time"

	"cnb.cool/dtapp/certflow/internal/notification"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// NotificationServiceWrapper 包装 notification.NotificationService 以适配 Wails v3 服务接口
type NotificationServiceWrapper struct {
	notifService *notification.NotificationService
}

// NewNotificationServiceWrapper 创建新的通知服务包装器
func NewNotificationServiceWrapper(notifService *notification.NotificationService) *NotificationServiceWrapper {
	return &NotificationServiceWrapper{notifService: notifService}
}

// NotificationItem 通知列表项（前端展示用）
type NotificationItem struct {
	ID        int    `json:"id"`         // 通知 ID
	Title     string `json:"title"`      // 通知标题
	Body      string `json:"body"`       // 通知内容
	Category  string `json:"category"`   // 通知分类
	Read      bool   `json:"read"`       // 是否已读
	CreatedAt string `json:"created_at"` // 创建时间
}

// SendTestNotification 发送测试通知
func (s *NotificationServiceWrapper) SendTestNotification() error {
	return s.notifService.SendCertApplied("test.example.com", "Let's Encrypt")
}

// CheckPermission 检查通知权限是否已授权
func (s *NotificationServiceWrapper) CheckPermission() bool {
	return s.notifService.CheckPermission()
}

// RequestPermission 请求通知权限
func (s *NotificationServiceWrapper) RequestPermission() bool {
	return s.notifService.RequestPermission()
}

// ListNotifications 获取通知列表
func (s *NotificationServiceWrapper) ListNotifications(limit int, offset int) ([]NotificationItem, error) {
	ctx := context.Background()
	items, err := s.notifService.ListNotifications(ctx, limit, offset)
	if err != nil {
		return nil, err
	}

	result := make([]NotificationItem, len(items))
	for i, item := range items {
		result[i] = NotificationItem{
			ID:        item.ID,
			Title:     item.Title,
			Body:      item.Body,
			Category:  item.Category.String(),
			Read:      item.Read,
			CreatedAt: item.CreatedAt.Format(time.DateTime),
		}
	}
	return result, nil
}

// CountUnread 获取未读通知数量
func (s *NotificationServiceWrapper) CountUnread() (int, error) {
	ctx := context.Background()
	return s.notifService.CountUnread(ctx)
}

// MarkAsRead 标记通知为已读
func (s *NotificationServiceWrapper) MarkAsRead(id int) error {
	ctx := context.Background()
	return s.notifService.MarkAsRead(ctx, id)
}

// MarkAllAsRead 标记所有通知为已读
func (s *NotificationServiceWrapper) MarkAllAsRead() error {
	ctx := context.Background()
	return s.notifService.MarkAllAsRead(ctx)
}

// DeleteNotification 删除通知
func (s *NotificationServiceWrapper) DeleteNotification(id int) error {
	ctx := context.Background()
	return s.notifService.DeleteNotification(ctx, id)
}

// ClearAllNotifications 清空所有通知
func (s *NotificationServiceWrapper) ClearAllNotifications() error {
	ctx := context.Background()
	return s.notifService.ClearAllNotifications(ctx)
}

// ServiceStartup 实现 Wails 服务接口
func (s *NotificationServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return s.notifService.ServiceStartup(ctx, options)
}

// ServiceShutdown 实现 Wails 服务接口
func (s *NotificationServiceWrapper) ServiceShutdown() error {
	return s.notifService.ServiceShutdown()
}

// ServiceName 实现 Wails 服务接口
func (s *NotificationServiceWrapper) ServiceName() string {
	return s.notifService.ServiceName()
}
