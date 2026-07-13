package schema

import (
	"encoding/json"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// HttpLog 独立的 HTTP 请求日志（入库到单独的日志数据库，避免污染主库）
type HttpLog struct {
	ent.Schema
}

// Annotations of the HttpLog.
func (HttpLog) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "http_log"},
		entsql.WithComments(true),
		schema.Comment("HTTP 请求日志"),
	}
}

// Fields of the HttpLog.
func (HttpLog) Fields() []ent.Field {
	return []ent.Field{
		field.String("hostname").
			Optional().
			Immutable().
			Comment("主机名").
			Annotations(entsql.WithComments(true)),

		field.String("method").
			Optional().
			Immutable().
			Comment("请求方法").
			Annotations(entsql.WithComments(true)),

		field.String("url").
			Optional().
			Immutable().
			Comment("请求 URL").
			Annotations(entsql.WithComments(true)),

		field.JSON("request_headers", map[string][]string{}).
			Optional().
			Immutable().
			Comment("请求头").
			Annotations(entsql.WithComments(true)),

		field.JSON("request_body", json.RawMessage{}).
			Optional().
			Immutable().
			Comment("请求体").
			Annotations(entsql.WithComments(true)),

		field.Int("status_code").
			Optional().
			Immutable().
			Comment("状态码").
			Annotations(entsql.WithComments(true)),

		field.JSON("response_headers", map[string][]string{}).
			Optional().
			Immutable().
			Comment("响应头").
			Annotations(entsql.WithComments(true)),

		field.JSON("response_body", json.RawMessage{}).
			Optional().
			Immutable().
			Comment("响应体").
			Annotations(entsql.WithComments(true)),

		field.Int64("elapse_time").
			Optional().
			Immutable().
			Comment("耗时（毫秒）").
			Annotations(entsql.WithComments(true)),

		field.Int64("process_elapse_time").
			Optional().
			Immutable().
			Comment("处理耗时（毫秒）").
			Annotations(entsql.WithComments(true)),

		field.Bool("is_error").
			Default(false).
			Optional().
			Immutable().
			Comment("是否出错（如 5xx、4xx）").
			Annotations(entsql.WithComments(true)),

		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("请求时间").
			Annotations(entsql.WithComments(true)),

		field.String("go_version").
			Optional().
			Immutable().
			Comment("Go 版本").
			Annotations(entsql.WithComments(true)),

		field.String("plugin_version").
			Optional().
			Immutable().
			Comment("插件版本").
			Annotations(entsql.WithComments(true)),
	}
}

// Edges of the HttpLog.
func (HttpLog) Edges() []ent.Edge {
	return []ent.Edge{}
}

// Indexes of the HttpLog.
func (HttpLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("hostname").
			Annotations(entsql.WithComments(true)),
		index.Fields("method").
			Annotations(entsql.WithComments(true)),
		index.Fields("url").
			Annotations(entsql.WithComments(true)),
		index.Fields("status_code").
			Annotations(entsql.WithComments(true)),
		index.Fields("created_at").
			Annotations(entsql.Desc(), entsql.WithComments(true)),
	}
}

// Hooks of the HttpLog.
func (HttpLog) Hooks() []ent.Hook {
	return []ent.Hook{}
}
