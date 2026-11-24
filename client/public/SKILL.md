---
name: detectradar
description: 使用 DetectRadar 公开接口排查终端进程与代理的出口 IP（IPv4/IPv6）、住宅/机房属性与 DNS 泄露，并按既定事实边界向用户汇报或引导网页端复测。
---

# DetectRadar 终端自检技能（SKILL.md）

本技能供 Antigravity、Claude Code、Codex、OpenClaw 等具备终端执行能力的 AI Agent 直接读取使用，无需安装任何额外 CLI。所有接口免鉴权，仅检测当前发起请求的连接本身，不支持查询任意第三方 IP。

## 适用场景

- 排查「浏览器访问正常，但终端 / Git / Agent 请求直连、超频或出口不一致」；
- 核对终端 IPv4 与 IPv6 双栈出口是否走了同一节点（或出现 IPv6 绕过代理直连）；
- 核对当前终端 DNS 解析器是否泄露回中国大陆宽带运营商；
- 获取客观出口事实（归属地、ASN、机房/宽带属性），辅助定位分流或代理配置问题。

## 标准检查步骤

### 1. 检查终端代理环境变量

先确认当前 Shell 是否配置了代理变量（不要打印其中的账密，若包含用户名密码请先脱敏）：

```bash
env | grep -iE '^(http|https|all|no)_proxy=' || echo "(未设置代理环境变量)"
```

### 2. 查询终端出口 IP（默认、IPv4、IPv6、直连对照）

```bash
# 当前终端默认出口（遵循代理环境变量）
curl -sS https://detectradar.com/api/v1/ip

# 分别核对 IPv4 与 IPv6 双栈出口（也可使用 -4 / -6）
curl -sS https://4.detectradar.com/api/v1/ip
curl -sS --max-time 4 https://6.detectradar.com/api/v1/ip || echo "(无 IPv6 连通)"

# 如需对照「绕过环境变量直连」的出口，可加 --noproxy '*'
curl --noproxy '*' -sS https://detectradar.com/api/v1/ip
```

### 3. 终端 3 步检测 DNS 泄露（可选）

仅在需要排查终端/系统 DNS 是否泄露时执行，每次测试生成独立随机子域，不受历史 A/AAAA 缓存影响：

```bash
# 第 1 步：创建一次性 DNS 探测任务，取得 id 与 test_domain
resp=$(curl -sS -X POST https://detectradar.com/api/v1/leaks/dns \
  -H "Content-Type: application/json" -d '{}')
id=$(printf '%s' "$resp" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
domain=$(printf '%s' "$resp" | sed -n 's/.*"test_domain":"\([^"]*\)".*/\1/p')

# 第 2 步：用当前系统解析器触发真实 DNS 查询（等待 2 秒让权威 NS 完成回溯）
dig +short "r1.${domain}" >/dev/null && sleep 2

# 第 3 步：取回实际捕获到的 DNS 解析器列表
curl -sS "https://detectradar.com/api/v1/leaks/dns/${id}"
```

说明：未关联网页扫描直接调用 `POST /api/v1/leaks/dns` 时，第 3 步返回的 `dns_servers` 包含实测捕获到的解析器（`ip`、`country`、`isp`、`asn`、`network_type`），但 `assessment.state` 会为 `unknown`（缺少出口绑定）。请把 `dns_servers` 与第 2 节拿到的出口国家 `country` 对照以下口径判断，或将 `{ exit, resolvers }` 提交至 `POST https://detectradar.com/api/v1/leaks/dns/assessment` 获取标准判定。

## 字段解读与严格约束（必须遵守）

1. **必须声明观测路径**
   向用户汇报时，先明确说明本次观测的是哪条链路（例如：`终端默认（命中 HTTP_PROXY）` / `终端直连（未设代理变量）` / `仅测得 IPv4，无 IPv6 连通`），避免把终端结果误当作浏览器结果。
2. **准确理解 `GET /api/v1/ip` 的口径与默认值**
   - `usage_type` 仅有三个枚举值：`isp`（普通家庭/办公/移动等非机房网络）、`hosting`（云服务器/托管机房）、`unknown`（无法确认）；**不存在** `datacenter` 字段值。
   - 默认调用仅走本地 `L1+L2`（本地地理/ASN 库 + 云网段/BGP/rDNS）或复用服务端同 IP 的已有深扫缓存；响应头固定 `Cache-Control: no-store`，切换代理后不会读到旧 IP。
   - 当 `tier` 不含 `L3` 时，`is_vpn` / `is_proxy` 为 `false` 仅表示本地层未标记，**不能**据此断言「绝对不是商业代理或住宅代理」。仅在确有必要时附加 `?deep=true` 请求深度核查（受服务端每日全局配额保护，超限时自动回退 `L1+L2`）。
3. **DNS 泄露判定口径（Policy v2）**
   - **唯一判为「泄露」的条件**：出口 IP 位于海外（`country != "CN"`）**且**捕获到中国大陆宽带运营商接入网 DNS（`country == "CN"` 且 `network_type == "access"`）。
   - **明确不判泄露**：出口本身在中国大陆直连；阿里 DNS（223.5.5.5/223.6.6.6）、腾讯 DNSPod（119.29.29.29）、Cloudflare、Google、AdGuard、Quad9、OpenDNS、NextDNS 等公共 DNS（`network_type == "public"`）；常规海外机房 DNS（`network_type == "hosting"`）。
4. **明确能力边界，禁止夸大**
   - 终端命令**测不到**浏览器 WebRTC 泄露、浏览器时区/语言一致性、Canvas/Audio/WebGL 指纹及自动化控制标记。
   - 任何涉及网页账号风控、浏览器环境或完整泄露排查的问题，必须在回复末尾引导用户在浏览器打开对应页面复测：
     - 综合环境检测：https://detectradar.com/
     - DNS 泄露检测：https://detectradar.com/dns-leak-test/
     - WebRTC 泄露检测：https://detectradar.com/webrtc-leak-test/
     - IPv6 泄露检测：https://detectradar.com/ipv6-leak-test/
   - 不得把「未见明显异常」解读为「绝对安全」或「保证不被某平台风控」。
   - **IPv6 泄露判断说明**：由于 `ip2region` 会将香港、澳门、台湾标为 `country: "CN"`，对比双栈出口时须结合 `region` 排除港澳台与海外机房分流，仅当 IPv4 位于海外（非中国大陆）且 IPv6 落在国内非机房运营商网段时才属于 IPv6 旁路泄露。
5. **调用礼仪与隐私脱敏**
   - 禁止在循环或后台轮询中高频请求接口；非明确排障需要，不要批量调用 `/api/v1/ip/reputation`。
   - 输出给用户的公开摘要或工单记录默认隐藏完整 IP（IPv4 写为 `104.194.*.*`，IPv6 写为 `2408:8207:*`），除非用户明确要求查看完整地址。
