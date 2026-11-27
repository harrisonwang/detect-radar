---
title: "How It Works: DNS, WebRTC & IPv6 Leak Methodology"
h1: "How DetectRadar Works"
description: "Learn how DetectRadar checks exit IP reputation, timezone and language consistency, DNS/WebRTC/IPv6 leaks, public data sources, CLI usage, and privacy limits."
canonical: "/en/about/"
locale: "en"
ogImage: "/og-about-en.png"
pageType: "article"
datePublished: "2026-10-23"
dateModified: "2026-11-27"
hasFaqs: true
---

Open DetectRadar to inspect your current exit IP (residential ISP vs. datacenter), ASN and 30-day ASN human traffic ratio, timezone and language consistency, DNS / WebRTC / IPv6 leaks, and browser fingerprint integrity. Below is the exact methodology behind each check.

## What Is Checked

- **Exit IP & Network Type**: Identifies geolocation, ISP/organization, and ASN, distinguishing consumer residential/mobile ISPs from cloud and hosting providers. Also shows Cloudflare Radar 30-day ASN-level human traffic ratio (network background context, not an individual IP score), public blocklists, open proxy ports, and Tor exit lists.
- **Connection Characteristics (Beta)**: Compares same-window HTTP RTT against server-side TCP RTT (RTT delta) and inspects peer TCP MSS via read-only server kernel socket queries to estimate path MTU (`MSS+40` for IPv4, `MSS+60` for IPv6). Outputs non-mutually-exclusive labels (`No Obvious Anomalies`, `PPPoE Characteristics`, `Low MTU Link`, `Suspected Tunneling`, `Suspected Proxy Relay`, `Inconclusive`) for informational reference only; excluded from scoring.
- **Timezone & Language**: Checks whether your browser timezone and preferred language align with the exit IP region. Hardware specs (CPU cores, RAM, touch points) are shown for context only and are never penalized.
- **Network Leaks**: Tests whether DNS lookups, WebRTC ICE candidates, or IPv6 traffic bypass your current exit.
- **Browser Fingerprint**: Checks whether native Canvas or Audio APIs appear overridden (hooked), checks `navigator.webdriver`, and verifies that the WebGL GPU renderer matches your declared OS (checked against OS only, never exit country).
- **Strict vs. Daily Mode**: Strict mode evaluates all signals including timezone, language, datacenter IPs, and fingerprint hooks. Daily mode focuses on leaks and high-risk IP flags (blocklists, known proxies, open proxy ports, high fraud score, WebGL OS mismatch, automation), showing timezone, language, clean datacenter IPs, and hooks as informational only.

<h2 id="dns">How DNS Leak Detection Works</h2>

The [DNS Leak Test](/en/dns-leak-test/) generates fresh random subdomains on every run to bypass local DNS caches and capture the actual recursive resolvers reaching our authoritative nameserver.

- **Leak detected**: Observed local ISP DNS resolvers in a different region than your exit IP (treating Mainland China, Hong Kong, Macao, and Taiwan distinctly), or local ISP resolvers on a different ASN bypassing a datacenter/proxy exit.
- **No leak**: Same-region direct ISP traffic, or public DNS resolvers (Cloudflare, Google, Quad9, OpenDNS, NextDNS, Control D, AdGuard, Mullvad DNS, AliDNS, DNSPod, etc.) and datacenter resolvers.
- **No DNS result / Inconclusive**: Shown as "No DNS result" if blockers or timeouts prevent queries from arriving, or "Inconclusive" when exit or resolver network data is incomplete.

<h2 id="webrtc">How WebRTC Leak Detection Works</h2>

WebRTC peer-to-peer connectivity can bypass browser proxies and expose real public or LAN IP addresses. The [WebRTC Leak Test](/en/webrtc-leak-test/) gathers browser ICE candidates via our self-hosted same-domain RFC 5389 UDP STUN server (`stun:detectradar.com:3478`) — avoiding third-party Google STUN dependencies, regional blockage, and split-tunnel routing mismatches — and compares exposed public IPs against your current exit IP without retaining candidate IPs.

- **Leak detected**: Public ICE candidates belonging to another region or another ISP/network, or bypassing a proxy/datacenter exit (including native IPv6 leaking around an IPv4-only tunnel). When the page uses a direct residential/mobile ISP exit while WebRTC routes through a proxy or datacenter node, it is also flagged as **Leak detected** with a dedicated **reverse split-tunnel** explanation (distinguished from classic `public-leak`).
- **No leak**: Public IP matching your exit, same-ISP dual-stack IPv4/IPv6, same `/64` IPv6 privacy prefix, shared CGNAT exits on the same ISP, LAN-only or mDNS (`*.local`) masked addresses, or WebRTC disabled.
- **Inconclusive**: Public candidates were observed, but your exit IP could not be retrieved for comparison.

<h2 id="ipv6">How IPv6 Leak Detection Works</h2>

Both the [IPv6 Leak Test](/en/ipv6-leak-test/) and the home diagnostic probe dedicated IPv4-only (`4.detectradar.com`) and IPv6-only (`6.detectradar.com`) endpoints concurrently, without depending on WebRTC.

- **Leak detected**: IPv6 connects directly via a cross-region ISP, or IPv4 uses a datacenter/proxy exit while IPv6 bypasses it via a local ISP on a different ASN.
- **No leak**: IPv4-only or IPv6-only connectivity, same-region direct dual-stack, or datacenter/WARP split routing.
- **Inconclusive**: Neither IPv4 nor IPv6 probe endpoints were reachable, or IPv6 exited in another region/alongside a proxy with an unclassified network type.

<h2 id="connection">How Connection Characteristics Work (Beta)</h2>

The bottom strip of the home Network Identity card observes two independent transport signals on your live connection: same-window **RTT Delta** (HTTP RTT minus server-side TCP RTT) and **Estimated MTU** derived from peer TCP MSS (`MSS+40` for IPv4, `MSS+60` for IPv6) via read-only server kernel queries (`NETLINK_SOCK_DIAG`). Because both signals are orthogonal, output tags are **non-mutually-exclusive** (for example, a residential PPPoE proxy or a proxy node using a WireGuard exit can match two tags simultaneously):

- **No Obvious Anomalies**: Both RTT delta and estimated MTU were observed without triggering encapsulation or relay signals (connections classified as mobile cellular or clamped at the probe server's own link MTU ceiling do not trigger false tunneling alerts).
- **PPPoE Characteristics**: Unclamped estimated MTU equals `1492` (IPv4 MSS `1452` / IPv6 MSS `1432`), typical of PPPoE broadband access.
- **Low MTU Link**: Non-hosting, non-mobile-classified ISP exit without proxy RTT delta whose estimated MTU is below `1492`, common on cellular links, low-MTU broadband, or WireGuard/IPsec tunnels.
- **Suspected Tunneling**: Hosting exit with estimated MTU below `1492`, or low MTU combined with proxy RTT delta (e.g. WireGuard, WARP, IPsec, or OpenVPN).
- **Suspected Proxy Relay**: Same-window RTT delta reaches `max(30 ms, server TCP RTT × 20%)`, suggesting the TCP connection is terminated and relayed at a proxy node.
- **Inconclusive**: Shown when either RTT delta or estimated MTU could not be observed and no positive characteristic matched, separating missing evidence from a clean result.

Caveats: MTU `1492` is not exclusive to PPPoE, and PPPoE itself can support `1500` MTU; when a probe server's own interface MTU is below `1500` (e.g. `1460` on some cloud VPCs), readings hitting that ceiling are marked clamped (`mtu_clamped`) and `1492` PPPoE vs. `1500` Ethernet cannot be distinguished; unclassified cellular connections may still trigger the neutral "Low MTU Link" badge; the current implementation inspects RTT delta and MSS/MTU only and does not claim OS TCP fingerprinting; and "No Obvious Anomalies" does not prove a direct connection or absence of a proxy. All Connection Characteristics readings and tags are Beta informational signals only and are excluded from risk scoring.

<h2 id="boundaries">Scope & Privacy</h2>

- DetectRadar only inspects your current connection and does not provide arbitrary third-party IP lookup.
- "No obvious issues" means no contradictions or leaks were observed during this run; it is not a guarantee against third-party anti-bot or risk systems. DetectRadar is a diagnostic tool only and does not provide proxy or fingerprint spoofing features.
- Canvas rendering output is hashed locally in your browser into an irreversible hash before submission (raw pixels are never uploaded); Audio checks inspect only whether native methods are overridden, and WebGL submits only the GPU renderer string to verify consistency with your declared OS. Shareable text summaries and PNG cards are generated locally with full IP addresses masked automatically. See our [Privacy Policy](/en/privacy/) and [Terms of Service](/en/terms/).

<h2 id="sources">Data Sources & Open Source</h2>

- **Geolocation & ASN**: Combines offline IP2Location / DB-IP databases, ip2region (Mainland China), PeeringDB global ISP ASNs, Team Cymru live BGP lookups, and rDNS heuristics.
- **Public Reputation & Ports**: Checks Spamhaus, SpamCop, Barracuda, and DroneBL public DNSBLs, Shodan InternetDB open ports, and the official Tor exit list.
- **Human Traffic Ratio**: Powered by Cloudflare Radar 30-day ASN HTTP traffic statistics (cached for 24 hours in Redis), reflecting overall ASN composition rather than an individual IP.
- **Live DNS, STUN & Dual-Stack Probes**: Operated on self-hosted CoreDNS + DNSTap authoritative nameservers, a same-domain RFC 5389 UDP STUN server (`3478/udp`), and dedicated single-stack endpoints (`4.detectradar.com` / `6.detectradar.com`).
- **Open Source**: DetectRadar is an independent, free, zero-signup project published under the MIT license on <a href="https://github.com/harrisonwang/detect-radar" target="_blank" rel="noopener noreferrer">GitHub</a>.

<h2 id="cli">CLI & AI Agent Integration</h2>

When debugging proxy routing in a terminal, Git, or an AI coding assistant, query DetectRadar directly with `curl` and `dig`:

### 1. Check Terminal Exit IP (IPv4 / IPv6)

```bash
curl -s https://detectradar.com/en/
curl -s "https://4.detectradar.com/?lang=en"
curl -s "https://6.detectradar.com/?lang=en"
```

CLI requests return a concise plain-text summary. For structured JSON, request `/api/v1/ip?lang=en` (also available on `4.detectradar.com` and `6.detectradar.com`). `usage_type` is strictly `isp`, `hosting`, or `unknown`, with `Cache-Control: no-store`. Default requests use fast local databases; append `&deep=true` for deep proxy checks (subject to a global daily quota with automatic local fallback).

### 2. Test Terminal DNS Resolvers

```bash
# 1. Create a one-time probe subdomain
curl -s -X POST https://detectradar.com/api/v1/leaks/dns \
  -H "Content-Type: application/json" -d '{}'

# 2. Trigger a live lookup against test_domain
dig +short "r1.<test_domain>"

# 3. Wait 1–2s and fetch observed recursive resolvers
curl -s "https://detectradar.com/api/v1/leaks/dns/<id>?lang=en"
```

Standalone probes (not bound to a `scan_id`) return `assessment.state: "unobserved"` when no queries arrive, or `"unknown"` when resolvers are observed without a bound exit; compare `dns_servers` against your exit IP or submit `{ exit: { ip, country, network, asn, type }, resolvers, observed_rounds: 1, total_rounds: 1 }` to `POST /api/v1/leaks/dns/assessment` (`exit.network` from `/api/v1/ip` `isp`/`org`, `exit.type`: `"access"` | `"hosting"` | `"proxy"`, `resolvers` from `dns_servers`).

### 3. Let an AI Agent Run the Check

Share <span class="break-all">[https://detectradar.com/en/SKILL.md](/en/SKILL.md)</span> with Antigravity, Claude Code, or Codex to automate terminal egress, IPv6, and DNS checks. CLI endpoints are unauthenticated and rate-limited per IP (avoid polling). They cannot observe browser WebRTC, timezone/language, or fingerprints — use the web app for browser checks.
