package main

import (
	"context"
	"time"

	"cnb.cool/dtapp/certflow/internal/ca"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// CAServiceWrapper 包装 ca.CAService 以适配 Wails v3 服务接口
type CAServiceWrapper struct {
	caService *ca.CAService
}

// NewCAServiceWrapper 创建新的 CA 服务包装器
func NewCAServiceWrapper(caService *ca.CAService) *CAServiceWrapper {
	return &CAServiceWrapper{caService: caService}
}

// CAListItem CA 列表项（前端展示用）
type CAListItem struct {
	ID           int    `json:"id"`            // CA 配置 ID
	Name         string `json:"name"`          // CA 名称
	DirectoryURL string `json:"directory_url"` // ACME 目录 URL
	AccountEmail string `json:"account_email"` // 注册邮箱
	IsActive     bool   `json:"is_active"`     // 是否启用
	CreatedAt    string `json:"created_at"`    // 创建时间
	UpdatedAt    string `json:"updated_at"`    // 更新时间
}

// CreateCACreateRequest 创建 CA 请求
type CreateCACreateRequest struct {
	Name         string `json:"name"`          // CA 名称
	DirectoryURL string `json:"directory_url"` // ACME 目录 URL
	AccountEmail string `json:"account_email"` // 注册邮箱
	IsActive     bool   `json:"is_active"`     // 是否启用
}

// CAUpdateRequest 更新 CA 请求
type CAUpdateRequest struct {
	Name         string `json:"name,omitempty"`          // CA 名称
	DirectoryURL string `json:"directory_url,omitempty"` // ACME 目录 URL
	AccountEmail string `json:"account_email,omitempty"` // 注册邮箱
	IsActive     *bool  `json:"is_active,omitempty"`     // 是否启用
}

// ListCA 获取所有 CA
func (s *CAServiceWrapper) ListCA() ([]CAListItem, error) {
	ctx := context.Background()
	cas, err := s.caService.List(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]CAListItem, len(cas))
	for i, c := range cas {
		items[i] = CAListItem{
			ID:           c.ID,
			Name:         c.Name,
			DirectoryURL: c.DirectoryURL,
			AccountEmail: c.AccountEmail,
			IsActive:     c.IsActive,
			CreatedAt:    c.CreatedAt.Format(time.DateTime),
			UpdatedAt:    c.UpdatedAt.Format(time.DateTime),
		}
	}
	return items, nil
}

// CreateCA 创建 CA
func (s *CAServiceWrapper) CreateCA(input CreateCACreateRequest) (*CAListItem, error) {
	ctx := context.Background()
	result, err := s.caService.Create(ctx, ca.CreateCAInput{
		Name:         input.Name,
		DirectoryURL: input.DirectoryURL,
		AccountEmail: input.AccountEmail,
		IsActive:     input.IsActive,
	})
	if err != nil {
		return nil, err
	}

	return &CAListItem{
		ID:           result.ID,
		Name:         result.Name,
		DirectoryURL: result.DirectoryURL,
		AccountEmail: result.AccountEmail,
		IsActive:     result.IsActive,
		CreatedAt:    result.CreatedAt.Format(time.DateTime),
		UpdatedAt:    result.UpdatedAt.Format(time.DateTime),
	}, nil
}

// UpdateCA 更新 CA
func (s *CAServiceWrapper) UpdateCA(id int, input CAUpdateRequest) (*CAListItem, error) {
	ctx := context.Background()
	result, err := s.caService.Update(ctx, id, ca.UpdateCAInput{
		Name:         input.Name,
		DirectoryURL: input.DirectoryURL,
		AccountEmail: input.AccountEmail,
		IsActive:     input.IsActive,
	})
	if err != nil {
		return nil, err
	}

	return &CAListItem{
		ID:           result.ID,
		Name:         result.Name,
		DirectoryURL: result.DirectoryURL,
		AccountEmail: result.AccountEmail,
		IsActive:     result.IsActive,
		CreatedAt:    result.CreatedAt.Format(time.DateTime),
		UpdatedAt:    result.UpdatedAt.Format(time.DateTime),
	}, nil
}

// DeleteCA 删除 CA
func (s *CAServiceWrapper) DeleteCA(id int) error {
	ctx := context.Background()
	return s.caService.Delete(ctx, id)
}

// TestCAConnection 测试 CA 连接
func (s *CAServiceWrapper) TestCAConnection(id int) (string, error) {
	ctx := context.Background()
	return s.caService.TestConnection(ctx, id)
}

// ServiceStartup 实现 Wails 服务接口
func (s *CAServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown 实现 Wails 服务接口
func (s *CAServiceWrapper) ServiceShutdown() error {
	return nil
}

// ServiceName 实现 Wails 服务接口
func (s *CAServiceWrapper) ServiceName() string {
	return "CAService"
}

// 确保 CAServiceWrapper 实现服务接口
var _ application.ServiceName = (*CAServiceWrapper)(nil)
var _ application.ServiceStartup = (*CAServiceWrapper)(nil)
var _ application.ServiceShutdown = (*CAServiceWrapper)(nil)
