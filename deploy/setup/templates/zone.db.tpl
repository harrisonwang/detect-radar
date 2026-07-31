$ORIGIN ${LEAK_ZONE}.
$TTL 60
@   IN SOA ns1.${LEAK_ZONE}. hostmaster.${DOMAIN}. (
        ${ZONE_SERIAL} ; serial
        300        ; refresh
        60         ; retry
        86400      ; expire
        60         ; minimum
)
; NS 与 glue 显式写 3600：子区 NS 的 TTL 会覆盖父区(Cloudflare)的委派，
; 若继承 $TTL 60，递归器每 60 秒就要重取一次委派，冷查询白付一个跨洋 RTT
@    3600 IN NS ns1.${LEAK_ZONE}.
ns1  3600 IN A  ${SERVER_IPV4}
; acme DNS-01：委派区内的 _acme-challenge 指回 Cloudflare 托管的别名，
; 让 *.${LEAK_ZONE} 证书也能走 dns_cf 全自动签发/续期
_acme-challenge 300 IN CNAME _acme-challenge.acme-leak.${DOMAIN}.
*         IN A  ${SERVER_IPV4}
