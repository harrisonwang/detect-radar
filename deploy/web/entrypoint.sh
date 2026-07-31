#!/bin/sh
# nginx 前台运行 + 后台 inotify：证书卷（acme 续期）或配置卷（setup 重渲染）一有变化
# 就校验并热 reload，占位证书 → 真证书的切换全自动，无需人工 reload。
set -eu

(
  while :; do
    inotifywait -qq -r -e close_write -e create -e moved_to \
      /etc/nginx/certs /etc/nginx/conf.d 2>/dev/null || sleep 10
    # 证书是多文件非原子写入，事件可能落在写入中途；校验不过就间隔重试，
    # 而不是干等下一个事件（处理窗口内到达的事件会被 inotifywait 漏掉）
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
