package deploycredential

import "cnb.cool/dtapp/certflow/internal/cloudcred"

// 面板/防火墙类部署凭证（site）：仅需 API Key，无 AppSecret / Secret。
// 各面板字段名基本一致（api_key + panel_url）；AcePanel 用 token_id/token_secret；
// OpenResty Manager 用 jwt_secret（本地签发 JWT，免登录/OTP）。

// BTPanelDeployCred 宝塔面板部署凭证
type BTPanelDeployCred struct {
	APIKey   string `json:"api_key" secret:"true"`
	PanelURL string `json:"panel_url"`
}

func (c BTPanelDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.APIKey, PanelURL: c.PanelURL}
}

// AAPanelDeployCred 宝塔国际版（aaPanel）部署凭证，API 与宝塔一致
type AAPanelDeployCred struct {
	APIKey   string `json:"api_key" secret:"true"`
	PanelURL string `json:"panel_url"`
}

func (c AAPanelDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.APIKey, PanelURL: c.PanelURL}
}

// OnePanelDeployCred 1Panel 面板部署凭证（仅需 API Key / Token）
type OnePanelDeployCred struct {
	APIKey   string `json:"api_key" secret:"true"`
	PanelURL string `json:"panel_url"`
}

func (c OnePanelDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.APIKey, PanelURL: c.PanelURL}
}

// AcePanelDeployCred AcePanel 面板部署凭证（令牌 ID + 令牌密钥）。
// TokenID 为令牌 ID，用作 Authorization Credential；
// TokenSecret 为令牌密钥，用作 HMAC 签名密钥。
type AcePanelDeployCred struct {
	TokenID     string `json:"token_id" secret:"true"`
	TokenSecret string `json:"token_secret" secret:"true"`
	PanelURL    string `json:"panel_url"`
}

func (c AcePanelDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.TokenID, AccessKeySecret: c.TokenSecret, PanelURL: c.PanelURL}
}

// AAWafDeployCred aaWAF 防火墙部署凭证（仅需 API Key / api_sk）
type AAWafDeployCred struct {
	APIKey   string `json:"api_key" secret:"true"`
	PanelURL string `json:"panel_url"`
}

func (c AAWafDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.APIKey, PanelURL: c.PanelURL}
}

// OpenRestyManagerDeployCred OpenResty Manager 面板部署凭证。
// 该面板鉴权为 echojwt（HS256 + JWT 密钥），仅校验签名与过期时间。
// 因登录接口需「用户名+OTP」（用户已开启 OTP），故采用客户端用 JWT 密钥本地签发 JWT 的方式，
// 免去登录与 OTP。jwt_secret 即面板的「JWT 密钥」（映射到 AccessKeySecret）。
// 兼容旧版 api_key 字段（早期版本用其承载 JWT 密钥），解析时优先使用 jwt_secret。
type OpenRestyManagerDeployCred struct {
	JWTSecret string `json:"jwt_secret" secret:"true"`
	APIKey    string `json:"api_key" secret:"true"`
	PanelURL  string `json:"panel_url"`
}

func (c OpenRestyManagerDeployCred) toCredentials() cloudcred.Credentials {
	// 优先 jwt_secret；旧凭证仅填 api_key 时向后兼容。
	secret := c.JWTSecret
	if secret == "" {
		secret = c.APIKey
	}
	return cloudcred.Credentials{AccessKeySecret: secret, PanelURL: c.PanelURL}
}

// SafelineDeployCred 雷池 SafeLine WAF（长亭）面板部署凭证。
// 鉴权为 OpenAPI 请求头 X-SLCE-API-TOKEN（API 令牌），仅需令牌（api_key），无 Secret。
// api_key 即 API 令牌（映射到 AccessKeyID），panel_url 标识面板地址。
type SafelineDeployCred struct {
	APIKey   string `json:"api_key" secret:"true"`
	PanelURL string `json:"panel_url"`
}

func (c SafelineDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.APIKey, PanelURL: c.PanelURL}
}
