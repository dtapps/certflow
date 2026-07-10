package logging

import "testing"

// FuzzParseLevel 模糊测试日志级别解析
func FuzzParseLevel(f *testing.F) {
	f.Add("DEBUG")
	f.Add("info")
	f.Add("WARN")
	f.Add("WARNING")
	f.Add("Error")
	f.Add("")
	f.Add("verbose")
	f.Add("trace")
	f.Add("debug123")
	f.Add("INFO\nINJECTION")
	f.Add(string([]byte{0, 0, 0}))

	f.Fuzz(func(t *testing.T, s string) {
		level := ParseLevel(s)
		// 不应 panic，且返回值应在已知范围内
		if level < DEBUG || level > ERROR {
			// UNKNOWN 级别映射为 INFO
			if level != INFO {
				t.Errorf("ParseLevel(%q) = %d, out of range", s, level)
			}
		}
	})
}

// FuzzParseLevelRoundTrip 模糊测试 ParseLevel 的往返一致性
func FuzzParseLevelRoundTrip(f *testing.F) {
	for _, l := range []Level{DEBUG, INFO, WARN, ERROR} {
		f.Add(l.String())
	}

	f.Fuzz(func(t *testing.T, s string) {
		level := ParseLevel(s)
		// 已知级别字符串解析后，String() 应回到原值
		roundTrip := ParseLevel(level.String())
		if roundTrip != level {
			t.Errorf("ParseLevel(%q) = %v, but ParseLevel(%v.String()) = %v", s, level, level, roundTrip)
		}
	})
}
