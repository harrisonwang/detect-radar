import Foundation

public enum DualStackEvaluator {
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
