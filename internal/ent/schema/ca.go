package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// CA 证书颁发机构
type CA struct {
	ent.Schema
}

func (CA) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			Comment("CA 名称"),
		field.String("directory_url").
			NotEmpty().
			Comment("ACME 目录 URL"),
		field.String("account_email").
			Optional().
			Comment("注册邮箱"),
		field.String("eab_kid").
			Optional().
			Comment("EAB KID（部分 CA 如 LiteSSL/ZeroSSL 需要）"),
		field.String("eab_hmac").
			Optional().
			Comment("EAB HMAC Key（部分 CA 如 LiteSSL/ZeroSSL 需要）"),
		field.Bool("is_active").
			Default(false).
			Comment("是否启用"),
		field.Bool("is_builtin").
			Default(false).
			Immutable().
			Comment("是否内置 CA（仅创建时可设置，禁止修改；内置 CA 禁止删除）"),
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

func (CA) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("certificates", Certificate.Type),
	}
}
