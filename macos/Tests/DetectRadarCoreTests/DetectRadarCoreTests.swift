import Foundation
import Testing
@testable import DetectRadarCore

@Test
func shortProviderAndDisplayFormatting() {
    let dmit = IPIntel(
        ip: "69.63.208.30", country: "US", city: "Los Angeles",
        org: "DMIT Cloud Services", isp: "DMIT Cloud Services",
        usageType: "hosting", isHosting: true
    )
    #expect(IPMasker.shortProviderName(for: dmit) == "DMIT")
    #expect(IPMasker.countryBadgeLine(for: dmit) == "🇺🇸 美国")
    #expect(IPMasker.providerDetailLine(for: dmit) == "DMIT · Los Angeles")
    #expect(IPMasker.locationLine(for: dmit) == "🇺🇸 美国 · Los Angeles")
    #expect(IPMasker.usageShortLabel(for: dmit) == "机房")

    let cf = IPIntel(ip: "1.1.1.1", country: "US", isp: "Cloudflare, Inc.", usageType: "hosting", isHosting: true)
    #expect(IPMasker.shortProviderName(for: cf) == "Cloudflare")
    #expect(IPMasker.providerDetailLine(for: cf) == "Cloudflare")

    let telecom = IPIntel(ip: "240e:390:100::1", country: "CN", region: "上海市", city: "上海", isp: "China Telecom", usageType: "isp")
    #expect(IPMasker.shortProviderName(for: telecom) == "中国电信")
    #expect(IPMasker.countryBadgeLine(for: telecom) == "🇨🇳 中国大陆")
    #expect(IPMasker.providerDetailLine(for: telecom) == "中国电信 · 上海")
    #expect(IPMasker.usageShortLabel(for: telecom) == "宽带")

    let sg = IPIntel(ip: "149.28.130.42", country: "SG", city: "Singapore", isp: "Vultr Holdings LLC", usageType: "hosting", isHosting: true)
    #expect(IPMasker.shortProviderName(for: sg) == "Vultr Holdings")
    #expect(IPMasker.countryBadgeLine(for: sg) == "🇸🇬 新加坡")
    #expect(IPMasker.providerDetailLine(for: sg) == "Vultr Holdings")
}

@Test
func hkMoTwExcludedFromMainlandCNAndIPv6LeakRules() {
    let usHosting = IPIntel(ip: "69.63.208.30", country: "US", usageType: "hosting", isHosting: true)
    let cnBroadbandV6 = IPIntel(ip: "240e:390:100::1", country: "CN", region: "上海市", isp: "China Telecom", usageType: "isp", isHosting: false)
    let hkHostingV6 = IPIntel(ip: "2600:1900:41a0:fd::", country: "CN", countryName: "香港", region: "香港特别行政区", city: "香港", usageType: "hosting", isHosting: true)
    let cnBroadbandV4 = IPIntel(ip: "223.104.1.1", country: "CN", region: "北京市", isp: "China Mobile", usageType: "isp", isHosting: false)

    #expect(IPMasker.isMainlandChina(cnBroadbandV4) == true)
    #expect(IPMasker.isMainlandChina(hkHostingV6) == false)
    #expect(IPMasker.effectiveCountryCode(hkHostingV6) == "HK")
    #expect(IPMasker.countryBadgeLine(for: hkHostingV6) == "🇭🇰 中国香港")
    #expect(IPMasker.providerDetailLine(for: hkHostingV6) == "未知网络")

    // 海外 IPv4 + 大陆运营商 IPv6 => 泄露
    let leaked = DualStackEvaluator.evaluateIPv6Leak(activeV4: usHosting, directV6: cnBroadbandV6)
    #expect(leaked == .leaked(carrier: "中国电信", ip: "240e:390:100::1"))

    // 海外 IPv4 + 香港机房 IPv6 => 不判泄露
    #expect(DualStackEvaluator.evaluateIPv6Leak(activeV4: usHosting, directV6: hkHostingV6) == .none)

    // 国内双栈直连 => 不判泄露
    #expect(DualStackEvaluator.evaluateIPv6Leak(activeV4: cnBroadbandV4, directV6: cnBroadbandV6) == .none)

    // 无 IPv4 只连 IPv6 => 不判泄露
    #expect(DualStackEvaluator.evaluateIPv6Leak(activeV4: nil, directV6: cnBroadbandV6) == .none)
}

@Test
func menuBarTitleOnlyWarnsOnIPv6Leak() {
    let usHosting = IPIntel(ip: "104.194.84.113", country: "US", usageType: "hosting", isHosting: true)
    let cnV6 = IPIntel(ip: "240e:390:100::1", country: "CN", region: "上海市", isp: "China Telecom", usageType: "isp")

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
        ipv6Leak: .leaked(carrier: "中国电信", ip: "240e:390:100::1")
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
    #expect(ep?.companionSocksPort == 7891)

    #expect(LocalIPv6Detector.isGlobalUnicastIPv6([0x24, 0x0e, 0x03, 0x90, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1]) == true)
    #expect(LocalIPv6Detector.isGlobalUnicastIPv6([0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1]) == false)
    #expect(LocalIPv6Detector.isGlobalUnicastIPv6([0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1]) == false)
}
