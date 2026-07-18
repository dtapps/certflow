package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// DNSProvider DNS 提供商
type DNSProvider struct {
	ent.Schema
}

func (DNSProvider) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			Comment("提供商名称"),
		field.Enum("provider_type").
			Values(
				"cloudflare", "aliyun", "tencentcloud", "huawei", "aws", "googlecloud",
				"baiducloud", "jdcloud", "volcengine", "edgeone", "aliesa",
				"ucloud", "westcn", "com35", "rainyun", "todaynic",
				"dnsla", "dns51", "xinnet",
			).
			Comment("提供商类型"),
		field.Bytes("config").
			Optional().
			Comment("配置 JSON"),
		field.Bool("is_active").
			Default(false).
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

func (DNSProvider) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("certificates", Certificate.Type),
		edge.To("deploy_targets", DeployTarget.Type).
			Comment("复用该提供商凭证的部署目标"),
	}
}
