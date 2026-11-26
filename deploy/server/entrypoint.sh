#!/bin/sh
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
