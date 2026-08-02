package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// MonitoredDomain 监控的域名
type MonitoredDomain struct {
	ent.Schema
}

func (MonitoredDomain) Fields() []ent.Field {
	return []ent.Field{
		field.String("domain").
			NotEmpty().
			Unique().
			Comment("域名"),
		field.Int("port").
			Default(443).
			Comment("端口号"),
		field.Enum("check_type").
			Values("https", "http").
			Default("https").
			Comment("检查类型"),
		field.String("url").
			Optional().
			Comment("HTTP 健康检查 URL"),
		field.Int("check_interval").
			Default(3600).
			Comment("检查间隔（秒）"),
		field.Bool("enabled").
			Default(false).
			Comment("是否启用监控"),
		field.Enum("status").
			Values("ok", "warning", "error", "expired", "unknown").
			Default("unknown").
			Comment("当前状态"),
		field.Text("cert_issuer").
			Optional().
			Comment("证书颁发者"),
		field.Time("cert_not_before").
			Optional().
			Comment("证书生效时间"),
		field.Time("cert_not_after").
			Optional().
			Comment("证书过期时间"),
		field.Text("cert_fingerprint").
			Optional().
			Comment("证书 SHA256 指纹"),
		field.Text("cert_subject").
			Optional().
			Comment("证书主题"),
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
			Comment("证书 SAN 列表"),
		field.Int("cert_remaining_days").
			Default(0).
			Comment("证书剩余天数"),
		field.Time("last_check_at").
			Optional().
			Comment("最后检查时间"),
		field.Text("last_check_error").
			Optional().
			Comment("最后检查错误"),
		field.Int("http_status_code").
			Default(0).
			Comment("HTTP 响应码"),
		field.Int("response_time_ms").
			Default(0).
			Comment("响应时间（毫秒）"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("创建时间"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("更新时间"),
	}
}

// Edges 定义与检查历史记录的关联
func (MonitoredDomain) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("check_logs", MonitorCheckLog.Type).
			Comment("检查历史记录"),
	}
}
