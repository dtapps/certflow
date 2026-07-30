package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// 中文用户的下载镜像策略：
//   - 版本检测仍以 GitHub 为准（Check 委托给 github provider）；
//   - 实际下载（Download）按 CNB → daocloud 代理 → 官方 GitHub 顺序回退，
//     任一级失败（HTTP 非 2xx 或文件大小不符）自动降级下一级；
//   - 英文用户直接走官方 GitHub（github provider 原始行为）。
//
// 注：CNB 不是标准 GitHub API 兼容镜像，其 release 资源下载路径与 GitHub 仓库
// 不同（CNB 仓库为 dtapp/certflow，GitHub 为 dtapps/certflow），故两个模板均写死
// 仓库路径；即便地址暂不正确，也会自动回退到 daocloud/官方。

const (
	// cnbDownloadTpl：CNB 镜像的 release 资源下载地址模板（写死仓库 dtapp/certflow）。
	// 占位符：{tag} {file}
	cnbDownloadTpl = "https://cnb.cool/dtapp/certflow/-/releases/download/{tag}/{file}"

	// daoCloudDownloadTpl：daocloud 的 GitHub 文件代理，路径与 GitHub 一致（dtapps/certflow）。
	// 占位符：{tag} {file}
	daoCloudDownloadTpl = "https://files.m.daocloud.io/github.com/dtapps/certflow/releases/download/{tag}/{file}"
)

// mirrorProvider 包装 github.Provider，仅重写下載地址。
type mirrorProvider struct {
	*github.Provider
	// checkProvider 用于版本检测，但关闭原生 GitHub 校验文件抓取：
	// SHA256SUMS 改为走镜像（见 Check），避免中文网络下 GitHub 不可达
	// 导致整个 Check 失败、连「发现新版本」都做不到。
	checkProvider *github.Provider
	client *http.Client
}

// newMirrorProvider 基于 github.Config 创建镜像 Provider。
func newMirrorProvider(cfg github.Config) (*mirrorProvider, error) {
	gh, err := github.New(cfg)
	if err != nil {
		return nil, err
	}
	// 版本检测用的 Provider 关闭校验文件抓取（SHA256SUMS 改为走镜像）。
	noChecksumCfg := cfg
	noChecksumCfg.ChecksumAsset = ""
	ghNoChecksum, err := github.New(noChecksumCfg)
	if err != nil {
		return nil, err
	}
	return &mirrorProvider{
		Provider:      gh,
		checkProvider: ghNoChecksum,
		client:        &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Name 实现 updater.Provider。
func (m *mirrorProvider) Name() string { return "github-mirror" }

// Check 版本检测仍以 GitHub 为准，但校验文件（SHA256SUMS）改为走镜像。
//   - 英文用户：直接使用官方 GitHub（含 SHA256SUMS 校验，原始行为）。
//   - 中文用户：用 checkProvider 检测版本（不抓校验），再从 CNB → daocloud
//     → 官方 GitHub 顺序拉 SHA256SUMS 并填充 Verification。
func (m *mirrorProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	if i18n.GetLocale() == string(i18n.EN_US) {
		// 英文：官方 GitHub 原始行为，含 SHA256SUMS 校验。
		return m.Provider.Check(ctx, req)
	}

	rel, err := m.checkProvider.Check(ctx, req)
	if err != nil {
		return nil, err
	}
	// 中文：从镜像拉 SHA256SUMS，失败则降级（下载本身仍走镜像，仅跳过校验）。
	if digest, ok := m.fetchChecksumViaMirror(ctx, rel, "SHA256SUMS"); ok {
		rel.Verification = &updater.Verification{
			DigestAlgo: "sha256",
			Digest:     digest,
		}
	} else {
		logging.Warn("%s", i18n.T("log.updater_checksum_mirror_failed"))
	}
	return rel, nil
}

// Download 实现 updater.Provider：中文走镜像，英文走官方。
func (m *mirrorProvider) Download(ctx context.Context, rel *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	if rel == nil || rel.Metadata == nil {
		return fmt.Errorf("github-mirror: release missing metadata")
	}
	// 英文用户：直接使用官方 GitHub 下载（原始行为）。
	if i18n.GetLocale() == string(i18n.EN_US) {
		return m.Provider.Download(ctx, rel, dst, onProgress)
	}

	tag, _ := rel.Metadata["github.release.tag"].(string)
	file := rel.Artifact.Filename
	ghURL, _ := rel.Metadata["github.asset.url"].(string)

	type candidate struct {
		source string
		url    string
	}
	var candidates []candidate
	if u := m.buildURL(cnbDownloadTpl, tag, file); u != "" {
		candidates = append(candidates, candidate{"cnb", u})
	}
	if u := m.buildURL(daoCloudDownloadTpl, tag, file); u != "" {
		candidates = append(candidates, candidate{"daocloud", u})
	}
	if ghURL != "" {
		candidates = append(candidates, candidate{"github", ghURL})
	}

	for _, c := range candidates {
		resetDst(dst)
		logging.Debug("%s", i18n.T("log.updater_mirror_download", "Source", c.source, "URL", c.url))
		if err := m.downloadFrom(ctx, c.url, rel, dst, onProgress); err != nil {
			logging.Warn("%s", i18n.T("log.updater_mirror_failed", "Source", c.source, "Error", err))
			continue
		}
		return nil
	}
	return fmt.Errorf("github-mirror: 所有下载源均失败")
}

// buildURL 将模板中的占位符替换为实际值。
func (m *mirrorProvider) buildURL(tpl, tag, file string) string {
	if tag == "" || file == "" {
		return ""
	}
	u := strings.ReplaceAll(tpl, "{tag}", tag)
	u = strings.ReplaceAll(u, "{file}", file)
	return u
}

// downloadFrom 从指定 URL 流式下载到 dst，并做基础防护。
func (m *mirrorProvider) downloadFrom(ctx context.Context, urlStr string, rel *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	expected := rel.Artifact.Size
	total := expected
	if total == 0 && resp.ContentLength > 0 {
		total = resp.ContentLength
	}
	written := int64(0)
	buf := make([]byte, 64*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			if onProgress != nil {
				onProgress(written, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	// 大小校验：期望大小已知且不符（多半是错误页/重定向页），回退下一源。
	if expected > 0 && written != expected {
		return fmt.Errorf("下载大小不符: 期望 %d, 实际 %d", expected, written)
	}
	return nil
}

// resetDst 在多次下载尝试间清空 dst，避免上一源的部分数据污染下一源。
func resetDst(dst io.Writer) {
	type seekTruncater interface {
		Seek(offset int64, whence int) (int64, error)
		Truncate(size int64) error
	}
	if st, ok := dst.(seekTruncater); ok {
		_, _ = st.Seek(0, io.SeekStart)
		_ = st.Truncate(0)
	}
}

// fetchChecksumViaMirror 从 CNB → daocloud → 官方 GitHub 顺序拉取校验侧车文件
// （如 SHA256SUMS），解析出目标资产（rel.Artifact.Filename）的 sha256 摘要。
// 任一级成功解析即返回；全部失败返回 (nil, false)。
func (m *mirrorProvider) fetchChecksumViaMirror(ctx context.Context, rel *updater.Release, sidecar string) ([]byte, bool) {
	tag, _ := rel.Metadata["github.release.tag"].(string)
	target := rel.Artifact.Filename

	var urls []string
	if u := m.buildURL(cnbDownloadTpl, tag, sidecar); u != "" {
		urls = append(urls, u)
	}
	if u := m.buildURL(daoCloudDownloadTpl, tag, sidecar); u != "" {
		urls = append(urls, u)
	}
	// 最终回退：用官方 GitHub 资产 URL 的目录替换为校验文件名。
	if ghURL, _ := rel.Metadata["github.asset.url"].(string); ghURL != "" {
		if idx := strings.LastIndex(ghURL, "/"); idx >= 0 {
			urls = append(urls, ghURL[:idx+1]+sidecar)
		}
	}

	for _, u := range urls {
		logging.Debug("%s", i18n.T("log.updater_checksum_download", "URL", u))
		body, err := m.fetchText(ctx, u)
		if err != nil {
			logging.Warn("%s", i18n.T("log.updater_checksum_source_failed", "URL", u, "Error", err))
			continue
		}
		if digest, ok := parseChecksumLine(string(body), target); ok {
			return digest, true
		}
		logging.Warn("%s", i18n.T("log.updater_checksum_parse_failed", "URL", u, "Target", target))
	}
	return nil, false
}

// fetchText 从指定 URL 拉取文本内容（用于校验侧车文件）。
func (m *mirrorProvider) fetchText(ctx context.Context, urlStr string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// parseChecksumLine 从 sha256sum 风格的清单中提取 target 的摘要。
// 每行格式为 "<hex-digest>  <filename>"，文件名按 base name 比较，
// 容忍 sha256sum --binary 在文件名前加的 "*" 标记。
func parseChecksumLine(body, target string) ([]byte, bool) {
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if filepath.Base(name) == target || name == target {
			if digest, err := hex.DecodeString(fields[0]); err == nil {
				return digest, true
			}
		}
	}
	return nil, false
}
