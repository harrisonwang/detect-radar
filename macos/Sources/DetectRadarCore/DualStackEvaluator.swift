import Foundation

public enum DualStackEvaluator {
    public static func evaluateIPv6Leak(
        activeV4: IPIntel?,
        directV6: IPIntel?,
        language: AppLanguage = .zh
    ) -> IPv6LeakStatus {
        guard let v4 = activeV4, let v6 = directV6 else {
            return .none
        }
        guard IPMasker.isAccessNetwork(v6) else {
            return .none
        }

        let v4Geo = IPMasker.effectiveCountryCode(v4)
        let v6Geo = IPMasker.effectiveCountryCode(v6)
        let v4ASN = IPMasker.formatASN(v4.asn)
        let v6ASN = IPMasker.formatASN(v6.asn)
        let sameASN = (v4ASN != nil && v6ASN != nil && v4ASN == v6ASN)
        let v4ProxyOrHosting = IPMasker.isProxyOrHosting(v4)

        if v4Geo.isEmpty || v6Geo.isEmpty {
            if v4ProxyOrHosting && !sameASN {
                return .leaked(carrier: IPMasker.shortProviderName(for: v6, language: language), ip: v6.ip)
            }
            return .none
        }

        let crossRegion = v4Geo != v6Geo
        let sameRegionProxyBypass = (v4Geo == v6Geo && v4ProxyOrHosting && !sameASN)
        if crossRegion || sameRegionProxyBypass {
            return .leaked(carrier: IPMasker.shortProviderName(for: v6, language: language), ip: v6.ip)
        }
        return .none
    }
}
