#!/bin/sh
# 在官方 neilpang/acme.sh 容器内运行：首发两张证书后交回守护模式（crond 自动续期）。
#   主站证书：dns_cf 直签（域名在 CF zone 内）。
#   *.leak 通配证书：leak zone 已 NS 委派给本机 CoreDNS，LE 顺委派查 _acme-challenge
#   时会命中 zone 里的静态 CNAME，被指回 CF 托管的别名 —— dns_cf 照常写 TXT 即可验证。
# 签发失败则退出，靠 compose 的 restart 策略退避重试（新委派的 DNS 生效可能要几分钟）。
set -u

: "${DOMAIN:?缺少 DOMAIN}" "${CF_Token:?缺少 CF_Token}" "${ACME_EMAIL:?缺少 ACME_EMAIL}"
LEAK_ZONE="leak.${DOMAIN}"
export CF_Token

acme.sh --set-default-ca --server letsencrypt >/dev/null 2>&1 || true
acme.sh --register-account -m "${ACME_EMAIL}" >/dev/null 2>&1 || true

ensure() { # domain install-dir [challenge-alias]
  d=$1; dir=$2; alias=${3:-}
  # 不用 --list 判断是否已签发：签发失败也会留下域名 conf 目录，会误判成功
  # 然后 install-cert 拷出空证书。以 --issue 退出码为准：0=新签出，2=证书仍有效跳过。
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
  # reloadcmd 会随续期一起触发：touch 共享卷标记文件，web 容器 inotify 到后热 reload
  acme.sh --install-cert -d "$d" --ecc \
    --fullchain-file "/certs/${dir}/fullchain.cer" \
    --key-file "/certs/${dir}/key.pem" \
    --reloadcmd "touch /certs/.reload"
}

ensure "${DOMAIN}" "${DOMAIN}"
ensure "*.${LEAK_ZONE}" "${LEAK_ZONE}" "acme-leak.${DOMAIN}"

echo "[acme] 证书就绪，进入守护模式（自动续期）"
exec /entry.sh daemon
