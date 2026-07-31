#!/bin/sh
# 首启播种地理库：卷里没有的文件才复制，用户手动更新过的库不会被镜像里的旧版覆盖。
# 更新地理库 = 换卷里文件 + docker compose restart server（mmdb 被 mmap，必须重启才生效）。
set -eu

mkdir -p /app/data
if [ -d /app/seed ]; then
  cd /app/seed
  find . -type f | while IFS= read -r f; do
    dest="/app/data/${f#./}"
    if [ ! -e "$dest" ]; then
      mkdir -p "$(dirname "$dest")"
      cp "$f" "$dest"
      echo "[server] 播种 ${f#./}"
    fi
  done
  cd /app
fi

exec /app/detectradar-server
