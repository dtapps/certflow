package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// AuthMethod 认证方式配置
type AuthMethod struct {
	ent.Schema
}

func (AuthMethod) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("method").
			Values("password", "totp", "passkey", "biometric").
			Comment("认证方式: password/totp/passkey/biometric"),
		field.Bool("is_active").
			Default(false).
			Comment("是否为当前激活的认证方式"),
		field.String("password_hash").
			Optional().
			Sensitive().
			Comment("密码哈希（bcrypt，仅 password 方式使用）"),
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

func (AuthMethod) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("totp_credentials", TOTPCredential.Type),
		edge.To("passkey_credentials", PasskeyCredential.Type),
	}
}
