---
name: detectradar
description: 用 DetectRadar 免费接口排查终端出口 IP（IPv4/IPv6）、住宅/机房属性、网段人类流量占比与 DNS 泄露，用简洁中文向用户汇报并按需引导网页复测。
---

# DetectRadar 终端自检指南（SKILL.md）

给具备终端执行能力的 AI 助手（Antigravity、Claude Code、Codex、OpenClaw 等）直接照着跑，不用装任何客户端，全接口免鉴权。

- **能测什么**：当前终端的出口 IP（IPv4 / IPv6）、地区/运营商/ASN、宽带还是机房、所属 ASN 近 30 天人类流量占比、实际使用的 DNS 服务器。
- **测不到什么**：不能查任意第三方 IP；也测不到浏览器里的 WebRTC、时区/语言一致性和指纹。

---

## 按需执行（别多跑）

### 1. 看代理环境变量（严禁打印账号密码）

只确认有没有设代理和协议/端口，如果值里带 `user:pass@` 必须先脱敏再给用户看：

```bash
env | grep -iE '^(http|https|all|no)_proxy=' | sed -E 's#://[^/@]+@#://***@#g' || echo "(未设置代理变量)"
```

### 2. 查终端出口（默认 + IPv4 / IPv6）

```bash
# 当前终端默认出口（走代理变量）
curl -sS -m 5 https://detectradar.com/api/v1/ip

# 分别测纯 IPv4 / 纯 IPv6（排查 IPv6 绕过代理直连）
curl -sS -m 5 https://4.detectradar.com/api/v1/ip
curl -sS -m 4 https://6.detectradar.com/api/v1/ip || echo "(无 IPv6 连通)"

# 需要对照「绕过环境变量直连」时再跑这一条
curl --noproxy '*' -sS -m 5 https://detectradar.com/api/v1/ip
```

**常见报错直接翻成大白话：**
- `curl: (7) Failed to connect to 127.0.0.1...` → 环境变量指到了没开启的本地代理端口（代理关了或端口变了）。
- `curl: (28) Operation timed out` → 终端连接超时（未走代理被阻断，或节点不通）。
- `curl: (6) Could not resolve host` → 终端 DNS 解析失败。

### 3. 查终端 DNS 泄露（仅怀疑 DNS 漏回国内时才跑）

每次生成一次性随机子域，不受旧缓存影响：

```bash
resp=$(curl -sS -m 5 -X POST https://detectradar.com/api/v1/leaks/dns -H "Content-Type: application/json" -d '{}')
id=$(printf '%s' "$resp" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
domain=$(printf '%s' "$resp" | sed -n 's/.*"test_domain":"\([^"]*\)".*/\1/p')

dig +short "r1.${domain}" >/dev/null && sleep 2
curl -sS -m 5 "https://detectradar.com/api/v1/leaks/dns/${id}"
```

说明：单跑这 3 步时返回的 `assessment.state` 会是 `unknown`（未绑定出口）。拿 `dns_servers` 对照第 2 步的出口国家判断，或把 `{ exit, resolvers }` 发给 `POST https://detectradar.com/api/v1/leaks/dns/assessment` 拿标准结论。

---

## 字段口径与判断标准（照这条读，别自己编）

| 项目 | 准确含义 / 判定口径 |
| --- | --- |
| `usage_type` | 只有三个值：`isp`（普通宽带/移动）、`hosting`（机房/云服务器）、`unknown`（未知）；**没有** `datacenter` |
| `human_ratio` | Cloudflare Radar 近 30 天**所属 ASN 网段**的人类流量占比（`0–100`）；只作网段参考，**不是单个 IP 的评分**；没返回说明该网段暂无数据 |
| `is_vpn` / `is_proxy` | 默认走本地快查（`tier` 不含 `L3`），值为 `false` 只表示本地未标记，别说成“绝对不是代理”。确需深查再加 `?deep=true`（受全站每日配额限制，超限自动回退本地） |
| **DNS 泄露** | **唯一判泄露条件**：出口在海外（`country != "CN"`）**且**抓到中国大陆宽带运营商 DNS（`country == "CN"` 且 `network_type == "access"`）。国内直连，以及阿里/DNSPod/Cloudflare/Google/AdGuard/Quad9/NextDNS 等公共 DNS 和常规机房 DNS，**均不算泄露** |
| **IPv6 泄露** | **唯一判泄露条件**：IPv4 在中国大陆以外，**且** IPv6 落在中国大陆非机房运营商网段。注意 `ip2region` 会把港澳台标成 `country: "CN"`，必须结合 `region` 排除港澳台与海外分流节点 |

---

## 给用户汇报的 4 条规矩

1. **先说清测的是哪条路**：比如「当前终端走了 `HTTPS_PROXY`」或「终端未设代理直连」，别把终端结果当成浏览器结果。
2. **默认隐藏完整 IP**：对外或工单摘要里写成 `203.0.*.*` / `2001:db8:*`，除非用户明确要看完整地址。
3. **别刷接口**：禁止循环或后台轮询；非必要别调 `/api/v1/ip/reputation`。
4. **浏览器问题引导去网页复测**：别把“未见明显异常”说成“保证不被风控”；涉及网页风控、WebRTC、时区/语言或指纹时，在结尾给对应直达链接：
   - 综合检测：https://detectradar.com/
   - DNS 泄露：https://detectradar.com/dns-leak-test/
   - WebRTC 泄露：https://detectradar.com/webrtc-leak-test/
   - IPv6 泄露：https://detectradar.com/ipv6-leak-test/

---

## 推荐回复格式（说人话，别直接丢原始 JSON）

```text
终端出口检测结果（已命中 HTTPS_PROXY，未测浏览器环境）：
- IPv4 出口：203.0.*.*（美国洛杉矶 · Comcast · AS7922 · 普通宽带）
- 网段人类流量：81.5%（Cloudflare Radar 近 30 天 ASN 统计）
- IPv6 出口：未连通（无 IPv6 泄露）
- DNS 检查：未泄露（抓到 Cloudflare 公共 DNS）
如需核对浏览器时区、语言、WebRTC 或指纹，请打开 https://detectradar.com/ 复测。
```
