package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cnb.cool/dtapp/certflow/internal/dnsprovider"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// DNSProviderServiceWrapper 包装 dnsprovider.DNSProviderService 以适配 Wails v3 服务接口
type DNSProviderServiceWrapper struct {
	dnsService *dnsprovider.DNSProviderService
}

// NewDNSProviderServiceWrapper 创建新的 DNS 提供商服务包装器
func NewDNSProviderServiceWrapper(dnsService *dnsprovider.DNSProviderService) *DNSProviderServiceWrapper {
	return &DNSProviderServiceWrapper{dnsService: dnsService}
}

// convertConfig 将 ent 的 json.RawMessage 转换为 map[string]string
func convertConfig(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// DNSProviderListItem DNS 提供商列表项（前端展示用）
type DNSProviderListItem struct {
	ID           int               `json:"id"`            // 提供商 ID
	Name         string            `json:"name"`          // 提供商名称
	ProviderType string            `json:"provider_type"` // 提供商类型
	Config       map[string]string `json:"config"`        // 配置参数
	IsActive     bool              `json:"is_active"`     // 是否启用
	Comment      string            `json:"comment"`       // 备注
	CreatedAt    string            `json:"created_at"`    // 创建时间
	UpdatedAt    string            `json:"updated_at"`    // 更新时间
}

// CreateDNSProviderRequest 创建 DNS 提供商请求
type CreateDNSProviderRequest struct {
	Name         string            `json:"name"`          // 提供商名称
	ProviderType string            `json:"provider_type"` // 提供商类型
	Config       map[string]string `json:"config"`        // 配置参数
	Comment      string            `json:"comment"`       // 备注
}

// UpdateDNSProviderRequest 更新 DNS 提供商请求
type UpdateDNSProviderRequest struct {
	Name         string            `json:"name,omitempty"`          // 提供商名称
	ProviderType string            `json:"provider_type,omitempty"` // 提供商类型
	Config       map[string]string `json:"config,omitempty"`        // 配置参数
	Comment      string            `json:"comment,omitempty"`       // 备注
}

// ListDNSProviders 获取所有 DNS 提供商
func (s *DNSProviderServiceWrapper) ListDNSProviders() ([]DNSProviderListItem, error) {
	ctx := context.Background()
	providers, err := s.dnsService.List(ctx)
	if err != nil {
		logging.Error("%s", i18n.T("log.dns_list_failed", "Error", err))
		return nil, err
	}

	items := make([]DNSProviderListItem, len(providers))
	for i, p := range providers {
		var configMap map[string]string
		if len(p.Config) > 0 {
			_ = json.Unmarshal(p.Config, &configMap)
		}
		items[i] = DNSProviderListItem{
			ID:           p.ID,
			Name:         p.Name,
			ProviderType: p.ProviderType.String(),
			Config:       convertConfig(p.Config),
			IsActive:     p.IsActive,
			Comment:      p.Comment,
			CreatedAt:    p.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    p.UpdatedAt.Format(time.RFC3339),
		}
	}
	logging.Debug("%s", i18n.T("log.dns_providers_loaded", "Count", len(items), "Data", fmt.Sprintf("%+v", items)))
	return items, nil
}

// CreateDNSProvider 创建 DNS 提供商
func (s *DNSProviderServiceWrapper) CreateDNSProvider(input CreateDNSProviderRequest) (*DNSProviderListItem, error) {
	ctx := context.Background()
	result, err := s.dnsService.Create(ctx, dnsprovider.CreateDNSProviderInput{
		Name:         input.Name,
		ProviderType: input.ProviderType,
		Config:       input.Config,
		Comment:      input.Comment,
	})
	if err != nil {
		logging.Error("%s", i18n.T("log.dns_create_failed", "Error", err))
		return nil, err
	}

	return &DNSProviderListItem{
		ID:           result.ID,
		Name:         result.Name,
		ProviderType: result.ProviderType.String(),
		Config:       convertConfig(result.Config),
		IsActive:     result.IsActive,
		Comment:      result.Comment,
		CreatedAt:    result.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    result.UpdatedAt.Format(time.RFC3339),
	}, nil
}

// UpdateDNSProvider 更新 DNS 提供商
func (s *DNSProviderServiceWrapper) UpdateDNSProvider(id int, input UpdateDNSProviderRequest) (*DNSProviderListItem, error) {
	ctx := context.Background()
	result, err := s.dnsService.Update(ctx, id, dnsprovider.UpdateDNSProviderInput{
		Name:         input.Name,
		ProviderType: input.ProviderType,
		Config:       input.Config,
		Comment:      input.Comment,
	})
	if err != nil {
		logging.Error("%s", i18n.T("log.dns_update_failed", "Error", err))
		return nil, err
	}

	return &DNSProviderListItem{
		ID:           result.ID,
		Name:         result.Name,
		ProviderType: result.ProviderType.String(),
		Config:       convertConfig(result.Config),
		IsActive:     result.IsActive,
		Comment:      result.Comment,
		CreatedAt:    result.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    result.UpdatedAt.Format(time.RFC3339),
	}, nil
}

// SetActive 设置 DNS 提供商的启用状态
func (s *DNSProviderServiceWrapper) SetActive(id int, active bool) (*DNSProviderListItem, error) {
	ctx := context.Background()
	result, err := s.dnsService.SetActive(ctx, id, active)
	if err != nil {
		logging.Error("%s", i18n.T("log.dns_setactive_failed", "Error", err))
		return nil, err
	}
	return &DNSProviderListItem{
		ID:           result.ID,
		Name:         result.Name,
		ProviderType: result.ProviderType.String(),
		Config:       convertConfig(result.Config),
		IsActive:     result.IsActive,
		Comment:      result.Comment,
		CreatedAt:    result.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    result.UpdatedAt.Format(time.RFC3339),
	}, nil
}

// DeleteDNSProvider 删除 DNS 提供商
func (s *DNSProviderServiceWrapper) DeleteDNSProvider(id int) error {
	ctx := context.Background()
	if err := s.dnsService.Delete(ctx, id); err != nil {
		logging.Error("%s", i18n.T("log.dns_delete_failed", "Error", err))
		return err
	}
	return nil
}

// GetDNSProviderTypes 获取支持的 DNS 提供商类型
func (s *DNSProviderServiceWrapper) GetDNSProviderTypes() []string {
	ctx := context.Background()
	return s.dnsService.GetProviderTypes(ctx)
}

// ServiceStartup 实现 Wails 服务接口
func (s *DNSProviderServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown 实现 Wails 服务接口
func (s *DNSProviderServiceWrapper) ServiceShutdown() error {
	return nil
}

// ServiceName 实现 Wails 服务接口
func (s *DNSProviderServiceWrapper) ServiceName() string {
	return "DNSProviderService"
}
