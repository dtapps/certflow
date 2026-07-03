package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// RenewalLog 续期日志
type RenewalLog struct {
	ent.Schema
}

func (RenewalLog) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("status").
			Values("success", "failed", "pending").
			Comment("续期状态"),
		field.Text("error_message").
			Optional().
			Comment("错误信息"),
		field.Text("cert_content").
			Optional().
			Comment("新证书 PEM 内容"),
		field.Text("key_content").
			Optional().
			Comment("新私钥 PEM 内容"),
		field.Time("attempt_at").
			Comment("尝试时间"),
		field.Time("completed_at").
			Optional().
			Comment("完成时间"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("创建时间"),
	}
}

func (RenewalLog) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("certificate", Certificate.Type).
			Ref("renewal_logs").
			Unique().
			Required().
			Comment("关联的证书"),
	}
}
