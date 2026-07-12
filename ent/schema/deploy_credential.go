package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// DeployCredential 部署凭证
type DeployCredential struct {
	ent.Schema
}

func (DeployCredential) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			Comment("凭证名称"),
		field.Enum("provider_type").
			Values(
				// 云厂商
				"aliyun", "tencentcloud", "huawei", "baiducloud",
				// 面板
				"btpanel", "1panel", "acepanel",
			).
			Comment("提供商类型"),
		field.Bytes("config").
			Optional().
			Comment("配置 JSON（API 密钥等）"),
		field.Bool("is_active").
			Default(true).
			Comment("是否启用"),
		field.String("comment").
			Optional().
			Comment("备注"),
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

func (DeployCredential) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("deploy_targets", DeployTarget.Type).
			Comment("使用该凭证的部署目标"),
	}
}
