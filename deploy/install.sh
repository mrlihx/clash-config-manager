#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == --remote ]]; then
  if [[ "$(uname -s)" != Linux ]]; then
    echo '远程一键安装适用于 Linux 服务器。' >&2
    exit 1
  fi
  if [[ "$EUID" -ne 0 ]]; then
    echo '请使用 sudo bash 执行远程安装。' >&2
    exit 1
  fi
  if ! command -v git >/dev/null 2>&1; then
    if command -v apt-get >/dev/null 2>&1; then
      apt-get update
      apt-get install -y git ca-certificates
    elif command -v dnf >/dev/null 2>&1; then
      dnf install -y git ca-certificates
    elif command -v yum >/dev/null 2>&1; then
      yum install -y git ca-certificates
    elif command -v apk >/dev/null 2>&1; then
      apk add --no-cache git ca-certificates
    else
      echo '请先安装 Git，再执行安装命令。' >&2
      exit 1
    fi
  fi
  install_dir=/opt/clash-config-manager
  if [[ -d "$install_dir/.git" ]]; then
    git -C "$install_dir" pull --ff-only origin main
  else
    git clone --branch main --single-branch https://github.com/mrlihx/clash-config-manager.git "$install_dir"
  fi
  # 交给刚拉取的版本部署，避免更新时继续运行旧脚本。
  exec bash "$install_dir/deploy/install.sh"
fi

cd "$(dirname "${BASH_SOURCE[0]}")/.."

if ! command -v docker >/dev/null 2>&1; then
  if [[ "$(uname -s)" != Linux ]]; then
    echo '请先安装并启动 Docker Desktop。' >&2
    exit 1
  fi
  installer=$(mktemp)
  trap 'rm -f "$installer"' EXIT
  curl -fsSL https://get.docker.com -o "$installer"
  sh "$installer"
fi

docker compose version >/dev/null
if [[ ! -f config.production.yaml ]]; then
  cp config.example.yaml config.production.yaml
fi
# 容器需要保存后台修改的密码；已有配置和数据不会被重置。
chgrp 10001 config.production.yaml
chmod 660 config.production.yaml
docker compose -f compose.quick.yaml up -d --build --wait
printf '\n部署完成：浏览器打开 http://服务器IP:8787\n首次安装默认密码：admin123；已有安装继续使用原密码。\n'
