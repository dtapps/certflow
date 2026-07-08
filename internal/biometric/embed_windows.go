//go:build windows && biometric

package biometric

import _ "embed"

// 嵌入 Windows 生物识别 Helper
//
//go:embed binaries/windows/biometric_helper.exe
var helperBinary []byte

// GetHelperBinary 获取当前平台对应的 Helper 二进制
func GetHelperBinary() ([]byte, error) {
	return helperBinary, nil
}
