package settings

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// DNSConfig DNS解析配置
type DNSConfig struct {
	ID      string   `json:"id" mapstructure:"id"`           // 唯一标识
	Name    string   `json:"name" mapstructure:"name"`       // 显示名称
	Enabled bool     `json:"enabled" mapstructure:"enabled"` // 是否启用
	Builtin bool     `json:"builtin" mapstructure:"builtin"` // 是否内置（不可删除）
	Servers []string `json:"servers" mapstructure:"servers"` // DNS 服务器地址列表
}

// ProxyConfig 代理配置
type ProxyConfig struct {
	Enabled  bool   `json:"enabled" mapstructure:"enabled"`             // 是否启用代理
	Protocol string `json:"protocol" mapstructure:"protocol"`           // 代理协议：http/https/socks5
	Host     string `json:"host" mapstructure:"host"`                   // 代理主机地址
	Port     int    `json:"port" mapstructure:"port"`                   // 代理端口
	Username string `json:"username,omitempty" mapstructure:"username"` // 代理用户名
	Password string `json:"password,omitempty" mapstructure:"password"` // 代理密码
}

// LogConfig 日志配置
type LogConfig struct {
	Level      string `json:"level" mapstructure:"level"`             // 日志级别：DEBUG/INFO/WARN/ERROR
	MaxMB      int    `json:"max_mb" mapstructure:"max_mb"`           // 单个日志文件最大大小（MB）
	MaxBackups int    `json:"max_backups" mapstructure:"max_backups"` // 保留的备份数量
}

// Settings 应用设置
type Settings struct {
	AutoRenewalEnabled bool        `json:"auto_renewal_enabled" mapstructure:"auto_renewal_enabled"` // 是否启用自动续期
	DefaultRenewalDays int         `json:"default_renewal_days" mapstructure:"default_renewal_days"` // 默认续期天数
	AutoCheckExpiry    bool        `json:"auto_check_expiry" mapstructure:"auto_check_expiry"`       // 是否自动检查过期
	CheckInterval      int         `json:"check_interval" mapstructure:"check_interval"`             // 检查间隔（小时）
	DataDir            string      `json:"data_dir" mapstructure:"data_dir"`                         // 数据目录
	Language           string      `json:"language" mapstructure:"language"`                         // 语言：zh-CN/en-US/auto
	Theme              string      `json:"theme" mapstructure:"theme"`                               // 主题：dark/light/auto
	DNSConfigs         []DNSConfig `json:"dns_configs" mapstructure:"dns_configs"`                   // DNS 解析配置列表
	Proxy              ProxyConfig `json:"proxy" mapstructure:"proxy"`                               // 代理配置
	Log                LogConfig   `json:"log" mapstructure:"log"`                                   // 日志配置
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
		AutoRenewalEnabled: true,
		DefaultRenewalDays: 30,
		AutoCheckExpiry:    true,
		CheckInterval:      6,
		DataDir:            "~/.certflow",
		Language:           "auto",
		Theme:              "auto",
		DNSConfigs:         builtinDNSConfigs(nil),
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

// OnChangeFunc 配置文件变更回调
type OnChangeFunc func(newSettings Settings)

// Service 设置服务
type Service struct {
	mu       sync.RWMutex
	settings Settings
	v        *viper.Viper
	filePath string
	onChange OnChangeFunc
	saving   bool // 标记正在保存，避免触发自身回调

	// seeded.json 独立 Viper 实例
	seededV *viper.Viper
}

// NewService 创建新的设置服务
func NewService(dataDir string) (*Service, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf(i18n.T("error.create_data_dir_failed", "Error", err))
	}

	filePath := filepath.Join(dataDir, "settings.json")

	v := viper.New()
	v.SetConfigFile(filePath)
	v.SetConfigType("json")
	v.AutomaticEnv()

	s := &Service{
		filePath: filePath,
		settings: DefaultSettings(),
		v:        v,
	}

	// 设置默认值
	s.setDefaults()

	// 尝试加载现有设置
	if err := v.ReadInConfig(); err != nil {
		// 首次运行，写入默认配置
		if os.IsNotExist(err) {
			if err := s.writeConfig(); err != nil {
				return nil, fmt.Errorf(i18n.T("error.write_settings_file_failed", "Error", err))
			}
		} else {
			return nil, fmt.Errorf(i18n.T("error.load_settings_failed", "Error", err))
		}
	}

	// 反序列化到内存
	if err := v.Unmarshal(&s.settings); err != nil {
		return nil, fmt.Errorf(i18n.T("error.load_settings_failed", "Error", err))
	}

	// 同步内置 DNS 条目
	s.updateDefaultDNS()
	// 通过 Viper 写入，保持内部状态同步
	if err := s.writeConfig(); err != nil {
		return nil, fmt.Errorf(i18n.T("error.write_settings_file_failed", "Error", err))
	}

	// 初始化 seeded.json 的 Viper 实例
	seededPath := filepath.Join(filepath.Dir(filePath), "seeded.json")
	seededV := viper.New()
	seededV.SetConfigFile(seededPath)
	seededV.SetConfigType("json")
	s.seededV = seededV

	// 启动文件监控
	s.startWatching()

	return s, nil
}

// setDefaults 设置 Viper 默认值
func (s *Service) setDefaults() {
	def := DefaultSettings()
	s.v.SetDefault("auto_renewal_enabled", def.AutoRenewalEnabled)
	s.v.SetDefault("default_renewal_days", def.DefaultRenewalDays)
	s.v.SetDefault("auto_check_expiry", def.AutoCheckExpiry)
	s.v.SetDefault("check_interval", def.CheckInterval)
	s.v.SetDefault("data_dir", def.DataDir)
	s.v.SetDefault("language", def.Language)
	s.v.SetDefault("theme", def.Theme)
	s.v.SetDefault("dns_configs", def.DNSConfigs)
	s.v.SetDefault("proxy", def.Proxy)
	s.v.SetDefault("log", def.Log)
}

// startWatching 启动配置文件监控
func (s *Service) startWatching() {
	var (
		debounceTimer *time.Timer
		timerMu       sync.Mutex
	)

	s.v.OnConfigChange(func(e fsnotify.Event) {
		s.mu.Lock()
		if s.saving {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		// 防抖：500ms 内多次变更只处理最后一次
		timerMu.Lock()
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceTimer = time.AfterFunc(500*time.Millisecond, func() {
			s.mu.Lock()
			defer s.mu.Unlock()

			if err := s.v.Unmarshal(&s.settings); err != nil {
				logging.Error(i18n.T("log.settings_reload_failed", "Error", err))
				return
			}
			logging.Info(i18n.T("log.settings_reloaded"))

			if s.onChange != nil {
				cb := s.onChange
				settings := s.settings
				go cb(settings)
			}
		})
		timerMu.Unlock()
	})

	s.v.WatchConfig()
}

// OnChange 注册配置变更回调
func (s *Service) OnChange(fn OnChangeFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = fn
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

// writeConfig 将当前设置写入文件（内部使用，调用方需持有锁）
func (s *Service) writeConfig() error {
	s.saving = true
	defer func() { s.saving = false }()

	// 同步到 Viper，保持内部状态一致
	s.v.Set("auto_renewal_enabled", s.settings.AutoRenewalEnabled)
	s.v.Set("default_renewal_days", s.settings.DefaultRenewalDays)
	s.v.Set("auto_check_expiry", s.settings.AutoCheckExpiry)
	s.v.Set("check_interval", s.settings.CheckInterval)
	s.v.Set("data_dir", s.settings.DataDir)
	s.v.Set("language", s.settings.Language)
	s.v.Set("theme", s.settings.Theme)
	s.v.Set("dns_configs", s.settings.DNSConfigs)
	s.v.Set("proxy", s.settings.Proxy)
	s.v.Set("log", s.settings.Log)

	return s.v.WriteConfig()
}

// Save 保存设置到文件
func (s *Service) Save(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.settings = settings
	return s.writeConfig()
}

// Get 获取当前设置
func (s *Service) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.settings
}

// IsSeeded 检查是否已预置默认数据（读取独立文件）
func (s *Service) IsSeeded() bool {
	if err := s.seededV.ReadInConfig(); err != nil {
		return false
	}
	return s.seededV.GetBool("seeded")
}

// MarkSeeded 标记已预置默认数据（写入独立文件）
func (s *Service) MarkSeeded() error {
	s.seededV.Set("seeded", true)
	return s.seededV.WriteConfig()
}
