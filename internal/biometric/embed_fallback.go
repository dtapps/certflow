//go:build !biometric || (!darwin && !windows)

package biometric

// GetHelperBinary 获取当前平台对应的 Helper 二进制
// 未启用 biometric tag 或平台不支持时返回 nil
func GetHelperBinary() ([]byte, error) {
	return nil, nil
}
