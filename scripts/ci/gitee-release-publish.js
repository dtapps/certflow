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
//   MANIFEST              默认 gitee-release.json
//   CERTFLOW_GITEE_API    默认 https://gitee.com/api/v5/repos/dtapps/certflow

const fs = require('fs');

const MANIFEST = process.env.MANIFEST || 'gitee-release.json';
const API = process.env.CERTFLOW_GITEE_API || 'https://gitee.com/api/v5/repos/dtapps/certflow';
const TOKEN = (process.env.CERTFLOW_GITEE_TOKEN || '').trim();

// 只保留最近 3 个日期 nightly（与 GitHub release.yml 的清理步骤同口径）。
// 镜像侧每天被动同步一次，tag 各不相同；而 Gitee 的附件配额是「仓库累计 1GB」，
// 旧 nightly 不删会持续吃掉配额（上传步骤里的配额跳过就是被它挤出来的）。
// 清理失败只告警，不影响本次发布结果——发布成功比清理重要。
async function cleanupOldNightly() {
  try {
    const res = await gt('GET', '/releases?per_page=100&page=1');
    if (res.status !== 200) {
      console.log('⚠ 查询 Release 列表失败（HTTP ' + res.status + '），跳过 nightly 清理');
      return;
    }
    let list = null;
    try { list = JSON.parse(res.text); } catch (e) { list = null; }
    if (!Array.isArray(list)) {
      console.log('⚠ Release 列表不是数组，跳过 nightly 清理');
      return;
    }
    // 只认「预发布 + tag 以 -nightly.YYYYMMDD 结尾」，rc / beta / alpha 与正式版不动。
    // 日期是定长数字，按 tag 字符串倒序即按时间倒序，无需解析版本号。
    const nightly = list
      .filter((r) => r && r.prerelease && /-nightly\.\d{8}$/.test(r.tag_name || ''))
      .sort((a, b) => (a.tag_name < b.tag_name ? 1 : a.tag_name > b.tag_name ? -1 : 0));
    if (nightly.length <= 3) {
      console.log('nightly Release 共 ' + nightly.length + ' 个，无需清理');
      return;
    }
    for (const old of nightly.slice(3)) {
      console.log('删除旧 nightly Release：' + old.tag_name);
      const del = await gt('DELETE', '/releases/' + old.id);
      if (del.status !== 204 && del.status !== 200) {
        console.log('⚠ 删除 Release 失败（HTTP ' + del.status + '），继续下一个');
        continue;
      }
      // tag 不随 Release 一起删，单独删；失败只告警（下次发布不受影响）
      const dt = await gt('DELETE', '/tags/' + encodeURIComponent(old.tag_name));
      if (dt.status !== 204 && dt.status !== 200) {
        console.log('⚠ 删除 tag 失败（HTTP ' + dt.status + '），忽略');
      }
    }
  } catch (err) {
    console.log('⚠ 清理旧 nightly 异常（忽略）：' + (err && err.message ? err.message : String(err)));
  }
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
  // 注意：Gitee 的 GET /releases/tags/{tag} 在「没有这个 Release」时返回 **200 + body `null`**，
  // 不是 404。所以这里不能只看状态码，还要判断解析结果。
  const existing = await gt('GET', '/releases/tags/' + encodeURIComponent(m.tag));
  let old = null;
  if (existing.status === 200) {
    try { old = JSON.parse(existing.text); } catch (e) { old = null; }
  } else if (existing.status !== 404) {
    fail('查询旧 Release 失败（HTTP ' + existing.status + '）：' + existing.text.slice(0, 300));
  }

  if (old && old.id) {
    console.log('删除同名旧 Release：' + old.id + '（' + (old.name || m.tag) + '）');
    const del = await gt('DELETE', '/releases/' + old.id);
    // 删不掉就别往下走：同名 Release 还在，创建会报 400，错误现场会被挪远
    if (del.status !== 204 && del.status !== 200) {
      fail('删除旧 Release 失败（HTTP ' + del.status + '）：' + del.text.slice(0, 300));
    }
  } else {
    console.log('无同名 Release，直接创建');
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
  // ── 清理旧 nightly（保留最近 3 个）──
  await cleanupOldNightly();

  console.log('发布完成：' + (release.html_url || m.tag));
}

main().catch((err) => fail(err && err.message ? err.message : String(err)));
