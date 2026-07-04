package settings

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"cnb.cool/dtapp/certflow/internal/i18n"
)

// DNSConfig DNS解析配置
type DNSConfig struct {
	ID      string   `json:"id"`      // 唯一标识
	Name    string   `json:"name"`    // 显示名称
	Enabled bool     `json:"enabled"` // 是否启用
	Builtin bool     `json:"builtin"` // 是否内置（不可删除）
	Servers []string `json:"servers"` // DNS 服务器地址列表
}

// ProxyConfig 代理配置
type ProxyConfig struct {
	Enabled  bool   `json:"enabled"`            // 是否启用代理
	Protocol string `json:"protocol"`           // 代理协议：http/https/socks5
	Host     string `json:"host"`               // 代理主机地址
	Port     int    `json:"port"`               // 代理端口
	Username string `json:"username,omitempty"` // 代理用户名
	Password string `json:"password,omitempty"` // 代理密码
}

// LogConfig 日志配置
type LogConfig struct {
	Level      string `json:"level"`       // 日志级别：DEBUG/INFO/WARN/ERROR
	MaxMB      int    `json:"max_mb"`      // 单个日志文件最大大小（MB）
	MaxBackups int    `json:"max_backups"` // 保留的备份数量
}

// Settings 应用设置
type Settings struct {
	AutoRenewalEnabled  bool        `json:"auto_renewal_enabled"` // 是否启用自动续期
	DefaultRenewalDays  int         `json:"default_renewal_days"` // 默认续期天数
	NotificationEnabled bool        `json:"notification_enabled"` // 是否启用通知
	AutoCheckExpiry     bool        `json:"auto_check_expiry"`    // 是否自动检查过期
	CheckInterval       int         `json:"check_interval"`       // 检查间隔（小时）
	DataDir             string      `json:"data_dir"`             // 数据目录
	Language            string      `json:"language"`             // 语言：zh-CN/en-US/auto
	Theme               string      `json:"theme"`                // 主题：dark/light/auto
	DNSConfigs          []DNSConfig `json:"dns_configs"`          // DNS 解析配置列表
	Proxy               ProxyConfig `json:"proxy"`                // 代理配置
	Log                 LogConfig   `json:"log"`                  // 日志配置
}

// getSystemDNS 跨平台获取系统 DNS 服务器地址
func getSystemDNS() []string {
	switch runtime.GOOS {
	case "windows":
		return getSystemDNSWindows()
	default:
		return getSystemDNSUnix()
	}
}

// getSystemDNSUnix 从 /etc/resolv.conf 获取 DNS
func getSystemDNSUnix() []string {
	f, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	defer f.Close()
	var servers []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "nameserver") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				addr := fields[1]
				// 去掉 IPv6 scope ID（如 %en1），保留有效 IP 地址
				if idx := strings.IndexByte(addr, '%'); idx != -1 {
					addr = addr[:idx]
				}
				servers = append(servers, addr)
			}
		}
	}
	return servers
}

// getSystemDNSWindows 从 netsh 获取 DNS
func getSystemDNSWindows() []string {
	out, err := exec.Command("netsh", "interface", "ip", "show", "dnsservers").Output()
	if err != nil {
		return nil
	}
	var servers []string
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		// 跳过空行和描述行
		if line == "" || strings.Contains(line, "DNS") || strings.Contains(line, "---") || strings.Contains(line, "配置") {
			continue
		}
		// 去掉可能的前缀
		if idx := strings.Index(line, ":"); idx >= 0 {
			line = strings.TrimSpace(line[idx+1:])
		}
		// 验证是合法 IP
		if net.ParseIP(line) != nil {
			servers = append(servers, line)
		}
	}
	return servers
}

// builtinDNSConfigs 返回内置 DNS 提供商定义（单一数据源）
func builtinDNSConfigs(systemDNS []string) []DNSConfig {
	return []DNSConfig{
		{ID: "default", Name: "默认 (系统DNS)", Enabled: true, Builtin: true, Servers: systemDNS},
		{ID: "aliyun", Name: "阿里云 DNS", Builtin: true, Servers: []string{"223.5.5.5", "223.6.6.6", "2400:3200::1", "2400:3200:baba::1"}},
		{ID: "tencent", Name: "腾讯云 DNS (DNSPod)", Builtin: true, Servers: []string{"119.29.29.29", "119.28.28.28", "182.254.116.116", "2402:4e00::"}},
		{ID: "huawei", Name: "华为云 DNS", Builtin: true, Servers: []string{"117.50.11.11", "52.80.66.66", "2400:c6c0::"}},
		{ID: "114dns", Name: "114DNS", Builtin: true, Servers: []string{"114.114.114.114", "114.114.115.115"}},
		{ID: "baidu", Name: "百度DNS", Builtin: true, Servers: []string{"180.76.76.76", "2400:da00::6666"}},
		{ID: "360", Name: "360 DNS", Builtin: true, Servers: []string{"101.226.4.6", "218.30.118.6"}},
		{ID: "cnnic", Name: "CNNIC DNS", Builtin: true, Servers: []string{"1.2.4.8", "210.2.4.8", "2001:dc7:1000::1"}},
		{ID: "volcengine", Name: "火山引擎 DNS", Builtin: true, Servers: []string{"180.184.1.1", "180.184.2.2"}},
		{ID: "google", Name: "Google DNS", Builtin: true, Servers: []string{"8.8.8.8", "8.8.4.4", "2001:4860:4860::8888", "2001:4860:4860::8844"}},
		{ID: "cloudflare", Name: "Cloudflare DNS", Builtin: true, Servers: []string{"1.1.1.1", "1.0.0.1", "2606:4700:4700::1111", "2606:4700:4700::1001"}},
	}
}

// DefaultSettings 返回默认设置
func DefaultSettings() Settings {
	return Settings{
		AutoRenewalEnabled:  true,
		DefaultRenewalDays:  30,
		NotificationEnabled: true,
		AutoCheckExpiry:     true,
		CheckInterval:       6,
		DataDir:             "~/.certflow",
		Language:            "auto",
		Theme:               "auto",
		DNSConfigs:          builtinDNSConfigs(nil),
		Proxy: ProxyConfig{
			Enabled:  false,
			Protocol: "http",
			Host:     "",
			Port:     8080,
		},
		Log: LogConfig{
			Level:      "INFO",
			MaxMB:      10,
			MaxBackups: 5,
		},
	}
}

// Service 设置服务
type Service struct {
	mu       sync.RWMutex
	settings Settings
	filePath string
}

// NewService 创建新的设置服务
func NewService(dataDir string) (*Service, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf(i18n.T("error.create_data_dir_failed", "Error", err))
	}

	filePath := filepath.Join(dataDir, "settings.json")
	s := &Service{
		filePath: filePath,
		settings: DefaultSettings(),
	}

	// 尝试加载现有设置
	if err := s.load(); err != nil {
		// 如果文件不存在，使用默认设置
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf(i18n.T("error.load_settings_failed", "Error", err))
		}
	}

	// 始终同步内置 DNS 条目并保存
	s.updateDefaultDNS()
	if err := s.save(); err != nil {
		return nil, fmt.Errorf(i18n.T("error.write_settings_file_failed", "Error", err))
	}

	return s, nil
}

// updateDefaultDNS 同步内置 DNS 条目：确保所有内置条目存在且服务器列表为最新
func (s *Service) updateDefaultDNS() {
	systemDNS := getSystemDNS()
	builtins := builtinDNSConfigs(systemDNS)

	existingIDs := make(map[string]int)
	for i, d := range s.settings.DNSConfigs {
		existingIDs[d.ID] = i
	}

	for _, def := range builtins {
		if idx, ok := existingIDs[def.ID]; ok {
			s.settings.DNSConfigs[idx].Servers = def.Servers
		} else {
			s.settings.DNSConfigs = append(s.settings.DNSConfigs, def)
		}
	}
}

// load 从文件加载设置
func (s *Service) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &s.settings)
}

// save 将当前设置写入文件（内部使用，不加锁）
func (s *Service) save() error {
	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return fmt.Errorf(i18n.T("error.serialize_settings_failed", "Error", err))
	}
	return os.WriteFile(s.filePath, data, 0644)
}

// Save 保存设置到文件
func (s *Service) Save(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.settings = settings

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf(i18n.T("error.serialize_settings_failed", "Error", err))
	}

	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		return fmt.Errorf(i18n.T("error.write_settings_file_failed", "Error", err))
	}

	return nil
}

// Get 获取当前设置
func (s *Service) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.settings
}

// IsSeeded 检查是否已预置默认数据（读取独立文件）
func (s *Service) IsSeeded() bool {
	seededPath := filepath.Join(filepath.Dir(s.filePath), "seeded.json")
	data, err := os.ReadFile(seededPath)
	if err != nil {
		return false
	}
	var seeded struct {
		Seeded bool `json:"seeded"`
	}
	if err := json.Unmarshal(data, &seeded); err != nil {
		return false
	}
	return seeded.Seeded
}

// MarkSeeded 标记已预置默认数据（写入独立文件）
func (s *Service) MarkSeeded() error {
	seededPath := filepath.Join(filepath.Dir(s.filePath), "seeded.json")
	data, err := json.MarshalIndent(map[string]bool{"seeded": true}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(seededPath, data, 0644)
}

// UpdateNotificationEnabled 更新通知启用状态
func (s *Service) UpdateNotificationEnabled(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.settings.NotificationEnabled = enabled

	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return fmt.Errorf(i18n.T("error.serialize_settings_failed", "Error", err))
	}

	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		return fmt.Errorf(i18n.T("error.write_settings_file_failed", "Error", err))
	}

	return nil
}
