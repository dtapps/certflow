#!/usr/bin/env node
'use strict';

// 按 gitee-release-download.js 写出的清单，把 Release 建好并逐个上传附件。
//
// 为什么不用 release@gitee 插件：
//   1. 插件的 assertFiles 是静态清单，做不到「某个文件超 100MB 就跳过它」；
//   2. 插件 400 时响应体为空，看不到 Gitee 到底说了什么；
//   3. 插件把正文当 KEY=VALUE 参数（不能换行、不能有裸 =、整个值当相对路径 open）。
//
// 环境变量（**不能以 GITEE_ 开头**：Gitee 流水线自己占用了这个前缀，
// 全局参数用 GITEE_xxx 会被平台拒绝或覆盖，所以统一走 CERTFLOW_ 前缀）：
//   CERTFLOW_GITEE_TOKEN  必填，个人令牌（repo 读写）
//   CERTFLOW_GITEE_REPO   默认 dtapps/certflow（owner/repo）
//   MANIFEST              默认 gitee-release.json
//   CERTFLOW_GITEE_API    默认 https://gitee.com/api/v5/repos/<REPO>
//
// 删除逻辑（同名旧 Release、旧 nightly）统一在 scripts/ci/delete-release.sh；
// 旧 nightly 的清理由 .workflow/release.yml 在发布后单独调用脚本完成。

const fs = require('fs');
const path = require('path');
const { execFileSync } = require('child_process');

const MANIFEST = process.env.MANIFEST || 'gitee-release.json';
const REPO = process.env.CERTFLOW_GITEE_REPO || 'dtapps/certflow';
const API = process.env.CERTFLOW_GITEE_API || 'https://gitee.com/api/v5/repos/' + REPO;
const TOKEN = (process.env.CERTFLOW_GITEE_TOKEN || '').trim();

// 调公共删除脚本：Gitee 没有删 tag 的 API，脚本内部用 git push --delete 处理。
// 历史版本（老 commit）里可能还没有这个脚本，跳过预删除即可——同名 Release 还在时
// 创建会返回 400，错误信息足够定位。
function runDeleteScript(args) {
  const script = path.join(__dirname, 'delete-release.sh');
  if (!fs.existsSync(script)) {
    console.log('⚠ 找不到 scripts/ci/delete-release.sh（旧版本），跳过同名 Release 预删除');
    return;
  }
  execFileSync('sh', [script, '--platform', 'gitee'].concat(args), {
    stdio: 'inherit',
    env: Object.assign({}, process.env, {
      RELEASE_GITEE_TOKEN: TOKEN,
      RELEASE_GITEE_REPO: REPO
    })
  });
}

function fail(msg) {
  console.error('错误：' + msg);
  process.exit(1);
}
function mb(n) { return (n / 1048576).toFixed(1) + ' MB'; }

// 只打印路径，不打印带 access_token 的完整 URL
async function gt(method, path, options) {
  const url = API + path + (path.indexOf('?') === -1 ? '?' : '&') +
    'access_token=' + encodeURIComponent(TOKEN);
  const res = await fetch(url, Object.assign({ method }, options || {}));
  const text = await res.text();
  console.log('  ' + method + ' ' + path + ' → HTTP ' + res.status +
    (text ? ' ' + text.slice(0, 300) : ''));
  return { status: res.status, text };
}

async function main() {
  if (!TOKEN) {
    fail('未配置 CERTFLOW_GITEE_TOKEN（Gitee 个人令牌，repo 读写）');
  }
  if (!fs.existsSync(MANIFEST)) {
    fail('找不到清单 ' + MANIFEST + '：请先跑 gitee-release-download.js');
  }
  const m = JSON.parse(fs.readFileSync(MANIFEST, 'utf8'));
  if (!m.files || !m.files.length) fail('清单里没有文件');

  console.log('发布 ' + m.tag + ' → ' + m.releaseName);
  console.log('待上传 ' + m.files.length + ' 个（' + mb(m.totalBytes || 0) + '）');

  // ── 同名先删，保证幂等重跑 ──
  // 删除逻辑统一在 scripts/ci/delete-release.sh：Gitee 的 GET /releases/tags/{tag}
  // 在「没有这个 Release」时返回 **200 + body `null`**，脚本内部按 body 判断。
  // --keep-tag 表示只删 Release，新建时复用原 tag（与之前行为一致）。
  console.log('检查同名旧 Release（scripts/ci/delete-release.sh）');
  try {
    runDeleteScript(['--tag', m.tag, '--keep-tag']);
  } catch (err) {
    fail('删除同名旧 Release 失败：' + (err && err.message ? err.message : String(err)));
  }

  // ── 建 Release ──
  const form = new URLSearchParams();
  form.set('access_token', TOKEN);
  form.set('tag_name', m.tag);
  form.set('name', m.releaseName);
  form.set('body', m.bodyHtml);
  form.set('target_commitish', m.targetCommitish || 'master');
  form.set('prerelease', m.prerelease ? 'true' : 'false');
  const created = await gt('POST', '/releases', { body: form });
  if (created.status !== 200 && created.status !== 201) {
    fail('创建 Release 失败（HTTP ' + created.status + '）：' + created.text.slice(0, 300));
  }
  // 同理：返回体可能是 null 或非 JSON，别让 JSON.parse 直接抛栈
  let release = null;
  try { release = JSON.parse(created.text); } catch (e) { release = null; }
  if (!release || !release.id) {
    fail('创建 Release 的响应里没有 id（HTTP ' + created.status + '）：' + created.text.slice(0, 300));
  }
  console.log('Release 已创建：id=' + release.id + ' ' + (release.html_url || ''));

  // ── 逐个上传：单个失败只记录，不中断其余文件 ──
  // Gitee 仓库附件总量 1GB（含历史发行版）。配额不足时返回
  //   {"message":"验证失败：文件大小已超出仓库附件配额：1 GB"}
  // 这属于「仓库容量不够，用户去 GitHub 下载即可」，不算发布失败，也不让流水线变红。
  // 其它 4xx / 网络异常仍算失败，避免把真实故障也吞掉。
  const QUOTA_RE = /附件配额|超出.*配额|quota/i;
  const ok = [];
  const quotaSkipped = [];
  const failed = [];
  // 配额是「仓库累计」，只会越用越满：某个 N 字节的文件被拒后，>=N 的必然也被拒。
  // 不再重复 POST 几十 MB 换取同一个 400，但比它小的文件仍要试（SHA256SUMS 往往能过）。
  let quotaMinSize = Infinity;
  for (const f of m.files) {
    if ((f.size || 0) >= quotaMinSize) {
      console.log('  - 已知仓库附件配额不足，跳过 ' + f.name + '（不再重复上传，不算失败）');
      quotaSkipped.push(f.name);
      continue;
    }
    console.log('上传 ' + f.name + '（' + mb(f.size || 0) + '）');
    try {
      const buf = await fs.promises.readFile(f.path);
      const fd = new FormData();
      fd.set('access_token', TOKEN);
      fd.set('file', new Blob([buf]), f.name);
      const res = await fetch(API + '/releases/' + release.id + '/attach_files', {
        method: 'POST',
        body: fd
      });
      const text = await res.text();
      if (res.status === 200 || res.status === 201) {
        console.log('  ✓ HTTP ' + res.status + ' ' + f.name);
        ok.push(f.name);
      } else if (QUOTA_RE.test(text)) {
        console.log('  - 仓库附件配额不足，跳过 ' + f.name + '（不算失败）');
        quotaMinSize = Math.min(quotaMinSize, f.size || 0);
        quotaSkipped.push(f.name);
      } else {
        console.log('  ✗ HTTP ' + res.status + ' ' + f.name + ' ' + text.slice(0, 300));
        failed.push(f.name + '（HTTP ' + res.status + ' ' + text.slice(0, 120) + '）');
      }
    } catch (err) {
      const msg = err && err.message ? err.message : String(err);
      console.log('  ✗ 异常 ' + f.name + '：' + msg);
      failed.push(f.name + '（' + msg + '）');
    }
  }

  console.log('---- 汇总 ----');
  console.log('成功 ' + ok.length + ' 个' + (ok.length ? '：' + ok.join(', ') : ''));
  if (quotaSkipped.length) {
    console.log('⚠ 仓库附件配额不足，跳过 ' + quotaSkipped.length + ' 个（不算失败）：');
    quotaSkipped.forEach((x) => console.log('  - ' + x));
    console.log('  这些附件请到 GitHub Release 下载；清理 Gitee 旧 Release 的附件后可重跑补齐。');
  }
  if (failed.length) {
    console.log('失败 ' + failed.length + ' 个：');
    failed.forEach((x) => console.log('  - ' + x));
    fail('有 ' + failed.length + ' 个附件上传失败，见上面的 HTTP 状态与响应体');
  }

  console.log('发布完成：' + (release.html_url || m.tag));
}

main().catch((err) => fail(err && err.message ? err.message : String(err)));
