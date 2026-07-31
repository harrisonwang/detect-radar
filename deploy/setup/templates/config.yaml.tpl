# 由 setup 容器渲染生成 —— 不要手改；改 deploy/.env 后执行：
#   docker compose up -d && docker compose restart server
server:
  port: "127.0.0.1:8080"
  read_timeout: 10s
  write_timeout: 10s
  trusted_proxies:
    - "127.0.0.1"
    - "::1"
  proxy_header: "X-Forwarded-For"

cors:
  allow_origins:
    - "https://${DOMAIN}"
  allow_methods:
    - "GET"
    - "POST"
    - "OPTIONS"
  allow_headers:
    - "Content-Type"
  allow_credentials: false

rate_limit:
  max: 100
  expiration: 60s
  lookup_max: 10

dns:
  domain: "${LEAK_ZONE}"
  ttl: 1h
  dnstap_addr: "127.0.0.1:6000"

ipintel:
  city_db_path: "./data/dbip-city-lite.mmdb"
  asn_db_path: "./data/dbip-asn-lite.mmdb"
  cn_v4_db_path: "./data/ip2region-cn/ip2region_v4.xdb"
  cn_v6_db_path: "./data/ip2region-cn/ip2region_v6.xdb"
  cache_ttl: 24h
  ipapiis_key: "${IPAPIIS_KEY}"
  bigdatacloud_key: "${BIGDATACLOUD_KEY}"
  ipregistry_key: "${IPREGISTRY_KEY}"
  ip2location_key: "${IP2LOCATION_KEY}"
  cloudflare_radar_token: "${CLOUDFLARE_RADAR_TOKEN}"

redis:
  # 默认不部署 Redis：连不上仅缓存降级，不影响功能
  addr: "127.0.0.1:6379"
  password: ""
  db: 0

sqlite:
  path: "./data/hosting.db"

rtt:
  # 本机经纬度：地理-延迟物理下限校验的锚点，setup 自动探测（.env 可覆盖）
  server_lat: ${SERVER_LAT}
  server_lon: ${SERVER_LON}
