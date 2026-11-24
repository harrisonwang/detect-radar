import Darwin
import Foundation

public enum LocalIPv6Detector {
    /// 仅在本地枚举活动网卡上的全局单播 IPv6 地址（<1ms），用于避免纯 IPv4 网络发起无谓的 IPv6 请求。
    public static func hasGlobalIPv6() -> Bool {
        var ifap: UnsafeMutablePointer<ifaddrs>?
        guard getifaddrs(&ifap) == 0, let first = ifap else { return false }
        defer { freeifaddrs(first) }

        var ptr: UnsafeMutablePointer<ifaddrs>? = first
        while let current = ptr {
            let flags = Int32(current.pointee.ifa_flags)
            let isUp = (flags & IFF_UP) != 0
            let isLoopback = (flags & IFF_LOOPBACK) != 0

            if isUp && !isLoopback, let sa = current.pointee.ifa_addr, sa.pointee.sa_family == UInt8(AF_INET6) {
                let sin6 = sa.withMemoryRebound(to: sockaddr_in6.self, capacity: 1) { $0.pointee }
                var addr = sin6.sin6_addr
                let bytes = withUnsafeBytes(of: &addr) { Array($0) }
                if isGlobalUnicastIPv6(bytes) {
                    return true
                }
            }
            ptr = current.pointee.ifa_next
        }
        return false
    }

    public static func isGlobalUnicastIPv6(_ bytes: [UInt8]) -> Bool {
        guard bytes.count == 16 else { return false }
        // :: (unspecified) 或 ::1 (loopback)
        if bytes[0..<15].allSatisfy({ $0 == 0 }) && (bytes[15] == 0 || bytes[15] == 1) {
            return false
        }
        // Multicast ff00::/8
        if bytes[0] == 0xff { return false }
        // Link-local fe80::/10
        if bytes[0] == 0xfe && (bytes[1] & 0xc0) == 0x80 { return false }
        // Unique local address fc00::/7
        if (bytes[0] & 0xfe) == 0xfc { return false }
        // IPv4-mapped ::ffff:0:0/96
        if bytes[0..<10].allSatisfy({ $0 == 0 }) && bytes[10] == 0xff && bytes[11] == 0xff {
            return false
        }
        // Global unicast 2000::/3
        return (bytes[0] & 0xe0) == 0x20
    }
}
