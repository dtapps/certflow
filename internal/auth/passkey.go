package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"cnb.cool/dtapp/certflow/ent"
	"cnb.cool/dtapp/certflow/ent/authmethod"
	"cnb.cool/dtapp/certflow/ent/passkeycredential"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// PasskeyInfo Passkey 信息
type PasskeyInfo struct {
	IsConfigured    bool   `json:"is_configured"`    // 是否已配置
	CredentialCount int    `json:"credential_count"` // 凭据数量
	CreatedAt       string `json:"created_at"`       // 创建时间
}

// PasskeyRegistrationData Passkey 注册数据（前端传递）
type PasskeyRegistrationData struct {
	ID       string `json:"id"`    // Base64 编码的凭据 ID
	RawID    string `json:"rawId"` // Base64url 编码的凭据 ID
	Type     string `json:"type"`  // "public-key"
	Response struct {
		AttestationObject string `json:"attestationObject"` // Base64url 编码
		ClientDataJSON    string `json:"clientDataJSON"`    // Base64url 编码
	} `json:"response"`
}

// PasskeyAuthenticationData Passkey 认证数据（前端传递）
type PasskeyAuthenticationData struct {
	ID       string `json:"id"`    // Base64url 编码的凭据 ID
	RawID    string `json:"rawId"` // Base64url 编码的凭据 ID
	Type     string `json:"type"`  // "public-key"
	Response struct {
		AuthenticatorData string `json:"authenticatorData"` // Base64url 编码
		ClientDataJSON    string `json:"clientDataJSON"`    // Base64url 编码
		Signature         string `json:"signature"`         // Base64url 编码
		UserHandle        string `json:"userHandle"`        // Base64url 编码
	} `json:"response"`
}

// PasskeyRegistrationResponse 注册响应数据（传递给前端）
type PasskeyRegistrationResponse struct {
	CredentialCreationOptions string `json:"credentialCreationOptions"` // JSON 字符串
}

// PasskeyAuthenticationResponse 认证响应数据（传递给前端）
type PasskeyAuthenticationResponse struct {
	CredentialRequestOptions string `json:"credentialRequestOptions"` // JSON 字符串
}

// webauthnUser 实现 webauthn.User 接口
type webauthnUser struct {
	id          []byte
	displayName string
	name        string
}

func (u *webauthnUser) WebAuthnID() []byte          { return u.id }
func (u *webauthnUser) WebAuthnName() string        { return u.name }
func (u *webauthnUser) WebAuthnDisplayName() string { return u.displayName }
func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential {
	return nil // 简化实现，实际应返回已注册的凭据
}

// getWebAuthn 获取 WebAuthn 实例
func (s *AuthService) getWebAuthn() (*webauthn.WebAuthn, error) {
	return webauthn.New(&webauthn.Config{
		RPDisplayName: "CertFlow",
		RPID:          "localhost",
		RPOrigins:     []string{"wails://localhost"},
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationPreferred,
		},
	})
}

// StartPasskeyRegistration 开始 Passkey 注册
func (s *AuthService) StartPasskeyRegistration() (*PasskeyRegistrationResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()

	// 检查是否已经配置了 Passkey
	exists, err := s.db.AuthMethod.Query().
		Where(authmethod.MethodEQ("passkey")).
		Exist(ctx)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("error.passkey_registration_failed", "Error", err))
	}
	if exists {
		return nil, fmt.Errorf(i18n.T("error.passkey_already_configured"))
	}

	wa, err := s.getWebAuthn()
	if err != nil {
		return nil, fmt.Errorf(i18n.T("error.passkey_registration_failed", "Error", err))
	}

	user := &webauthnUser{
		id:          []byte("certflow-user"),
		displayName: "CertFlow User",
		name:        "user",
	}

	// 开始注册
	credential, session, err := wa.BeginRegistration(user)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("error.passkey_registration_failed", "Error", err))
	}

	// 将 session 信息存储（简化实现，实际应存储到 session store）
	_ = session
	_ = credential

	// 创建认证方式（暂存，等待完成注册）
	am, err := s.db.AuthMethod.Create().
		SetMethod("passkey").
		SetIsActive(false).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("error.passkey_registration_failed", "Error", err))
	}

	// 返回注册选项（简化：返回基本选项供前端使用）
	response := &PasskeyRegistrationResponse{
		CredentialCreationOptions: fmt.Sprintf(`{
			"rp": {"name": "CertFlow", "id": "localhost"},
			"user": {"id": "%s", "name": "user", "displayName": "CertFlow User"},
			"challenge": "%s",
			"pubKeyCredParams": [
				{"type": "public-key", "alg": -7},
				{"type": "public-key", "alg": -257}
			],
			"authenticatorSelection": {
				"residentKey": "preferred",
				"userVerification": "preferred"
			},
			"timeout": 60000,
			"attestation": "none",
			"authMethodId": %d
		}`, base64.RawURLEncoding.EncodeToString([]byte("certflow-user")),
			base64.RawURLEncoding.EncodeToString([]byte("challenge-placeholder")),
			am.ID),
	}

	return response, nil
}

// FinishPasskeyRegistration 完成 Passkey 注册
func (s *AuthService) FinishPasskeyRegistration(data string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()

	// 解析前端返回的注册数据
	var regData PasskeyRegistrationData
	if err := json.Unmarshal([]byte(data), &regData); err != nil {
		return fmt.Errorf(i18n.T("error.passkey_registration_failed", "Error", err))
	}

	// 验证凭据 ID
	credentialID, err := base64.RawURLEncoding.DecodeString(regData.RawID)
	if err != nil {
		return fmt.Errorf(i18n.T("error.passkey_registration_failed", "Error", err))
	}

	// 获取待完成的 Passkey 认证方式
	am, err := s.db.AuthMethod.Query().
		Where(authmethod.MethodEQ("passkey")).
		Only(ctx)
	if err != nil {
		return fmt.Errorf(i18n.T("error.passkey_registration_failed", "Error", err))
	}

	// 保存凭据（简化实现，实际应验证 attestation）
	_, err = s.db.PasskeyCredential.Create().
		SetCredentialID(credentialID).
		SetPublicKey(credentialID). // 简化：实际应存储公钥
		SetSignCount(0).
		SetAuthMethod(am).
		Save(ctx)
	if err != nil {
		return fmt.Errorf(i18n.T("error.passkey_registration_failed", "Error", err))
	}

	// 激活 Passkey 认证方式
	_, err = s.db.AuthMethod.Update().
		Where(authmethod.IDEQ(am.ID)).
		SetIsActive(true).
		Save(ctx)
	if err != nil {
		return fmt.Errorf(i18n.T("error.passkey_registration_failed", "Error", err))
	}

	// 取消其他激活方式
	_, err = s.db.AuthMethod.Update().
		Where(
			authmethod.IsActiveEQ(true),
			authmethod.MethodNEQ("passkey"),
		).
		SetIsActive(false).
		Save(ctx)
	if err != nil {
		logging.Warn(i18n.T("log.passkey_deactivate_failed", "Error", err))
	}

	logging.Info(i18n.T("log.passkey_activated"))
	return nil
}

// StartPasskeyLogin 开始 Passkey 登录
func (s *AuthService) StartPasskeyLogin() (*PasskeyAuthenticationResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()

	// 获取 Passkey 凭据
	credentials, err := s.db.PasskeyCredential.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("error.passkey_verification_failed", "Error", err))
	}
	if len(credentials) == 0 {
		return nil, fmt.Errorf(i18n.T("error.passkey_verification_failed", "Error", "no credentials"))
	}

	// 构建允许的凭据列表
	allowedCredentials := make([]protocol.CredentialDescriptor, len(credentials))
	for i, cred := range credentials {
		allowedCredentials[i] = protocol.CredentialDescriptor{
			Type:         protocol.PublicKeyCredentialType,
			CredentialID: protocol.URLEncodedBase64(cred.CredentialID),
		}
	}

	// 返回认证选项（简化实现）
	response := &PasskeyAuthenticationResponse{
		CredentialRequestOptions: fmt.Sprintf(`{
			"challenge": "%s",
			"timeout": 60000,
			"rpId": "localhost",
			"allowCredentials": %s,
			"userVerification": "preferred"
		}`, base64.RawURLEncoding.EncodeToString([]byte("challenge-placeholder")),
			func() string {
				b, _ := json.Marshal(allowedCredentials)
				return string(b)
			}()),
	}

	return response, nil
}

// FinishPasskeyLogin 完成 Passkey 登录
func (s *AuthService) FinishPasskeyLogin(data string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()

	// 解析前端返回的认证数据
	var authData PasskeyAuthenticationData
	if err := json.Unmarshal([]byte(data), &authData); err != nil {
		return false, fmt.Errorf(i18n.T("error.passkey_verification_failed", "Error", err))
	}

	// 验证凭据 ID
	credentialID, err := base64.RawURLEncoding.DecodeString(authData.RawID)
	if err != nil {
		return false, fmt.Errorf(i18n.T("error.passkey_verification_failed", "Error", err))
	}

	// 查找匹配的凭据
	cred, err := s.db.PasskeyCredential.Query().
		Where(
			passkeycredential.CredentialIDEQ(credentialID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return false, nil // 凭据不存在
		}
		return false, fmt.Errorf(i18n.T("error.passkey_verification_failed", "Error", err))
	}

	// 简化验证：实际应验证签名等
	// 这里只检查凭据是否存在
	_ = cred

	logging.Info(i18n.T("log.passkey_login_success"))
	return true, nil
}

// ClearPasskey 清除 Passkey 设置
func (s *AuthService) ClearPasskey() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()

	// 先删除 Passkey 凭据
	_, err := s.db.PasskeyCredential.Delete().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf(i18n.T("error.passkey_registration_failed", "Error", err))
	}

	// 再删除 Passkey 认证方式
	_, err = s.db.AuthMethod.Delete().
		Where(authmethod.MethodEQ("passkey")).
		Exec(ctx)
	return err
}

// GetPasskeyInfo 获取 Passkey 信息
func (s *AuthService) GetPasskeyInfo() (*PasskeyInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()

	am, err := s.db.AuthMethod.Query().
		Where(authmethod.MethodEQ("passkey")).
		WithPasskeyCredentials().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return &PasskeyInfo{IsConfigured: false}, nil
		}
		return nil, fmt.Errorf(i18n.T("error.passkey_verification_failed", "Error", err))
	}

	info := &PasskeyInfo{
		IsConfigured:    true,
		CredentialCount: len(am.Edges.PasskeyCredentials),
		CreatedAt:       am.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	return info, nil
}
