// Package deploycredential 解析「部署凭证」的 config JSON。
// 按 provider_type 拆为两类：
//   - site（面板/防火墙）：btpanel / aapanel / 1panel / acepanel / aawaf，结构体见 site.go
//   - domain（CDN/云厂商）：aliyun / huawei / tencentcloud / baiducloud / ctyun / volcengine，结构体见 domain.go
package deploycredential

import (
	"fmt"

	"cnb.cool/dtapp/certflow/internal/cloudcred"
	"cnb.cool/dtapp/certflow/internal/config"
)

// credentialer 是部署凭证的统一接口，各结构体实现 toCredentials 返回 cloudcred.Credentials。
type credentialer interface {
	toCredentials() cloudcred.Credentials
}

// parseCred 将 JSON 反序列化为具体凭证类型 T，并转为 cloudcred.Credentials。
func parseCred[T credentialer](raw []byte) (cloudcred.Credentials, error) {
	v, err := config.ParseConfig[T](raw)
	if err != nil {
		return cloudcred.Credentials{}, err
	}
	return v.toCredentials(), nil
}

// Parse 根据 provider_type 选择对应结构体解析部署凭证 config。
func Parse(providerType string, raw []byte) (cloudcred.Credentials, error) {
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
