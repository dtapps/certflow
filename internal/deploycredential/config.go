package deploycredential

import (
	"fmt"

	"cnb.cool/dtapp/certflow/internal/cloudcred"
	"cnb.cool/dtapp/certflow/internal/config"
)

// 部署凭证（deploy_credential 来源）各厂商的结构化配置。
// JSON tag 写死该厂商在「部署凭证」场景下的字段名，编译期即可保证字段名正确，
// 取代原先依赖 credKeySet 字符串查表的方式。敏感字段用 secret:"true" 标记，
// 以便通过 config.StripSecrets 向前端返回配置时自动剔除。

// AliyunDeployCred 阿里云部署凭证
type AliyunDeployCred struct {
	AccessKeyID     string `json:"access_key_id" secret:"true"`
	AccessKeySecret string `json:"access_key_secret" secret:"true"`
	RegionID        string `json:"region_id"`
}

// HuaweiDeployCred 华为云部署凭证
type HuaweiDeployCred struct {
	AccessKeyID     string `json:"access_key_id" secret:"true"`
	SecretAccessKey string `json:"secret_access_key" secret:"true"`
	Region          string `json:"region"`
}

// TencentDeployCred 腾讯云部署凭证
type TencentDeployCred struct {
	SecretID  string `json:"secret_id" secret:"true"`
	SecretKey string `json:"secret_key" secret:"true"`
	Region    string `json:"region"`
}

// BaiduDeployCred 百度云部署凭证（部署来源使用 access_key_secret）
type BaiduDeployCred struct {
	AccessKeyID     string `json:"access_key_id" secret:"true"`
	AccessKeySecret string `json:"access_key_secret" secret:"true"`
	Region          string `json:"region"`
}

// CtyunDeployCred 天翼云部署凭证
type CtyunDeployCred struct {
	AccessKeyID     string `json:"access_key_id" secret:"true"`
	AccessKeySecret string `json:"access_key_secret" secret:"true"`
}

// VolcengineDeployCred 火山引擎部署凭证
type VolcengineDeployCred struct {
	AccessKeyID     string `json:"access_key_id" secret:"true"`
	AccessKeySecret string `json:"access_key_secret" secret:"true"`
	Region          string `json:"region"`
}

// regionOf 优先返回 region，为空时回退 region_id。
func regionOf(region, regionID string) string {
	if region != "" {
		return region
	}
	return regionID
}

func (c AliyunDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.AccessKeyID, AccessKeySecret: c.AccessKeySecret, Region: regionOf("", c.RegionID)}
}

func (c HuaweiDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.AccessKeyID, AccessKeySecret: c.SecretAccessKey, Region: regionOf(c.Region, "")}
}

func (c TencentDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.SecretID, AccessKeySecret: c.SecretKey, Region: regionOf(c.Region, "")}
}

func (c BaiduDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.AccessKeyID, AccessKeySecret: c.AccessKeySecret, Region: regionOf(c.Region, "")}
}

func (c CtyunDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.AccessKeyID, AccessKeySecret: c.AccessKeySecret}
}

func (c VolcengineDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.AccessKeyID, AccessKeySecret: c.AccessKeySecret, Region: regionOf(c.Region, "")}
}

// credentialer 可由结构化配置提取统一凭证的接口（方法为包内可见）。
type credentialer interface {
	toCredentials() cloudcred.Credentials
}

// parseCred 泛型解析为具体厂商凭证结构体并提取统一凭证，错误上抛。
func parseCred[T credentialer](raw []byte) (cloudcred.Credentials, error) {
	v, err := config.ParseConfig[T](raw)
	if err != nil {
		return cloudcred.Credentials{}, err
	}
	return v.toCredentials(), nil
}

// Parse 按厂商标识将存储的配置字节解析为统一凭证 cloudcred.Credentials。
// 使用泛型 config.ParseConfig 完成反序列化，故称「泛型解析」。
func Parse(providerType string, raw []byte) (cloudcred.Credentials, error) {
	switch providerType {
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
