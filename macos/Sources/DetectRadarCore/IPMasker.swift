import Foundation

public enum IPMasker {
    private static let hkMoTwTokens = [
        "香港", "澳门", "澳門", "台湾", "臺灣",
        "hong kong", "macau", "macao", "taiwan",
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

    public static func regionDisplayName(for intel: IPIntel?, language: AppLanguage = .zh) -> String {
        let code = effectiveCountryCode(intel)
        if language == .en {
            switch code {
            case "HK": return "Hong Kong"
            case "MO": return "Macao"
            case "TW": return "Taiwan"
            case "CN": return "Mainland China"
            case "": return "Unknown"
            default:
                return Locale(identifier: "en_US").localizedString(forRegionCode: code) ?? code
            }
        }
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

    public static func shortProviderName(for intel: IPIntel?, language: AppLanguage = .zh) -> String {
        guard let intel else { return language == .en ? "Unknown" : "未知" }
        var raw = ""
        for candidate in [intel.isp, intel.org] {
            let trimmed = (candidate ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            if !trimmed.isEmpty, trimmed != "-", trimmed.lowercased() != "unknown" {
                raw = trimmed
                break
            }
        }
        if raw.isEmpty {
            return formatASN(intel.asn) ?? (language == .en ? "Unknown network" : "未知网络")
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
        return usage == "hosting" || raw == "hosting" || raw == "datacenter"
    }

    public static func isProxyOrHosting(_ intel: IPIntel) -> Bool {
        isHosting(intel)
            || intel.isVPN == true
            || intel.isProxy == true
            || intel.isTor == true
            || intel.isRelay == true
            || intel.isAnonymous == true
    }

    public static func isAccessNetwork(_ intel: IPIntel) -> Bool {
        if isProxyOrHosting(intel) { return false }
        let usage = (intel.usageType ?? "").lowercased()
        let raw = (intel.usageTypeRaw ?? "").lowercased()
        return intel.isMobile == true
            || usage == "isp"
            || ["residential", "mobile", "isp", "business", "education", "government", "satellite"].contains(raw)
    }

    public static func usageKind(for intel: IPIntel) -> UsageKind {
        if isHosting(intel) { return .hosting }
        let usage = (intel.usageType ?? "").lowercased()
        let raw = (intel.usageTypeRaw ?? "").lowercased()
        if intel.isMobile == true
            || usage == "isp"
            || ["residential", "mobile", "isp", "business", "education", "government", "satellite"].contains(raw) {
            return .isp
        }
        return .unknown
    }

    public static func usageShortLabel(for intel: IPIntel, language: AppLanguage = .zh) -> String {
        switch usageKind(for: intel) {
        case .hosting:
            return language == .en ? "DC" : "机房"
        case .isp:
            return language == .en ? "ISP" : "宽带"
        case .unknown:
            return language == .en ? "Unknown" : "未知"
        }
    }

    public static func usageBadgeLabel(for intel: IPIntel, language: AppLanguage = .zh) -> String {
        if language == .en {
            switch usageKind(for: intel) {
            case .hosting: return "Datacenter"
            case .isp: return "ISP"
            case .unknown: return "Unknown"
            }
        }
        return usageShortLabel(for: intel, language: .zh)
    }

    private static func nonRedundantCity(for intel: IPIntel?, language: AppLanguage = .zh) -> String? {
        let code = effectiveCountryCode(intel)
        let region = regionDisplayName(for: intel, language: language)
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

    public static func countryBadgeLine(for intel: IPIntel?, language: AppLanguage = .zh) -> String {
        let code = effectiveCountryCode(intel)
        if language == .en {
            return code.isEmpty ? "🌐 Unknown" : "\(flagEmoji(for: code)) \(code)"
        }
        return "\(flagEmoji(for: code)) \(regionDisplayName(for: intel, language: .zh))"
    }

    public static func providerDetailLine(for intel: IPIntel?, language: AppLanguage = .zh) -> String {
        let provider = shortProviderName(for: intel, language: language)
        if let city = nonRedundantCity(for: intel, language: language) {
            return "\(provider) · \(city)"
        }
        return provider
    }

    public static func menuBarTitle(snapshot: EgressSnapshot?, isLoading: Bool, language: AppLanguage = .zh) -> String {
        guard let snapshot else {
            return isLoading ? (language == .en ? "◎ Checking" : "◎ 检测中") : "DetectRadar"
        }
        guard let primary = snapshot.activeV4 ?? snapshot.directV6 else {
            return isLoading ? (language == .en ? "◎ Checking" : "◎ 检测中") : (language == .en ? "⚠️ Offline" : "⚠️ 离线")
        }
        let flag = flagEmoji(for: effectiveCountryCode(primary))
        let kind = usageShortLabel(for: primary, language: language)
        let base = "\(flag) \(kind)"
        return snapshot.ipv6Leak.isLeaked ? "\(base) ⚠️" : base
    }
}
