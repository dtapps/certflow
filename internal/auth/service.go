package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
)

// AuthService 认证服务
type AuthService struct {
	mu           sync.RWMutex
	filePath     string
	v            *viper.Viper
	passwordHash string
}

// NewAuthService 创建新的认证服务
func NewAuthService(dataDir string) (*AuthService, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf(i18n.T("error.create_data_dir_failed", "Error", err))
	}

	filePath := filepath.Join(dataDir, "auth.json")

	v := viper.New()
	v.SetConfigFile(filePath)
	v.SetConfigType("json")

	s := &AuthService{
		filePath: filePath,
		v:        v,
	}

	// 尝试加载现有密码
	if err := v.ReadInConfig(); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf(i18n.T("error.load_auth_failed", "Error", err))
		}
		// 文件不存在，初始化空密码
		s.passwordHash = ""
	} else {
		s.passwordHash = v.GetString("password_hash")
	}

	return s, nil
}

// save 保存密码哈希到文件
func (s *AuthService) save() error {
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0755); err != nil {
		return fmt.Errorf(i18n.T("error.create_data_dir_failed", "Error", err))
	}
	s.v.Set("password_hash", s.passwordHash)
	return s.v.WriteConfig()
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
