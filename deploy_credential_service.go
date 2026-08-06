package main

import (
	"context"
	"time"

	"cnb.cool/dtapp/certflow/internal/deploycredential"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// DeployCredentialServiceWrapper 包装 deploycredential.Service 以适配 Wails v3 服务接口
type DeployCredentialServiceWrapper struct {
	app               *application.App
	credentialService *deploycredential.Service
}

// NewDeployCredentialServiceWrapper 创建新的部署凭证服务包装器
func NewDeployCredentialServiceWrapper(credentialService *deploycredential.Service) *DeployCredentialServiceWrapper {
	return &DeployCredentialServiceWrapper{credentialService: credentialService}
}

// SetApp 设置 app 引用（用于获取应用生命周期）
func (s *DeployCredentialServiceWrapper) SetApp(app *application.App) {
	s.app = app
}

// DeployCredentialListItem 部署凭证列表项（前端展示用）
type DeployCredentialListItem struct {
	ID           int               `json:"id"`            // 部署凭证 ID
	Name         string            `json:"name"`          // 凭证名称
	ProviderType string            `json:"provider_type"` // 提供商类型（见 ent/schema/provider_types.go 的 DeployProviderTypes）
	Config       map[string]string `json:"config"`        // 配置参数（API 密钥等）
	IsActive     bool              `json:"is_active"`     // 是否启用
	Comment      string            `json:"comment"`       // 备注
	CreatedAt    string            `json:"created_at"`    // 创建时间
	UpdatedAt    string            `json:"updated_at"`    // 更新时间
}

// CreateDeployCredentialRequest 创建部署凭证请求
type CreateDeployCredentialRequest struct {
	Name         string            `json:"name"`          // 凭证名称
	ProviderType string            `json:"provider_type"` // 提供商类型
	Config       map[string]string `json:"config"`        // 配置参数
	Comment      string            `json:"comment"`       // 备注
}

// UpdateDeployCredentialRequest 更新部署凭证请求
type UpdateDeployCredentialRequest struct {
	Name         string            `json:"name"`          // 凭证名称
	ProviderType string            `json:"provider_type"` // 提供商类型
	Config       map[string]string `json:"config"`        // 配置参数
	Comment      string            `json:"comment"`       // 备注
}

// ListDeployCredentials 获取所有部署凭证
func (s *DeployCredentialServiceWrapper) ListDeployCredentials() ([]DeployCredentialListItem, error) {
	ctx := s.app.Context()
	list, err := s.credentialService.List(ctx)
	if err != nil {
		logging.Error("%s", i18n.T("log.deploy_cred_list_failed", "Error", err))
		return nil, err
	}

	items := make([]DeployCredentialListItem, len(list))
	for i, item := range list {
		items[i] = DeployCredentialListItem{
			ID:           item.ID,
			Name:         item.Name,
			ProviderType: item.ProviderType,
			Config:       item.Config,
			IsActive:     item.IsActive,
			Comment:      item.Comment,
			CreatedAt:    item.CreatedAt,
			UpdatedAt:    item.UpdatedAt,
		}
	}
	return items, nil
}

// GetDeployCredential 获取单个部署凭证
func (s *DeployCredentialServiceWrapper) GetDeployCredential(id int) (*DeployCredentialListItem, error) {
	ctx := s.app.Context()
	item, err := s.credentialService.GetByID(ctx, id)
	if err != nil {
		logging.Error("%s", i18n.T("log.deploy_cred_get_failed", "Error", err))
		return nil, err
	}

	return &DeployCredentialListItem{
		ID:           item.ID,
		Name:         item.Name,
		ProviderType: item.ProviderType,
		Config:       item.Config,
		IsActive:     item.IsActive,
		Comment:      item.Comment,
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}, nil
}

// CreateDeployCredential 创建部署凭证
func (s *DeployCredentialServiceWrapper) CreateDeployCredential(input CreateDeployCredentialRequest) (*DeployCredentialListItem, error) {
	ctx := s.app.Context()
	result, err := s.credentialService.Create(ctx, deploycredential.CreateDeployCredentialInput{
		Name:         input.Name,
		ProviderType: input.ProviderType,
		Config:       input.Config,
		Comment:      input.Comment,
	})
	if err != nil {
		logging.Error("%s", i18n.T("log.deploy_cred_create_failed", "Error", err))
		return nil, err
	}

	return &DeployCredentialListItem{
		ID:           result.ID,
		Name:         result.Name,
		ProviderType: result.ProviderType,
		Config:       result.Config,
		IsActive:     result.IsActive,
		Comment:      result.Comment,
		CreatedAt:    result.CreatedAt,
		UpdatedAt:    result.UpdatedAt,
	}, nil
}

// UpdateDeployCredential 更新部署凭证
func (s *DeployCredentialServiceWrapper) UpdateDeployCredential(id int, input UpdateDeployCredentialRequest) (*DeployCredentialListItem, error) {
	ctx := s.app.Context()
	result, err := s.credentialService.Update(ctx, id, deploycredential.UpdateDeployCredentialInput{
		Name:         input.Name,
		ProviderType: input.ProviderType,
		Config:       input.Config,
		Comment:      input.Comment,
	})
	if err != nil {
		logging.Error("%s", i18n.T("log.deploy_cred_update_failed", "Error", err))
		return nil, err
	}

	return &DeployCredentialListItem{
		ID:           result.ID,
		Name:         result.Name,
		ProviderType: result.ProviderType,
		Config:       result.Config,
		IsActive:     result.IsActive,
		Comment:      result.Comment,
		CreatedAt:    result.CreatedAt,
		UpdatedAt:    result.UpdatedAt,
	}, nil
}

// SetActive 设置部署凭证的启用状态
func (s *DeployCredentialServiceWrapper) SetActive(id int, active bool) (*DeployCredentialListItem, error) {
	ctx := s.app.Context()
	result, err := s.credentialService.SetActive(ctx, id, active)
	if err != nil {
		logging.Error("%s", i18n.T("log.deploy_cred_setactive_failed", "Error", err))
		return nil, err
	}
	return &DeployCredentialListItem{
		ID:           result.ID,
		Name:         result.Name,
		ProviderType: result.ProviderType,
		Config:       result.Config,
		IsActive:     result.IsActive,
		Comment:      result.Comment,
		CreatedAt:    result.CreatedAt,
		UpdatedAt:    result.UpdatedAt,
	}, nil
}

// DeleteDeployCredential 删除部署凭证
func (s *DeployCredentialServiceWrapper) DeleteDeployCredential(id int) error {
	ctx := s.app.Context()
	if err := s.credentialService.Delete(ctx, id); err != nil {
		logging.Error("%s", i18n.T("log.deploy_cred_delete_failed", "Error", err))
		return err
	}
	return nil
}

// ServiceStartup 实现 Wails 服务接口
func (s *DeployCredentialServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown 实现 Wails 服务接口
func (s *DeployCredentialServiceWrapper) ServiceShutdown() error {
	return nil
}

// ServiceName 实现 Wails 服务接口
func (s *DeployCredentialServiceWrapper) ServiceName() string {
	return "DeployCredentialService"
}

// 确保 time 包被使用
var _ = time.Now
