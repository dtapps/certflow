package certificate

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	legocert "github.com/go-acme/lego/v5/certificate"
)

// makeSelfSigned 造一份「真」的自签证书 PEM，用来验证 PEM 相关的解析/生成逻辑。
// 这些函数在线上下发证书后一定会被调用，解析失败就等于证书装不上。
func makeSelfSigned(t *testing.T, domain string) (certPEM, keyPEM []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}

	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1234),
		Subject:               pkix.Name{CommonName: domain},
		DNSNames:              []string{domain, "www." + domain},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("签发自签证书失败: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("序列化私钥失败: %v", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

func TestParseCertificate_SelfSigned(t *testing.T) {
	certPEM, _ := makeSelfSigned(t, "example.com")

	cert, err := parseCertificate(certPEM)
	if err != nil {
		t.Fatalf("parseCertificate err: %v", err)
	}
	if cert.Subject.CommonName != "example.com" {
		t.Fatalf("CN 不符: %q", cert.Subject.CommonName)
	}
	if len(cert.DNSNames) != 2 || cert.DNSNames[0] != "example.com" {
		t.Fatalf("DNSNames 不符: %v", cert.DNSNames)
	}
	if !cert.NotAfter.After(time.Now()) {
		t.Fatalf("NotAfter 应是未来时间: %v", cert.NotAfter)
	}
}

// 传入私钥 PEM 时 pem.Decode 能成功、x509.ParseCertificate 必然失败，
// 这条路径必须返回错误而不是返回一个「空证书」让上层算出错误的到期时间。
func TestParseCertificate_WrongPEMType(t *testing.T) {
	_, keyPEM := makeSelfSigned(t, "example.com")
	if _, err := parseCertificate(keyPEM); err == nil {
		t.Fatal("用私钥 PEM 当证书解析应报错")
	}
}

func TestGenerateCertPEM_Trims(t *testing.T) {
	certPEM, keyPEM := makeSelfSigned(t, "example.com")

	certOut, keyOut := generateCertPEM(&legocert.Resource{
		Certificate: append([]byte("\n\n"), append(certPEM, []byte("\n\n ")...)...),
		PrivateKey:  append([]byte("\n"), append(keyPEM, '\n')...),
	})

	if strings.HasPrefix(certOut, "\n") || strings.HasSuffix(certOut, " ") || strings.HasSuffix(certOut, "\n") {
		t.Fatalf("证书内容首尾空白未清除: %q", certOut[:10])
	}
	if !strings.HasPrefix(keyOut, "-----BEGIN EC PRIVATE KEY-----") {
		t.Fatalf("私钥内容不符: %q", keyOut)
	}

	// 清理后仍必须可解析
	if _, err := parseCertificate([]byte(certOut)); err != nil {
		t.Fatalf("清理后证书不可解析: %v", err)
	}
	if _, err := loadPrivateKeyFromContent([]byte(keyOut)); err != nil {
		t.Fatalf("清理后私钥不可解析: %v", err)
	}
}

func TestParseKeyType(t *testing.T) {
	// lego 只认 EC256 / EC384 / RSA2048 / RSA4096 / RSA8192（没有 EC512）
	cases := map[string]string{
		"":        "EC256", // 未指定时默认 EC256
		"EC256":   "EC256",
		"EC384":   "EC384",
		"RSA2048": "RSA2048",
		"RSA4096": "RSA4096",
		"RSA8192": "RSA8192",
		// 非法值一律兜底成 EC256：不能让一个错字让整个签发流程 panic
		"EC512":   "EC256",
		"P256":    "EC256",
		"unknown": "EC256",
	}

	for in, want := range cases {
		if got := string(parseKeyType(in)); got != want {
			t.Errorf("parseKeyType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGenerateKeyByType(t *testing.T) {
	cases := []struct {
		keyType string
		want    string // 期望的具体类型
	}{
		{"", "ECDSA"},
		{"EC256", "ECDSA"},
		{"EC384", "ECDSA"},
		{"RSA2048", "RSA"},
		{"RSA4096", "RSA"},
		{"garbage", "ECDSA"}, // 非法值兜底
		{"EC512", "ECDSA"},   // lego 不支持 EC512，兜底成 EC256
	}

	for _, c := range cases {
		t.Run(c.keyType, func(t *testing.T) {
			signer, err := generateKeyByType(c.keyType)
			if err != nil {
				t.Fatalf("generateKeyByType(%q) err: %v", c.keyType, err)
			}
			switch v := signer.(type) {
			case *ecdsa.PrivateKey:
				if c.want != "ECDSA" {
					t.Fatalf("%q 应生成 %s, 实际 ECDSA", c.keyType, c.want)
				}
				want := elliptic.P256()
				switch c.keyType {
				case "EC384":
					want = elliptic.P384()
				}
				if v.Curve != want {
					t.Fatalf("%q 曲线应为 %v", c.keyType, want.Params().Name)
				}
			case *rsa.PrivateKey:
				if c.want != "RSA" {
					t.Fatalf("%q 应生成 %s, 实际 RSA", c.keyType, c.want)
				}
				if bits := v.N.BitLen(); bits != 2048 && bits != 4096 {
					t.Fatalf("%q 密钥长度异常: %d", c.keyType, bits)
				}
			default:
				t.Fatalf("%q 返回了未知类型 %T", c.keyType, signer)
			}

			// 生成的密钥必须能真正签名（generateKeyByType 返回 nil,nil 之类的假值会在 ACME 阶段炸）
			digest := sha256.Sum256([]byte("hello"))
			sig, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
			if err != nil {
				t.Fatalf("%q 生成的密钥无法签名: %v", c.keyType, err)
			}
			if len(sig) == 0 {
				t.Fatalf("%q 签名为空", c.keyType)
			}
		})
	}
}

func TestFormatRecords(t *testing.T) {
	if got := formatRecords(nil); got != "" {
		t.Fatalf("空记录应返回空串, 实际 %q", got)
	}
	got := formatRecords([]TXTRecord{
		{Name: "_acme-challenge.example.com", Value: "abc"},
		{Name: "_acme-challenge.www.example.com", Value: "def"},
	})
	want := "_acme-challenge.example.com = abc\n_acme-challenge.www.example.com = def"
	if got != want {
		t.Fatalf("格式不符:\n got %q\n want %q", got, want)
	}
}

// TestWaitForChallenge 手动 DNS 模式下，前端靠这个接口拿 TXT 记录；
// 记录齐了要立刻返回（不能让用户干等），记录没齐要超时报错（不能静默成功）。
func TestWaitForChallenge(t *testing.T) {
	t.Run("记录齐全时立即返回", func(t *testing.T) {
		p := NewManualDNSProvider()
		if err := p.Present(t.Context(), "example.com", "tok", "tok.thumb"); err != nil {
			t.Fatalf("Present err: %v", err)
		}

		start := time.Now()
		info, err := p.WaitForChallenge(10*time.Second, 1)
		if err != nil {
			t.Fatalf("WaitForChallenge err: %v", err)
		}
		if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
			t.Fatalf("记录已就绪却等了 %v", elapsed)
		}
		if info == nil || len(info.Records) != 1 || info.Domain != "example.com" {
			t.Fatalf("返回内容不符: %+v", info)
		}
	})

	t.Run("记录不足且超时", func(t *testing.T) {
		p := NewManualDNSProvider()
		if err := p.Present(t.Context(), "example.com", "tok", "tok.thumb"); err != nil {
			t.Fatalf("Present err: %v", err)
		}
		// 期望 2 条（通配符 + 主域名），只有 1 条 → 超时后仍会把已有记录带回去
		info, err := p.WaitForChallenge(600*time.Millisecond, 2)
		if err != nil {
			t.Fatalf("有记录时超时不应报错: %v", err)
		}
		if info == nil || len(info.Records) != 1 {
			t.Fatalf("应带回已有的 1 条记录, 实际 %+v", info)
		}
	})

	t.Run("完全没有记录时报超时", func(t *testing.T) {
		p := NewManualDNSProvider()
		if _, err := p.WaitForChallenge(600*time.Millisecond, 1); err == nil {
			t.Fatal("无记录应返回超时错误")
		}
	})
}

func TestAcmeUser_Getters(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}
	u := &acmeUser{Email: "a@b.c", Key: key}

	if u.GetEmail() != "a@b.c" {
		t.Fatalf("GetEmail 不符: %q", u.GetEmail())
	}
	if u.GetPrivateKey() != key {
		t.Fatal("GetPrivateKey 应返回同一个 signer")
	}
	if u.GetRegistration() != nil {
		t.Fatal("未注册时 GetRegistration 应为 nil")
	}
}

// TestLoadPrivateKeyFromContent_KeyTypes 撤销证书要读出库里存的私钥，
// 用户可选 EC256/EC384/RSA2048/RSA4096/RSA8192 —— 每种都必须能读回来，
// 只认 EC 会让选 RSA 的证书永远撤销失败。
func TestLoadPrivateKeyFromContent_KeyTypes(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成 EC 密钥失败: %v", err)
	}
	ecDER, err := x509.MarshalECPrivateKey(ecKey)
	if err != nil {
		t.Fatalf("序列化 EC 密钥失败: %v", err)
	}

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("生成 RSA 密钥失败: %v", err)
	}
	rsaPKCS1 := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaKey)})

	pkcs8DER, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	if err != nil {
		t.Fatalf("序列化 PKCS8 失败: %v", err)
	}
	rsaPKCS8 := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8DER})

	t.Run("EC", func(t *testing.T) {
		key, err := loadPrivateKeyFromContent(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: ecDER}))
		if err != nil {
			t.Fatalf("EC 私钥应能解析: %v", err)
		}
		// 用公钥比对（ecdsa.PrivateKey.D 自 Go 1.26 起已废弃，直接读标量会触发 SA1019）
		if !key.PublicKey.Equal(&ecKey.PublicKey) {
			t.Fatal("解析出来的 EC 私钥不一致")
		}
	})

	// RSA 必须能解析成 crypto.Signer（撤销链路走这个函数）
	for name, keyPEM := range map[string][]byte{"PKCS1": rsaPKCS1, "PKCS8": rsaPKCS8} {
		t.Run("RSA_"+name, func(t *testing.T) {
			signer, err := loadSignerFromContent(keyPEM)
			if err != nil {
				t.Fatalf("%s RSA 私钥应能解析: %v", name, err)
			}
			if _, ok := signer.(*rsa.PrivateKey); !ok {
				t.Fatalf("%s 应解析为 *rsa.PrivateKey, 实际 %T", name, signer)
			}
		})
	}

	t.Run("垃圾内容", func(t *testing.T) {
		if _, err := loadSignerFromContent([]byte("not a pem")); err == nil {
			t.Fatal("非 PEM 内容应报错")
		}
	})
}
