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
        let httpsEnabled = (dict[kCFNetworkProxiesHTTPSEnable as String] as? NSNumber)?.boolValue ?? false
        let httpsHost = (dict[kCFNetworkProxiesHTTPSProxy as String] as? String)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let httpsPort = (dict[kCFNetworkProxiesHTTPSPort as String] as? NSNumber)?.intValue ?? 0
        if httpsEnabled, !httpsHost.isEmpty, httpsPort > 0 {
            return ProxyEndpoint(
                scheme: .http,
                host: httpsHost,
                port: httpsPort
            )
        }

        let httpEnabled = (dict[kCFNetworkProxiesHTTPEnable as String] as? NSNumber)?.boolValue ?? false
        let httpHost = (dict[kCFNetworkProxiesHTTPProxy as String] as? String)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let httpPort = (dict[kCFNetworkProxiesHTTPPort as String] as? NSNumber)?.intValue ?? 0
        if httpEnabled, !httpHost.isEmpty, httpPort > 0 {
            return ProxyEndpoint(
                scheme: .http,
                host: httpHost,
                port: httpPort
            )
        }

        let socksEnabled = (dict[kCFNetworkProxiesSOCKSEnable as String] as? NSNumber)?.boolValue ?? false
        let socksHost = (dict[kCFNetworkProxiesSOCKSProxy as String] as? String)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let socksPort = (dict[kCFNetworkProxiesSOCKSPort as String] as? NSNumber)?.intValue ?? 0
        if socksEnabled, !socksHost.isEmpty, socksPort > 0 {
            return ProxyEndpoint(
                scheme: .socks5,
                host: socksHost,
                port: socksPort
            )
        }

        return nil
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
