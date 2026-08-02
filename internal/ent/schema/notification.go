package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// Notification 通知记录
type Notification struct {
	ent.Schema
}

func (Notification) Fields() []ent.Field {
	return []ent.Field{
		field.String("title").
			NotEmpty().
			Comment("通知标题"),
		field.String("body").
			Optional().
			Comment("通知内容"),
		field.Enum("category").
			Values("cert", "dns", "monitor", "system", "deploy").
			Default("system").
			Optional().
			Comment("通知业务分类"),
		field.Enum("level").
			Values("success", "error", "warning", "info").
			Default("info").
			Optional().
			Comment("通知状态（成功/错误/警告/信息）"),
		field.Bool("read").
			Default(false).
			Comment("是否已读"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("创建时间"),
	}
}
