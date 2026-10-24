#!/usr/bin/env node
// 将 Vitest 的 JSON reporter 输出（默认 test-results.json）转换为 JUnit XML（默认 test-report.xml），
// 供 codecov/test-results-action 上传测试分析结果（Test Analytics）。
//
// 用法：
//   node scripts/vitest-json-to-junit.mjs [input.json] [output.xml]
//
// 为什么需要它：Vitest 5.0.5 没有内置 junit reporter，社区 @vitest/junit 包在 npm 上也已不可得；
// 但 Vitest 有内置 json reporter，这里用一份零依赖脚本完成 json -> junit 的转换。
import { readFileSync, statSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const input = process.argv[2] || "test-results.json";
const output = process.argv[3] || "test-report.xml";

// 仓库根：本脚本固定位于 <repo>/frontend/scripts/ 下，往上两级即根目录。
const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

// 判断「仓库相对路径」是否真的指向仓库内的文件（跨树兜底匹配用）
const isFileInRepo = (relPath) => {
  try {
    return statSync(path.join(repoRoot, relPath)).isFile();
  } catch {
    return false;
  }
};

// Vitest 的 json reporter 里 testResults[].name 是测试文件的**绝对路径**
// （CI 上形如 /__w/certflow/certflow/frontend/src/test/format.test.ts）。
// 这个值会被写进 JUnit 的 classname，而 Codecov 正是拿 classname 当用例的
// 归属文件（分组展示 + 跨 run 关联 + 失败位置跳转）。绝对路径带的是 runner
// 专属前缀，GitHub Actions 与 CNB 前缀不同、和仓库内文件对不上，于是 UI 上
// 就出现一条条 "/__w/certflow/certflow/frontend/src/test/xxx.test.ts" 的怪路径
// （Go 侧 gotestsum 的 classname 是包导入路径，天然相对、所以后端正常）。
// 这里统一转成「相对仓库根 + POSIX 分隔」的路径，与 Jest 的 {filepath} 口径一致。
const toRepoRelative = (p) => {
  if (!p) return "";
  const abs = path.isAbsolute(p) ? p : path.resolve(process.cwd(), p);
  const rel = path.relative(repoRoot, abs);
  // 常规情况：JSON 与脚本在同一份 checkout 树内，直接相减即可。
  if (rel !== "" && !rel.startsWith("..") && !path.isAbsolute(rel)) {
    return rel.split(path.sep).join("/");
  }
  // 兜底：路径不在这棵树里。例如测试在容器内跑（/__w/...、/workspace/...）、
  // 归一化在宿主上跑；或本机复现别的 CI（/home/runner/work/...）产出的 JSON。
  // 这时任何「前缀」都不可用，改为从尾部取最长的「在仓库里真实存在」的后缀，
  // 即 frontend/src/test/xxx.test.ts —— 与绝对前缀长什么样完全无关。
  const segs = abs.split(/[\\/]+/).filter(Boolean);
  for (let i = segs.length; i >= 2; i--) {
    const candidate = segs.slice(segs.length - i).join("/");
    // 必须真的是文件：否则可能命中 frontend/src/test 这类同名目录
    if (isFileInRepo(candidate)) return candidate;
  }
  return p; // 实在认不出来：原样返回（至少不凭空编造路径）
};

const data = JSON.parse(readFileSync(input, "utf8"));
const suites = Array.isArray(data.testResults) ? data.testResults : [];

const escapeXml = (s) =>
  String(s ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&apos;");

let total = 0;
let failures = 0;
let skipped = 0;
let totalTime = 0;
const suitesXml = [];

for (const suite of suites) {
  const name = toRepoRelative(suite.name) || "unknown";
  const cases = Array.isArray(suite.assertionResults) ? suite.assertionResults : [];
  let sFail = 0;
  let sSkip = 0;
  let sTime = 0;
  const casesXml = [];

  for (const t of cases) {
    const status = t.status || "unknown";
    const dur = (typeof t.duration === "number" ? t.duration : 0) / 1000;
    sTime += dur;
    total++;
    totalTime += dur;

    const testName = t.fullName || t.title || "unknown";
    if (status === "failed") {
      failures++;
      sFail++;
    }
    if (status === "skipped" || status === "pending" || status === "todo") {
      skipped++;
      sSkip++;
    }

    let inner = "";
    if (status === "failed") {
      const msg = (t.failureMessages || []).join("\n");
      const head = msg.split("\n")[0] || "failed";
      inner = `<failure message="${escapeXml(head)}">${escapeXml(msg)}</failure>`;
    } else if (status === "skipped" || status === "pending" || status === "todo") {
      inner = "<skipped/>";
    }

    casesXml.push(
      `    <testcase name="${escapeXml(testName)}" classname="${escapeXml(name)}" time="${dur.toFixed(3)}">${inner}</testcase>`
    );
  }

  // 整 suite 运行期错误（无 assertionResults 但有 message）
  if (suite.status && suite.status !== "passed" && cases.length === 0) {
    failures++;
    total++;
    sFail++;
    const msg = suite.message || "suite failed";
    casesXml.push(
      `    <testcase name="${escapeXml(name)}" classname="${escapeXml(name)}" time="0"><failure message="${escapeXml(
        msg.split("\n")[0]
      )}">${escapeXml(msg)}</failure></testcase>`
    );
  }

  suitesXml.push(
    `  <testsuite name="${escapeXml(name)}" tests="${cases.length}" failures="${sFail}" skipped="${sSkip}" time="${sTime.toFixed(3)}">\n${casesXml.join(
      "\n"
    )}\n  </testsuite>`
  );
}

const xml =
  `<?xml version="1.0" encoding="UTF-8"?>\n` +
  `<testsuites name="vitest" tests="${total}" failures="${failures}" skipped="${skipped}" time="${totalTime.toFixed(3)}">\n` +
  `${suitesXml.join("\n")}\n` +
  `</testsuites>\n`;

writeFileSync(output, xml);
console.log(
  `JUnit XML 已写出 ${output}：${total} 用例，${failures} 失败，${skipped} 跳过`
);
