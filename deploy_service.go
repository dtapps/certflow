package main

import (
	"context"
	"encoding/json"
	"time"

	"cnb.cool/dtapp/certflow/internal/deploy"
	"cnb.cool/dtapp/certflow/internal/ent"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// DeployServiceWrapper 包装 deploy.DeployService 以适配 Wails v3 服务接口
type DeployServiceWrapper struct {
	deployService *deploy.DeployService
}

// NewDeployServiceWrapper 创建部署服务包装器
func NewDeployServiceWrapper(deployService *deploy.DeployService) *DeployServiceWrapper {
	return &DeployServiceWrapper{deployService: deployService}
}

// DeployTargetListItem 部署目标列表项（前端展示用）
type DeployTargetListItem struct {
	ID                   int               `json:"id"`                     // 部署目标 ID
	Name                 string            `json:"name"`                   // 部署目标名称
	ProviderType         string            `json:"provider_type"`          // 云厂商类型（aliyun/tencentcloud/huawei）
	DeployService        string            `json:"deploy_service"`         // 部署服务（cdn/dcdn/elb/waf 等）
	Config               map[string]string `json:"config"`                 // 部署参数（region、域名等，按厂商/服务不同）
	Region               string            `json:"region"`                 // 区域代码
	CredentialSource     string            `json:"credential_source"`      // 凭证来源（dns_provider/deploy_credential）
	DNSProviderID        *int              `json:"dns_provider_id"`        // 关联 DNS 凭证 ID（可为空）
	DNSProviderName      string            `json:"dns_provider_name"`      // 关联 DNS 凭证名称
	DeployCredentialID   *int              `json:"deploy_credential_id"`   // 关联部署凭证 ID（可为空）
	DeployCredentialName string            `json:"deploy_credential_name"` // 关联部署凭证名称
	IsActive             bool              `json:"is_active"`              // 是否启用
	Comment              string            `json:"comment"`                // 备注
	CertIds              []int             `json:"cert_ids"`               // 关联证书 ID 列表
	LastStatus           string            `json:"last_status"`            // 最近一次部署状态（success/failed）
	LastError            string            `json:"last_error"`             // 最近一次部署错误信息
	LastDeployedAt       string            `json:"last_deployed_at"`       // 最近部署时间
	CreatedAt            string            `json:"created_at"`             // 创建时间
	UpdatedAt            string            `json:"updated_at"`             // 更新时间
}

// CreateDeployTargetRequest 创建部署目标请求
type CreateDeployTargetRequest struct {
	Name               string            `json:"name"`                           // 部署目标名称
	ProviderType       string            `json:"provider_type"`                  // 云厂商类型
	DeployService      string            `json:"deploy_service"`                 // 部署服务
	Config             map[string]string `json:"config"`                         // 部署参数
	CredentialSource   string            `json:"credential_source"`              // 凭证来源
	DNSProviderID      *int              `json:"dns_provider_id,omitempty"`      // 关联 DNS 凭证 ID（可选）
	DeployCredentialID *int              `json:"deploy_credential_id,omitempty"` // 关联部署凭证 ID（可选）
	IsActive           bool              `json:"is_active"`                      // 是否启用
	Comment            string            `json:"comment"`                        // 备注
}

// UpdateDeployTargetRequest 更新部署目标请求
type UpdateDeployTargetRequest struct {
	Name               string            `json:"name,omitempty"`                 // 部署目标名称
	ProviderType       string            `json:"provider_type,omitempty"`        // 云厂商类型
	DeployService      string            `json:"deploy_service,omitempty"`       // 部署服务
	Config             map[string]string `json:"config,omitempty"`               // 部署参数
	CredentialSource   string            `json:"credential_source,omitempty"`    // 凭证来源
	DNSProviderID      *int              `json:"dns_provider_id,omitempty"`      // 关联 DNS 凭证 ID（可选）
	DeployCredentialID *int              `json:"deploy_credential_id,omitempty"` // 关联部署凭证 ID（可选）
	IsActive           *bool             `json:"is_active,omitempty"`            // 是否启用
	Comment            string            `json:"comment,omitempty"`              // 备注
}

// DeployOutcomeDTO 部署结果（前端展示用）
type DeployOutcomeDTO struct {
	TargetID    int    `json:"target_id"`     // 部署目标 ID
	TargetName  string `json:"target_name"`   // 部署目标名称
	CloudCertID string `json:"cloud_cert_id"` // 云端证书 ID
	Success     bool   `json:"success"`       // 是否部署成功
	Message     string `json:"message"`       // 部署结果描述
	RawResponse string `json:"raw_response"`  // 云厂商原始响应（调试用）
}

// toDeployTargetListItem 将 ent 实体转换为前端列表项
func toDeployTargetListItem(t *ent.DeployTarget) DeployTargetListItem {
	item := DeployTargetListItem{
		ID:               t.ID,
		Name:             t.Name,
		ProviderType:     t.ProviderType.String(),
		DeployService:    t.DeployService,
		CredentialSource: t.CredentialSource.String(),
		IsActive:         t.IsActive,
		Comment:          t.Comment,
		LastStatus:       t.LastStatus,
		LastError:        t.LastError,
		CreatedAt:        t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        t.UpdatedAt.Format(time.RFC3339),
	}
	if len(t.Config) > 0 {
		_ = json.Unmarshal(t.Config, &item.Config)
		// region 存储在 config 中：阿里云用 region_id，其余用 region
		item.Region = deploy.RegionFromConfig(item.Config)
	}
	if t.Edges.DNSProvider != nil {
		id := t.Edges.DNSProvider.ID
		item.DNSProviderID = &id
		item.DNSProviderName = t.Edges.DNSProvider.Name
	}
	if t.Edges.DeployCredential != nil {
		id := t.Edges.DeployCredential.ID
		item.DeployCredentialID = &id
		item.DeployCredentialName = t.Edges.DeployCredential.Name
	}
	if len(t.Edges.Certificates) > 0 {
		ids := make([]int, len(t.Edges.Certificates))
		for i, c := range t.Edges.Certificates {
			ids[i] = c.ID
		}
		item.CertIds = ids
	}
	if !t.LastDeployedAt.IsZero() {
		item.LastDeployedAt = t.LastDeployedAt.Format(time.RFC3339)
	}
	return item
}

// ListDeployTargets 获取所有部署目标
func (s *DeployServiceWrapper) ListDeployTargets() ([]DeployTargetListItem, error) {
	ctx := context.Background()
	targets, err := s.deployService.List(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]DeployTargetListItem, len(targets))
	for i, t := range targets {
		items[i] = toDeployTargetListItem(t)
	}
	return items, nil
}

// GetDeployTarget 获取单个部署目标
func (s *DeployServiceWrapper) GetDeployTarget(id int) (*DeployTargetListItem, error) {
	ctx := context.Background()
	t, err := s.deployService.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	item := toDeployTargetListItem(t)
	return &item, nil
}

// CreateDeployTarget 创建部署目标
func (s *DeployServiceWrapper) CreateDeployTarget(input CreateDeployTargetRequest) (*DeployTargetListItem, error) {
	ctx := context.Background()
	t, err := s.deployService.Create(ctx, deploy.CreateDeployTargetInput{
		Name:               input.Name,
		ProviderType:       input.ProviderType,
		DeployService:      input.DeployService,
		Config:             input.Config,
		CredentialSource:   input.CredentialSource,
		DNSProviderID:      input.DNSProviderID,
		DeployCredentialID: input.DeployCredentialID,
		IsActive:           input.IsActive,
		Comment:            input.Comment,
	})
	if err != nil {
		return nil, err
	}
	item := toDeployTargetListItem(t)
	return &item, nil
}

// UpdateDeployTarget 更新部署目标
func (s *DeployServiceWrapper) UpdateDeployTarget(id int, input UpdateDeployTargetRequest) (*DeployTargetListItem, error) {
	ctx := context.Background()
	t, err := s.deployService.Update(ctx, id, deploy.UpdateDeployTargetInput{
		Name:               input.Name,
		ProviderType:       input.ProviderType,
		DeployService:      input.DeployService,
		Config:             input.Config,
		CredentialSource:   input.CredentialSource,
		DNSProviderID:      input.DNSProviderID,
		DeployCredentialID: input.DeployCredentialID,
		IsActive:           input.IsActive,
		Comment:            input.Comment,
	})
	if err != nil {
		return nil, err
	}
	item := toDeployTargetListItem(t)
	return &item, nil
}

// DeleteDeployTarget 删除部署目标
func (s *DeployServiceWrapper) DeleteDeployTarget(id int) error {
	ctx := context.Background()
	return s.deployService.Delete(ctx, id)
}

// LinkCert 关联证书到部署目标
func (s *DeployServiceWrapper) LinkCert(targetID, certID int) error {
	ctx := context.Background()
	return s.deployService.LinkCert(ctx, targetID, certID)
}

// UnlinkCert 取消证书与部署目标的关联
func (s *DeployServiceWrapper) UnlinkCert(targetID, certID int) error {
	ctx := context.Background()
	return s.deployService.UnlinkCert(ctx, targetID, certID)
}

// ListTargetsByCert 获取关联了某证书的部署目标
func (s *DeployServiceWrapper) ListTargetsByCert(certID int) ([]DeployTargetListItem, error) {
	ctx := context.Background()
	targets, err := s.deployService.ListByCert(ctx, certID)
	if err != nil {
		return nil, err
	}
	items := make([]DeployTargetListItem, len(targets))
	for i, t := range targets {
		items[i] = toDeployTargetListItem(t)
	}
	return items, nil
}

// DeployCertificate 将证书部署到指定目标（domain 可指定 CDN 域名；siteID 为面板/防火墙类的站点 ID，云厂商忽略）
func (s *DeployServiceWrapper) DeployCertificate(targetID, certID int, domain, siteID string) (*DeployOutcomeDTO, error) {
	ctx := context.Background()
	outcome, err := s.deployService.DeployCertificate(ctx, targetID, certID, domain, siteID)
	if err != nil {
		return nil, err
	}
	return &DeployOutcomeDTO{
		TargetID:    outcome.TargetID,
		TargetName:  outcome.TargetName,
		CloudCertID: outcome.CloudCertID,
		Success:     outcome.Success,
		Message:     outcome.Message,
		RawResponse: outcome.RawResponse,
	}, nil
}

// DeployAllForCert 将证书部署到所有关联目标
func (s *DeployServiceWrapper) DeployAllForCert(certID int) ([]DeployOutcomeDTO, error) {
	ctx := context.Background()
	outcomes, err := s.deployService.DeployAllForCert(ctx, certID)
	if err != nil {
		return nil, err
	}
	dtos := make([]DeployOutcomeDTO, len(outcomes))
	for i, o := range outcomes {
		dtos[i] = DeployOutcomeDTO{
			TargetID:    o.TargetID,
			TargetName:  o.TargetName,
			CloudCertID: o.CloudCertID,
			Success:     o.Success,
			Message:     o.Message,
		}
	}
	return dtos, nil
}

// FetchCDNDomainsRequest 拉取 CDN 域名请求（内联凭证，用于新建目标时选择）
type FetchCDNDomainsRequest struct {
	ProviderType       string            `json:"provider_type"`                  // 云厂商类型
	DeployService      string            `json:"deploy_service"`                 // 部署服务（cdn/dcdn/elb/waf 等）
	CredentialSource   string            `json:"credential_source"`              // 凭证来源（dns_provider/deploy_credential）
	DNSProviderID      *int              `json:"dns_provider_id,omitempty"`      // 关联 DNS 凭证 ID（可选）
	DeployCredentialID *int              `json:"deploy_credential_id,omitempty"` // 关联部署凭证 ID（可选）
	Region             string            `json:"region"`                         // 区域代码
	Config             map[string]string `json:"config"`                         // 部署参数（按厂商/服务不同）
}

// FetchCDNDomains 根据内联凭证拉取 CDN 域名列表
func (s *DeployServiceWrapper) FetchCDNDomains(input FetchCDNDomainsRequest) ([]string, error) {
	ctx := context.Background()
	return s.deployService.FetchCDNDomains(ctx, deploy.FetchDomainsInput{
		ProviderType:       input.ProviderType,
		DeployService:      input.DeployService,
		CredentialSource:   input.CredentialSource,
		DNSProviderID:      input.DNSProviderID,
		DeployCredentialID: input.DeployCredentialID,
		Region:             input.Region,
		Config:             input.Config,
	})
}

// ListCDNDomains 拉取指定部署目标下的 CDN 域名列表
func (s *DeployServiceWrapper) ListCDNDomains(targetID int) ([]string, error) {
	ctx := context.Background()
	return s.deployService.ListCDNDomains(ctx, targetID)
}

// DeployLogListItem 部署历史列表项（前端展示用）
type DeployLogListItem struct {
	ID            int    `json:"id"`             // 部署历史 ID
	TargetName    string `json:"target_name"`    // 部署目标名称
	CertID        int    `json:"cert_id"`        // 证书 ID
	CertDomain    string `json:"cert_domain"`    // 证书域名
	DeployDomain  string `json:"deploy_domain"`  // 部署到的加速域名
	ProviderType  string `json:"provider_type"`  // 云厂商类型
	DeployService string `json:"deploy_service"` // 部署服务
	Success       bool   `json:"success"`        // 是否部署成功
	Message       string `json:"message"`        // 部署结果描述
	Response      string `json:"response"`       // 云厂商原始响应（调试用）
	CloudCertID   string `json:"cloud_cert_id"`  // 云端证书 ID
	CreatedAt     string `json:"created_at"`     // 部署时间
}

// ListDeployLogs 获取某部署目标的部署历史（按时间倒序）
func (s *DeployServiceWrapper) ListDeployLogs(targetID int) ([]DeployLogListItem, error) {
	ctx := context.Background()
	logs, err := s.deployService.ListDeployLogs(ctx, targetID)
	if err != nil {
		return nil, err
	}
	items := make([]DeployLogListItem, len(logs))
	for i, l := range logs {
		items[i] = DeployLogListItem{
			ID:            l.ID,
			TargetName:    l.TargetName,
			CertID:        l.CertID,
			CertDomain:    l.CertDomain,
			DeployDomain:  l.DeployDomain,
			ProviderType:  l.ProviderType,
			DeployService: l.DeployService,
			Success:       l.Success,
			Message:       l.Message,
			Response:      l.Response,
			CloudCertID:   l.CloudCertID,
			CreatedAt:     l.CreatedAt.Format(time.RFC3339),
		}
	}
	return items, nil
}

// ServiceStartup 实现 Wails 服务接口
func (s *DeployServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown 实现 Wails 服务接口
func (s *DeployServiceWrapper) ServiceShutdown() error {
	return nil
}

// ServiceName 实现 Wails 服务接口
func (s *DeployServiceWrapper) ServiceName() string {
	return "DeployService"
}

// CurrentCertDTO 云端/面板当前生效证书信息（前端展示用）。
type CurrentCertDTO struct {
	CommonName   string   `json:"common_name"`
	SANs         []string `json:"sans"`
	Issuer       string   `json:"issuer"`
	NotBefore    string   `json:"not_before"`
	NotAfter     string   `json:"not_after"`
	SerialNumber string   `json:"serial_number"`
	Supported    bool     `json:"supported"`
	Error        string   `json:"error,omitempty"`
}

// CurrentCertsResultDTO GetCurrentCerts 返回（按资源 key 索引）。
type CurrentCertsResultDTO struct {
	Results map[string]*CurrentCertDTO `json:"results"`
}

// GetCurrentCerts 查询部署目标下所有资源当前生效证书（本地+云端对比用）。
func (s *DeployServiceWrapper) GetCurrentCerts(targetID int) (*CurrentCertsResultDTO, error) {
	m, err := s.deployService.GetCurrentCerts(context.Background(), targetID)
	if err != nil {
		return nil, err
	}
	results := make(map[string]*CurrentCertDTO, len(m))
	for k, v := range m {
		dto := &CurrentCertDTO{Supported: v.Supported, Error: v.Error}
		if v.CurrentCert != nil {
			dto.CommonName = v.CommonName
			dto.SANs = v.SANs
			dto.Issuer = v.Issuer
			dto.NotBefore = v.NotBefore
			dto.NotAfter = v.NotAfter
			dto.SerialNumber = v.SerialNumber
		}
		results[k] = dto
	}
	return &CurrentCertsResultDTO{Results: results}, nil
}
