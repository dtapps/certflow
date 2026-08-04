package deploycredential

import (
	"context"
	"encoding/json"
	"time"

	"cnb.cool/dtapp/certflow/internal/ent"
	"cnb.cool/dtapp/certflow/internal/ent/deploycredential"
)

// Service 部署凭证服务
type Service struct {
	db *ent.Client
}

// NewService 创建部署凭证服务
func NewService(db *ent.Client) *Service {
	return &Service{db: db}
}

// DeployCredentialListItem 部署凭证列表项
type DeployCredentialListItem struct {
	ID           int               `json:"id"`
	Name         string            `json:"name"`
	ProviderType string            `json:"provider_type"`
	Config       map[string]string `json:"config"`
	IsActive     bool              `json:"is_active"`
	Comment      string            `json:"comment"`
	CreatedAt    string            `json:"created_at"`
	UpdatedAt    string            `json:"updated_at"`
}

// CreateDeployCredentialInput 创建部署凭证输入
type CreateDeployCredentialInput struct {
	Name         string            `json:"name"`
	ProviderType string            `json:"provider_type"`
	Config       map[string]string `json:"config"`
	Comment      string            `json:"comment"`
}

// UpdateDeployCredentialInput 更新部署凭证输入
type UpdateDeployCredentialInput struct {
	Name         string            `json:"name"`
	ProviderType string            `json:"provider_type"`
	Config       map[string]string `json:"config"`
	Comment      string            `json:"comment"`
}

// List 列出所有部署凭证
func (s *Service) List(ctx context.Context) ([]*DeployCredentialListItem, error) {
	list, err := s.db.DeployCredential.
		Query().
		Order(ent.Asc(deploycredential.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*DeployCredentialListItem, 0, len(list))
	for _, item := range list {
		config := make(map[string]string)
		if item.Config != nil {
			config = parseConfig(item.Config)
		}
		result = append(result, &DeployCredentialListItem{
			ID:           item.ID,
			Name:         item.Name,
			ProviderType: string(item.ProviderType),
			Config:       config,
			IsActive:     item.IsActive,
			Comment:      item.Comment,
			CreatedAt:    item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    item.UpdatedAt.Format(time.RFC3339),
		})
	}
	return result, nil
}

// GetByID 根据 ID 获取部署凭证
func (s *Service) GetByID(ctx context.Context, id int) (*DeployCredentialListItem, error) {
	item, err := s.db.DeployCredential.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	config := make(map[string]string)
	if item.Config != nil {
		config = parseConfig(item.Config)
	}
	return &DeployCredentialListItem{
		ID:           item.ID,
		Name:         item.Name,
		ProviderType: string(item.ProviderType),
		Config:       config,
		IsActive:     item.IsActive,
		Comment:      item.Comment,
		CreatedAt:    item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    item.UpdatedAt.Format(time.RFC3339),
	}, nil
}

// Create 创建部署凭证
func (s *Service) Create(ctx context.Context, input CreateDeployCredentialInput) (*DeployCredentialListItem, error) {
	configBytes := marshalConfig(input.Config)
	item, err := s.db.DeployCredential.
		Create().
		SetName(input.Name).
		SetProviderType(deploycredential.ProviderType(input.ProviderType)).
		SetConfig(configBytes).
		SetComment(input.Comment).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return &DeployCredentialListItem{
		ID:           item.ID,
		Name:         item.Name,
		ProviderType: string(item.ProviderType),
		Config:       input.Config,
		IsActive:     item.IsActive,
		Comment:      item.Comment,
		CreatedAt:    item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    item.UpdatedAt.Format(time.RFC3339),
	}, nil
}

// Update 更新部署凭证
func (s *Service) Update(ctx context.Context, id int, input UpdateDeployCredentialInput) (*DeployCredentialListItem, error) {
	configBytes := marshalConfig(input.Config)
	item, err := s.db.DeployCredential.
		UpdateOneID(id).
		SetName(input.Name).
		SetProviderType(deploycredential.ProviderType(input.ProviderType)).
		SetConfig(configBytes).
		SetComment(input.Comment).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return &DeployCredentialListItem{
		ID:           item.ID,
		Name:         item.Name,
		ProviderType: string(item.ProviderType),
		Config:       input.Config,
		IsActive:     item.IsActive,
		Comment:      item.Comment,
		CreatedAt:    item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    item.UpdatedAt.Format(time.RFC3339),
	}, nil
}

// SetActive 设置部署凭证的启用状态
func (s *Service) SetActive(ctx context.Context, id int, active bool) (*DeployCredentialListItem, error) {
	if _, err := s.db.DeployCredential.Get(ctx, id); err != nil {
		return nil, err
	}
	item, err := s.db.DeployCredential.UpdateOneID(id).SetIsActive(active).Save(ctx)
	if err != nil {
		return nil, err
	}
	config := make(map[string]string)
	if item.Config != nil {
		config = parseConfig(item.Config)
	}
	return &DeployCredentialListItem{
		ID:           item.ID,
		Name:         item.Name,
		ProviderType: string(item.ProviderType),
		Config:       config,
		IsActive:     item.IsActive,
		Comment:      item.Comment,
		CreatedAt:    item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    item.UpdatedAt.Format(time.RFC3339),
	}, nil
}

// Delete 删除部署凭证
func (s *Service) Delete(ctx context.Context, id int) error {
	return s.db.DeployCredential.DeleteOneID(id).Exec(ctx)
}

func parseConfig(b []byte) map[string]string {
	result := make(map[string]string)
	if len(b) == 0 {
		return result
	}
	_ = json.Unmarshal(b, &result)
	return result
}

func marshalConfig(config map[string]string) []byte {
	if config == nil {
		return nil
	}
	b, _ := json.Marshal(config)
	return b
}
