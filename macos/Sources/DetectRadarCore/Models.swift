import Foundation

public struct IPIntel: Codable, Equatable, Sendable {
    public let ip: String
    public let country: String?
    public let countryName: String?
    public let region: String?
    public let city: String?
    public let asn: String?
    public let org: String?
    public let isp: String?
    public let usageType: String?
    public let usageTypeRaw: String?
    public let isHosting: Bool?
    public let isMobile: Bool?
    public let isVPN: Bool?
    public let isProxy: Bool?
    public let isTor: Bool?
    public let isRelay: Bool?
    public let isAnonymous: Bool?

    enum CodingKeys: String, CodingKey {
        case ip
        case country
        case countryName = "country_name"
        case region
        case city
        case asn
        case org
        case isp
        case usageType = "usage_type"
        case usageTypeRaw = "usage_type_raw"
        case isHosting = "is_hosting"
        case isMobile = "is_mobile"
        case isVPN = "is_vpn"
        case isProxy = "is_proxy"
        case isTor = "is_tor"
        case isRelay = "is_relay"
        case isAnonymous = "is_anonymous"
    }

    public init(
        ip: String,
        country: String? = nil,
        countryName: String? = nil,
        region: String? = nil,
        city: String? = nil,
        asn: String? = nil,
        org: String? = nil,
        isp: String? = nil,
        usageType: String? = nil,
        usageTypeRaw: String? = nil,
        isHosting: Bool? = nil,
        isMobile: Bool? = nil,
        isVPN: Bool? = nil,
        isProxy: Bool? = nil,
        isTor: Bool? = nil,
        isRelay: Bool? = nil,
        isAnonymous: Bool? = nil
    ) {
        self.ip = ip
        self.country = country
        self.countryName = countryName
        self.region = region
        self.city = city
        self.asn = asn
        self.org = org
        self.isp = isp
        self.usageType = usageType
        self.usageTypeRaw = usageTypeRaw
        self.isHosting = isHosting
        self.isMobile = isMobile
        self.isVPN = isVPN
        self.isProxy = isProxy
        self.isTor = isTor
        self.isRelay = isRelay
        self.isAnonymous = isAnonymous
    }
}

public enum ProxyScheme: String, Codable, Equatable, Sendable {
    case http
    case https
    case socks5
}

public struct ProxyEndpoint: Equatable, Sendable {
    public let scheme: ProxyScheme
    public let host: String
    public let port: Int

    public init(
        scheme: ProxyScheme,
        host: String,
        port: Int
    ) {
        self.scheme = scheme
        self.host = host
        self.port = port
    }
}

public enum IPv6LeakStatus: Equatable, Sendable {
    case none
    case leaked(carrier: String, ip: String)

    public var isLeaked: Bool {
        if case .leaked = self { return true }
        return false
    }
}

public struct EgressSnapshot: Equatable, Sendable {
    public let timestamp: Date
    public let activeV4: IPIntel?
    public let directV6: IPIntel?
    public let systemProxy: ProxyEndpoint?
    public let ipv6Leak: IPv6LeakStatus
    public let errorMessage: String?

    public init(
        timestamp: Date,
        activeV4: IPIntel?,
        directV6: IPIntel?,
        systemProxy: ProxyEndpoint?,
        ipv6Leak: IPv6LeakStatus,
        errorMessage: String? = nil
    ) {
        self.timestamp = timestamp
        self.activeV4 = activeV4
        self.directV6 = directV6
        self.systemProxy = systemProxy
        self.ipv6Leak = ipv6Leak
        self.errorMessage = errorMessage
    }
}

public enum AppConfig {
    public static var baseDomain: String {
        let raw = UserDefaults.standard.string(forKey: "baseDomain")?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return raw.isEmpty ? "detectradar.com" : raw
    }

    public static var languageOverride: String? {
        let raw = UserDefaults.standard.string(forKey: "language")?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return raw.isEmpty ? nil : raw
    }
}
