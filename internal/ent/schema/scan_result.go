package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// ScanResult 证书扫描结果
type ScanResult struct {
	ent.Schema
}

func (ScanResult) Fields() []ent.Field {
	return []ent.Field{
		field.String("domain").
			NotEmpty().
			Comment("扫描的域名"),
		field.Int("port").
			Default(443).
			Comment("端口号"),
		field.Enum("scan_type").
			Values("https", "http").
			Default("https").
			Comment("扫描类型"),
		field.Time("scanned_at").
			Default(time.Now).
			Immutable().
			Comment("扫描时间"),
		field.Int("response_time_ms").
			Default(0).
			Comment("TLS 握手耗时（毫秒）"),
		field.Text("cert_issuer").
			Optional().
			Comment("证书颁发者"),
		field.Text("cert_subject").
			Optional().
			Comment("证书主题 (CN)"),
		field.Time("cert_not_before").
			Optional().
			Comment("证书生效时间"),
		field.Time("cert_not_after").
			Optional().
			Comment("证书过期时间"),
		field.Int("cert_remaining_days").
			Default(0).
			Comment("证书剩余天数"),
		field.Text("cert_fingerprint").
			Optional().
			Comment("证书 SHA256 指纹"),
		field.Text("cert_signature_algo").
			Optional().
			Comment("签名算法"),
		field.Text("cert_public_key_algo").
			Optional().
			Comment("公钥算法"),
		field.Int("cert_public_key_bits").
			Default(0).
			Comment("公钥位数"),
		field.JSON("cert_sans", []string{}).
			Optional().
			Comment("SAN 列表"),
		field.Text("cert_serial_number").
			Optional().
			Comment("证书序列号"),
		field.Text("error_message").
			Optional().
			Comment("扫描错误信息"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("创建时间"),
	}
}
