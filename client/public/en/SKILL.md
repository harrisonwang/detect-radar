---
name: detectradar
description: Run zero-signup terminal exit IP (IPv4/IPv6), residential vs. datacenter, 30-day ASN human traffic ratio, and DNS leak diagnostics via DetectRadar's free API, then summarize concisely in English.
---

# DetectRadar Terminal Self-Check Skill (`SKILL.md`)

Designed for AI coding agents with terminal access (Antigravity, Claude Code, Codex, OpenClaw, etc.). Zero installation, zero API keys required.
Chinese version: https://detectradar.com/SKILL.md

- **What this checks**: Current terminal exit IP (IPv4 / IPv6), region / ISP / ASN, residential vs. datacenter classification, Cloudflare Radar 30-day ASN human traffic ratio, and real recursive DNS resolvers.
- **What this cannot check**: Arbitrary third-party IP addresses, or browser-side WebRTC, timezone/language consistency, and browser fingerprints.

---

## Run Only What Is Needed

### 1. Inspect Proxy Environment Variables (Redact Credentials)

Check whether proxy variables are configured; always mask any `user:pass@` before printing:

```bash
env | grep -iE '^(http|https|all|no)_proxy=' | sed -E 's#://[^/@]+@#://***@#g' || echo "(no proxy env vars)"
```

### 2. Check Terminal Exit IP (Default + IPv4 / IPv6)

```bash
# Default terminal exit (honors proxy env vars)
curl -sS -m 5 "https://detectradar.com/api/v1/ip?lang=en"

# Dedicated IPv4-only and IPv6-only endpoints (detects IPv6 proxy bypass)
curl -sS -m 5 "https://4.detectradar.com/api/v1/ip?lang=en"
curl -sS -m 4 "https://6.detectradar.com/api/v1/ip?lang=en" || echo "(no IPv6 connectivity)"

# Optional: compare against direct connection bypassing proxy env vars
curl --noproxy '*' -sS -m 5 "https://detectradar.com/api/v1/ip?lang=en"
```

**Translate common curl errors into plain English:**
- `curl: (7) Failed to connect to 127.0.0.1...` → Proxy env var points to a local port that is closed or changed.
- `curl: (28) Operation timed out` → Connection timed out (blocked direct path or unreachable proxy node).
- `curl: (6) Could not resolve host` → Terminal DNS resolution failed.

### 3. Check Terminal DNS Leaks (Run Only When Diagnosing DNS)

Generates a fresh random subdomain every run to bypass local DNS caches:

```bash
resp=$(curl -sS -m 5 -X POST https://detectradar.com/api/v1/leaks/dns -H "Content-Type: application/json" -d '{}')
id=$(printf '%s' "$resp" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
domain=$(printf '%s' "$resp" | sed -n 's/.*"test_domain":"\([^"]*\)".*/\1/p')

dig +short "r1.${domain}" >/dev/null && sleep 2
curl -sS -m 5 "https://detectradar.com/api/v1/leaks/dns/${id}?lang=en"
```

Note: Standalone probes return `assessment.state: "unobserved"` if no queries arrive, or `"unknown"` when resolvers arrive without a bound exit. Compare `dns_servers` against your exit IP from Step 2, or POST `{ exit: { ip, country, network, asn, type }, resolvers, observed_rounds: 1, total_rounds: 1 }` to `https://detectradar.com/api/v1/leaks/dns/assessment` for the canonical verdict (`exit.network` from `isp`/`org`, `exit.type`: `"access"` | `"hosting"` | `"proxy"`, `resolvers` from `dns_servers`).

---

## Field Contracts & Leak Rules (Strict Facts Only)

| Field / Check | Canonical Definition |
| --- | --- |
| `usage_type` | Strictly one of three values: `isp` (residential/mobile), `hosting` (cloud/datacenter), `unknown` — **never** `datacenter` |
| `human_ratio` | Cloudflare Radar 30-day **ASN-level** HTTP human traffic percentage (`0–100`); network background context only, **not** an individual IP score; omitted when unavailable |
| `is_vpn` / `is_proxy` | Default requests use fast local `L1+L2` (or reuse an existing 24h deep-scan cache entry for the same IP); `false` only means not flagged locally and does not guarantee a non-proxy IP. Append `&deep=true` only when needed (subject to global daily quota with automatic local fallback) |
| **DNS Leak** | **Policy v3**: Flagged as a leak when a local ISP resolver (`network_type == "access"`) is observed in a different region than the exit IP (Mainland China, HK, MO, TW treated as distinct regions), **or** when a datacenter/proxy exit leaks to a local ISP resolver on a different ASN. Same-region direct ISP traffic, public DNS (Cloudflare, Google, Quad9, OpenDNS, NextDNS, Control D, AdGuard, Mullvad DNS, AliDNS, DNSPod, etc.), and datacenter resolvers are **not** leaks |
| **IPv6 Leak** | Flagged as a leak when IPv6 connects directly through an ISP in a different region than IPv4 (Mainland China, HK, MO, TW treated as distinct regions), **or** when IPv4 exits via a datacenter/proxy while IPv6 connects directly via a local ISP on a different ASN. IPv4-only, IPv6-only, same-region direct dual-stack, and datacenter/WARP split routing are **not** leaks |

---

## Reporting Rules

1. **State the observed path first**: Clarify whether the terminal used `HTTPS_PROXY` or connected directly, and never confuse terminal routing with browser routing.
2. **Mask full IPs by default**: Write `203.0.*.*` or `2001:db8:*` unless the user explicitly asks for the full address.
3. **Respect rate limits**: Never poll in a loop; avoid calling `/api/v1/ip/reputation` unless needed.
4. **Point browser issues to the web app**: Never claim "No obvious issues" guarantees passing third-party fraud checks. For WebRTC, browser timezone/language, or fingerprints, link directly:
   - Full Environment Check: https://detectradar.com/en/
   - DNS Leak Test: https://detectradar.com/en/dns-leak-test/
   - WebRTC Leak Test: https://detectradar.com/en/webrtc-leak-test/
   - IPv6 Leak Test: https://detectradar.com/en/ipv6-leak-test/

---

## Recommended Summary Format

```text
Terminal egress check (via HTTPS_PROXY; browser signals not tested):
- IPv4 exit: 203.0.*.* (Los Angeles, United States · Comcast · AS7922 · Residential / ISP)
- ASN human traffic: 81.5% (Cloudflare Radar 30-day ASN ratio)
- IPv6 exit: Unreachable (no IPv6 leak)
- DNS check: No leak (Cloudflare Public DNS observed)
To verify browser timezone, language, WebRTC, or fingerprint consistency, open https://detectradar.com/en/.
```
