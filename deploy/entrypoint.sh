#!/bin/sh
# 更新包位于持久卷中；75 表示程序请求重新启动。
child=
trap 'if [ -n "$child" ]; then kill -TERM "$child" 2>/dev/null; wait "$child"; fi; exit 0' TERM INT
while :; do
  app=/usr/local/bin/clash-config-manager
  if [ -x /app/data/bin/clash-config-manager ]; then
    app=/app/data/bin/clash-config-manager
  fi
  "$app" -managed "$@" &
  child=$!
  wait "$child"
  status=$?
  child=
  if [ "$status" -ne 75 ]; then exit "$status"; fi
done
