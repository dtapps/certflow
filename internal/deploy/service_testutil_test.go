package deploy

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"

	"cnb.cool/dtapp/certflow/internal/ent"
	"cnb.cool/dtapp/certflow/internal/ent/certificate"
	"cnb.cool/dtapp/certflow/internal/ent/deploycredential"
	"cnb.cool/dtapp/certflow/internal/ent/deploytarget"
	"cnb.cool/dtapp/certflow/internal/ent/dnsprovider"
	"cnb.cool/dtapp/certflow/internal/ent/enttest"
	"cnb.cool/dtapp/certflow/internal/ent/schema"
	"cnb.cool/dtapp/certflow/internal/sqlite"
)

// newDeployTestService 建临时库并返回部署服务。
func newDeployTestService(t *testing.T) (*DeployService, *ent.Client) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, sqlite.BuildDSN(filepath.Join(t.TempDir(), "deploy.db")))
	t.Cleanup(func() { _ = client.Close() })
	return NewDeployService(client), client
}

// mustCreateCert 创建一张证书（内容为真实生成的 EC 密钥对，便于面板类部署器校验配对）。
func mustCreateCert(t *testing.T, client *ent.Client, domain string) *ent.Certificate {
	t.Helper()
	keyPEM, certPEM := genKeyCert(t)
	c, err := client.Certificate.Create().
		SetDomain(domain).
		SetSans([]string{domain, "www." + domain}).
		SetIssuer("Test CA").
		SetStatus(certificate.StatusActive).
		SetCertContent(certPEM).
		SetKeyContent(keyPEM).
		SetNotBefore(time.Now().Add(-time.Hour)).
		SetNotAfter(time.Now().Add(60 * 24 * time.Hour)).
		Save(context.Background())
	if err != nil {
		t.Fatalf("创建证书失败: %v", err)
	}
	return c
}

// mustCreateDeployCredential 创建部署凭证。
func mustCreateDeployCredential(t *testing.T, client *ent.Client, name, providerType string, cfg schema.DeployCredentialConfig) *ent.DeployCredential {
	t.Helper()
	c, err := client.DeployCredential.Create().
		SetName(name).
		SetProviderType(deploycredential.ProviderType(providerType)).
		SetConfig(cfg).
		SetIsActive(true).
		Save(context.Background())
	if err != nil {
		t.Fatalf("创建部署凭证失败: %v", err)
	}
	return c
}

// mustCreateDNSProvider 创建 DNS 提供商（供 dns_provider 凭证来源复用）。
func mustCreateDNSProvider(t *testing.T, client *ent.Client, name, providerType string, cfg schema.DNSProviderConfig) *ent.DNSProvider {
	t.Helper()
	p, err := client.DNSProvider.Create().
		SetName(name).
		SetProviderType(dnsprovider.ProviderType(providerType)).
		SetConfig(cfg).
		SetIsActive(true).
		Save(context.Background())
	if err != nil {
		t.Fatalf("创建 DNS 提供商失败: %v", err)
	}
	return p
}

// targetOpt 部署目标可选配置。
type targetOpt func(*ent.DeployTargetCreate)

func withDeployServiceParam(svc string) targetOpt {
	return func(b *ent.DeployTargetCreate) { b.SetDeployService(svc) }
}

func withDomainConfig(cfg DeployTargetConfig) targetOpt {
	return func(b *ent.DeployTargetCreate) { b.SetConfig(cfg) }
}

func withCredSourceDeployCred(id int) targetOpt {
	return func(b *ent.DeployTargetCreate) {
		b.SetCredentialSource(deploytarget.CredentialSourceDeployCredential).SetDeployCredentialID(id)
	}
}

func withCredSourceDNSProvider(id int) targetOpt {
	return func(b *ent.DeployTargetCreate) {
		b.SetCredentialSource(deploytarget.CredentialSourceDNSProvider).SetDNSProviderID(id)
	}
}

func withActive(v bool) targetOpt {
	return func(b *ent.DeployTargetCreate) { b.SetIsActive(v) }
}

func withLinkedCerts(ids ...int) targetOpt {
	return func(b *ent.DeployTargetCreate) { b.AddCertificateIDs(ids...) }
}

// mustCreateTarget 创建部署目标。
func mustCreateTarget(t *testing.T, client *ent.Client, name string, providerType deploytarget.ProviderType, opts ...targetOpt) *ent.DeployTarget {
	t.Helper()
	b := client.DeployTarget.Create().
		SetName(name).
		SetProviderType(providerType).
		SetDeployService("cdn").
		SetConfig(DeployTargetConfig{}).
		SetCredentialSource(deploytarget.CredentialSourceDeployCredential).
		SetIsActive(true)
	for _, o := range opts {
		o(b)
	}
	tg, err := b.Save(context.Background())
	if err != nil {
		t.Fatalf("创建部署目标失败: %v", err)
	}
	return tg
}

// withRegistry 临时替换部署器注册表项，测试结束恢复。
func withRegistry(t *testing.T, provider string, d Deployer) {
	t.Helper()
	old, existed := registry[provider]
	registry[provider] = d
	t.Cleanup(func() {
		if existed {
			registry[provider] = old
			return
		}
		delete(registry, provider)
	})
}

// fakeDeployer 基础假部署器：只实现 Deployer，用于验证「未实现可选接口」的降级行为。
type fakeDeployer struct {
	provider string

	mu          sync.Mutex
	uploadCalls int
	deployCalls int
	listCalls   int
	siteCalls   int

	uploadID  string
	uploadRaw string
	uploadErr error

	deployErr error
	deployRes *DeployResult

	domains []string
	sites   []string
	listErr error

	lastSvc    map[string]string
	lastCertID string
	lastDomain string
	lastDeploy string
}

func (f *fakeDeployer) Provider() string { return f.provider }

func (f *fakeDeployer) UploadCert(_ context.Context, _ Credentials, cert CertContent, svc map[string]string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploadCalls++
	f.lastDomain = cert.Domain
	f.lastSvc = svc
	if f.uploadErr != nil {
		return "", f.uploadRaw, f.uploadErr
	}
	id := f.uploadID
	if id == "" {
		id = "cloud-cert-1"
	}
	return id, f.uploadRaw, nil
}

func (f *fakeDeployer) DeployCert(_ context.Context, _ Credentials, certID string, svc string, svcConfig map[string]string) (*DeployResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deployCalls++
	f.lastCertID = certID
	f.lastDeploy = svc
	f.lastSvc = svcConfig
	if f.deployErr != nil {
		return f.deployRes, f.deployErr
	}
	if f.deployRes != nil {
		return f.deployRes, nil
	}
	return &DeployResult{CloudCertID: certID, Message: "ok", RawResponse: `{"code":0}`}, nil
}

func (f *fakeDeployer) ListDomains(_ context.Context, _ Credentials, _, _, _ string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	return f.domains, f.listErr
}

func (f *fakeDeployer) ListSites(_ context.Context, _ Credentials, _, _, _ string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.siteCalls++
	return f.sites, f.listErr
}

func (f *fakeDeployer) counters() (upload, deploy int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.uploadCalls, f.deployCalls
}

// fakeDeployerFull 在基础假部署器上追加可选接口：证书校验 / 当前生效证书查询 / 批量前置钩子。
type fakeDeployerFull struct {
	*fakeDeployer

	validateOK    bool
	validateErr   error
	validateCalls int

	currentCert  *CurrentCert
	currentErr   error
	currentCalls int
	batchCalls   int
}

func (f *fakeDeployerFull) ValidateCert(_ Credentials, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.validateCalls++
	return f.validateOK, f.validateErr
}

func (f *fakeDeployerFull) GetCurrentCert(_ context.Context, _ Credentials, _ string, svcConfig map[string]string) (*CurrentCert, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.currentCalls++
	f.lastSvc = svcConfig
	if f.currentErr != nil {
		return nil, f.currentErr
	}
	if f.currentCert != nil {
		return f.currentCert, nil
	}
	return &CurrentCert{CommonName: "current.example.com"}, nil
}

func (f *fakeDeployerFull) BeforeCurrentCerts(context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batchCalls++
}

// errBoom 测试用错误。
var errBoom = errors.New("boom")

// aliyunCredConfig 阿里云部署凭证配置（非面板，走 domain 路径）。
func aliyunCredConfig() schema.DeployCredentialConfig {
	return schema.DeployCredentialConfig{
		AccessKeyID:     "LTAI-test",
		AccessKeySecret: "secret-test",
		RegionID:        "cn-hangzhou",
	}
}

// btPanelCredConfig 宝塔面板凭证配置（面板类，走 site 路径）。
func btPanelCredConfig() schema.DeployCredentialConfig {
	return schema.DeployCredentialConfig{
		APIKey:   "bt-key",
		PanelURL: "https://panel.example.com:8888",
	}
}
