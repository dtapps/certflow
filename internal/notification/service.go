package notification

import (
	"context"
	"fmt"

	"cnb.cool/dtapp/certflow/ent"
	entnotification "cnb.cool/dtapp/certflow/ent/notification"
	"cnb.cool/dtapp/certflow/internal/events"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// NotificationService 提供系统通知功能
type NotificationService struct {
	notifService *notifications.NotificationService
	app          *application.App
	db           *ent.Client
}

// SetApp 设置 Wails 应用引用（用于向前端发送事件）
func (s *NotificationService) SetApp(app *application.App) {
	s.app = app
}

// SetDB 设置数据库客户端
func (s *NotificationService) SetDB(db *ent.Client) {
	s.db = db
}

// NewNotificationService 创建新的通知服务
func NewNotificationService() *NotificationService {
	ns := notifications.New()
	return &NotificationService{
		notifService: ns,
	}
}

// NotificationOption 通知选项
type NotificationOption struct {
	Title    string
	Subtitle string
	Body     string
	Category string
	Data     map[string]any
	SkipDB   bool // 跳过数据库保存（用于测试通知）
}

// Init 初始化通知服务（需要在 Wails 应用启动时调用）
func (s *NotificationService) Init() error {
	// 请求通知权限
	authorized, err := s.notifService.RequestNotificationAuthorization()
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.request_notification_auth_failed", "Error", err))
	}
	if !authorized {
		return fmt.Errorf("%s", i18n.T("error.notification_permission_denied"))
	}
	return nil
}

// GetService 获取底层通知服务（用于注册到 Wails）
func (s *NotificationService) GetService() *notifications.NotificationService {
	return s.notifService
}

// SendNotification 发送系统通知
func (s *NotificationService) SendNotification(opt NotificationOption) error {
	logging.Debug(i18n.T("log.notification_send", "Title", opt.Title, "Category", opt.Category))
	id, _ := uuid.NewV7()
	options := notifications.NotificationOptions{
		ID:    id.String(),
		Title: opt.Title,
		Body:  opt.Body,
		Data:  opt.Data,
	}

	if opt.Subtitle != "" {
		options.Subtitle = opt.Subtitle
	}

	if opt.Category != "" {
		options.CategoryID = opt.Category
	}

	err := s.notifService.SendNotification(options)

	// 保存到数据库（测试通知跳过）
	if s.db != nil && !opt.SkipDB {
		_, dbErr := s.db.Notification.Create().
			SetTitle(opt.Title).
			SetBody(opt.Body).
			SetCategory(entnotification.Category(opt.Category)).
			Save(context.Background())
		if dbErr != nil {
			logging.Error(i18n.T("log.save_notification_db_failed", "Error", dbErr))
		}
	}

	// 向前端发送通知事件
	if s.app != nil {
		if ok := s.app.Event.Emit(events.EventNotification, events.NotificationPayload{
			Title:    opt.Title,
			Body:     opt.Body,
			Category: opt.Category,
		}); !ok {
			logging.Warn("%s", i18n.T("error.notification_failed"))
		}
	}

	return err
}

// ListNotifications 获取通知列表
func (s *NotificationService) ListNotifications(ctx context.Context, limit, offset int) ([]*ent.Notification, error) {
	if s.db == nil {
		return nil, fmt.Errorf("%s", i18n.T("error.db_not_initialized"))
	}
	return s.db.Notification.Query().
		Order(ent.Desc("created_at")).
		Limit(limit).
		Offset(offset).
		All(ctx)
}

// CountUnread 获取未读通知数量
func (s *NotificationService) CountUnread(ctx context.Context) (int, error) {
	if s.db == nil {
		return 0, fmt.Errorf("%s", i18n.T("error.db_not_initialized"))
	}
	return s.db.Notification.Query().
		Where(entnotification.Read(false)).
		Count(ctx)
}

// MarkAsRead 标记通知为已读
func (s *NotificationService) MarkAsRead(ctx context.Context, id int) error {
	if s.db == nil {
		return fmt.Errorf("%s", i18n.T("error.db_not_initialized"))
	}
	return s.db.Notification.UpdateOneID(id).
		SetRead(true).
		Exec(ctx)
}

// MarkAllAsRead 标记所有通知为已读
func (s *NotificationService) MarkAllAsRead(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("%s", i18n.T("error.db_not_initialized"))
	}
	return s.db.Notification.Update().
		SetRead(true).
		Exec(ctx)
}

// DeleteNotification 删除通知
func (s *NotificationService) DeleteNotification(ctx context.Context, id int) error {
	if s.db == nil {
		return fmt.Errorf("%s", i18n.T("error.db_not_initialized"))
	}
	return s.db.Notification.DeleteOneID(id).Exec(ctx)
}

// ClearAllNotifications 清空所有通知
func (s *NotificationService) ClearAllNotifications(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("%s", i18n.T("error.db_not_initialized"))
	}
	_, err := s.db.Notification.Delete().Exec(ctx)
	return err
}

// SendCertApplied 发送证书申请成功通知
func (s *NotificationService) SendCertApplied(domain, issuer string) error {
	return s.SendNotification(NotificationOption{
		Title:    i18n.T("notification.cert_applied.title"),
		Body:     i18n.T("notification.cert_applied.body", "Domain", domain, "Issuer", issuer),
		Category: entnotification.CategoryCert.String(),
	})
}

// SendCertRenewed 发送证书续期成功通知
func (s *NotificationService) SendCertRenewed(domain, issuer, notAfter string) error {
	return s.SendNotification(NotificationOption{
		Title:    i18n.T("notification.cert_renewed.title"),
		Body:     i18n.T("notification.cert_renewed.body", "Domain", domain, "Issuer", issuer, "ValidUntil", notAfter),
		Category: entnotification.CategoryCert.String(),
	})
}

// SendCertRevoked 发送证书撤销通知
func (s *NotificationService) SendCertRevoked(domain string) error {
	return s.SendNotification(NotificationOption{
		Title:    i18n.T("notification.cert_revoked.title"),
		Body:     i18n.T("notification.cert_revoked.body", "Domain", domain),
		Category: entnotification.CategoryCert.String(),
	})
}

// SendCertExpiring 发送证书即将过期警告
func (s *NotificationService) SendCertExpiring(domain string, daysLeft int) error {
	subtitle := ""
	if daysLeft <= 7 {
		subtitle = i18n.T("notification.cert_expiring.subtitle")
	}
	return s.SendNotification(NotificationOption{
		Title:    i18n.T("notification.cert_expiring.title"),
		Subtitle: subtitle,
		Body:     i18n.T("notification.cert_expiring.body", "Domain", domain, "Days", daysLeft),
		Category: entnotification.CategoryCert.String(),
	})
}

// SendCertApplyFailed 发送证书申请失败通知
func (s *NotificationService) SendCertApplyFailed(domain, reason string) error {
	return s.SendNotification(NotificationOption{
		Title:    i18n.T("notification.cert_apply_failed.title"),
		Body:     i18n.T("notification.cert_apply_failed.body", "Domain", domain, "Error", reason),
		Category: entnotification.CategoryCert.String(),
	})
}

// SendCertRenewFailed 发送证书续期失败通知
func (s *NotificationService) SendCertRenewFailed(domain, reason string) error {
	return s.SendNotification(NotificationOption{
		Title:    i18n.T("notification.cert_renew_failed.title"),
		Body:     i18n.T("notification.cert_renew_failed.body", "Domain", domain, "Error", reason),
		Category: entnotification.CategoryCert.String(),
	})
}

// SendDeploySuccess 发送证书部署成功通知
func (s *NotificationService) SendDeploySuccess(domain, target string) error {
	return s.SendNotification(NotificationOption{
		Title:    i18n.T("notification.deploy_success.title"),
		Body:     i18n.T("notification.deploy_success.body", "Domain", domain, "Target", target),
		Category: entnotification.CategoryDeploy.String(),
	})
}

// SendDeployFailed 发送证书部署失败通知
func (s *NotificationService) SendDeployFailed(domain, target, reason string) error {
	return s.SendNotification(NotificationOption{
		Title:    i18n.T("notification.deploy_failed.title"),
		Body:     i18n.T("notification.deploy_failed.body", "Domain", domain, "Target", target, "Error", reason),
		Category: entnotification.CategoryDeploy.String(),
	})
}

// OnNotificationResponse 设置通知响应回调
func (s *NotificationService) OnNotificationResponse(callback func(result notifications.NotificationResult)) {
	s.notifService.OnNotificationResponse(callback)
}

// RegisterNotificationCategory 注册通知类别
func (s *NotificationService) RegisterNotificationCategory(category notifications.NotificationCategory) error {
	return s.notifService.RegisterNotificationCategory(category)
}

// ServiceStartup 服务启动（用于 Wails 服务注册）
func (s *NotificationService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return s.notifService.ServiceStartup(ctx, options)
}

// CheckPermission 检查通知权限是否已授权
func (s *NotificationService) CheckPermission() bool {
	authorized, err := s.notifService.CheckNotificationAuthorization()
	if err != nil {
		logging.Error(i18n.T("log.check_notification_permission_failed", "Error", err))
		return false
	}
	return authorized
}

// RequestPermission 请求通知权限（在用户开启通知时调用）
func (s *NotificationService) RequestPermission() bool {
	authorized, err := s.notifService.RequestNotificationAuthorization()
	if err != nil {
		logging.Error(i18n.T("log.request_notification_auth_failed", "Error", err))
		return false
	}
	if authorized {
		logging.Info(i18n.T("log.notification_authorized"))
	} else {
		logging.Warn(i18n.T("log.notification_denied"))
	}
	return authorized
}

// ServiceShutdown 服务关闭
func (s *NotificationService) ServiceShutdown() error {
	return s.notifService.ServiceShutdown()
}

// ServiceName 服务名称
func (s *NotificationService) ServiceName() string {
	return s.notifService.ServiceName()
}
