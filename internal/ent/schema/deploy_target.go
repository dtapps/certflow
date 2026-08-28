package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// DeployTarget 证书部署目标（云厂商资源，如阿里云 CDN / 腾讯云 CDN / 华为云 SCM）
type DeployTarget struct {
	ent.Schema
}

func (DeployTarget) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			Comment("部署目标名称"),
		field.Enum("provider_type").
			Values(DeployProviderTypes...).
			Comment("云厂商"),
		field.String("deploy_service").
			NotEmpty().
			Comment("部署服务：cdn / clb / slb / scm / oss 等"),
		field.JSON("config", DeployTargetConfig{}).
			Optional().
			Comment("服务配置 JSON（region、资源 ID、证书名等）"),
		field.Enum("credential_source").
			Values("dns_provider", "deploy_credential").
			Default("dns_provider").
			Comment("凭证来源：复用 DNS 凭证 或 部署凭证"),
		field.Bool("is_active").
			Default(true).
			Comment("是否启用"),
		field.String("comment").
			Optional().
			Comment("备注"),
		field.Time("last_deployed_at").
			Optional().
			Comment("最后部署时间"),
		field.String("last_status").
			Optional().
			Comment("最后部署状态"),
		field.String("last_error").
			Optional().
			Comment("最后部署错误"),
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

func (DeployTarget) Edges() []ent.Edge {
	return []ent.Edge{
		// 凭证复用：一个部署目标可关联一个 DNS 提供商（多对一），直接复用其 AccessKey/Secret
		edge.From("dns_provider", DNSProvider.Type).
			Ref("deploy_targets").
			Unique().
			Comment("凭证复用的 DNS 提供商"),
		// 凭证关联：一个部署目标可关联一个部署凭证（多对一）
		edge.From("deploy_credential", DeployCredential.Type).
			Ref("deploy_targets").
			Unique().
			Comment("关联的部署凭证"),
		// 关联证书（多对多）：一个目标可部署多个证书，一个证书可部署到多个目标
		edge.To("certificates", Certificate.Type).
			Comment("关联部署的证书"),
		// 部署历史记录（一对多）：一个目标可有多条部署日志
		edge.To("deploy_logs", DeployLog.Type).
			Comment("部署历史记录"),
	}
}
