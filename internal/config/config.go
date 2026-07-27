// Package config 提供配置解析的泛型基础设施。
// 各实体的 config（deploy_credentials / deploy_targets / dns_providers）均存储为 JSON 字节，
// 解析时使用泛型函数反序列化为各自的强类型结构体，避免散落的 map 字符串魔法键。
package config

import (
	"encoding/json"
	"reflect"
	"strings"
)

// ParseConfig 泛型解析：将存储的 JSON 字节反序列化为类型 T。
// 空输入返回 T 的零值，调用方无需额外判空。
func ParseConfig[T any](raw []byte) (T, error) {
	var t T
	if len(raw) == 0 {
		return t, nil
	}
	err := json.Unmarshal(raw, &t)
	return t, err
}

// MustParseConfig 与 ParseConfig 行为一致，但忽略解析错误（用于配置缺失可安全降级为零值的场景）。
func MustParseConfig[T any](raw []byte) T {
	t, _ := ParseConfig[T](raw)
	return t
}

// Marshal 将任意配置结构体序列化回存储字节。
func Marshal[T any](v T) ([]byte, error) {
	return json.Marshal(v)
}

// AsMap 将结构体转回 map[string]string，用于兼容仍按 key 读取的下游消费者。
// 字段名以结构体的 json tag 为准（omitempty 的空字段会被丢弃）。
// 非字符串字段（如 []string）会被重新序列化为 JSON 字符串，保证信息不丢失。
func AsMap[T any](v T) map[string]string {
	b, err := json.Marshal(v)
	if err != nil {
		return map[string]string{}
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return map[string]string{}
	}
	m := map[string]string{}
	for k, val := range raw {
		if s, ok := val.(string); ok {
			m[k] = s
		} else if bb, err := json.Marshal(val); err == nil {
			m[k] = string(bb)
		}
	}
	return m
}

// secretFieldNames 通过反射收集结构体 T 中带 secret:"true" tag 的字段的 JSON 名。
// 敏感字段声明随结构体定义一处维护，无需额外的字符串映射表。
func secretFieldNames[T any]() map[string]bool {
	names := map[string]bool{}
	t := reflect.TypeFor[T]()
	for f := range t.Fields() {
		if _, ok := f.Tag.Lookup("secret"); !ok {
			continue
		}
		jt, ok := f.Tag.Lookup("json")
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(jt, ",")
		if name == "" || name == "-" {
			continue
		}
		names[name] = true
	}
	return names
}

// StripSecrets 从原始配置字节中剔除带 secret:"true" tag 的字段后返回，
// 用于向前端返回配置时去除 AK/SK 等敏感信息。
func StripSecrets[T any](raw []byte) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw, err
	}
	drop := secretFieldNames[T]()
	for k := range drop {
		delete(m, k)
	}
	return json.Marshal(m)
}
