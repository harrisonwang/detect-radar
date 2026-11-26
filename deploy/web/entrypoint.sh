#!/bin/sh
set -eu

(
  while :; do
    inotifywait -qq -r -e close_write -e create -e moved_to \
      /etc/nginx/certs /etc/nginx/conf.d 2>/dev/null || sleep 10
    for i in 1 2 3 4 5; do
      sleep 3
      if nginx -t >/dev/null 2>&1; then
        nginx -s reload
        echo "[web] 检测到证书/配置变化，nginx 已热 reload"
        break
      fi
      echo "[web] 配置暂未就绪，重试 ${i}/5" >&2
    done
  done
) &

exec nginx -g "daemon off;"
