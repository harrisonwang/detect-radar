#!/bin/sh
set -u

: "${DOMAIN:?缺少 DOMAIN}" "${CF_Token:?缺少 CF_Token}" "${ACME_EMAIL:?缺少 ACME_EMAIL}"
LEAK_ZONE="leak.${DOMAIN}"
export CF_Token

acme.sh --set-default-ca --server letsencrypt >/dev/null 2>&1 || true
acme.sh --register-account -m "${ACME_EMAIL}" >/dev/null 2>&1 || true

ensure() { # domain install-dir [challenge-alias]
  d=$1; dir=$2; alias=${3:-}
  n=0
  while :; do
    acme.sh --issue -d "$d" --dns dns_cf --keylength ec-256 \
      ${alias:+--challenge-alias "$alias"}
    rc=$?
    if [ "$rc" -eq 0 ] || [ "$rc" -eq 2 ]; then
      break
    fi
    n=$((n + 1))
    if [ "$n" -ge 5 ]; then
      echo "[acme] ${d} 签发失败（已重试 ${n} 次），退出等待容器重启后再试" >&2
      exit 1
    fi
    echo "[acme] ${d} 签发失败(rc=${rc})，60s 后重试（第 ${n}/5 次）"
    sleep 60
  done
  mkdir -p "/certs/${dir}"
  acme.sh --install-cert -d "$d" --ecc \
    --fullchain-file "/certs/${dir}/fullchain.cer" \
    --key-file "/certs/${dir}/key.pem" \
    --reloadcmd "touch /certs/.reload"
}

ensure "${DOMAIN}" "${DOMAIN}"
ensure "*.${LEAK_ZONE}" "${LEAK_ZONE}" "acme-leak.${DOMAIN}"

echo "[acme] 证书就绪，进入守护模式（自动续期）"
exec /entry.sh daemon
