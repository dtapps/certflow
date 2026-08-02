package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// TOTPCredential TOTP 凭据
type TOTPCredential struct {
	ent.Schema
}

func (TOTPCredential) Fields() []ent.Field {
	return []ent.Field{
		field.String("secret").
			NotEmpty().
			Sensitive().
			Comment("TOTP 密钥（Base32 编码）"),
		field.String("issuer").
			Default("CertFlow").
			Comment("发行者名称"),
		field.String("account_name").
			Default("user").
			Comment("账户名"),
		field.Int("auth_method_id").
			Comment("关联的认证方式 ID"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("创建时间"),
	}
}

func (TOTPCredential) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("auth_method", AuthMethod.Type).
			Ref("totp_credentials").
			Field("auth_method_id").
			Unique().
			Required(),
	}
}
