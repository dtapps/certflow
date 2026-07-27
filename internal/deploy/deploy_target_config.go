package deploy

import "encoding/json"

// StringSlice 是 []string 的兼容类型，反序列化时同时接受 JSON 数组和 JSON 字符串。
// 前端通过 map[string]string 提交数组字段时会双重编码（如 "["a","b"]"），
// 标准 json.Unmarshal 无法将字符串直接解入 []string，此类兜底处理。
type StringSlice []string

func (s *StringSlice) UnmarshalJSON(data []byte) error {
	var slice []string
	if err := json.Unmarshal(data, &slice); err == nil {
		*s = slice
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		if str == "" {
			*s = nil
			return nil
		}
		var inner []string
		if err := json.Unmarshal([]byte(str), &inner); err == nil {
			*s = inner
			return nil
		}
		*s = []string{str}
		return nil
	}
	return nil
}
