#!/usr/bin/env bash
set -euo pipefail
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT

if [[ "${1:-}" == --remote ]]; then
  if [[ "$(uname -s)" != Linux ]]; then
    echo '远程一键安装适用于 Linux 服务器。' >&2
    exit 1
  fi
  if [[ "$EUID" -ne 0 ]]; then
    echo '请使用 sudo bash 执行远程安装。' >&2
    exit 1
  fi
  base_url=https://raw.githubusercontent.com/mrlihx/clash-config-manager/main
  # 只下载部署文件，不拉源码，也不在服务器编译。
  curl -fsSL "$base_url/compose.quick.yaml" -o "$work_dir/compose.quick.yaml"
  curl -fsSL "$base_url/config.example.yaml" -o "$work_dir/config.example.yaml"
  mkdir -p /opt/clash-config-manager
  cp "$work_dir/compose.quick.yaml" /opt/clash-config-manager/compose.quick.yaml
  cp "$work_dir/config.example.yaml" /opt/clash-config-manager/config.example.yaml
  cd /opt/clash-config-manager
else
  cd "$(dirname "${BASH_SOURCE[0]}")/.."
fi

if ! command -v docker >/dev/null 2>&1; then
  if [[ "$(uname -s)" != Linux ]]; then
    echo '请先安装并启动 Docker Desktop。' >&2
    exit 1
  fi
  curl -fsSL https://get.docker.com -o "$work_dir/install-docker.sh"
  sh "$work_dir/install-docker.sh"
fi

docker compose version >/dev/null
if [[ ! -f config.production.yaml ]]; then
  cp config.example.yaml config.production.yaml
fi
# 容器需要保存后台修改的密码；沿用原项目名和卷，兼容旧部署数据。
chgrp 10001 config.production.yaml
chmod 660 config.production.yaml
docker compose -p clash-config-manager -f compose.quick.yaml pull
docker compose -p clash-config-manager -f compose.quick.yaml up -d --no-build --wait
printf '\n部署完成：浏览器打开 http://服务器IP:8787\n首次安装默认密码：admin123；已有安装继续使用原密码。\n'
