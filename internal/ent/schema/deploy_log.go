package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// DeployLog 部署历史记录（每次部署尝试写入一条）
type DeployLog struct {
	ent.Schema
}

func (DeployLog) Fields() []ent.Field {
	return []ent.Field{
		field.Int("cert_id").
			Comment("证书 ID"),
		field.String("cert_domain").
			Comment("证书主域名"),
		field.String("deploy_domain").
			Comment("实际部署到的域名 / CDN 域名"),
		field.String("target_name").
			Optional().
			Comment("部署目标名称（冗余，便于历史查看）"),
		field.String("provider_type").
			Optional().
			Comment("云厂商"),
		field.String("deploy_service").
			Optional().
			Comment("部署服务"),
		field.Bool("success").
			Comment("是否成功"),
		field.String("message").
			Optional().
			Comment("结果 / 错误信息"),
		field.String("response").
			Optional().
			Comment("接口原始反馈 / 响应体（便于排查云端返回）"),
		field.String("cloud_cert_id").
			Optional().
			Comment("云厂商证书 ID"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("创建时间"),
	}
}

func (DeployLog) Edges() []ent.Edge {
	return []ent.Edge{
		// 所属部署目标（多对一）
		edge.From("deploy_target", DeployTarget.Type).
			Ref("deploy_logs").
			Unique().
			Required().
			Comment("所属部署目标"),
	}
}
