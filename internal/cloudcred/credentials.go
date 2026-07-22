// Package cloudcred 定义跨包共享的云厂商凭证类型。
// 抽离到独立中性包，避免 deploy 与 deploycredential / dnsprovider 之间因
// Credentials 类型归属而产生的循环依赖。
package cloudcred

// Credentials 云厂商访问凭证（统一逻辑凭证，屏蔽各厂商字段名差异）
type Credentials struct {
	AccessKeyID     string
	AccessKeySecret string
	Region          string
}
