package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CertUpload 记录「证书 + 云账号」到云端证书 ID 的映射，用于跨进程去重。
// 确保同一张证书对同一个云账号（厂商 + AccessKey + 区域）只真正上传一次，
// 之后的部署直接复用云端证书 ID，而不是每次都重新上传（也不依赖各云厂商的原生去重能力）。
type CertUpload struct {
	ent.Schema
}

func (CertUpload) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider").Comment("云厂商：aliyun / tencentcloud / huawei"),
		field.String("access_key_id").Comment("用于区分云账号的 AccessKeyId（非密钥）"),
		field.String("region").Comment("区域"),
		field.String("cert_fingerprint").Comment("证书内容指纹（sha256），用于标识同一张证书"),
		field.String("cloud_cert_id").Comment("云端返回的证书 ID"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (CertUpload) Indexes() []ent.Index {
	return []ent.Index{
		// 同一云账号下，一张证书只应有一条记录，作为去重的唯一键。
		index.Fields("provider", "access_key_id", "region", "cert_fingerprint").Unique(),
	}
}
