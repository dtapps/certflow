//go:build darwin && biometric

package biometric

import _ "embed"

// 嵌入 macOS 生物识别 Helper
//
//go:embed binaries/macos/biometric_helper
var helperBinary []byte

// GetHelperBinary 获取当前平台对应的 Helper 二进制
func GetHelperBinary() ([]byte, error) {
	return helperBinary, nil
}
