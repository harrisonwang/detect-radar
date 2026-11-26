#!/bin/sh
set -eu

: "${DOMAIN:?缺少 DOMAIN}" "${CF_Token:?缺少 CF_Token}" "${ACME_EMAIL:?缺少 ACME_EMAIL}"
LEAK_ZONE="leak.${DOMAIN}"
CF_API="https://api.cloudflare.com/client/v4"

log() { echo "[setup] $*"; }
die() { echo "[setup] 错误: $*" >&2; exit 1; }

cf() { # method path [json-body]
  if [ -n "${3:-}" ]; then
    curl -sS -X "$1" "${CF_API}/$2" \
      -H "Authorization: Bearer ${CF_Token}" -H "Content-Type: application/json" --data "$3"
  else
    curl -sS -X "$1" "${CF_API}/$2" -H "Authorization: Bearer ${CF_Token}"
  fi
}

if [ "${SERVER_IPV4:-auto}" = "auto" ] || [ -z "${SERVER_IPV4:-}" ]; then
  SERVER_IPV4=$(curl -4fsS --max-time 10 https://api.ipify.org 2>/dev/null \
             || curl -4fsS --max-time 10 https://ifconfig.me 2>/dev/null || true)
fi
[ -n "${SERVER_IPV4:-}" ] || die "公网 IPv4 探测失败，请在 .env 显式设置 SERVER_IPV4"
log "公网 IPv4: ${SERVER_IPV4}"

if [ "${DNS_LISTEN_IP:-auto}" = "auto" ] || [ -z "${DNS_LISTEN_IP:-}" ]; then
  DNS_LISTEN_IP=$(ip route get 1.1.1.1 2>/dev/null | sed -n 's/.*src \([0-9.]*\).*/\1/p' | head -1)
  [ -n "$DNS_LISTEN_IP" ] || DNS_LISTEN_IP="$SERVER_IPV4"
fi
log "CoreDNS 监听: ${DNS_LISTEN_IP}"

if [ -z "${SERVER_LAT:-}" ] || [ -z "${SERVER_LON:-}" ]; then
  geo=$(curl -fsS --max-time 10 "http://ip-api.com/json/${SERVER_IPV4}?fields=status,lat,lon" 2>/dev/null || true)
  if [ "$(echo "$geo" | jq -r .status 2>/dev/null)" = "success" ]; then
    SERVER_LAT=$(echo "$geo" | jq -r .lat)
    SERVER_LON=$(echo "$geo" | jq -r .lon)
    log "服务器定位: ${SERVER_LAT}, ${SERVER_LON}（来自 ip-api.com，.env 可覆盖）"
  else
    die "无法自动定位服务器经纬度（RTT 物理校验依赖），请在 .env 设置 SERVER_LAT / SERVER_LON"
  fi
fi

if [ ! -f /out/coredns/.rendered ]; then
  if netstat -uln 2>/dev/null | awk '{print $4}' | grep -qE "^(${DNS_LISTEN_IP}|0\.0\.0\.0):53$"; then
    log "警告: ${DNS_LISTEN_IP}:53 已被占用。systemd-resolved 只占 127.0.0.53 不冲突；"
    log "警告: 若是 dnsmasq/named 占了 0.0.0.0:53，CoreDNS 将起不来，需先释放 53 端口"
  fi
fi

candidate="$DOMAIN"; ZONE_ID=""; ZONE_NAME=""
while echo "$candidate" | grep -q '\.'; do
  ZONE_ID=$(cf GET "zones?name=${candidate}&status=active" | jq -r '.result[0].id // empty')
  if [ -n "$ZONE_ID" ]; then ZONE_NAME="$candidate"; break; fi
  candidate=${candidate#*.}
done
[ -n "$ZONE_ID" ] || die "在 Cloudflare 找不到 ${DOMAIN} 所属 zone；确认域名已托管到 CF 且 Token 有 Zone.Read 权限"
log "Cloudflare zone: ${ZONE_NAME} (${ZONE_ID})"

upsert() { # type name content [proxiable]
  t=$1; n=$2; c=$3
  if [ "${4:-}" = "y" ]; then
    body=$(jq -nc --arg t "$t" --arg n "$n" --arg c "$c" '{type:$t,name:$n,content:$c,ttl:300,proxied:false}')
  else
    body=$(jq -nc --arg t "$t" --arg n "$n" --arg c "$c" '{type:$t,name:$n,content:$c,ttl:300}')
  fi
  existing=$(cf GET "zones/${ZONE_ID}/dns_records?type=${t}&name=${n}")
  rid=$(echo "$existing" | jq -r '.result[0].id // empty')
  was_proxied=$(echo "$existing" | jq -r '.result[0].proxied // false')
  if [ -n "$rid" ]; then
    if [ "$was_proxied" = "true" ]; then
      log "警告: ${n} 此前开着橙云（proxied），已强制改为仅 DNS —— 经 CF 代理会毁掉源 IP 与 RTT 检测"
    fi
    out=$(cf PUT "zones/${ZONE_ID}/dns_records/${rid}" "$body")
  else
    out=$(cf POST "zones/${ZONE_ID}/dns_records" "$body")
  fi
  [ "$(echo "$out" | jq -r .success)" = "true" ] \
    || die "写入 DNS 记录失败 ${t} ${n}: $(echo "$out" | jq -c .errors)"
  log "DNS 记录就绪: ${t} ${n} -> ${c}"
}

upsert A "${DOMAIN}" "${SERVER_IPV4}" y
if [ -n "${SERVER_IPV6:-}" ]; then
  upsert AAAA "${DOMAIN}" "${SERVER_IPV6}" y
fi
upsert A "ns1.${LEAK_ZONE}" "${SERVER_IPV4}" y
if [ -n "${SERVER_IPV6:-}" ]; then
  upsert AAAA "ns1.${LEAK_ZONE}" "${SERVER_IPV6}" y
fi
upsert NS "${LEAK_ZONE}" "ns1.${LEAK_ZONE}"

export DOMAIN LEAK_ZONE SERVER_IPV4 SERVER_LAT SERVER_LON \
       IPAPIIS_KEY BIGDATACLOUD_KEY IPREGISTRY_KEY IP2LOCATION_KEY CLOUDFLARE_RADAR_TOKEN
BIND_IPS="${DNS_LISTEN_IP}${SERVER_IPV6:+ ${SERVER_IPV6}}"
ZONE_SERIAL=$(date +%s)
export BIND_IPS ZONE_SERIAL

mkdir -p /out/server /out/nginx /out/coredns/zones /out/acme /out/certs

envsubst '${DOMAIN} ${LEAK_ZONE} ${SERVER_LAT} ${SERVER_LON} ${IPAPIIS_KEY} ${BIGDATACLOUD_KEY} ${IPREGISTRY_KEY} ${IP2LOCATION_KEY} ${CLOUDFLARE_RADAR_TOKEN}' \
  < /templates/config.yaml.tpl > /out/server/config.yaml
envsubst '${DOMAIN}' < /templates/site.conf.tpl > /out/nginx/site.conf
envsubst '${LEAK_ZONE}' < /templates/leak.conf.tpl > /out/nginx/leak.conf
envsubst '${LEAK_ZONE} ${BIND_IPS}' < /templates/Corefile.tpl > /out/coredns/Corefile
envsubst '${DOMAIN} ${LEAK_ZONE} ${SERVER_IPV4} ${ZONE_SERIAL}' \
  < /templates/zone.db.tpl > "/out/coredns/zones/${LEAK_ZONE}.db"
if [ -n "${SERVER_IPV6:-}" ]; then
  printf 'ns1  3600 IN AAAA %s\n*         IN AAAA %s\n' \
    "$SERVER_IPV6" "$SERVER_IPV6" >> "/out/coredns/zones/${LEAK_ZONE}.db"
fi
cp /assets/acme-bootstrap.sh /out/acme/bootstrap.sh
touch /out/coredns/.rendered
log "配置渲染完成"

placeholder() { # dir-name cert-cn
  d="/out/certs/$1"
  [ -s "${d}/fullchain.cer" ] && return 0
  mkdir -p "$d"
  openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
    -keyout "${d}/key.pem" -out "${d}/fullchain.cer" -days 30 -nodes \
    -subj "/CN=$2" -addext "subjectAltName=DNS:$2" >/dev/null 2>&1
  log "生成占位证书: $2"
}
placeholder "${DOMAIN}" "${DOMAIN}"
placeholder "${LEAK_ZONE}" "*.${LEAK_ZONE}"

log "初始化完成。稍后验证:"
log "  dig NS ${LEAK_ZONE} @1.1.1.1        # 应答 ns1.${LEAK_ZONE}"
log "  dig A test.${LEAK_ZONE} @${SERVER_IPV4}  # 应答 ${SERVER_IPV4}"
log "  curl https://${DOMAIN}/health       # acme 签完真证书后应返回 healthy"
