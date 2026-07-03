package dnsprovider

import (
	"context"
	"encoding/json"
	"fmt"

	"cnb.cool/dtapp/certflow/ent"
	"cnb.cool/dtapp/certflow/ent/dnsprovider"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
)

// DNSProviderService 提供 DNS 提供商管理功能
type DNSProviderService struct {
	db *ent.Client
}

// NewDNSProviderService 创建新的 DNS 提供商服务
func NewDNSProviderService(client *ent.Client) *DNSProviderService {
	return &DNSProviderService{db: client}
}

// CreateDNSProviderInput 创建 DNS 提供商输入
type CreateDNSProviderInput struct {
	Name         string            `json:"name"`          // 提供商名称
	ProviderType string            `json:"provider_type"` // 提供商类型
	Config       map[string]string `json:"config"`        // 配置参数
	IsDefault    bool              `json:"is_default"`    // 是否设为默认
	IsActive     bool              `json:"is_active"`     // 是否启用
	Comment      string            `json:"comment"`       // 备注
}

// UpdateDNSProviderInput 更新 DNS 提供商输入
type UpdateDNSProviderInput struct {
	Name         string            `json:"name,omitempty"`          // 提供商名称
	ProviderType string            `json:"provider_type,omitempty"` // 提供商类型
	Config       map[string]string `json:"config,omitempty"`        // 配置参数
	IsDefault    *bool             `json:"is_default,omitempty"`    // 是否设为默认
	IsActive     *bool             `json:"is_active,omitempty"`     // 是否启用
	Comment      string            `json:"comment,omitempty"`       // 备注
}

// Create 创建新的 DNS 提供商
func (s *DNSProviderService) Create(ctx context.Context, input CreateDNSProviderInput) (*ent.DNSProvider, error) {
	configJSON, err := json.Marshal(input.Config)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("error.dns_config_marshal_failed", "Error", err))
	}
	builder := s.db.DNSProvider.Create().
		SetName(input.Name).
		SetProviderType(dnsprovider.ProviderType(input.ProviderType)).
		SetConfig(configJSON).
		SetIsDefault(input.IsDefault).
		SetIsActive(input.IsActive).
		SetComment(input.Comment)

	result, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("error.dns_create_failed", "Error", err))
	}
	return result, nil
}

// GetByID 根据 ID 获取 DNS 提供商
func (s *DNSProviderService) GetByID(ctx context.Context, id int) (*ent.DNSProvider, error) {
	result, err := s.db.DNSProvider.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf(i18n.T("error.dns_not_found"))
		}
		return nil, fmt.Errorf(i18n.T("error.get_dns_failed", "Error", err))
	}
	return result, nil
}

// List 获取所有 DNS 提供商
func (s *DNSProviderService) List(ctx context.Context) ([]*ent.DNSProvider, error) {
	results, err := s.db.DNSProvider.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("error.list_dns_failed", "Error", err))
	}
	return results, nil
}

// ListActive 获取所有启用的 DNS 提供商
func (s *DNSProviderService) ListActive(ctx context.Context) ([]*ent.DNSProvider, error) {
	results, err := s.db.DNSProvider.Query().
		Where(dnsprovider.IsActiveEQ(true)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("error.list_dns_failed", "Error", err))
	}
	return results, nil
}

// ListByType 按类型获取 DNS 提供商
func (s *DNSProviderService) ListByType(ctx context.Context, providerType string) ([]*ent.DNSProvider, error) {
	results, err := s.db.DNSProvider.Query().
		Where(dnsprovider.ProviderTypeEQ(dnsprovider.ProviderType(providerType))).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("error.list_dns_failed", "Error", err))
	}
	return results, nil
}

// Update 更新 DNS 提供商
func (s *DNSProviderService) Update(ctx context.Context, id int, input UpdateDNSProviderInput) (*ent.DNSProvider, error) {
	logging.Info(i18n.T("log.dns_update_start", "ID", id))
	builder := s.db.DNSProvider.UpdateOneID(id)

	if input.Name != "" {
		builder.SetName(input.Name)
	}
	if input.ProviderType != "" {
		builder.SetProviderType(dnsprovider.ProviderType(input.ProviderType))
	}
	if input.Config != nil {
		configJSON, err := json.Marshal(input.Config)
		if err != nil {
			return nil, fmt.Errorf(i18n.T("error.dns_config_marshal_failed", "Error", err))
		}
		builder.SetConfig(configJSON)
	}
	if input.IsDefault != nil {
		builder.SetIsDefault(*input.IsDefault)
	}
	if input.IsActive != nil {
		builder.SetIsActive(*input.IsActive)
	}
	if input.Comment != "" {
		builder.SetComment(input.Comment)
	}

	result, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf(i18n.T("error.dns_not_found"))
		}
		return nil, fmt.Errorf(i18n.T("error.dns_update_failed", "Error", err))
	}
	return result, nil
}

// Delete 删除 DNS 提供商
func (s *DNSProviderService) Delete(ctx context.Context, id int) error {
	err := s.db.DNSProvider.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf(i18n.T("error.dns_not_found"))
		}
		return fmt.Errorf(i18n.T("error.dns_delete_failed", "Error", err))
	}
	return nil
}

// SetDefault 设置指定 DNS 提供商为默认提供商（会取消其他提供商的默认状态）
func (s *DNSProviderService) SetDefault(ctx context.Context, id int) (*ent.DNSProvider, error) {
	// 先取消所有 DNS 提供商的默认状态
	err := s.db.DNSProvider.Update().
		SetIsDefault(false).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("error.unset_default_dns_failed", "Error", err))
	}

	// 设置指定 DNS 提供商为默认
	result, err := s.db.DNSProvider.UpdateOneID(id).
		SetIsDefault(true).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf(i18n.T("error.dns_not_found"))
		}
		return nil, fmt.Errorf(i18n.T("error.set_default_dns_failed", "Error", err))
	}
	return result, nil
}

// GetProviderTypes 获取支持的 DNS 提供商类型列表
func (s *DNSProviderService) GetProviderTypes(ctx context.Context) []string {
	return []string{
		"cloudflare", "aliyun", "tencentcloud", "huawei",
		"aws", "googlecloud", "baiducloud", "jdcloud",
		"volcengine", "edgeone", "aliesa", "ucloud",
		"westcn", "com35", "rainyun", "todaynic",
		"dnsla", "dns51", "xinnet",
		"porkbun", "namecheap", "godaddy", "gandiv5",
		"dynadot", "azuredns", "digitalocean", "vultr",
		"hetzner", "linode", "ovh", "dnsimple", "ns1",
	}
}
