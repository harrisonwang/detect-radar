import Foundation

public enum IPMasker {
    private static let hkMoTwTokens = [
        "香港", "澳门", "澳門", "台湾", "臺灣",
        "hong kong", "macau", "macao", "taiwan",
    ]

    private static let carrierAliases: [String: String] = [
        "chinanet": "中国电信",
        "chinanet backbone": "中国电信",
        "chinanet-backbone": "中国电信",
        "china telecom": "中国电信",
        "电信": "中国电信",
        "中国电信": "中国电信",
        "cmnet": "中国移动",
        "china mobile": "中国移动",
        "china mobile communications": "中国移动",
        "china mobile communications group co ltd": "中国移动",
        "中国移动通信": "中国移动",
        "移动": "中国移动",
        "中国移动": "中国移动",
        "china unicom": "中国联通",
        "china unicom backbone": "中国联通",
        "china unicom china169 backbone": "中国联通",
        "china169-backbone": "中国联通",
        "联通": "中国联通",
        "中国联通": "中国联通",
    ]

    private static let verboseSuffixes = [
        "Communications Group Co., Ltd.",
        "Communications Co., Ltd.",
        "Cloud Services",
        "Internet Services",
        "Web Services",
        "Corporation",
        "Company",
        "Limited",
        "Co., Ltd.",
        "Co.,Ltd.",
        "Co. Ltd.",
        "Inc.",
        "Inc",
        "LLC",
        "Ltd.",
        "Ltd",
        "Corp.",
        "Corp",
        "GmbH",
        "Pte.",
        "Pte",
        "S.A.",
        "B.V.",
    ]

    public static func maskIP(_ ip: String) -> String {
        let trimmed = ip.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return "—" }

        let v4Parts = trimmed.split(separator: ".", omittingEmptySubsequences: false)
        if v4Parts.count == 4, v4Parts.allSatisfy({ Int($0) != nil }) {
            return "\(v4Parts[0]).\(v4Parts[1]).*.*"
        }

        if trimmed.contains(":") {
            let v6Parts = trimmed.split(separator: ":", omittingEmptySubsequences: true)
            if v6Parts.count >= 2 {
                return "\(v6Parts[0]):\(v6Parts[1]):*"
            }
            if let first = v6Parts.first {
                return "\(first):*"
            }
            return "*:*"
        }

        return "***"
    }

    public static func isMainlandChina(_ intel: IPIntel?) -> Bool {
        guard let intel else { return false }
        let code = (intel.country ?? "").trimmingCharacters(in: .whitespacesAndNewlines).uppercased()
        guard code == "CN" else { return false }
        let haystack = [intel.region, intel.countryName, intel.city]
            .compactMap { $0 }
            .joined(separator: " ")
            .lowercased()
        for token in hkMoTwTokens where haystack.contains(token) {
            return false
        }
        return true
    }

    public static func effectiveCountryCode(_ intel: IPIntel?) -> String {
        guard let intel else { return "" }
        let code = (intel.country ?? "").trimmingCharacters(in: .whitespacesAndNewlines).uppercased()
        guard code.count == 2, code != "XX", code != "ZZ" else { return "" }

        if code == "CN" {
            let haystack = [intel.region, intel.countryName, intel.city]
                .compactMap { $0 }
                .joined(separator: " ")
                .lowercased()
            if haystack.contains("香港") || haystack.contains("hong kong") { return "HK" }
            if haystack.contains("澳门") || haystack.contains("澳門") || haystack.contains("macau") || haystack.contains("macao") { return "MO" }
            if haystack.contains("台湾") || haystack.contains("臺灣") || haystack.contains("taiwan") { return "TW" }
        }
        return code
    }

    public static func flagEmoji(for countryCode: String) -> String {
        let upper = countryCode.trimmingCharacters(in: .whitespacesAndNewlines).uppercased()
        guard upper.count == 2 else { return "🌐" }
        var scalars = String.UnicodeScalarView()
        for scalar in upper.unicodeScalars {
            guard scalar.value >= 65 && scalar.value <= 90,
                  let indicator = UnicodeScalar(127397 + scalar.value) else {
                return "🌐"
            }
            scalars.append(indicator)
        }
        return String(scalars)
    }

    public static func regionDisplayName(for intel: IPIntel?) -> String {
        let code = effectiveCountryCode(intel)
        switch code {
        case "HK": return "中国香港"
        case "MO": return "中国澳门"
        case "TW": return "中国台湾"
        case "CN": return "中国大陆"
        case "": return "地区未知"
        default:
            return Locale(identifier: "zh_CN").localizedString(forRegionCode: code) ?? code
        }
    }

    /// 去除冗余公司/云后缀并归一化常见国内运营商短名（如 "DMIT Cloud Services" -> "DMIT"）。
    public static func shortProviderName(for intel: IPIntel?) -> String {
        guard let intel else { return "未知" }
        var raw = ""
        for candidate in [intel.isp, intel.org] {
            let trimmed = (candidate ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            if !trimmed.isEmpty, trimmed != "-", trimmed.lowercased() != "unknown" {
                raw = trimmed
                break
            }
        }
        if raw.isEmpty {
            return formatASN(intel.asn) ?? "未知网络"
        }

        let normalizedKey = raw
            .lowercased()
            .replacingOccurrences(of: ".", with: "")
            .replacingOccurrences(of: ",", with: "")
            .split(whereSeparator: { $0.isWhitespace })
            .joined(separator: " ")

        if let alias = carrierAliases[normalizedKey] {
            return alias
        }
        if normalizedKey.contains("china telecom") || normalizedKey.contains("chinanet") {
            return "中国电信"
        }
        if normalizedKey.contains("china mobile") || normalizedKey == "cmnet" {
            return "中国移动"
        }
        if normalizedKey.contains("china unicom") {
            return "中国联通"
        }

        var current = raw
        var changed = true
        while changed {
            changed = false
            current = current.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines.union(CharacterSet(charactersIn: ",.")))
            for suffix in verboseSuffixes {
                if current.count > suffix.count,
                   current.lowercased().hasSuffix(suffix.lowercased()) {
                    let prefixEnd = current.index(current.endIndex, offsetBy: -suffix.count)
                    let before = current[current.startIndex..<prefixEnd]
                    if before.last?.isWhitespace == true || before.last == "," {
                        current = String(before)
                        changed = true
                        break
                    }
                }
            }
        }
        current = current.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines.union(CharacterSet(charactersIn: ",.")))
        return current.isEmpty ? raw : current
    }

    public static func formatASN(_ raw: String?) -> String? {
        guard let trimmed = raw?.trimmingCharacters(in: .whitespacesAndNewlines), !trimmed.isEmpty else {
            return nil
        }
        let upper = trimmed.uppercased()
        if upper == "-" || upper == "UNKNOWN" { return nil }
        return upper.hasPrefix("AS") ? upper : "AS\(trimmed)"
    }

    public static func isHosting(_ intel: IPIntel) -> Bool {
        if intel.isHosting == true { return true }
        let usage = (intel.usageType ?? "").lowercased()
        let raw = (intel.usageTypeRaw ?? "").lowercased()
        return usage == "hosting" || usage == "datacenter" || raw == "hosting" || raw == "datacenter"
    }

    public static func usageShortLabel(for intel: IPIntel) -> String {
        if isHosting(intel) { return "机房" }
        let usage = (intel.usageType ?? "").lowercased()
        let raw = (intel.usageTypeRaw ?? "").lowercased()
        if intel.isMobile == true || usage == "isp" || ["residential", "mobile", "business", "education", "government"].contains(raw) {
            return "宽带"
        }
        return "未知"
    }

    private static func nonRedundantCity(for intel: IPIntel?) -> String? {
        let code = effectiveCountryCode(intel)
        let region = regionDisplayName(for: intel)
        let city = (intel?.city ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        if city.isEmpty {
            return nil
        }
        let lowerCity = city.lowercased()
        let redundantCityStates: Set<String> = ["singapore", "新加坡", "hong kong", "香港", "macau", "macao", "澳门", "澳門"]
        if lowerCity == region.lowercased() || (["SG", "HK", "MO"].contains(code) && redundantCityStates.contains(lowerCity)) {
            return nil
        }
        return city
    }

    /// 如 "🇺🇸 美国"、"🇭🇰 中国香港"。
    public static func countryBadgeLine(for intel: IPIntel?) -> String {
        "\(flagEmoji(for: effectiveCountryCode(intel))) \(regionDisplayName(for: intel))"
    }

    /// 如 "DMIT · Los Angeles"、"中国电信 · 上海"、"Vultr Holdings"。
    public static func providerDetailLine(for intel: IPIntel?) -> String {
        let provider = shortProviderName(for: intel)
        if let city = nonRedundantCity(for: intel) {
            return "\(provider) · \(city)"
        }
        return provider
    }

    /// 如 "🇺🇸 美国 · Los Angeles"，城邦/直辖与国家重复时自动省略冗余城市。
    public static func locationLine(for intel: IPIntel?) -> String {
        let badge = countryBadgeLine(for: intel)
        if let city = nonRedundantCity(for: intel) {
            return "\(badge) · \(city)"
        }
        return badge
    }

    public static func menuBarTitle(snapshot: EgressSnapshot?, isLoading: Bool) -> String {
        guard let snapshot else {
            return isLoading ? "◎ 检测中" : "DetectRadar"
        }
        guard let primary = snapshot.activeV4 ?? snapshot.directV6 else {
            return isLoading ? "◎ 检测中" : "⚠️ 离线"
        }
        let flag = flagEmoji(for: effectiveCountryCode(primary))
        let kind = usageShortLabel(for: primary)
        let base = "\(flag) \(kind)"
        return snapshot.ipv6Leak.isLeaked ? "\(base) ⚠️" : base
    }
}
