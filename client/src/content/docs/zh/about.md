---
title: "检测说明：DNS、WebRTC 与 IPv6 泄露判断及隐私说明"
h1: "检测说明"
description: "说明 DetectRadar 检测哪些内容、DNS / WebRTC / IPv6 泄露判断口径、公开数据来源与项目背景、终端与 AI 助手用法及常见问题。"
canonical: "/about/"
locale: "zh-CN"
ogImage: "/og-about.png"
pageType: "article"
datePublished: "2026-10-23"
dateModified: "2026-11-26"
hasFaqs: true
---

打开 DetectRadar 就能查看你当前的出口 IP（普通宽带还是机房）、ASN 与所属网段人类流量占比、时区和语言对不对得上、有没有 DNS / WebRTC / IPv6 泄露，以及浏览器指纹是否正常。下面说明各项结果是怎么判断的。

## 检测哪些内容

- **出口 IP 与线路类型**：查看出口地区、运营商与 ASN，区分普通家庭或办公宽带（住宅 IP）与云服务器/机房 IP；同时展示 Cloudflare Radar 近 30 天所属 ASN 网段的人类流量占比（仅作网段背景参考，非单 IP 评分），并核对公开黑名单、常见代理开放端口及 Tor 出口节点。
- **时区与语言**：核对浏览器时区、常用语言是否与出口地区一致。页面显示的 CPU、内存等设备参数仅作参考，不计入异常。
- **网络泄露**：检查 DNS、WebRTC 和 IPv6 有没有绕开当前出口，走回本地宽带或其他线路。
- **浏览器指纹**：检查网页绘图与音频接口（Canvas / Audio）有没有被插件改写、是否存在自动化脚本标记，以及 WebGL 显卡型号与操作系统是否相符（只核对系统，不对照出口地区）。
- **严格模式 / 日常模式**：首页默认按「严格模式」（含时区、语言、机房 IP 与指纹 Hook）评分；切换到「日常模式」时，仅保留 DNS / WebRTC / IPv6 泄露及黑名单、已知代理、开放代理端口、高欺诈分与自动化等风险扣分，时区、语言、普通机房 IP 与指纹 Hook 只作展示不扣分。

<h2 id="dns">DNS 泄露怎么判断</h2>

[DNS 泄露检测](/dns-leak-test/)每次都会使用全新随机域名，避开浏览器和系统缓存，直接看是哪台 DNS 服务器在替你解析。

- **泄露**：测到了跨地区（中国大陆与港澳台分开算）接入网运营商的本地 DNS；或当前为机房/代理出口，却测到了不同 ASN 的同地区接入网运营商 DNS。
- **未泄露**：同地区直连接入网；或测到 Cloudflare、Google、Quad9、OpenDNS、NextDNS、Control D、AdGuard、Mullvad DNS、阿里 DNS、腾讯 DNSPod 等公共 DNS 与常规机房 DNS。
- **无 DNS 结果 / 无法判断**：请求被插件或网络拦截导致没有记录时显示「无 DNS 结果」；缺少归属信息或采样不完整时显示「无法判断」。

<h2 id="webrtc">WebRTC 泄露怎么判断</h2>

WebRTC 是浏览器自带的音视频直连功能，有时会绕开代理透出本机 IP。[WebRTC 泄露检测](/webrtc-leak-test/)会核对暴露出的公网与内网地址是否与当前出口同地区、同运营商（不留存记录）。

- **泄露**：暴露出跨地区（中国大陆与港澳台分开算）或跨运营商的公网 IP，或绕过代理漏出本机地址（包括代理只管了 IPv4 却漏出原生 IPv6）。
- **未泄露**：公网 IP 与出口一致；同运营商直连下的 IPv4/IPv6、共享出口或同网段临时 IPv6；仅看到局域网内网地址或浏览器隐藏真实 IP 后生成的 `*.local` 地址；或已关闭 WebRTC。
- **无法判断**：测到了公网 IP，但未能获取当前出口或归属信息进行比对。

<h2 id="ipv6">IPv6 泄露怎么判断</h2>

[IPv6 泄露检测](/ipv6-leak-test/)与首页综合检测都会同时测试纯 IPv4 与纯 IPv6 两条线路，分别查看实际出口。

- **泄露**：IPv6 经跨地区接入网运营商直连；或 IPv4 为机房/代理出口，而 IPv6 经不同 ASN 的本机接入网运营商直连。
- **未泄露**：只通 IPv4 或只通 IPv6；同地区直连双栈；或 IPv6 走机房/分流节点（如 WARP）。
- **无法判断**：IPv4 与 IPv6 两条线路均未连通。

<h2 id="boundaries">局限性与隐私保护</h2>

- 只检测你当前这一条网络连线，不支持输入任意第三方 IP 来查。
- 结果写着「未见明显异常」，只代表这次各项信息彼此对得上、没发现泄露，不保证目标网站不会风控；DetectRadar 只做检测，不提供代理或改指纹功能。
- Canvas、Audio、WebGL 的渲染结果和扩展列表都在浏览器本地算成无法还原原样的匿名哈希后再提交（不上传原始画面和插件明细，仅显卡型号原文核对系统）；分享文字与截图也在本地生成并自动隐藏完整 IP。详情见[隐私政策](/privacy/)和[服务条款](/terms/)。

<h2 id="sources">数据来源与项目背景</h2>

- **地理与 ASN 归属**：综合 IP2Location / DB-IP 离线库、ip2region（中国大陆省市与运营商）、PeeringDB 全球接入网 ASN、Team Cymru 实时 BGP 路由校准及 rDNS 反向解析。
- **公开信誉与端口**：核对 Spamhaus、SpamCop、Barracuda、DroneBL 公开黑名单，以及 Shodan InternetDB 开放端口与 Tor 官方出口列表。
- **人类流量占比**：来自 Cloudflare Radar 近 30 天 ASN 维度 HTTP 流量统计（经服务端 Redis 缓存 24 小时），仅反映所属网段整体构成。
- **DNS 与双栈实测**：基于自建权威 DNS（CoreDNS + DNSTap）与独立纯 IPv4 / 纯 IPv6 接入点（`4.detectradar.com` / `6.detectradar.com`）直接观测。
- **关于本站**：DetectRadar 是由独立开发者维护的开源网络诊断工具，全部源码以 MIT 协议公开在 <a href="https://github.com/harrisonwang/detect-radar" target="_blank" rel="noopener noreferrer">GitHub</a>。全站免费、免注册，不销售代理服务，也无弹窗广告。

<h2 id="cli">终端查询与 AI 助手</h2>

排查终端、Git 或 AI 编程工具走错代理时，可以直接用系统自带的 `curl` 和 `dig` 查看当前终端的出口 IP 与 DNS：

### 1. 查看终端出口（IPv4 / IPv6）

```bash
curl -s https://detectradar.com
curl -s https://4.detectradar.com
curl -s https://6.detectradar.com
```

命令行直接请求根路径会返回纯文本出口摘要（英文输出可访问 `https://detectradar.com/en/` 或加 `?lang=en`）；需要 JSON 结构化字段时可请求 `/api/v1/ip`（同样支持 `4.detectradar.com` 与 `6.detectradar.com`）。返回内容包含当前连接的地区、运营商、网络编号（ASN）和线路类型（`isp` 为普通宽带，`hosting` 为机房，`unknown` 为未知），响应不走客户端缓存，切换代理后再查会立刻看到新出口。默认仅查本地库；想进一步核查是否属于已知代理可附加 `?deep=true`（受全站每日配额限制，超限自动回退本地结果）。

### 2. 终端检测 DNS 出口

```bash
# 1. 创建一次性探测域名
curl -s -X POST https://detectradar.com/api/v1/leaks/dns \
  -H "Content-Type: application/json" -d '{}'

# 2. 用返回的 test_domain 触发一次真实解析
dig +short "r1.<test_domain>"

# 3. 等待 1–2 秒后，用返回的 id 取回实际捕获到的 DNS 服务器列表
curl -s "https://detectradar.com/api/v1/leaks/dns/<id>"
```

单测未绑定出口时 `assessment.state` 为 `unknown`，把查到的 DNS 列表和当前出口地区对照，或提交给 `POST /api/v1/leaks/dns/assessment` 即可拿到标准结论。

### 3. 让 AI 助手自动排查

把 <a href="/SKILL.md" class="break-all">https://detectradar.com/SKILL.md</a> 发给 Antigravity、Claude Code 或 Codex，就能自动跑完上面的检查。终端接口免鉴权并按 IP 限流（请勿高频轮询），且测不到浏览器 WebRTC、时区语言与指纹，排查网页环境请直接打开首页检测。
