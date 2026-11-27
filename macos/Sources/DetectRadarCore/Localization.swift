import Foundation

public enum AppLanguage: String, Equatable, Sendable {
    case zh
    case en

    public var apiLangParam: String {
        rawValue
    }

    public var webPathPrefix: String {
        self == .en ? "/en" : ""
    }

    public static func resolve(
        override: String? = AppConfig.languageOverride,
        preferredLanguages: [String] = Locale.preferredLanguages
    ) -> AppLanguage {
        if let raw = override?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased(), !raw.isEmpty {
            if raw.hasPrefix("en") { return .en }
            if raw.hasPrefix("zh") { return .zh }
        }
        if let first = preferredLanguages.first?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased(), !first.isEmpty {
            return first.hasPrefix("zh") ? .zh : .en
        }
        return .zh
    }
}

public enum UsageKind: Equatable, Sendable {
    case hosting
    case isp
    case unknown
}
