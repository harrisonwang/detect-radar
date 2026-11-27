import Foundation
import Testing
@testable import DetectRadarCore

@Test
func shortProviderAndDisplayFormatting() {
    let provider = IPIntel(
        ip: "203.0.113.42", country: "US", city: "Los Angeles",
        org: "Example Cloud Services", isp: "Example Cloud Services",
        usageType: "hosting", isHosting: true
    )
    #expect(IPMasker.shortProviderName(for: provider) == "Example")
    #expect(IPMasker.countryBadgeLine(for: provider) == "🇺🇸 美国")
    #expect(IPMasker.providerDetailLine(for: provider) == "Example · Los Angeles")
    #expect(IPMasker.usageShortLabel(for: provider) == "机房")

    let cf = IPIntel(ip: "1.1.1.1", country: "US", isp: "Cloudflare, Inc.", usageType: "hosting", isHosting: true)
    #expect(IPMasker.shortProviderName(for: cf) == "Cloudflare")
    #expect(IPMasker.providerDetailLine(for: cf) == "Cloudflare")

    let telecom = IPIntel(ip: "2001:db8:390::1", country: "CN", region: "上海市", city: "上海", isp: "中国电信", usageType: "isp")
    #expect(IPMasker.shortProviderName(for: telecom) == "中国电信")
    #expect(IPMasker.countryBadgeLine(for: telecom) == "🇨🇳 中国大陆")
    #expect(IPMasker.providerDetailLine(for: telecom) == "中国电信 · 上海")
    #expect(IPMasker.usageShortLabel(for: telecom) == "宽带")

    let sg = IPIntel(ip: "198.51.100.42", country: "SG", city: "Singapore", isp: "Vultr Holdings LLC", usageType: "hosting", isHosting: true)
    #expect(IPMasker.shortProviderName(for: sg) == "Vultr Holdings")
    #expect(IPMasker.countryBadgeLine(for: sg) == "🇸🇬 新加坡")
    #expect(IPMasker.providerDetailLine(for: sg) == "Vultr Holdings")
}

@Test
func hkMoTwExcludedFromMainlandCNAndIPv6LeakRules() {
    let usHosting = IPIntel(ip: "203.0.113.42", country: "US", usageType: "hosting", isHosting: true)
    let cnBroadbandV6 = IPIntel(ip: "2001:db8:390::1", country: "CN", region: "上海市", isp: "中国电信", usageType: "isp", isHosting: false)
    let hkHostingV6 = IPIntel(ip: "2001:db8:852::1", country: "CN", countryName: "香港", region: "香港特别行政区", city: "香港", usageType: "hosting", isHosting: true)
    let cnBroadbandV4 = IPIntel(ip: "192.0.2.10", country: "CN", region: "北京市", isp: "中国移动", usageType: "isp", isHosting: false)

    #expect(IPMasker.isMainlandChina(cnBroadbandV4) == true)
    #expect(IPMasker.isMainlandChina(hkHostingV6) == false)
    #expect(IPMasker.effectiveCountryCode(hkHostingV6) == "HK")
    #expect(IPMasker.countryBadgeLine(for: hkHostingV6) == "🇭🇰 中国香港")
    #expect(IPMasker.providerDetailLine(for: hkHostingV6) == "未知网络")

    let leaked = DualStackEvaluator.evaluateIPv6Leak(activeV4: usHosting, directV6: cnBroadbandV6)
    #expect(leaked == .leaked(carrier: "中国电信", ip: "2001:db8:390::1"))

    #expect(DualStackEvaluator.evaluateIPv6Leak(activeV4: usHosting, directV6: hkHostingV6) == .none)

    #expect(DualStackEvaluator.evaluateIPv6Leak(activeV4: cnBroadbandV4, directV6: cnBroadbandV6) == .none)

    #expect(DualStackEvaluator.evaluateIPv6Leak(activeV4: nil, directV6: cnBroadbandV6) == .none)
}

@Test
func menuBarTitleOnlyWarnsOnIPv6Leak() {
    let usHosting = IPIntel(ip: "203.0.113.42", country: "US", usageType: "hosting", isHosting: true)
    let cnV6 = IPIntel(ip: "2001:db8:390::1", country: "CN", region: "上海市", isp: "中国电信", usageType: "isp")

    let cleanSnap = EgressSnapshot(
        timestamp: Date(),
        activeV4: usHosting,
        directV6: nil,
        systemProxy: ProxyEndpoint(scheme: .http, host: "127.0.0.1", port: 7890),
        ipv6Leak: .none
    )
    #expect(IPMasker.menuBarTitle(snapshot: cleanSnap, isLoading: false) == "🇺🇸 机房")

    let leakSnap = EgressSnapshot(
        timestamp: Date(),
        activeV4: usHosting,
        directV6: cnV6,
        systemProxy: nil,
        ipv6Leak: .leaked(carrier: "中国电信", ip: "2001:db8:390::1")
    )
    #expect(IPMasker.menuBarTitle(snapshot: leakSnap, isLoading: false) == "🇺🇸 机房 ⚠️")
}

@Test
func systemProxyParsingAndLocalIPv6() {
    let sysDict: [String: Any] = [
        "HTTPEnable": 1,
        "HTTPProxy": "127.0.0.1",
        "HTTPPort": 7890,
        "HTTPSEnable": 1,
        "HTTPSProxy": "127.0.0.1",
        "HTTPSPort": 7890,
        "SOCKSEnable": 1,
        "SOCKSProxy": "127.0.0.1",
        "SOCKSPort": 7891,
    ]
    let ep = SystemProxyReader.parseSystemProxyDict(sysDict)
    #expect(ep?.port == 7890)
    #expect(ep?.scheme == .http)

    #expect(LocalIPv6Detector.isGlobalUnicastIPv6([0x24, 0x0e, 0x03, 0x90, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1]) == true)
    #expect(LocalIPv6Detector.isGlobalUnicastIPv6([0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1]) == false)
    #expect(LocalIPv6Detector.isGlobalUnicastIPv6([0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1]) == false)
}

@Test
func appLanguageResolutionAndEnglishFormatting() {
    #expect(AppLanguage.resolve(override: "en", preferredLanguages: ["zh-Hans-CN"]) == .en)
    #expect(AppLanguage.resolve(override: "zh-CN", preferredLanguages: ["en-US"]) == .zh)
    #expect(AppLanguage.resolve(override: nil, preferredLanguages: ["en-US"]) == .en)
    #expect(AppLanguage.resolve(override: nil, preferredLanguages: ["ja-JP"]) == .en)
    #expect(AppLanguage.resolve(override: nil, preferredLanguages: ["zh-Hans-CN"]) == .zh)
    #expect(AppLanguage.resolve(override: nil, preferredLanguages: ["zh-Hant-TW"]) == .zh)
    #expect(AppLanguage.resolve(override: nil, preferredLanguages: []) == .zh)

    let hosting = IPIntel(
        ip: "203.0.113.42", country: "US", countryName: "United States",
        city: "Los Angeles", asn: "AS14061", isp: "DigitalOcean",
        usageType: "hosting", isHosting: true
    )
    let isp = IPIntel(
        ip: "198.51.100.10", country: "US", countryName: "United States",
        asn: "AS7922", isp: "Comcast", usageType: "isp"
    )
    let mobile = IPIntel(
        ip: "198.51.100.20", country: "JP", countryName: "Japan",
        asn: "AS2516", isp: "KDDI", usageType: "isp", isMobile: true
    )
    let hk = IPIntel(
        ip: "2001:db8:852::1", country: "CN", countryName: "香港",
        region: "香港特别行政区", city: "香港", usageType: "isp"
    )
    let cn = IPIntel(
        ip: "192.0.2.10", country: "CN", region: "北京市",
        isp: "China Mobile", usageType: "isp"
    )
    let unknown = IPIntel(ip: "192.0.2.99")

    #expect(IPMasker.countryBadgeLine(for: hosting, language: .en) == "🇺🇸 US")
    #expect(IPMasker.regionDisplayName(for: hosting, language: .en) == "United States")
    #expect(IPMasker.regionDisplayName(for: hk, language: .en) == "Hong Kong")
    #expect(IPMasker.regionDisplayName(for: cn, language: .en) == "Mainland China")
    #expect(IPMasker.usageShortLabel(for: hosting, language: .en) == "DC")
    #expect(IPMasker.usageShortLabel(for: isp, language: .en) == "ISP")
    #expect(IPMasker.usageShortLabel(for: mobile, language: .en) == "ISP")
    #expect(IPMasker.usageShortLabel(for: mobile, language: .zh) == "宽带")
    #expect(IPMasker.usageShortLabel(for: unknown, language: .en) == "Unknown")
    #expect(IPMasker.usageBadgeLabel(for: hosting, language: .en) == "Datacenter")
    #expect(IPMasker.usageBadgeLabel(for: isp, language: .en) == "ISP")
    #expect(IPMasker.usageBadgeLabel(for: mobile, language: .en) == "ISP")
    #expect(IPMasker.usageBadgeLabel(for: mobile, language: .zh) == "宽带")
    #expect(IPMasker.usageBadgeLabel(for: unknown, language: .en) == "Unknown")

    let cleanSnap = EgressSnapshot(
        timestamp: Date(), activeV4: hosting, directV6: nil, systemProxy: nil, ipv6Leak: .none
    )
    let leakSnap = EgressSnapshot(
        timestamp: Date(), activeV4: hosting, directV6: isp, systemProxy: nil,
        ipv6Leak: .leaked(carrier: "Comcast", ip: isp.ip)
    )
    let ispSnap = EgressSnapshot(
        timestamp: Date(), activeV4: isp, directV6: nil, systemProxy: nil, ipv6Leak: .none
    )
    let offlineSnap = EgressSnapshot(
        timestamp: Date(), activeV4: nil, directV6: nil, systemProxy: nil, ipv6Leak: .none
    )
    #expect(IPMasker.menuBarTitle(snapshot: cleanSnap, isLoading: false, language: .en) == "🇺🇸 DC")
    #expect(IPMasker.menuBarTitle(snapshot: leakSnap, isLoading: false, language: .en) == "🇺🇸 DC ⚠️")
    #expect(IPMasker.menuBarTitle(snapshot: ispSnap, isLoading: false, language: .en) == "🇺🇸 ISP")
    #expect(IPMasker.menuBarTitle(snapshot: nil, isLoading: true, language: .en) == "◎ Checking")
    #expect(IPMasker.menuBarTitle(snapshot: offlineSnap, isLoading: false, language: .en) == "⚠️ Offline")
}

@Test
func globalIPv6LeakRulesAndEndpoints() {
    let usHosting = IPIntel(
        ip: "203.0.113.42", country: "US", asn: "AS14061",
        isp: "DigitalOcean", usageType: "hosting", isHosting: true
    )
    let deISP = IPIntel(
        ip: "2001:db8:49::1", country: "DE", asn: "AS3320",
        isp: "Deutsche Telekom AG", usageType: "isp", isHosting: false
    )
    let usISP = IPIntel(
        ip: "2001:db8:7922::1", country: "US", asn: "AS7922",
        isp: "Comcast", usageType: "isp", isHosting: false
    )
    let usISP2 = IPIntel(
        ip: "198.51.100.10", country: "US", asn: "AS7922",
        isp: "Comcast", usageType: "isp", isHosting: false
    )
    let deHosting = IPIntel(
        ip: "2001:db8:49::2", country: "DE", asn: "AS24940",
        isp: "Hetzner", usageType: "hosting", isHosting: true
    )
    let unknownGeoISP = IPIntel(
        ip: "2001:db8:99::1", country: "", asn: "AS7922",
        isp: "Comcast", usageType: "isp", isHosting: false
    )

    #expect(
        DualStackEvaluator.evaluateIPv6Leak(activeV4: usHosting, directV6: deISP, language: .en)
            == .leaked(carrier: "Deutsche Telekom AG", ip: "2001:db8:49::1")
    )
    #expect(
        DualStackEvaluator.evaluateIPv6Leak(activeV4: usHosting, directV6: usISP, language: .en)
            == .leaked(carrier: "Comcast", ip: "2001:db8:7922::1")
    )
    #expect(DualStackEvaluator.evaluateIPv6Leak(activeV4: usISP2, directV6: usISP, language: .en) == .none)
    #expect(DualStackEvaluator.evaluateIPv6Leak(activeV4: usHosting, directV6: deHosting, language: .en) == .none)
    #expect(
        DualStackEvaluator.evaluateIPv6Leak(activeV4: usHosting, directV6: unknownGeoISP, language: .en)
            == .leaked(carrier: "Comcast", ip: "2001:db8:99::1")
    )

    let zhEp = ProbeEndpoints(baseDomain: "detectradar.com", language: .zh)
    #expect(zhEp.v4IPURL.absoluteString == "https://4.detectradar.com/api/v1/ip?lang=zh")
    #expect(zhEp.v6IPURL.absoluteString == "https://6.detectradar.com/api/v1/ip?lang=zh")
    #expect(zhEp.webHomeURL.absoluteString == "https://detectradar.com/")
    #expect(zhEp.ipv6CheckURL.absoluteString == "https://detectradar.com/ipv6-leak-test/")

    let enEp = ProbeEndpoints(baseDomain: "detectradar.com", language: .en)
    #expect(enEp.v4IPURL.absoluteString == "https://4.detectradar.com/api/v1/ip?lang=en")
    #expect(enEp.v6IPURL.absoluteString == "https://6.detectradar.com/api/v1/ip?lang=en")
    #expect(enEp.webHomeURL.absoluteString == "https://detectradar.com/en/")
    #expect(enEp.ipv6CheckURL.absoluteString == "https://detectradar.com/en/ipv6-leak-test/")
}
