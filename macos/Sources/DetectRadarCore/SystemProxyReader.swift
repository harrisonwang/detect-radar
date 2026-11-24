import CFNetwork
import Foundation
import SystemConfiguration

public enum SystemProxyReader {
    public static func readSystemProxy() -> ProxyEndpoint? {
        guard let dict = SCDynamicStoreCopyProxies(nil) as? [String: Any] else {
            return nil
        }
        return parseSystemProxyDict(dict)
    }

    public static func parseSystemProxyDict(_ dict: [String: Any]) -> ProxyEndpoint? {
        let socksEnabled = (dict[kCFNetworkProxiesSOCKSEnable as String] as? NSNumber)?.boolValue ?? false
        let socksHost = (dict[kCFNetworkProxiesSOCKSProxy as String] as? String)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let socksPort = (dict[kCFNetworkProxiesSOCKSPort as String] as? NSNumber)?.intValue ?? 0
        let validSocksPort: Int? = (socksEnabled && !socksHost.isEmpty && socksPort > 0) ? socksPort : nil

        let httpsEnabled = (dict[kCFNetworkProxiesHTTPSEnable as String] as? NSNumber)?.boolValue ?? false
        let httpsHost = (dict[kCFNetworkProxiesHTTPSProxy as String] as? String)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let httpsPort = (dict[kCFNetworkProxiesHTTPSPort as String] as? NSNumber)?.intValue ?? 0
        if httpsEnabled, !httpsHost.isEmpty, httpsPort > 0 {
            return ProxyEndpoint(
                scheme: .http,
                host: httpsHost,
                port: httpsPort,
                companionSocksPort: (validSocksPort != httpsPort) ? validSocksPort : nil
            )
        }

        let httpEnabled = (dict[kCFNetworkProxiesHTTPEnable as String] as? NSNumber)?.boolValue ?? false
        let httpHost = (dict[kCFNetworkProxiesHTTPProxy as String] as? String)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let httpPort = (dict[kCFNetworkProxiesHTTPPort as String] as? NSNumber)?.intValue ?? 0
        if httpEnabled, !httpHost.isEmpty, httpPort > 0 {
            return ProxyEndpoint(
                scheme: .http,
                host: httpHost,
                port: httpPort,
                companionSocksPort: (validSocksPort != httpPort) ? validSocksPort : nil
            )
        }

        if let port = validSocksPort {
            return ProxyEndpoint(
                scheme: .socks5,
                host: socksHost,
                port: port,
                companionSocksPort: nil
            )
        }

        return nil
    }

    public static func parseProxyURL(_ raw: String) -> ProxyEndpoint? {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }

        let normalized = trimmed.contains("://") ? trimmed : "http://\(trimmed)"
        guard let components = URLComponents(string: normalized),
              let host = components.host, !host.isEmpty,
              let port = components.port, port > 0 && port <= 65535 else {
            return nil
        }

        let schemeStr = (components.scheme ?? "http").lowercased()
        let scheme: ProxyScheme
        switch schemeStr {
        case "socks5", "socks5h", "socks":
            scheme = .socks5
        case "https":
            scheme = .https
        default:
            scheme = .http
        }
        return ProxyEndpoint(scheme: scheme, host: host, port: port)
    }

    public static func connectionProxyDictionary(for endpoint: ProxyEndpoint) -> [AnyHashable: Any] {
        switch endpoint.scheme {
        case .http, .https:
            return [
                kCFNetworkProxiesHTTPEnable as String: 1,
                kCFNetworkProxiesHTTPProxy as String: endpoint.host,
                kCFNetworkProxiesHTTPPort as String: endpoint.port,
                kCFNetworkProxiesHTTPSEnable as String: 1,
                kCFNetworkProxiesHTTPSProxy as String: endpoint.host,
                kCFNetworkProxiesHTTPSPort as String: endpoint.port,
            ]
        case .socks5:
            return [
                kCFNetworkProxiesSOCKSEnable as String: 1,
                kCFNetworkProxiesSOCKSProxy as String: endpoint.host,
                kCFNetworkProxiesSOCKSPort as String: endpoint.port,
            ]
        }
    }
}
