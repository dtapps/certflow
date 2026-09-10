#!/bin/bash
# 合并 Go 后端 i18n 拆分文件到主文件
# 用法: ./scripts/merge-go-i18n.sh
#
# 目录结构:
#   internal/i18n/locales/split/
#   ├── zh-CN/
#   │   ├── errors.json
#   │   ├── logs.json
#   │   ├── messages.json
#   │   ├── notifications.json
#   │   └── other.json
#   └── en-US/
#       └── (同上)
#
# 输出:
#   internal/i18n/locales/zh-CN.json
#   internal/i18n/locales/en-US.json

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
SPLIT_DIR="$PROJECT_DIR/internal/i18n/locales/split"
LOCALES_DIR="$PROJECT_DIR/internal/i18n/locales"

merge_locale() {
    local locale="$1"
    local split_dir="$SPLIT_DIR/$locale"
    local output_file="$LOCALES_DIR/$locale.json"
    
    if [ ! -d "$split_dir" ]; then
        echo "错误: 目录不存在 $split_dir"
        return 1
    fi
    
    echo "合并 $locale ..."
    
    local tmp_file=$(mktemp)

    # 一次性读取所有 split 文件并用 reduce 深度合并，避免逐文件 `.[0] * .[1]`
    # 累加在某些 jq 版本下丢键（如 app.name_version）的问题。
    jq -s 'reduce .[] as $f ({}; . * $f)' "$split_dir"/*.json > "$tmp_file"

    # 注意：不能用 `.[1:]` 切片来跳过 _comment 再排序——jq 对拼接数组做切片会丢失
    # 元素（曾导致 app.name_version 等首键从合并产物中消失）。正确做法是把 _comment
    # 一并并入数组后整体 sort_by(.key)，再 from_entries。
    jq 'to_entries | ([{"key":"_comment","value":"⚠️ 此文件由 scripts/merge-go-i18n.sh 自动生成，请勿手动编辑本文件！\n\n新增或修改 i18n key 的正确流程：\n  1. 编辑 split/<locale>/ 下对应的拆分文件（按前缀对应：log.*→logs.json，error.* 与 deploy.error.*→errors.json，notification.*→notifications.json，deploy.message.* 与 message.*→messages.json，settings.*、app.*、menu.*、systray.*、updater.*→other.json）\n  2. 运行 make i18n 或 ./scripts/merge-go-i18n.sh 重新生成此主文件\n\n重要警告：本文件每次合并都 100% 由 split/ 重新生成，手动修改这里会被整体覆盖、改动直接丢失，所以永远不要手改本文件！"}] + .) | sort_by(.key) | from_entries' "$tmp_file" > "$output_file"
    rm -f "$tmp_file"
    
    local count=$(jq 'length' "$output_file")
    echo "  -> $output_file ($count keys)"
}

# 主流程
echo "=== Go 后端 i18n 文件合并 ==="
echo ""

merge_locale "zh-CN"
merge_locale "en-US"

echo ""
echo "完成!"
