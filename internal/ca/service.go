package ca

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"cnb.cool/dtapp/certflow/internal/ent"
	"cnb.cool/dtapp/certflow/internal/ent/ca"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/network"
	"cnb.cool/dtapp/certflow/internal/settings"
)

// CAService 提供 CA 管理功能
type CAService struct {
	db               *ent.Client
	dataDir          string
	settingsProvider func() settings.Settings
}

// NewCAService 创建新的 CA 服务
func NewCAService(client *ent.Client, dataDir string) *CAService {
	return &CAService{db: client, dataDir: dataDir}
}

// SetSettingsProvider 注入设置提供者（用于 DNS/代理配置）
func (s *CAService) SetSettingsProvider(fn func() settings.Settings) {
	s.settingsProvider = fn
}

// builtinCARequirement 描述某内置 CA 启用时必须填写的字段
type builtinCARequirement struct {
	NeedEmail bool // 需要注册邮箱
	NeedEAB   bool // 需要 EAB KID 与 EAB HMAC
}

// builtinCARequirements 按 ACME 目录 URL 区分各内置 CA 的启用条件
// 内置 CA 类型已知，因此可明确其必填项：
//   - 仅 Let's Encrypt / Buypass 等需要注册邮箱
//   - ZeroSSL / LiteSSL 等需要注册邮箱 + EAB 凭据
var builtinCARequirements = map[string]builtinCARequirement{
	"https://acme-v02.api.letsencrypt.org/directory":         {NeedEmail: true},
	"https://acme-staging-v02.api.letsencrypt.org/directory": {NeedEmail: true},
	"https://api.buypass.com/acme/directory":                 {NeedEmail: true},
	"https://acme.zerossl.com/v2/DV90/directory":             {NeedEmail: true, NeedEAB: true},
	"https://acme.litessl.com/acme/v2/directory":             {NeedEmail: true, NeedEAB: true},
}

// validateBuiltinActivation 校验内置 CA 启用时是否满足必填条件，不满足返回错误
func validateBuiltinActivation(name, dirURL, email, eabKid, eabHmac string) error {
	req, ok := builtinCARequirements[dirURL]
	if !ok {
		// 非已知内置类型，不强制要求
		return nil
	}
	needEmail := req.NeedEmail && email == ""
	needEAB := req.NeedEAB && (eabKid == "" || eabHmac == "")
	switch {
	case needEmail && needEAB:
		return fmt.Errorf("%s", i18n.T("error.ca_activate_requires_email_eab", "Name", name))
	case needEmail:
		return fmt.Errorf("%s", i18n.T("error.ca_activate_requires_email", "Name", name))
	case needEAB:
		return fmt.Errorf("%s", i18n.T("error.ca_activate_requires_eab", "Name", name))
	}
	return nil
}

// CreateCAInput 创建 CA 输入参数
type CreateCAInput struct {
	Name         string `json:"name"`          // CA 名称
	DirectoryURL string `json:"directory_url"` // ACME 目录 URL
	AccountEmail string `json:"account_email"` // 注册邮箱
	EabKid       string `json:"eab_kid"`       // EAB KID（部分 CA 需要）
	EabHmac      string `json:"eab_hmac"`      // EAB HMAC Key（部分 CA 需要）
	IsBuiltin    bool   `json:"is_builtin"`    // 是否内置 CA
}

// UpdateCAInput 更新 CA 输入参数
type UpdateCAInput struct {
	Name         string  `json:"name,omitempty"`          // CA 名称
	DirectoryURL string  `json:"directory_url,omitempty"` // ACME 目录 URL
	AccountEmail string  `json:"account_email,omitempty"` // 注册邮箱
	EabKid       *string `json:"eab_kid,omitempty"`       // EAB KID（部分 CA 需要）
	EabHmac      *string `json:"eab_hmac,omitempty"`      // EAB HMAC Key（部分 CA 需要）
}

// Create 创建新的 CA
// 注意：启用状态由独立的 SetActive 接口管理，创建时固定为非启用（ent 默认值）
func (s *CAService) Create(ctx context.Context, input CreateCAInput) (*ent.CA, error) {
	builder := s.db.CA.Create().
		SetName(input.Name).
		SetDirectoryURL(input.DirectoryURL).
		SetAccountEmail(input.AccountEmail).
		SetIsBuiltin(input.IsBuiltin)
	if input.EabKid != "" {
		builder.SetEabKid(input.EabKid)
	}
	if input.EabHmac != "" {
		builder.SetEabHmac(input.EabHmac)
	}

	result, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.ca_create_failed", "Error", err))
	}
	return result, nil
}

// GetByID 根据 ID 获取 CA
func (s *CAService) GetByID(ctx context.Context, id int) (*ent.CA, error) {
	result, err := s.db.CA.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%s", i18n.T("error.ca_not_found"))
		}
		return nil, fmt.Errorf("%s", i18n.T("error.get_ca_failed", "Error", err))
	}
	return result, nil
}

// List 获取所有 CA
func (s *CAService) List(ctx context.Context) ([]*ent.CA, error) {
	results, err := s.db.CA.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.list_ca_failed", "Error", err))
	}
	return results, nil
}

// ListActive 获取所有启用的 CA
func (s *CAService) ListActive(ctx context.Context) ([]*ent.CA, error) {
	results, err := s.db.CA.Query().
		Where(ca.IsActiveEQ(true)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.list_ca_failed", "Error", err))
	}
	return results, nil
}

// Update 更新 CA
func (s *CAService) Update(ctx context.Context, id int, input UpdateCAInput) (*ent.CA, error) {
	logging.Info(i18n.T("log.ca_update_start", "ID", id))

	// 先读取现有实体，供内置 CA 启用校验使用
	existing, err := s.db.CA.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%s", i18n.T("error.ca_not_found"))
		}
		return nil, fmt.Errorf("%s", i18n.T("error.get_ca_failed", "Error", err))
	}

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
	if input.EabKid != nil {
		if *input.EabKid == "" {
			builder.ClearEabKid()
		} else {
			builder.SetEabKid(*input.EabKid)
		}
	}
	if input.EabHmac != nil {
		if *input.EabHmac == "" {
			builder.ClearEabHmac()
		} else {
			builder.SetEabHmac(*input.EabHmac)
		}
	}

	// 内置 CA 若已启用，修改必填项时需重新校验（启用状态本身由 SetActive 接口管理）
	if existing.IsBuiltin && existing.IsActive {
		effEmail := existing.AccountEmail
		effKid := existing.EabKid
		effHmac := existing.EabHmac
		if input.AccountEmail != "" {
			effEmail = input.AccountEmail
		}
		if input.EabKid != nil {
			if *input.EabKid == "" {
				effKid = ""
			} else {
				effKid = *input.EabKid
			}
		}
		if input.EabHmac != nil {
			if *input.EabHmac == "" {
				effHmac = ""
			} else {
				effHmac = *input.EabHmac
			}
		}
		if err := validateBuiltinActivation(existing.Name, existing.DirectoryURL, effEmail, effKid, effHmac); err != nil {
			return nil, err
		}
	}

	result, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%s", i18n.T("error.ca_not_found"))
		}
		return nil, fmt.Errorf("%s", i18n.T("error.ca_update_failed", "Error", err))
	}
	return result, nil
}

// Delete 删除 CA
func (s *CAService) Delete(ctx context.Context, id int) error {
	// 内置 CA 禁止删除
	caEntity, err := s.db.CA.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%s", i18n.T("error.ca_not_found"))
		}
		return fmt.Errorf("%s", i18n.T("error.ca_delete_failed", "Error", err))
	}
	if caEntity.IsBuiltin {
		return fmt.Errorf("%s", i18n.T("error.ca_builtin_no_delete"))
	}

	err = s.db.CA.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%s", i18n.T("error.ca_not_found"))
		}
		return fmt.Errorf("%s", i18n.T("error.ca_delete_failed", "Error", err))
	}
	return nil
}

// SetActive 启用/禁用 CA（独立接口，统一校验）
func (s *CAService) SetActive(ctx context.Context, id int, active bool) (*ent.CA, error) {
	logging.Info(i18n.T("log.ca_set_active", "ID", id, "Active", active))
	existing, err := s.db.CA.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%s", i18n.T("error.ca_not_found"))
		}
		return nil, fmt.Errorf("%s", i18n.T("error.get_ca_failed", "Error", err))
	}

	// 内置 CA 启用时校验必填条件
	if existing.IsBuiltin && active {
		if err := validateBuiltinActivation(existing.Name, existing.DirectoryURL, existing.AccountEmail, existing.EabKid, existing.EabHmac); err != nil {
			return nil, err
		}
	}

	result, err := s.db.CA.UpdateOneID(id).SetIsActive(active).Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%s", i18n.T("error.ca_not_found"))
		}
		return nil, fmt.Errorf("%s", i18n.T("error.ca_update_failed", "Error", err))
	}
	return result, nil
}

// SeedDefaults 插入默认 CA（仅在首次启动时执行一次）。
// 关键：按 directory_url 幂等去重。即便 seeded 标记丢失导致重复调用，
// 也不会重复插入内置 CA、不会覆盖用户已配置的内置 CA（含 EAB），
// 从而避免“IsBuiltin 被恢复 / EabKid、EabHmac 被清空”的问题。
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
			IsBuiltin:    true,
		},
		{
			Name:         "Buypass",
			DirectoryURL: "https://api.buypass.com/acme/directory",
			AccountEmail: "",
			IsBuiltin:    true,
		},
		{
			Name:         "ZeroSSL",
			DirectoryURL: "https://acme.zerossl.com/v2/DV90/directory",
			AccountEmail: "",
			IsBuiltin:    true,
		},
		{
			Name:         "LiteSSL",
			DirectoryURL: "https://acme.litessl.com/acme/v2/directory",
			AccountEmail: "",
			EabKid:       "",
			EabHmac:      "",
			IsBuiltin:    true,
		},
		{
			Name:         "Let's Encrypt (测试环境)",
			DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
			AccountEmail: "",
			IsBuiltin:    true,
		},
	}

	// 已存在的 directory_url 集合，重复启动时不重复插入
	existing, err := s.List(ctx)
	if err != nil {
		return err
	}
	have := make(map[string]struct{}, len(existing))
	for _, c := range existing {
		have[c.DirectoryURL] = struct{}{}
	}

	for _, input := range defaults {
		if _, ok := have[input.DirectoryURL]; ok {
			logging.Debug(i18n.T("log.ca_seed_skip_existing", "URL", input.DirectoryURL))
			continue
		}
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

// CheckDirectoryURL 验证 ACME 目录 URL 是否可访问且为有效的 ACME 目录
func (s *CAService) CheckDirectoryURL(ctx context.Context, rawURL string) (string, error) {
	logging.Info(i18n.T("log.ca_test_connection", "URL", rawURL))
	if rawURL == "" {
		return "", fmt.Errorf("%s", i18n.T("error.ca_directory_url_required"))
	}

	// 复用统一 HTTP 客户端（自定义 DNS + 代理 + httplog），未注入时回退默认
	var client *http.Client
	if s.settingsProvider != nil {
		client = network.BuildHTTPClient(s.settingsProvider())
	} else {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	// 单独控制本次请求超时，避免受默认客户端 60s 影响
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("%s", i18n.T("error.ca_invalid_url", "URL", rawURL))
	}
	req.Header.Set("User-Agent", "CertFlow/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s", i18n.T("error.ca_dir_url_unreachable", "Error", err.Error()))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s", i18n.T("error.ca_dir_url_unreachable", "Error", fmt.Sprintf("HTTP %d", resp.StatusCode)))
	}

	var dir map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&dir); err != nil {
		return "", fmt.Errorf("%s", i18n.T("error.ca_dir_url_invalid"))
	}
	if _, ok := dir["newNonce"]; !ok {
		return "", fmt.Errorf("%s", i18n.T("error.ca_dir_url_invalid"))
	}

	return i18n.T("ca.dir_url_ok"), nil
}
