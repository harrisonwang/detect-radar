import AppKit
import Foundation
import Network
import SystemConfiguration

private final class MonitorBox: @unchecked Sendable {
    weak var monitor: NetworkEventMonitor?
    init(_ monitor: NetworkEventMonitor) {
        self.monitor = monitor
    }
}

private func proxyStoreCallback(
    _ store: SCDynamicStore,
    _ changedKeys: CFArray,
    _ info: UnsafeMutableRawPointer?
) {
    guard let info else { return }
    let box = Unmanaged<MonitorBox>.fromOpaque(info).takeUnretainedValue()
    box.monitor?.notifyRawChange(reason: "proxy")
}

/// 纯事件驱动网络监听：零后台轮询定时器。仅监听网卡路径切换、系统代理变动与休眠唤醒，并做 3s 防抖与 10s 冷却。
public final class NetworkEventMonitor: @unchecked Sendable {
    private let queue = DispatchQueue(label: "com.detectradar.bar.netmonitor")
    private let debounceSeconds: TimeInterval
    private let cooldownSeconds: TimeInterval
    private let onTrigger: @MainActor @Sendable (String) -> Void

    private var pathMonitor: NWPathMonitor?
    private var proxyStore: SCDynamicStore?
    private var wakeObserver: NSObjectProtocol?
    private var debounceWorkItem: DispatchWorkItem?
    private var lastTriggeredAt: Date = .distantPast
    private var hasIgnoredInitialPath = false

    public init(
        debounceSeconds: TimeInterval = 3.0,
        cooldownSeconds: TimeInterval = 10.0,
        onTrigger: @escaping @MainActor @Sendable (String) -> Void
    ) {
        self.debounceSeconds = debounceSeconds
        self.cooldownSeconds = cooldownSeconds
        self.onTrigger = onTrigger
    }

    public func start() {
        queue.async { [weak self] in
            self?.setupMonitors()
        }
    }

    public func stop() {
        queue.async { [weak self] in
            guard let self else { return }
            self.debounceWorkItem?.cancel()
            self.debounceWorkItem = nil
            self.pathMonitor?.cancel()
            self.pathMonitor = nil
            if let store = self.proxyStore {
                SCDynamicStoreSetDispatchQueue(store, nil)
                self.proxyStore = nil
            }
            if let observer = self.wakeObserver {
                NSWorkspace.shared.notificationCenter.removeObserver(observer)
                self.wakeObserver = nil
            }
        }
    }

    deinit {
        pathMonitor?.cancel()
        if let store = proxyStore {
            SCDynamicStoreSetDispatchQueue(store, nil)
        }
        if let observer = wakeObserver {
            NSWorkspace.shared.notificationCenter.removeObserver(observer)
        }
    }

    private func setupMonitors() {
        // 1. NWPathMonitor (Wi-Fi / Ethernet / TUN 切换)
        let monitor = NWPathMonitor()
        monitor.pathUpdateHandler = { [weak self] path in
            guard let self else { return }
            if !self.hasIgnoredInitialPath {
                self.hasIgnoredInitialPath = true
                return
            }
            if path.status == .satisfied {
                self.scheduleTrigger(reason: "path")
            }
        }
        monitor.start(queue: queue)
        self.pathMonitor = monitor

        // 2. SCDynamicStore 监听系统代理变动
        let box = MonitorBox(self)
        let retained = Unmanaged.passRetained(box)
        var context = SCDynamicStoreContext(
            version: 0,
            info: retained.toOpaque(),
            retain: { ptr in
                _ = Unmanaged<MonitorBox>.fromOpaque(ptr).retain()
                return ptr
            },
            release: { ptr in
                Unmanaged<MonitorBox>.fromOpaque(ptr).release()
            },
            copyDescription: nil
        )
        let store = SCDynamicStoreCreate(nil, "com.detectradar.bar.proxymonitor" as CFString, proxyStoreCallback, &context)
        retained.release()

        if let store {
            let keys = ["State:/Network/Global/Proxies"] as CFArray
            if SCDynamicStoreSetNotificationKeys(store, keys, nil) {
                SCDynamicStoreSetDispatchQueue(store, queue)
                self.proxyStore = store
            }
        }

        // 3. 系统唤醒监听
        self.wakeObserver = NSWorkspace.shared.notificationCenter.addObserver(
            forName: NSWorkspace.didWakeNotification,
            object: nil,
            queue: nil
        ) { [weak self] _ in
            self?.notifyRawChange(reason: "wake")
        }
    }

    fileprivate func notifyRawChange(reason: String) {
        queue.async { [weak self] in
            self?.scheduleTrigger(reason: reason)
        }
    }

    private func scheduleTrigger(reason: String) {
        debounceWorkItem?.cancel()
        let work = DispatchWorkItem { [weak self] in
            guard let self else { return }
            let now = Date()
            guard now.timeIntervalSince(self.lastTriggeredAt) >= self.cooldownSeconds else { return }
            self.lastTriggeredAt = now
            let cb = self.onTrigger
            Task { @MainActor in
                cb(reason)
            }
        }
        debounceWorkItem = work
        queue.asyncAfter(deadline: .now() + debounceSeconds, execute: work)
    }
}
