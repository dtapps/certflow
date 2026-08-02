package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// PasskeyCredential Passkey/WebAuthn 凭据
type PasskeyCredential struct {
	ent.Schema
}

func (PasskeyCredential) Fields() []ent.Field {
	return []ent.Field{
		field.Bytes("credential_id").
			NotEmpty().
			Comment("WebAuthn 凭据 ID"),
		field.Bytes("public_key").
			NotEmpty().
			Comment("公钥（CBOR 编码）"),
		field.Uint64("sign_count").
			Default(0).
			Comment("签名计数器"),
		field.String("attestation_type").
			Optional().
			Comment("证明类型"),
		field.String("authenticator_aaguid").
			Optional().
			Comment("认证器 AAGUID"),
		field.Bytes("authenticator_public_key").
			Optional().
			Comment("认证器公钥"),
		field.Int("auth_method_id").
			Comment("关联的认证方式 ID"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("创建时间"),
	}
}

func (PasskeyCredential) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("auth_method", AuthMethod.Type).
			Ref("passkey_credentials").
			Field("auth_method_id").
			Unique().
			Required(),
	}
}
