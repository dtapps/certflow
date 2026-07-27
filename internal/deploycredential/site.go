package deploycredential

import "cnb.cool/dtapp/certflow/internal/cloudcred"

// 面板/防火墙类部署凭证（site）：仅需 API Key（api_key），无 AppSecret / Secret。
// 各面板字段名一致（api_key + panel_url），仅 AcePanel 使用 token_id/token_secret。

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
