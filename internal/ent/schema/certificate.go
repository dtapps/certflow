package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// TXTRecord DNS TXT 记录
type TXTRecord struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Certificate SSL 证书
type Certificate struct {
	ent.Schema
}

func (Certificate) Fields() []ent.Field {
	return []ent.Field{
		field.String("domain").
			NotEmpty().
			Comment("主域名"),
		field.Strings("sans").
			Optional().
			Comment("备用域名"),
		field.Text("cert_content").
			Optional().
			Comment("证书 PEM 内容"),
		field.Text("key_content").
			Optional().
			Comment("私钥 PEM 内容"),
		field.String("issuer").
			Optional().
			Comment("证书颁发者"),
		field.Time("not_before").
			Optional().
			Comment("生效时间"),
		field.Time("not_after").
			Optional().
			Comment("过期时间"),
		field.Enum("status").
			Values("pending", "active", "expired", "revoked", "failed").
			Default("pending").
			Comment("证书状态"),
		field.Bool("auto_renew").
			Default(true).
			Comment("是否自动续期"),
		field.Int("renewal_days").
			Default(30).
			Comment("到期前多少天自动续期"),
		field.JSON("challenge_records", []TXTRecord{}).
			Optional().
			Comment("手动DNS挑战TXT记录列表（通配符可能多条）"),
		field.Enum("key_type").
			Values("EC256", "EC384", "RSA2048", "RSA3072", "RSA4096", "RSA8192").
			Default("EC256").
			Comment("密钥类型"),
		field.String("last_error").
			Optional().
			Comment("最后一次错误信息"),
		field.Time("last_renewed_at").
			Optional().
			Comment("最后续期时间"),
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

func (Certificate) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("ca", CA.Type).
			Ref("certificates").
			Unique().
			Comment("关联的 CA"),
		edge.From("dns_provider", DNSProvider.Type).
			Ref("certificates").
			Unique().
			Comment("关联的 DNS 提供商"),
		edge.To("renewal_logs", RenewalLog.Type),
	}
}
