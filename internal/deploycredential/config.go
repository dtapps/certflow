// Package deploycredential 解析「部署凭证」的 config JSON。
// 按 provider_type 拆为两类：
//   - site（面板/防火墙）：btpanel / aapanel / 1panel / acepanel / aawaf，结构体见 site.go
//   - domain（CDN/云厂商）：aliyun / huawei / tencentcloud / baiducloud / ctyun / volcengine，结构体见 domain.go
package deploycredential

import (
	"encoding/json"
	"fmt"

	"cnb.cool/dtapp/certflow/internal/cloudcred"
	"cnb.cool/dtapp/certflow/internal/config"
	"cnb.cool/dtapp/certflow/internal/ent/schema"
)

// DeployCredentialConfig 是部署凭证配置的统一结构体，引用自 schema 包。
type DeployCredentialConfig = schema.DeployCredentialConfig

// credentialer 是部署凭证的统一接口，各结构体实现 toCredentials 返回 cloudcred.Credentials。
type credentialer interface {
	toCredentials() cloudcred.Credentials
}

// mustMarshal 将 DeployCredentialConfig 序列化为 JSON 字节。
func mustMarshal(raw DeployCredentialConfig) []byte {
	b, _ := json.Marshal(raw)
	return b
}

// parseCred 将 JSON 反序列化为具体凭证类型 T，并转为 cloudcred.Credentials。
func parseCred[T credentialer](raw DeployCredentialConfig) (cloudcred.Credentials, error) {
	v, err := config.ParseConfig[T](mustMarshal(raw))
	if err != nil {
		return cloudcred.Credentials{}, err
	}
	return v.toCredentials(), nil
}

// Parse 根据 provider_type 选择对应结构体解析部署凭证 config。
func Parse(providerType string, raw DeployCredentialConfig) (cloudcred.Credentials, error) {
	switch providerType {
	// site（面板/防火墙）
	case "btpanel":
		return parseCred[BTPanelDeployCred](raw)
	case "aapanel":
		return parseCred[AAPanelDeployCred](raw)
	case "1panel":
		return parseCred[OnePanelDeployCred](raw)
	case "acepanel":
		return parseCred[AcePanelDeployCred](raw)
	case "aawaf":
		return parseCred[AAWafDeployCred](raw)
	case "openrestymanager":
		return parseCred[OpenRestyManagerDeployCred](raw)
	case "safeline":
		return parseCred[SafelineDeployCred](raw)
	// domain（CDN/云厂商）
	case "aliyun":
		return parseCred[AliyunDeployCred](raw)
	case "huawei":
		return parseCred[HuaweiDeployCred](raw)
	case "tencentcloud":
		return parseCred[TencentDeployCred](raw)
	case "baiducloud":
		return parseCred[BaiduDeployCred](raw)
	case "ctyun":
		return parseCred[CtyunDeployCred](raw)
	case "volcengine":
		return parseCred[VolcengineDeployCred](raw)
	default:
		return cloudcred.Credentials{}, fmt.Errorf("unsupported deploy credential provider: %s", providerType)
	}
}
