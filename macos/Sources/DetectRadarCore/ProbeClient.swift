import Foundation

public struct ProbeEndpoints: Equatable, Sendable {
    public let mainBaseURL: URL
    public let v4IPURL: URL
    public let v6IPURL: URL

    public init(baseDomain: String = AppConfig.baseDomain) {
        let cleaned = baseDomain
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: "https://", with: "")
            .replacingOccurrences(of: "http://", with: "")
            .trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        let host = cleaned.isEmpty ? "detectradar.com" : cleaned

        self.mainBaseURL = URL(string: "https://\(host)")!
        self.v4IPURL = URL(string: "https://4.\(host)/api/v1/ip")!
        self.v6IPURL = URL(string: "https://6.\(host)/api/v1/ip")!
    }
}

public enum ProbeError: LocalizedError, Sendable {
    case rateLimited
    case invalidResponse
    case wrongIPFamily
    case unreachable(String)

    public var errorDescription: String? {
        switch self {
        case .rateLimited: "请求过于频繁"
        case .invalidResponse: "响应格式异常"
        case .wrongIPFamily: "协议簇不匹配"
        case .unreachable(let msg): msg
        }
    }
}

public final class ProbeClient: Sendable {
    private let userAgent = "DetectRadarBar/0.1"

    public init() {}

    public func probeIP(
        url: URL,
        proxyEndpoint: ProxyEndpoint? = nil,
        expectedFamily: String? = nil,
        timeoutSeconds: TimeInterval = 5.0
    ) async throws -> IPIntel {
        let config = URLSessionConfiguration.ephemeral
        config.requestCachePolicy = .reloadIgnoringLocalCacheData
        config.timeoutIntervalForRequest = timeoutSeconds
        config.timeoutIntervalForResource = timeoutSeconds + 1.0
        if let proxyEndpoint {
            config.connectionProxyDictionary = SystemProxyReader.connectionProxyDictionary(for: proxyEndpoint)
        } else {
            config.connectionProxyDictionary = [:]
        }

        let session = URLSession(configuration: config)
        defer { session.finishTasksAndInvalidate() }

        var request = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: timeoutSeconds)
        request.httpMethod = "GET"
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue(userAgent, forHTTPHeaderField: "User-Agent")
        request.setValue("no-store", forHTTPHeaderField: "Cache-Control")

        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch {
            throw ProbeError.unreachable(error.localizedDescription)
        }

        guard let http = response as? HTTPURLResponse else {
            throw ProbeError.invalidResponse
        }
        if http.statusCode == 429 {
            throw ProbeError.rateLimited
        }
        guard (200..<300).contains(http.statusCode) else {
            throw ProbeError.unreachable("HTTP \(http.statusCode)")
        }

        let intel: IPIntel
        do {
            intel = try JSONDecoder().decode(IPIntel.self, from: data)
        } catch {
            throw ProbeError.invalidResponse
        }

        let trimmedIP = intel.ip.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmedIP.isEmpty else { throw ProbeError.invalidResponse }
        if expectedFamily == "v4", trimmedIP.contains(":") {
            throw ProbeError.wrongIPFamily
        }
        if expectedFamily == "v6", !trimmedIP.contains(":") {
            throw ProbeError.wrongIPFamily
        }

        return intel
    }

    public func runProbe(baseDomain: String = AppConfig.baseDomain) async -> EgressSnapshot {
        let endpoints = ProbeEndpoints(baseDomain: baseDomain)
        let systemProxy = SystemProxyReader.readSystemProxy()
        let hasLocalV6 = LocalIPv6Detector.hasGlobalIPv6()

        async let activeV4Task: Result<IPIntel, Error> = {
            if let proxy = systemProxy,
               let proxied = try? await probeIP(url: endpoints.v4IPURL, proxyEndpoint: proxy, expectedFamily: "v4", timeoutSeconds: 5.0) {
                return .success(proxied)
            }
            do {
                return .success(try await probeIP(url: endpoints.v4IPURL, proxyEndpoint: nil, expectedFamily: "v4", timeoutSeconds: 5.0))
            } catch {
                return .failure(error)
            }
        }()

        async let directV6Task: IPIntel? = {
            guard hasLocalV6 else { return nil }
            return try? await probeIP(url: endpoints.v6IPURL, proxyEndpoint: nil, expectedFamily: "v6", timeoutSeconds: 3.5)
        }()

        let activeV4Res = await activeV4Task
        let directV6 = await directV6Task

        let activeV4: IPIntel?
        let v4Err: String?
        switch activeV4Res {
        case .success(let intel):
            activeV4 = intel
            v4Err = nil
        case .failure(let err):
            activeV4 = nil
            v4Err = err.localizedDescription
        }

        let ipv6Leak = DualStackEvaluator.evaluateIPv6Leak(activeV4: activeV4, directV6: directV6)
        let errMessage: String? = (activeV4 == nil && directV6 == nil) ? (v4Err ?? "无法连接到 DetectRadar") : nil

        return EgressSnapshot(
            timestamp: Date(),
            activeV4: activeV4,
            directV6: directV6,
            systemProxy: systemProxy,
            ipv6Leak: ipv6Leak,
            errorMessage: errMessage
        )
    }
}
