package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// MonitorCheckLog 域名检查历史记录（趋势图数据源）
type MonitorCheckLog struct {
	ent.Schema
}

func (MonitorCheckLog) Fields() []ent.Field {
	return []ent.Field{
		field.Time("checked_at").
			Default(time.Now).
			Comment("检查时间"),
		field.Enum("status").
			Values("ok", "warning", "error", "expired", "unknown").
			Default("unknown").
			Comment("检查状态"),
		field.Int("cert_remaining_days").
			Optional().
			Nillable().
			Comment("证书剩余天数（HTTP 检查无证书时为 null）"),
		field.Int("response_time_ms").
			Optional().
			Nillable().
			Comment("响应时间（毫秒）"),
		field.Int("http_status_code").
			Optional().
			Nillable().
			Comment("HTTP 响应码"),
		field.Text("last_check_error").
			Optional().
			Comment("检查错误信息"),
	}
}

func (MonitorCheckLog) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("domain", MonitoredDomain.Type).
			Ref("check_logs").
			Unique().
			Required().
			Comment("关联的监控域名"),
	}
}
