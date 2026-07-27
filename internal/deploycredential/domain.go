package deploycredential

import "cnb.cool/dtapp/certflow/internal/cloudcred"

// CDN/云厂商类部署凭证（domain）：使用 AK/SK + Region。
// 各厂商字段名不同，通过 json tag 精确映射存储的 JSON 键名。

// regionOf 优先返回 region，为空时回退 region_id。
func regionOf(region, regionID string) string {
	if region != "" {
		return region
	}
	return regionID
}

// AliyunDeployCred 阿里云部署凭证
type AliyunDeployCred struct {
	AccessKeyID     string `json:"access_key_id" secret:"true"`
	AccessKeySecret string `json:"access_key_secret" secret:"true"`
	RegionID        string `json:"region_id"`
}

func (c AliyunDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.AccessKeyID, AccessKeySecret: c.AccessKeySecret, Region: regionOf("", c.RegionID)}
}

// HuaweiDeployCred 华为云部署凭证
type HuaweiDeployCred struct {
	AccessKeyID     string `json:"access_key_id" secret:"true"`
	SecretAccessKey string `json:"secret_access_key" secret:"true"`
	Region          string `json:"region"`
}

func (c HuaweiDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.AccessKeyID, AccessKeySecret: c.SecretAccessKey, Region: regionOf(c.Region, "")}
}

// TencentDeployCred 腾讯云部署凭证
type TencentDeployCred struct {
	SecretID  string `json:"secret_id" secret:"true"`
	SecretKey string `json:"secret_key" secret:"true"`
	Region    string `json:"region"`
}

func (c TencentDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.SecretID, AccessKeySecret: c.SecretKey, Region: regionOf(c.Region, "")}
}

// BaiduDeployCred 百度云部署凭证（部署来源使用 access_key_secret）
type BaiduDeployCred struct {
	AccessKeyID     string `json:"access_key_id" secret:"true"`
	AccessKeySecret string `json:"access_key_secret" secret:"true"`
	Region          string `json:"region"`
}

func (c BaiduDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.AccessKeyID, AccessKeySecret: c.AccessKeySecret, Region: regionOf(c.Region, "")}
}

// CtyunDeployCred 天翼云部署凭证
type CtyunDeployCred struct {
	AccessKeyID     string `json:"access_key_id" secret:"true"`
	AccessKeySecret string `json:"access_key_secret" secret:"true"`
}

func (c CtyunDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.AccessKeyID, AccessKeySecret: c.AccessKeySecret}
}

// VolcengineDeployCred 火山引擎部署凭证
type VolcengineDeployCred struct {
	AccessKeyID     string `json:"access_key_id" secret:"true"`
	AccessKeySecret string `json:"access_key_secret" secret:"true"`
	Region          string `json:"region"`
}

func (c VolcengineDeployCred) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.AccessKeyID, AccessKeySecret: c.AccessKeySecret, Region: regionOf(c.Region, "")}
}
