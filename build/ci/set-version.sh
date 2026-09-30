#!/usr/bin/env bash
#
# 从 Git tag 推导版本号，写入各平台的构建配置。
#
# 用法：
#   build/ci/set-version.sh            # 从 GITHUB_REF_TYPE/NAME 推导
#   build/ci/set-version.sh 0.2.0      # 显式指定
#
# 产出：
#   1. 写入 build/config.yml、build/windows/info.json、build/darwin/Info.plist 的版本字段
#   2. 导出 APP_VERSION / APP_VERSION_CODE 到 $GITHUB_ENV，供后续构建步骤使用
#
# 注意：Go 侧的 AppVersion 不在这里改。各平台 Taskfile 的 BUILD_FLAGS 会在
# APP_VERSION 存在时注入 `-X kinh-desktop/service.AppVersion=<版本>`，
# 由链接器覆盖 service/update.go 里的变量。因此本脚本不碰任何 .go 文件。
set -euo pipefail

version="${1:-}"

if [ -z "$version" ]; then
  if [ "${GITHUB_REF_TYPE:-}" = "tag" ]; then
    version="${GITHUB_REF_NAME#v}"
  elif [ -n "${GITHUB_RUN_NUMBER:-}" ]; then
    # 手动触发（workflow_dispatch）没有 tag，用流水线序号造一个合法的开发版本
    version="0.0.${GITHUB_RUN_NUMBER}"
  else
    ref="$(git describe --tags --always 2>/dev/null || echo '')"
    version="${ref#v}"
  fi
fi

# 只接受 0.1 / 0.1.2 这类纯数字点分格式。挡住的是把分支名、commit hash 写进
# 版本字段的情况 —— 那会让 Windows 资源文件与 macOS Info.plist 生成非法版本串。
if ! printf '%s' "$version" | grep -Eq '^[0-9]+(\.[0-9]+)*$'; then
  echo "版本号格式不合法: '$version'（应为 0.1 或 0.1.2 形式）" >&2
  exit 1
fi

# Android 的 versionCode 必须是随发布递增的整数，否则无法覆盖安装 / 上架。
# 从语义化版本推导：major*10000 + minor*100 + patch（0.2.0 → 200）
IFS='.' read -r v_major v_minor v_patch <<< "$version"
v_minor="${v_minor:-0}"
v_patch="${v_patch:-0}"
version_code=$(( v_major * 10000 + v_minor * 100 + v_patch ))

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# sed -i 在 GNU 与 BSD 上语义不同，统一用 -i.bak 再删备份
subst() {
  local file="$1" expr="$2"
  if [ ! -f "$file" ]; then
    echo "跳过（文件不存在）: $file" >&2
    return 0
  fi
  sed -i.bak -E "$expr" "$file"
  rm -f "$file.bak"
}

echo "同步版本号 -> $version (versionCode=$version_code)"

# 1) wails 构建配置。它同时是 info.json / Info.plist 的来源，
#    CI 不跑 update:build-assets（那会覆盖对 nsis 等资产的定制），故下面单独改
subst "$root/build/config.yml" "s/^  version: \"[^\"]*\"/  version: \"$version\"/"

# 2) Linux 包元数据（deb / rpm / Arch 三者的包版本）
subst "$root/build/linux/nfpm/nfpm.yaml" "s/^version: \"[^\"]*\"/version: \"$version\"/"

# 3) Windows 安装包版本（NSIS 的 INFO_PRODUCTVERSION）
subst "$root/build/windows/nsis/project.nsi" \
  "s/^!define INFO_PRODUCTVERSION +\"[^\"]*\"/!define INFO_PRODUCTVERSION \"$version\"/"

# 4) 前端包版本（仅为保持一致，构建产物不依赖它）
subst "$root/frontend/package.json" "s/^  \"version\": \"[^\"]*\",/  \"version\": \"$version\","

# 2) Windows 可执行文件的文件属性（资源管理器「属性 → 详细信息」里看到的版本）
subst "$root/build/windows/info.json" \
  "s/(\"file_version\": *)\"[^\"]*\"/\\1\"$version\"/"
subst "$root/build/windows/info.json" \
  "s/(\"ProductVersion\": *)\"[^\"]*\"/\\1\"$version\"/"

# 3) macOS .app 的版本（Finder 显示、以及系统更新判断依据）
#    plist 里是 key 一行、string 一行的结构，用 n 跳到下一行再替换
plist="$root/build/darwin/Info.plist"
if [ -f "$plist" ]; then
  sed -i.bak -E \
    "/<key>CFBundleVersion<\/key>/{n;s|<string>[^<]*</string>|<string>${version}</string>|;}" \
    "$plist"
  sed -i.bak -E \
    "/<key>CFBundleShortVersionString<\/key>/{n;s|<string>[^<]*</string>|<string>${version}</string>|;}" \
    "$plist"
  rm -f "$plist.bak"
fi

# Android 的版本无需在此改写：build/android/app/build.gradle 直接读环境变量

# 回显实际写入结果，CI 日志里能直接核对
echo "--- 写入结果 ---"
grep -n '^  version:' "$root/build/config.yml" || true
grep -n 'file_version\|ProductVersion' "$root/build/windows/info.json" || true
grep -n -A 1 'CFBundleVersion\|CFBundleShortVersionString' "$plist" 2>/dev/null || true

# 导出给后续构建步骤（Taskfile 读 APP_VERSION 决定是否注入 -X）
if [ -n "${GITHUB_ENV:-}" ]; then
  {
    echo "APP_VERSION=$version"
    echo "APP_VERSION_CODE=$version_code"
  } >> "$GITHUB_ENV"
  echo "已导出 APP_VERSION=$version APP_VERSION_CODE=$version_code"
else
  echo "（非 CI 环境，未导出环境变量）"
fi
