#!/usr/bin/env node
'use strict';

// 把上游 Release 的附件搬到本地 assets/，写出发布清单 gitee-release.json。
//
// 整条链只认一个源，由 DOWNLOAD_SOURCE 决定（cnb 默认 / github），绝不混用：
// 包里注入了 RepoSource，客户端据此选择升级通道；同一次发布里混两个来源，
// 等于给用户随机分配升级通道。CNB 源的附件名固定、大小向 CNB 现问、正文本地
// 生成，全程不碰 GitHub API（CNB API 匿名 401）；github 源才读 GitHub 元数据。
//
// 唯一的跳过规则：**单文件 > 100MB**（Gitee 单附件上限），这类文件在境内链路要传
// 几分钟，只是为了让服务端拒收，所以连下载都不做。
// 其它产物一律照常下载并上传 —— updater-*、BUILD_TIME、GIT_COMMIT、SHA256SUMS、
// 以及某次体积没超 100MB 的 AppImage 都不特殊处理。
//
// 环境变量：
//   DOWNLOAD_SOURCE         cnb（默认）/ github，决定本次发布的唯一来源
//   RELEASE_VERSION         可选，v1.2.3 / 1.2.3 / v1.2.4-nightly.20260101；
//                           留空时：github 源取最新 Release，cnb 源直接报错
//                           （兼容别名 VERSION / TAG）
//   CNB_REPO                cnb 仓库路径，默认 dtapp/certflow
//   CERTFLOW_GITHUB_TOKEN   可选，避开 api.github.com 匿名限流（兼容 GH_TOKEN）
//   PRERELEASE              可选，shell 层由版本号后缀算出的判定结果，优先于正文关键词
//   OUT_DIR                 默认 assets
//   MANIFEST                默认 gitee-release.json
//   MAX_FILE_BYTES          默认 100MB（Gitee 单附件上限）
//   MAX_TOTAL_BYTES         默认 800MB（仓库 1GB 留余量）
//   SINGLE_LINE_BODY        设 1 时把正文压成单行 HTML（release@gitee 插件参数通道用）：
//                           裸 = 转 &#61;，每 200 字节插 <br/> 防按路径 open 超 255 崩溃。
//                           本仓库用 gitee-release-publish.js 走 v5 API，默认不需要。
//
// 命令行：
//   node gitee-release-download.js --resolve-tag    只打印最新 tag 后退出（不下载、不写清单）
//
// 只用 node 内置模块：Gitee 镜像容器里没有 python3 / jq。

const fs = require('fs');
const path = require('path');
const http = require('http');
const https = require('https');

const VERSION_IN = (process.env.RELEASE_VERSION || process.env.VERSION || process.env.TAG || '').trim();
const OUT_DIR = (process.env.OUT_DIR || 'assets').trim();
const MANIFEST = (process.env.MANIFEST || 'gitee-release.json').trim();
const CNB_REPO = (process.env.CNB_REPO || 'dtapp/certflow').trim();
const GH_API = process.env.GH_API || 'https://api.github.com/repos/dtapps/certflow';
const GH_TOKEN = (process.env.CERTFLOW_GITHUB_TOKEN || process.env.GH_TOKEN || '').trim();
const SOURCE = (process.env.DOWNLOAD_SOURCE || 'cnb').trim().toLowerCase();
const SINGLE_LINE = (process.env.SINGLE_LINE_BODY || '').trim() === '1';
const MAX_FILE = Number(process.env.MAX_FILE_BYTES || 100 * 1024 * 1024);
const MAX_TOTAL = Number(process.env.MAX_TOTAL_BYTES || 800 * 1024 * 1024);

// 境内直连 GitHub 资产只有 10~30 KB/s 且常被掐断，所以先走代理镜像，直连只做兜底。
// 镜像里还是 GitHub 那个文件，字节相同；只用于 github 源，cnb 源走 CNB 直链。
const MIRRORS = [
  { name: 'daocloud', wrap: (u) => 'https://files.m.daocloud.io/' + u.replace(/^https?:\/\//, '') },
  { name: 'ghfast', wrap: (u) => 'https://ghfast.top/' + u },
  { name: 'gh-proxy', wrap: (u) => 'https://gh-proxy.com/' + u },
  { name: '直连', wrap: (u) => u }
];
const CNB_CHAIN = [{ name: 'CNB', wrap: (u) => u }];

const CONCURRENCY = 3;

let VER = VERSION_IN.replace(/^v/i, '');
let TAG = VER ? 'v' + VER : '';

function fail(msg) {
  console.error('错误：' + msg);
  process.exit(1);
}

function mb(n) {
  return (n / 1048576).toFixed(1) + ' MB';
}
function rate(bps) {
  if (bps >= 1048576) return mb(bps) + '/s';
  if (bps >= 1024) return (bps / 1024).toFixed(0) + ' KB/s';
  return bps.toFixed(0) + ' B/s';
}
function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms));
}

// ── HTTP ──────────────────────────────────────────────────────────────────
// 断点续传要带 Range 头，重定向到签名地址时也得一路带着，否则续传变成从头下。
// 重定向地址可能是相对路径，用 new URL(loc, url) 补全。
function request(url, redirects, withAuth, rangeFrom) {
  return new Promise((resolve, reject) => {
    const lib = url.indexOf('https:') === 0 ? https : http;
    const headers = { 'User-Agent': 'certflow-release-sync' };
    if (GH_TOKEN && withAuth) headers.Authorization = 'Bearer ' + GH_TOKEN;
    if (rangeFrom > 0) headers.Range = 'bytes=' + rangeFrom + '-';
    const req = lib.get(url, { headers }, (res) => {
      if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        res.resume();
        if (!redirects) return reject(new Error('重定向次数过多: ' + url));
        return resolve(request(new URL(res.headers.location, url).href, redirects - 1, withAuth, rangeFrom));
      }
      if (res.statusCode !== 200 && res.statusCode !== 206) {
        let body = '';
        res.on('data', (c) => { body += c; });
        res.on('end', () => {
          reject(new Error('HTTP ' + res.statusCode + ' ' + url + '\n' + body.slice(0, 300)));
        });
        return;
      }
      resolve(res);
    });
    req.on('error', reject);
    req.setTimeout(60000, () => req.destroy(new Error('请求超时: ' + url)));
  });
}

function readText(url) {
  return request(url, 5, true, 0).then((res) => new Promise((resolve, reject) => {
    let body = '';
    res.setEncoding('utf8');
    res.on('data', (c) => { body += c; });
    res.on('end', () => resolve(body));
    res.on('error', reject);
  }));
}

// cnb 源的大小不能复用 GitHub 的 asset.size：各平台自己编译，同版本字节数不同；
// 而且 CNB API 匿名 401 拿不到元数据。发一个 Range: bytes=1- 请求，从
// Content-Range 尾部读真实总长度（服务端忽略 Range 回 200 时退回 Content-Length）。
function probeSize(url) {
  return request(url, 5, false, 1).then((res) => {
    const cr = res.headers['content-range'];
    res.resume();
    if (cr) {
      const m = /\/(\d+)\s*$/.exec(cr);
      if (m) return Number(m[1]);
    }
    const cl = Number(res.headers['content-length']);
    return res.statusCode === 200 && cl > 0 ? cl : 0;
  });
}

// 落盘校验：网关/反代拿 200 回一个 HTML 错误页时，大小可能对得上，内容却全是文本。
// deb 必须是 ar 归档（!<arch>\n），fpk 必须是 gzip（1f 8b）。
function looksRight(dest, name) {
  try {
    const fd = fs.openSync(dest, 'r');
    const buf = Buffer.alloc(8);
    try {
      const n = fs.readSync(fd, buf, 0, 8, 0);
      if (n < 2) return false;
      if (/\.deb$/i.test(name)) return buf.slice(0, 8).toString('ascii') === '!<arch>\n';
      if (/\.fpk$/i.test(name)) return buf[0] === 0x1f && buf[1] === 0x8b;
      return true;
    } finally {
      fs.closeSync(fd);
    }
  } catch (e) {
    return false;
  }
}

function urlFor(asset) {
  if (SOURCE === 'cnb') {
    return 'https://cnb.cool/' + CNB_REPO + '/-/releases/download/' + TAG + '/' + asset.name;
  }
  return asset.browser_download_url;
}

function candidatesFor() {
  return SOURCE === 'cnb' ? CNB_CHAIN : MIRRORS;
}

function downloadOnce(url, dest, from) {
  return request(url, 5, false, from).then((res) => new Promise((resolve, reject) => {
    // 只有服务端确认 206 才续写，否则截断重来
    const append = from > 0 && res.statusCode === 206;
    const out = fs.createWriteStream(dest, { flags: append ? 'a' : 'w' });
    let written = append ? from : 0;
    let lastAt = Date.now();
    let lastBytes = written;
    let done = false;
    let lowStreak = 0;

    function abort(err) {
      if (done) return;
      done = true;
      out.destroy();
      res.destroy();
      reject(err);
    }

    res.on('data', (chunk) => {
      written += chunk.length;
      const now = Date.now();
      if (now - lastAt >= 5000) {
        const bps = (written - lastBytes) / ((now - lastAt) / 1000);
        console.log('    ' + mb(written) + '  ' + rate(bps));
        // 15 秒持续低于 30 KB/s 就换源：慢而不断的连接比断掉更耗时间
        lowStreak = bps < 30 * 1024 ? lowStreak + 1 : 0;
        if (lowStreak >= 3) {
          const err = new Error('速度过低（' + rate(bps) + '）');
          err.slow = true;
          abort(err);
          return;
        }
        lastAt = now;
        lastBytes = written;
      }
    });
    res.on('error', abort);
    res.on('aborted', () => abort(new Error('连接被中断')));
    out.on('error', abort);
    out.on('finish', () => {
      if (done) return;
      done = true;
      resolve(written);
    });
    res.pipe(out);
  }));
}

// 单个文件：累计重试 8 次，每次从已落盘的字节续传；低速立刻换源，
// 普通中断两轮换一次（同一个源上反复失败只会浪费次数），间隔 1.5 秒。
function downloadFile(asset, dest, expectedSize) {
  const chain = candidatesFor();
  let idx = 0;
  let tries = 0;

  function step() {
    let from = fs.existsSync(dest) ? fs.statSync(dest).size : 0;
    if (expectedSize > 0 && from === expectedSize) {
      console.log('  已完整（' + mb(from) + '）');
      return Promise.resolve();
    }
    if (expectedSize > 0 && from > expectedSize) {
      fs.unlinkSync(dest); // 上次写了脏数据：文件删了就必须把续传起点也归零
      from = 0;
    }
    return downloadOnce(chain[idx].wrap(urlFor(asset)), dest, from).then((written) => {
      if (expectedSize > 0 && written !== expectedSize) {
        throw new Error('大小不符：' + written + ' != ' + expectedSize);
      }
      console.log('  完成 ' + mb(written) + '（' + chain[idx].name + '）');
    }).catch((err) => {
      tries++;
      const now = fs.existsSync(dest) ? fs.statSync(dest).size : 0;
      console.log('  中断：' + (err && err.message ? err.message : err) +
        '，已下 ' + mb(now) + '，第 ' + tries + ' 次');
      if (tries >= 8) throw new Error('下载失败（重试 ' + tries + ' 次）：' + urlFor(asset));
      if ((err && err.slow) || tries % 2 === 0) {
        if (idx < chain.length - 1) {
          idx++;
          console.log('  切换下载源 → ' + chain[idx].name);
        }
      }
      return sleep(1500).then(step);
    });
  }
  return step();
}

// ── cnb 源的固定附件清单 ─────────────────────────────────────────────────
// 与 GitHub Release 上的产物名逐字一致（{V} 换成去掉 v 的版本号）。CNB 的 API 匿名 401，
// 所以不查元数据、不依赖 GitHub 的资产列表：名字写死在这里，大小用 probeSize 现问。
// 上游增减产物时要同步改这份清单（发布脚本按名字找文件）。
const CNB_TEMPLATES = [
  { tpl: 'CertFlow-{V}-darwin-amd64.zip', plat: 'macOS', arch: 'x86_64', desc: 'macOS 应用包（zip，解压得到 CertFlow.app）' },
  { tpl: 'CertFlow-{V}-darwin-arm64.zip', plat: 'macOS', arch: 'ARM64', desc: 'macOS 应用包（zip，解压得到 CertFlow.app）' },
  { tpl: 'CertFlow-{V}-linux-amd64.AppImage', plat: 'Linux', arch: 'x86_64', desc: 'AppImage 便携版（chmod +x 后直接运行）' },
  { tpl: 'CertFlow-{V}-linux-arm64.AppImage', plat: 'Linux', arch: 'ARM64', desc: 'AppImage 便携版（chmod +x 后直接运行）' },
  { tpl: 'CertFlow-{V}-linux-amd64.deb', plat: 'Linux', arch: 'x86_64', desc: 'Debian/Ubuntu 安装包（dpkg -i）' },
  { tpl: 'CertFlow-{V}-linux-arm64.deb', plat: 'Linux', arch: 'ARM64', desc: 'Debian/Ubuntu 安装包（dpkg -i）' },
  { tpl: 'CertFlow-{V}-linux-amd64.rpm', plat: 'Linux', arch: 'x86_64', desc: 'Fedora/RHEL 安装包（rpm -i）' },
  { tpl: 'CertFlow-{V}-linux-arm64.rpm', plat: 'Linux', arch: 'ARM64', desc: 'Fedora/RHEL 安装包（rpm -i）' },
  { tpl: 'CertFlow-{V}-linux-amd64.pkg.tar.zst', plat: 'Linux', arch: 'x86_64', desc: 'Arch Linux 安装包（pacman -U）' },
  { tpl: 'CertFlow-{V}-linux-arm64.pkg.tar.zst', plat: 'Linux', arch: 'ARM64', desc: 'Arch Linux 安装包（pacman -U）' },
  { tpl: 'CertFlow-{V}-windows-amd64-install.exe', plat: 'Windows', arch: 'x86_64', desc: 'Windows 安装程序（exe，双击安装）' },
  { tpl: 'CertFlow-{V}-windows-arm64-install.exe', plat: 'Windows', arch: 'ARM64', desc: 'Windows 安装程序（exe，双击安装）' },
  { tpl: 'updater-CertFlow-{V}-darwin-amd64.zip', plat: '', arch: '', desc: '' },
  { tpl: 'updater-CertFlow-{V}-darwin-arm64.zip', plat: '', arch: '', desc: '' },
  { tpl: 'updater-CertFlow-{V}-windows-amd64.zip', plat: '', arch: '', desc: '' },
  { tpl: 'updater-CertFlow-{V}-windows-arm64.zip', plat: '', arch: '', desc: '' },
  { tpl: 'updater-CertFlow-{V}-linux-amd64.tar.gz', plat: '', arch: '', desc: '' },
  { tpl: 'updater-CertFlow-{V}-linux-arm64.tar.gz', plat: '', arch: '', desc: '' },
  { tpl: 'SHA256SUMS', plat: '', arch: '', desc: '' }
];
const PRE_EXTRA = [
  { tpl: 'BUILD_TIME', plat: '', arch: '', desc: '' },
  { tpl: 'GIT_COMMIT', plat: '', arch: '', desc: '' }
];

function cnbAssets(ver, pre) {
  const tpls = pre ? CNB_TEMPLATES.concat(PRE_EXTRA) : CNB_TEMPLATES.slice();
  return tpls.map((t) => Object.assign({}, t, { name: t.tpl.replace('{V}', ver) }));
}

// cnb 源的正文本地生成，不跨平台去读 GitHub 的 body（否则附件从 CNB 来、
// 更新内容从 GitHub 来，等于一次发布里挂着两个平台的叙事）。
function localBody(sizes, pre) {
  const dl = 'https://gitee.com/dtapps/certflow/releases/download/' + TAG;
  const now = new Date().toISOString().replace('T', ' ').slice(0, 19) + ' UTC';
  const lines = [];
  if (pre) {
    lines.push('## ⚠️ 预发布版本', '', '> 这是预发布版本，可能包含未充分测试的功能。', '');
  }
  lines.push('## 发布信息', '', '| 项目 | 内容 |', '| --- | --- |');
  lines.push('| 版本号 | ' + TAG + ' |');
  lines.push('| 发布类型 | ' + (pre ? '🚧 预发布版本 (Pre-release)' : '✅ 正式发布版本 (Stable)') + ' |');
  lines.push('| 构建时间 | ' + now + ' |');
  lines.push('', '## 下载', '', '| 平台 | 架构 | 文件 | 大小 | 说明 |', '| --- | --- | --- | --- | --- |');
  CNB_TEMPLATES.forEach((t) => {
    if (!t.plat) return;
    const name = t.tpl.replace('{V}', VER);
    lines.push('| ' + t.plat + ' | ' + t.arch + ' | [' + name + '](' + dl + '/' + name + ') | ' +
      mb(sizes[name] || 0) + ' | ' + t.desc + ' |');
  });
  lines.push('', '## 更新内容', '');
  lines.push('- 详见 [CHANGELOG.md](https://gitee.com/dtapps/certflow/blob/' + TAG + '/CHANGELOG.md)');
  lines.push('');
  return lines.join('\n');
}

// ── 正文渲染 ──────────────────────────────────────────────────────────────
// 去掉 markdown 里的一整节（形如 "## 更新内容" 到下一个 "## "）
function dropSection(text, title) {
  const out = [];
  let skip = false;
  for (const line of text.split('\n')) {
    if (/^##\s+/.test(line)) skip = line.trim() === '## ' + title;
    if (!skip) out.push(line);
  }
  return out.join('\n');
}

// 按字节每 200 插 <br/>，只在标签/文本边界切，不切断多字节汉字与 HTML 标签。
// release@gitee 插件把参数值当相对路径 open，单段超过 255 字节直接 file name too long，
// <br/> 自带的 / 把路径切成短段，open 走 ENOENT 分支降级成正文。
function withBreaks(block) {
  const tokens = block.match(/<[^>]*>|[^<]+/g) || [];
  const LIMIT = 200;
  let out = '';
  let size = 0;
  for (const t of tokens) {
    if (t.charAt(0) === '<') {
      out += t;
      size += Buffer.byteLength(t, 'utf8');
    } else {
      let buf = Buffer.from(t, 'utf8');
      while (buf.length > 0) {
        if (size >= LIMIT) {
          out += '<br/>';
          size = 0;
          continue;
        }
        let end = Math.min(LIMIT - size, buf.length);
        while (end > 0 && (buf[end] & 0xc0) === 0x80) end--; // 回退到 UTF-8 字符边界
        out += buf.slice(0, end).toString('utf8');
        size += end;
        buf = buf.slice(end);
      }
    }
    if (size >= LIMIT) {
      out += '<br/>';
      size = 0;
    }
  }
  return out;
}

function toSingleLine(html) {
  const one = html
    .replace(/\s+[a-zA-Z][\w:-]*="[^"]*"/g, '') // 去属性，顺带去掉属性值里的 =
    .replace(/=/g, '&#61;')
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
    .map(withBreaks)
    .join('');
  if (one.indexOf('=') !== -1) throw new Error('正文里还有裸 =，会被参数解析截断');
  const longest = Math.max.apply(null, one.split('/').map((p) => Buffer.byteLength(p, 'utf8')));
  if (longest > 200) throw new Error('正文分段后仍有 ' + longest + ' 字节的片段，会被插件当超长文件名');
  return one;
}

// GitHub markdown → Gitee 正文 HTML。
// 只保留：预发布提示 / 发布信息 / 下载 / 校验；「更新内容」整节换成一句指引。
function renderBody(md, tag) {
  const { marked } = require('marked');
  const note = SOURCE === 'cnb'
    ? '\n\n> 更新内容见 CHANGELOG.md。\n'
    : '\n\n> 更新内容见 GitHub Release；超过 100MB 的附件（如 AppImage）只在 GitHub 提供。\n';
  let html = marked.parse(dropSection(md, '更新内容') + note);
  // 只留链接文字：Gitee 页面自带附件列表，正文里的外链点了也是跳走
  html = html.replace(
    /<a\b[^>]*\bhref="([^"]*)"[^>]*>([\s\S]*?)<\/a>/g,
    (m, url, text) => text.replace(/<[^>]+>/g, '').trim()
  ).replace(/<a\b[^>]*>\s*<\/a>/g, '');
  if (SINGLE_LINE) {
    html = toSingleLine(html);
    console.log('正文：已压成单行 HTML（裸 = 转义，每 200 字节插 <br/>）');
  } else {
    console.log('正文：' + Buffer.byteLength(html) + ' 字节 HTML（更新内容已改为指引）');
  }
  return html;
}

// 按 3 路并发跑队列；序号在出队时定，避免并发日志里出现重复编号
async function runQueue(items, total, downloadOne) {
  const queue = items.slice();
  let seq = 0;
  async function worker() {
    while (queue.length) {
      await downloadOne(queue.shift(), ++seq, total);
    }
  }
  const pool = [];
  for (let i = 0; i < Math.min(CONCURRENCY, queue.length); i++) pool.push(worker());
  await Promise.all(pool);
}

async function runCnb(pre) {
  console.log('附件来源：CNB（固定 ' + CNB_TEMPLATES.length + ' 个产物；大小现问，正文本地生成，不碰 GitHub）');
  const wanted = cnbAssets(VER, pre);
  const sizes = {};
  const skipped = [];
  const files = [];

  await runQueue(wanted, wanted.length, async (item, n) => {
    const dest = path.join(OUT_DIR, item.name);
    const expected = await probeSize(urlFor(item)).catch(() => 0);
    if (expected > MAX_FILE) {
      skipped.push(item.name + '（' + mb(expected) + ' 超过 Gitee 单附件 100MB，跳过该文件）');
      return;
    }
    console.log('[' + n + '/' + wanted.length + '] ' + item.name + '（' + mb(expected) + '）');
    await downloadFile(item, dest, expected);
    if (!looksRight(dest, item.name)) {
      fs.unlinkSync(dest);
      throw new Error(item.name + ' 落盘内容不像 ' + path.extname(item.name) + '，可能是错误页');
    }
    const size = fs.statSync(dest).size;
    sizes[item.name] = size;
    files.push({ name: item.name, size, path: dest });
  });

  skipped.forEach((s) => console.log('  跳过 ' + s));
  return { files, skipped, sizes, body: localBody(sizes, pre) };
}

async function runGithub(pre) {
  const relUrl = TAG
    ? GH_API + '/releases/tags/' + encodeURIComponent(TAG)
    : GH_API + '/releases/latest';
  console.log('版本号: ' + (TAG || '未指定，取 GitHub 最新 Release'));
  const rel = JSON.parse(await readText(relUrl));
  if (!TAG) {
    TAG = String(rel.tag_name || '').trim();
    if (!TAG) fail('GitHub 最新 Release 没有 tag_name，无法判定版本');
    VER = TAG.replace(/^v/i, '');
    console.log('自动判定版本：' + TAG);
  }

  const all = rel.assets || [];
  if (!all.length) fail('GitHub Release ' + TAG + ' 没有附件');

  const skipped = [];
  const assets = all.filter((a) => {
    if ((a.size || 0) > MAX_FILE) {
      skipped.push(a.name + '（' + mb(a.size || 0) + ' 超过 Gitee 单附件 100MB，跳过该文件）');
      return false;
    }
    return true;
  });
  if (!assets.length) fail('过滤后没有可上传 Gitee 的附件');
  skipped.forEach((s) => console.log('  跳过 ' + s));

  const files = [];
  await runQueue(assets, assets.length, async (asset, n) => {
    const dest = path.join(OUT_DIR, asset.name);
    console.log('[' + n + '/' + assets.length + '] ' + asset.name + '（' + mb(asset.size || 0) + '）');
    await downloadFile(asset, dest, asset.size || 0);
    if (!looksRight(dest, asset.name)) {
      fs.unlinkSync(dest);
      throw new Error(asset.name + ' 落盘内容不像 ' + path.extname(asset.name) + '，可能是错误页');
    }
    files.push({ name: asset.name, size: fs.statSync(dest).size, path: dest });
  });

  const body = String(rel.body || '')
    .replace(/\r\n/g, '\n')
    .split('https://github.com/dtapps/certflow').join('https://gitee.com/dtapps/certflow')
    .trim();
  return { files, skipped, body: body || ('CertFlow ' + TAG), rel };
}

async function main() {
  // 容器启动时工作目录可能已被删除（日志里的 shell-init getcwd 报错），先兜一下
  try { process.cwd(); } catch (e) { process.chdir('/'); }

  if (SOURCE !== 'cnb' && SOURCE !== 'github') {
    fail('DOWNLOAD_SOURCE 只能是 cnb 或 github，收到：' + JSON.stringify(SOURCE));
  }

  // 只解析版本用：打印后退出，不下载、不写清单。
  // shell 在 VERSION 为空时先调它拿 tag，再做版本号后缀判定。
  if (process.argv.indexOf('--resolve-tag') !== -1) {
    if (!TAG && SOURCE === 'github') {
      const rel = JSON.parse(await readText(GH_API + '/releases/latest'));
      TAG = String(rel.tag_name || '').trim();
    }
    if (!TAG) {
      fail(SOURCE === 'cnb'
        ? 'VERSION 为空：CNB 的 API 匿名访问返回 401，推不出最新 tag，请填 VERSION'
        : 'VERSION 为空：查 GitHub 最新 Release 也失败了，请填 VERSION');
    }
    process.stdout.write(TAG + '\n');
    return;
  }

  if (/^nightly$/i.test(VER)) {
    fail('请传最终 nightly 版本（如 v1.2.4-nightly.20260101），不要传裸 nightly');
  }
  if (!TAG) {
    fail(SOURCE === 'cnb'
      ? 'VERSION 为空：CNB 的 API 匿名访问返回 401，推不出最新 tag，请填 VERSION'
      : 'VERSION 为空且未拿到 GitHub 最新 Release，请填 VERSION');
  }

  if (!fs.existsSync(OUT_DIR)) fs.mkdirSync(OUT_DIR, { recursive: true });

  const preByTag = /[-.](rc|beta|alpha|nightly|pre|preview)\d*(\.|\b)/i.test(VER);
  const preEnv = (process.env.PRERELEASE || '').trim();
  const pre = preEnv === 'true' ? true : preEnv === 'false' ? false : preByTag;

  const result = SOURCE === 'cnb' ? await runCnb(pre) : await runGithub(pre);
  const { files, skipped } = result;
  if (!files.length) fail('过滤后没有可上传 Gitee 的附件');

  const totalBytes = files.reduce((s, f) => s + f.size, 0);
  console.log('共下载 ' + files.length + ' 个附件（' + mb(totalBytes) + '）');
  if (totalBytes > MAX_TOTAL) {
    fail('待上传 ' + mb(totalBytes) + ' 超过安全阈值 ' + mb(MAX_TOTAL) +
      '（Gitee 仓库附件总量 1GB，含历史发行版）。请先清理旧 Release 的附件');
  }

  const bodyHtml = renderBody(result.body || ('CertFlow ' + TAG), TAG);
  const manifest = {
    tag: TAG,
    ver: VER,
    prerelease: pre,
    releaseName: 'CertFlow ' + TAG + (pre ? ' (预发布)' : ''),
    targetCommitish: (result.rel && String(result.rel.target_commitish || '')) || 'master',
    bodyHtml,
    outDir: OUT_DIR,
    totalBytes,
    files,
    skipped
  };
  fs.writeFileSync(MANIFEST, JSON.stringify(manifest, null, 2) + '\n', 'utf8');

  console.log('---- 下载完成 ----');
  console.log('清单：' + MANIFEST + '（' + files.length + ' 个文件，' + mb(totalBytes) + '）');
  manifest.files.forEach((f) => console.log('  - ' + f.name + ' ' + mb(f.size)));
}

main().catch((err) => fail(err && err.message ? err.message : String(err)));
