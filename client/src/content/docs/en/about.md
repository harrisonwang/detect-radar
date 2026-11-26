---
title: "How It Works: DNS, WebRTC & IPv6 Leak Methodology"
h1: "How DetectRadar Works"
description: "How DetectRadar inspects your exit IP, browser consistency, DNS/WebRTC/IPv6 leak rules, public data sources, CLI usage, and privacy boundaries."
canonical: "/en/about/"
locale: "en"
ogImage: "/og-about-en.png"
pageType: "article"
datePublished: "2026-10-23"
dateModified: "2026-11-26"
hasFaqs: true
---

Open DetectRadar to inspect your current exit IP (residential ISP vs. datacenter), ASN and 30-day ASN human traffic ratio, timezone and language consistency, DNS / WebRTC / IPv6 leaks, and browser fingerprint integrity. Below is the exact methodology behind each check.

## What Is Checked

- **Exit IP & Network Type**: Identifies geolocation, operator, and ASN, distinguishing consumer residential/mobile ISPs from cloud and hosting providers. Also shows Cloudflare Radar 30-day ASN-level human traffic ratio (network background context, not an individual IP score), public blocklists, open proxy ports, and Tor exit lists.
- **Timezone & Language**: Checks whether your browser timezone and preferred language align with the exit IP region. Hardware specs (CPU cores, RAM, touch points) are shown for context only and are never penalized.
- **Network Leaks**: Tests whether DNS lookups, WebRTC ICE candidates, or IPv6 traffic bypass your current exit.
- **Browser Fingerprint**: Checks whether native Canvas or Audio APIs appear overridden (hooked), checks `navigator.webdriver`, and verifies that the WebGL GPU renderer matches your declared OS (checked against OS only, never exit country).
- **Strict vs. Daily Mode**: Strict mode evaluates all signals including timezone, language, datacenter IPs, and fingerprint hooks. Daily mode focuses on leaks and high-risk IP flags (blocklists, known proxies, open proxy ports, high fraud score, automation), showing timezone, language, clean datacenter IPs, and hooks as informational only.

<h2 id="dns">How DNS Leak Detection Works</h2>

The [DNS Leak Test](/en/dns-leak-test/) generates fresh random subdomains on every run to bypass local DNS caches and capture the actual recursive resolvers reaching our authoritative nameserver.

- **Leak detected**: Observed access-ISP DNS resolvers in a different region than your exit IP, or local access-ISP resolvers on a different ASN bypassing a datacenter/proxy exit.
- **No leak**: Same-region direct ISP traffic, or public DNS resolvers (Cloudflare, Google, Quad9, OpenDNS, NextDNS, Control D, AdGuard, Mullvad DNS, AliDNS, DNSPod, etc.) and datacenter resolvers.
- **No DNS result / Inconclusive**: Shown as "No DNS result" if blockers or timeouts prevent queries from arriving, or "Inconclusive" when exit or resolver attribution is incomplete.

<h2 id="webrtc">How WebRTC Leak Detection Works</h2>

WebRTC peer-to-peer connectivity can bypass browser proxies and expose real public or LAN IP addresses. The [WebRTC Leak Test](/en/webrtc-leak-test/) compares gathered ICE candidates against your current exit IP (without retaining candidate IPs).

- **Leak detected**: Public ICE candidates belonging to another region or another operator, or bypassing a proxy/datacenter exit (including native IPv6 leaking around an IPv4-only tunnel).
- **No leak**: Public IP matching your exit, same-ISP dual-stack IPv4/IPv6, same `/64` IPv6 privacy prefix, shared CGNAT exits on the same ISP, LAN-only or mDNS (`*.local`) masked addresses, or WebRTC disabled.
- **Inconclusive**: Public candidates were observed, but your exit IP or attribution data could not be retrieved for comparison.

<h2 id="ipv6">How IPv6 Leak Detection Works</h2>

Both the [IPv6 Leak Test](/en/ipv6-leak-test/) and the home diagnostic probe dedicated IPv4-only (`4.detectradar.com`) and IPv6-only (`6.detectradar.com`) endpoints concurrently, without depending on WebRTC.

- **Leak detected**: IPv6 connects directly via a cross-region access ISP, or IPv4 uses a datacenter/proxy exit while IPv6 bypasses it via a local access ISP on a different ASN.
- **No leak**: IPv4-only or IPv6-only connectivity, same-region direct dual-stack, or hosting/WARP split routing.
- **Inconclusive**: Neither IPv4 nor IPv6 probe endpoints were reachable.

<h2 id="boundaries">Scope & Privacy</h2>

- DetectRadar only inspects your current connection and does not provide arbitrary third-party IP lookup.
- "No obvious issues" means no contradictions or leaks were observed during this run; it is not a guarantee against third-party anti-bot or risk systems. DetectRadar is a diagnostic tool only and does not provide proxy or fingerprint spoofing features.
- Canvas, Audio, and WebGL rendering outputs are hashed locally in your browser into irreversible hashes before submission (only the WebGL renderer string is checked against your declared OS). Shareable text summaries and PNG cards are generated locally with full IP addresses masked automatically. See our [Privacy Policy](/en/privacy/) and [Terms of Service](/en/terms/).

<h2 id="sources">Data Sources & Open Source</h2>

- **Geolocation & ASN**: Combines offline IP2Location / DB-IP databases, ip2region (Mainland China), PeeringDB global access-ISP ASNs, Team Cymru live BGP lookups, and rDNS heuristics.
- **Public Reputation & Ports**: Checks Spamhaus, SpamCop, Barracuda, and DroneBL public DNSBLs, Shodan InternetDB open ports, and the official Tor exit list.
- **Human Traffic Ratio**: Powered by Cloudflare Radar 30-day ASN HTTP traffic statistics (cached for 24 hours in Redis), reflecting overall ASN composition rather than an individual IP.
- **Live DNS & Dual-Stack Probes**: Operated on self-hosted CoreDNS + DNSTap authoritative nameservers and dedicated single-stack endpoints (`4.detectradar.com` / `6.detectradar.com`).
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
curl -s "https://detectradar.com/api/v1/leaks/dns/<id>"
```

Unbound probes return `assessment.state: "unknown"`; compare `dns_servers` against your exit IP or submit `{ exit, resolvers }` to `POST /api/v1/leaks/dns/assessment`.

### 3. Let an AI Agent Run the Check

Share <a href="/en/SKILL.md" class="break-all">https://detectradar.com/en/SKILL.md</a> with Antigravity, Claude Code, or Codex to automate terminal egress, IPv6, and DNS checks. CLI endpoints are unauthenticated and rate-limited per IP (avoid polling) and cannot observe browser WebRTC, timezone/language, or fingerprints — use the web app for browser checks.
