package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"golang.org/x/crypto/bcrypt"
)

// AuthService 认证服务
type AuthService struct {
	mu           sync.RWMutex
	filePath     string
	passwordHash string
}

// NewAuthService 创建新的认证服务
func NewAuthService(dataDir string) (*AuthService, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf(i18n.T("error.create_data_dir_failed", "Error", err))
	}

	filePath := filepath.Join(dataDir, "auth.json")
	s := &AuthService{
		filePath: filePath,
	}

	// 尝试加载现有密码
	if err := s.load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf(i18n.T("error.load_auth_failed", "Error", err))
		}
	}

	return s, nil
}

// load 从文件加载密码哈希
func (s *AuthService) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	var auth struct {
		PasswordHash string `json:"password_hash"`
	}
	if err := json.Unmarshal(data, &auth); err != nil {
		return err
	}

	s.passwordHash = auth.PasswordHash
	return nil
}

// save 保存密码哈希到文件
func (s *AuthService) save() error {
	data, err := json.MarshalIndent(map[string]string{
		"password_hash": s.passwordHash,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf(i18n.T("error.serialize_auth_failed", "Error", err))
	}

	if err := os.WriteFile(s.filePath, data, 0600); err != nil {
		return fmt.Errorf(i18n.T("error.write_auth_file_failed", "Error", err))
	}

	return nil
}

// IsPasswordSet 检查是否已设置密码
func (s *AuthService) IsPasswordSet() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.passwordHash != ""
}

// SetPassword 设置密码（明文传入，内部哈希存储）
func (s *AuthService) SetPassword(plainPassword string) error {
	if len(plainPassword) < 6 {
		return fmt.Errorf(i18n.T("error.password_too_short"))
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf(i18n.T("error.password_hash_failed", "Error", err))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.passwordHash = string(hash)
	return s.save()
}

// VerifyPassword 验证密码
func (s *AuthService) VerifyPassword(plainPassword string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.passwordHash == "" {
		logging.Debug(i18n.T("log.auth_no_password_set"))
		return true // 未设置密码时，验证通过
	}

	result := bcrypt.CompareHashAndPassword([]byte(s.passwordHash), []byte(plainPassword)) == nil
	if !result {
		logging.Warn(i18n.T("log.auth_password_wrong"))
	}
	return result
}

// ChangePassword 修改密码（需要验证旧密码）
func (s *AuthService) ChangePassword(oldPassword, newPassword string) error {
	if !s.VerifyPassword(oldPassword) {
		return fmt.Errorf(i18n.T("error.password_incorrect"))
	}

	if len(newPassword) < 6 {
		return fmt.Errorf(i18n.T("error.password_too_short"))
	}

	return s.SetPassword(newPassword)
}

// ClearPassword 清除密码（取消密码保护）
func (s *AuthService) ClearPassword() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.passwordHash = ""
	return s.save()
}
