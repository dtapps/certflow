package deploy

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"cnb.cool/dtapp/certflow/internal/i18n"
)

// parseCertPEM 从证书 PEM（可能含链）中提取第一张（叶子）证书并解析关键信息。
func parseCertPEM(certPEM string) (*CurrentCert, error) {
	certPEM = strings.TrimSpace(certPEM)
	if certPEM == "" {
		return nil, i18n.NewError("deploy.error.current_cert_empty")
	}
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, i18n.NewError("deploy.error.current_cert_no_pem")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.current_cert_parse")
	}
	sans := make([]string, 0, len(cert.DNSNames)+len(cert.IPAddresses))
	sans = append(sans, cert.DNSNames...)
	for _, ip := range cert.IPAddresses {
		sans = append(sans, ip.String())
	}
	return &CurrentCert{
		CommonName:   cert.Subject.CommonName,
		SANs:         sans,
		Issuer:       cert.Issuer.CommonName,
		NotBefore:    cert.NotBefore.Format(time.RFC3339),
		NotAfter:     cert.NotAfter.Format(time.RFC3339),
		SerialNumber: fmt.Sprintf("%X", cert.SerialNumber),
	}, nil
}
