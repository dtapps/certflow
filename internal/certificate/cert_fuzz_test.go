package certificate

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/go-acme/lego/v5/certcrypto"
)

// FuzzParseCertificate 模糊测试证书解析
func FuzzParseCertificate(f *testing.F) {
	// 有效 PEM 证书种子
	validCert, _ := generateTestCert()
	f.Add(validCert)
	f.Add([]byte("-----BEGIN CERTIFICATE-----\nMIIBkTCB+wIJAL...\n-----END CERTIFICATE-----"))
	f.Add([]byte("not a certificate"))
	f.Add([]byte{})
	f.Add([]byte{0, 0, 0})
	f.Add([]byte("-----BEGIN CERTIFICATE-----\n"))
	f.Add(make([]byte, 10000))

	f.Fuzz(func(t *testing.T, certBytes []byte) {
		// 不应 panic
		cert, err := parseCertificate(certBytes)
		if err != nil {
			return
		}
		// 解析成功即为有效路径
		_ = cert
	})
}

// FuzzLoadPrivateKey 模糊测试私钥解析
func FuzzLoadPrivateKey(f *testing.F) {
	// 有效 PEM 私钥种子
	validKey, _ := generateTestPrivateKey()
	f.Add(validKey)
	f.Add([]byte("-----BEGIN EC PRIVATE KEY-----\nMIIBk...\n-----END EC PRIVATE KEY-----"))
	f.Add([]byte("not a key"))
	f.Add([]byte{})
	f.Add([]byte{0, 0, 0})
	f.Add(make([]byte, 5000))

	f.Fuzz(func(t *testing.T, keyBytes []byte) {
		// 不应 panic
		key, err := loadPrivateKeyFromContent(keyBytes)
		if err != nil {
			return
		}
		// 如果解析成功，应能签名
		if key != nil {
			_ = key.Public()
		}
	})
}

// FuzzParseKeyType 模糊测试密钥类型解析
func FuzzParseKeyType(f *testing.F) {
	f.Add("EC256")
	f.Add("EC384")
	f.Add("RSA2048")
	f.Add("RSA4096")
	f.Add("")
	f.Add("unknown")
	f.Add("EC256\nINJECTION")
	f.Add(string([]byte{0}))

	f.Fuzz(func(t *testing.T, keyType string) {
		result := parseKeyType(keyType)
		// 应返回有效的 KeyType
		switch result {
		case certcrypto.EC256, certcrypto.EC384, certcrypto.RSA2048, certcrypto.RSA4096:
			// 有效
		default:
			t.Errorf("parseKeyType(%q) = %v, unexpected value", keyType, result)
		}
	})
}

// FuzzFormatRecords 模糊测试 TXT 记录格式化
func FuzzFormatRecords(f *testing.F) {
	f.Add("example.com", "value1")
	f.Add("", "")
	f.Add("_acme-challenge.example.com", "abc123")
	f.Add(string([]byte{0}), "test")

	f.Fuzz(func(t *testing.T, name, value string) {
		records := []TXTRecord{{Name: name, Value: value}}
		result := formatRecords(records)
		_ = result
	})
}

// generateTestCert 生成测试用 PEM 证书
func generateTestCert() ([]byte, error) {
	key, _ := generateTestPrivateKeyEcdsa()
	template := &x509.Certificate{
		DNSNames: []string{"example.com"},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), nil
}

// generateTestPrivateKey 生成测试用 PEM 私钥
func generateTestPrivateKey() ([]byte, error) {
	key, err := generateTestPrivateKeyEcdsa()
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
}

func generateTestPrivateKeyEcdsa() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}
