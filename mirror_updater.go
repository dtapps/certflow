package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
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
	client *http.Client
}

// newMirrorProvider 基于 github.Config 创建镜像 Provider。
func newMirrorProvider(cfg github.Config) (*mirrorProvider, error) {
	gh, err := github.New(cfg)
	if err != nil {
		return nil, err
	}
	return &mirrorProvider{
		Provider: gh,
		client:   &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Name 实现 updater.Provider。
func (m *mirrorProvider) Name() string { return "github-mirror" }

// Check 委托给 github provider，版本检测仍以 GitHub 为准。
func (m *mirrorProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	return m.Provider.Check(ctx, req)
}

// Download 实现 updater.Provider：中文走镜像，英文走官方。
func (m *mirrorProvider) Download(ctx context.Context, rel *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	if rel == nil || rel.Metadata == nil {
		return fmt.Errorf("github-mirror: release missing metadata")
	}
	// 英文用户：直接使用官方 GitHub 下载（原始行为）。
	if i18n.GetLocale() == "en-US" {
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
	if u := m.buildURL(cnbDownloadTpl, tag, stripUpdaterPrefix(file)); u != "" {
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
		logging.Debug(i18n.T("log.updater_mirror_download", "Source", c.source, "URL", c.url))
		if err := m.downloadFrom(ctx, c.url, rel, dst, onProgress); err != nil {
			logging.Warn(i18n.T("log.updater_mirror_failed", "Source", c.source, "Error", err))
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

// stripUpdaterPrefix 去掉 Wails updater 给升级文件加的 updater- 前缀。
// CNB 镜像仓库的升级文件使用原始文件名（无前缀），而 GitHub/daocloud 保留前缀，
// 故拼 CNB 下载地址时需去掉该前缀，否则命中 404。
func stripUpdaterPrefix(name string) string {
	return strings.TrimPrefix(name, "updater-")
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
