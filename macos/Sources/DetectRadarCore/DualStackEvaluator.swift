import Foundation

public enum DualStackEvaluator {
    /// 仅当生效 IPv4 出口在海外（中国大陆以外）且本机直连 IPv6 落在国内非机房运营商网段时，判为 IPv6 旁路泄露。
    public static func evaluateIPv6Leak(activeV4: IPIntel?, directV6: IPIntel?) -> IPv6LeakStatus {
        guard let v4 = activeV4, let v6 = directV6 else {
            return .none
        }
        if !IPMasker.isMainlandChina(v4),
           IPMasker.isMainlandChina(v6),
           !IPMasker.isHosting(v6) {
            return .leaked(carrier: IPMasker.shortProviderName(for: v6), ip: v6.ip)
        }
        return .none
    }
}
