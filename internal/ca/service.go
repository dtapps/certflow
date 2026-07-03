package ca

import (
	"context"
	"fmt"

	"cnb.cool/dtapp/certflow/ent"
	"cnb.cool/dtapp/certflow/ent/ca"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/settings"
)

// CAService 提供 CA 管理功能
type CAService struct {
	db      *ent.Client
	dataDir string
}

// NewCAService 创建新的 CA 服务
func NewCAService(client *ent.Client, dataDir string) *CAService {
	return &CAService{db: client, dataDir: dataDir}
}

// CreateCAInput 创建 CA 输入参数
type CreateCAInput struct {
	Name         string `json:"name"`          // CA 名称
	DirectoryURL string `json:"directory_url"` // ACME 目录 URL
	AccountEmail string `json:"account_email"` // 注册邮箱
	IsDefault    bool   `json:"is_default"`    // 是否设为默认
	IsActive     bool   `json:"is_active"`     // 是否启用
}

// UpdateCAInput 更新 CA 输入参数
type UpdateCAInput struct {
	Name         string `json:"name,omitempty"`          // CA 名称
	DirectoryURL string `json:"directory_url,omitempty"` // ACME 目录 URL
	AccountEmail string `json:"account_email,omitempty"` // 注册邮箱
	IsDefault    *bool  `json:"is_default,omitempty"`    // 是否设为默认
	IsActive     *bool  `json:"is_active,omitempty"`     // 是否启用
}

// Create 创建新的 CA
func (s *CAService) Create(ctx context.Context, input CreateCAInput) (*ent.CA, error) {
	builder := s.db.CA.Create().
		SetName(input.Name).
		SetDirectoryURL(input.DirectoryURL).
		SetAccountEmail(input.AccountEmail).
		SetIsDefault(input.IsDefault).
		SetIsActive(input.IsActive)

	result, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("error.ca_create_failed", "Error", err))
	}
	return result, nil
}

// GetByID 根据 ID 获取 CA
func (s *CAService) GetByID(ctx context.Context, id int) (*ent.CA, error) {
	result, err := s.db.CA.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf(i18n.T("error.ca_not_found"))
		}
		return nil, fmt.Errorf(i18n.T("error.get_ca_failed", "Error", err))
	}
	return result, nil
}

// List 获取所有 CA
func (s *CAService) List(ctx context.Context) ([]*ent.CA, error) {
	results, err := s.db.CA.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("error.list_ca_failed", "Error", err))
	}
	return results, nil
}

// ListActive 获取所有启用的 CA
func (s *CAService) ListActive(ctx context.Context) ([]*ent.CA, error) {
	results, err := s.db.CA.Query().
		Where(ca.IsActiveEQ(true)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("error.list_ca_failed", "Error", err))
	}
	return results, nil
}

// Update 更新 CA
func (s *CAService) Update(ctx context.Context, id int, input UpdateCAInput) (*ent.CA, error) {
	logging.Info(i18n.T("log.ca_update_start", "ID", id))
	builder := s.db.CA.UpdateOneID(id)

	if input.Name != "" {
		builder.SetName(input.Name)
	}
	if input.DirectoryURL != "" {
		builder.SetDirectoryURL(input.DirectoryURL)
	}
	if input.AccountEmail != "" {
		builder.SetAccountEmail(input.AccountEmail)
	}
	if input.IsDefault != nil {
		builder.SetIsDefault(*input.IsDefault)
	}
	if input.IsActive != nil {
		builder.SetIsActive(*input.IsActive)
	}

	result, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf(i18n.T("error.ca_not_found"))
		}
		return nil, fmt.Errorf(i18n.T("error.ca_update_failed", "Error", err))
	}
	return result, nil
}

// Delete 删除 CA
func (s *CAService) Delete(ctx context.Context, id int) error {
	err := s.db.CA.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf(i18n.T("error.ca_not_found"))
		}
		return fmt.Errorf(i18n.T("error.ca_delete_failed", "Error", err))
	}
	return nil
}

// SetDefault 设置指定 CA 为默认 CA（会取消其他 CA 的默认状态）
func (s *CAService) SetDefault(ctx context.Context, id int) (*ent.CA, error) {
	// 先取消所有 CA 的默认状态
	err := s.db.CA.Update().
		SetIsDefault(false).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("error.unset_default_ca_failed", "Error", err))
	}

	// 设置指定 CA 为默认
	result, err := s.db.CA.UpdateOneID(id).
		SetIsDefault(true).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf(i18n.T("error.ca_not_found"))
		}
		return nil, fmt.Errorf(i18n.T("error.set_default_ca_failed", "Error", err))
	}
	return result, nil
}

// SeedDefaults 插入默认 CA（仅在首次启动时执行一次）
func (s *CAService) SeedDefaults(ctx context.Context) error {
	logging.Info(i18n.T("log.ca_seed_start"))
	// 检查 settings 中的 seeded 标记
	settingsSvc, err := settings.NewService(s.dataDir)
	if err != nil {
		return err
	}
	if settingsSvc.IsSeeded() {
		logging.Debug(i18n.T("log.ca_seed_skip"))
		return nil
	}

	defaults := []CreateCAInput{
		{
			Name:         "Let's Encrypt",
			DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
			AccountEmail: "",
			IsDefault:    true,
			IsActive:     true,
		},
		{
			Name:         "Buypass",
			DirectoryURL: "https://api.buypass.com/acme/directory",
			AccountEmail: "",
			IsDefault:    false,
			IsActive:     true,
		},
		{
			Name:         "ZeroSSL",
			DirectoryURL: "https://acme.zerossl.com/v2/DV90/directory",
			AccountEmail: "",
			IsDefault:    false,
			IsActive:     true,
		},
		{
			Name:         "Let's Encrypt (测试环境)",
			DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
			AccountEmail: "",
			IsDefault:    false,
			IsActive:     true,
		},
	}

	for _, input := range defaults {
		if _, err := s.Create(ctx, input); err != nil {
			return err
		}
	}

	// 标记已预置，防止重启后重复插入
	return settingsSvc.MarkSeeded()
}

// TestConnection 测试 CA 连接（占位，后续实现 ACME 目录验证）
func (s *CAService) TestConnection(ctx context.Context, id int) (string, error) {
	logging.Info(i18n.T("log.ca_test_connection", "ID", id))
	caEntity, err := s.GetByID(ctx, id)
	if err != nil {
		return "", err
	}

	// TODO: 实现 ACME 目录 URL 验证
	return fmt.Sprintf(i18n.T("error.ca_test_connection_success", "Name", caEntity.Name, "URL", caEntity.DirectoryURL)), nil
}
