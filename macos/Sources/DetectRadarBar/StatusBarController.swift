import AppKit
import DetectRadarCore
import Foundation
import ServiceManagement

@MainActor
final class StatusBarController: NSObject, NSPopoverDelegate {
    private var statusItem: NSStatusItem?
    private let popover = NSPopover()
    private let contentController = PopoverContentViewController()
    private let probeClient = ProbeClient()
    private var monitor: NetworkEventMonitor?
    private var outsideClickMonitor: Any?

    private var snapshot: EgressSnapshot?
    private var isRefreshing = false
    private var lastRefreshAt: Date = .distantPast
    private let isBundledApp = Bundle.main.bundleURL.pathExtension == "app"

    func start() {
        let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        if let button = item.button {
            button.font = NSFont.monospacedDigitSystemFont(ofSize: NSFont.systemFontSize, weight: .medium)
            button.title = IPMasker.menuBarTitle(snapshot: nil, isLoading: true)
            button.toolTip = "DetectRadar"
            button.target = self
            button.action = #selector(togglePopover(_:))
        }
        statusItem = item

        popover.behavior = .transient
        popover.animates = true
        popover.delegate = self
        popover.contentViewController = contentController

        syncPopoverContent(forceRebuild: true)

        let monitor = NetworkEventMonitor(debounceSeconds: 3.0, cooldownSeconds: 10.0) { [weak self] _ in
            Task { @MainActor [weak self] in
                await self?.refresh(force: false)
            }
        }
        self.monitor = monitor
        monitor.start()

        Task {
            await refresh(force: true)
        }
    }

    func refresh(force: Bool) async {
        if isRefreshing { return }
        if force && Date().timeIntervalSince(lastRefreshAt) < 1.5 {
            return
        }

        let previousSnapshot = snapshot
        isRefreshing = true
        updateTitle()
        if previousSnapshot == nil {
            syncPopoverContent(forceRebuild: false)
        }

        defer {
            isRefreshing = false
            lastRefreshAt = Date()
            updateTitle()
            syncPopoverContent(forceRebuild: snapshot != previousSnapshot || previousSnapshot == nil)
        }

        snapshot = await probeClient.runProbe()
    }

    private func updateTitle() {
        statusItem?.button?.title = IPMasker.menuBarTitle(snapshot: snapshot, isLoading: isRefreshing)
    }

    private func syncPopoverContent(forceRebuild: Bool) {
        let loginEnabled = isBundledApp && SMAppService.mainApp.status == .enabled
        contentController.configure(
            snapshot: snapshot,
            isRefreshing: isRefreshing,
            launchAtLoginEnabled: loginEnabled,
            launchAtLoginAvailable: isBundledApp,
            forceRebuild: forceRebuild,
            onCopyIP: { [weak self] ip in
                self?.copyToPasteboard(ip)
            },
            onOpenIPv6Check: { [weak self] in
                self?.closePopover(nil)
                self?.openIPv6Check()
            },
            onRetry: { [weak self] in
                Task { @MainActor [weak self] in
                    await self?.refresh(force: true)
                }
            },
            onOpenWebHome: { [weak self] in
                self?.closePopover(nil)
                self?.openWebHome()
            },
            onToggleLaunchAtLogin: { [weak self] in
                self?.toggleLaunchAtLogin()
            },
            onSilentRefresh: { [weak self] in
                Task { @MainActor [weak self] in
                    await self?.refresh(force: true)
                }
            },
            onClosePopover: { [weak self] in
                self?.closePopover(nil)
            },
            onQuit: { [weak self] in
                self?.quitApp()
            }
        )
        popover.contentSize = contentController.contentView.fittingPanelSize()
    }

    @objc private func togglePopover(_ sender: AnyObject?) {
        guard let button = statusItem?.button else { return }
        if popover.isShown {
            closePopover(sender)
            return
        }

        syncPopoverContent(forceRebuild: false)
        popover.show(relativeTo: button.bounds, of: button, preferredEdge: .minY)
        NSApp.activate(ignoringOtherApps: true)
        popover.contentViewController?.view.window?.makeKey()
        installOutsideClickMonitor()

        let stale = Date().timeIntervalSince(lastRefreshAt) >= 2.0
        if !isRefreshing && (snapshot == nil || snapshot?.errorMessage != nil || stale) {
            Task { @MainActor [weak self] in
                await self?.refresh(force: false)
            }
        }
    }

    private func closePopover(_ sender: AnyObject?) {
        popover.performClose(sender)
        removeOutsideClickMonitor()
    }

    private func installOutsideClickMonitor() {
        removeOutsideClickMonitor()
        outsideClickMonitor = NSEvent.addGlobalMonitorForEvents(matching: [.leftMouseDown, .rightMouseDown]) { [weak self] _ in
            Task { @MainActor [weak self] in
                guard let self, self.popover.isShown else { return }
                if let button = self.statusItem?.button,
                   let buttonWin = button.window {
                    let buttonScreenRect = buttonWin.convertToScreen(button.convert(button.bounds, to: nil))
                    if buttonScreenRect.contains(NSEvent.mouseLocation) {
                        return
                    }
                }
                self.closePopover(nil)
            }
        }
    }

    private func removeOutsideClickMonitor() {
        if let monitor = outsideClickMonitor {
            NSEvent.removeMonitor(monitor)
            outsideClickMonitor = nil
        }
    }

    func popoverDidClose(_ notification: Notification) {
        removeOutsideClickMonitor()
    }

    private func copyToPasteboard(_ text: String) {
        let pb = NSPasteboard.general
        pb.clearContents()
        pb.setString(text, forType: .string)
    }

    private func openIPv6Check() {
        let url = ProbeEndpoints().mainBaseURL.appendingPathComponent("ipv6-leak-test/")
        NSWorkspace.shared.open(url)
    }

    private func openWebHome() {
        NSWorkspace.shared.open(ProbeEndpoints().mainBaseURL)
    }

    private func toggleLaunchAtLogin() {
        guard isBundledApp else { return }
        do {
            if SMAppService.mainApp.status == .enabled {
                try SMAppService.mainApp.unregister()
            } else {
                try SMAppService.mainApp.register()
            }
        } catch {
            NSSound.beep()
        }
        syncPopoverContent(forceRebuild: true)
    }

    private func quitApp() {
        NSApplication.shared.terminate(nil)
    }
}

@MainActor
final class PopoverContentViewController: NSViewController {
    let contentView = PopoverContentView()

    override func loadView() {
        view = contentView
    }

    func configure(
        snapshot: EgressSnapshot?,
        isRefreshing: Bool,
        launchAtLoginEnabled: Bool,
        launchAtLoginAvailable: Bool,
        forceRebuild: Bool,
        onCopyIP: @escaping (String) -> Void,
        onOpenIPv6Check: @escaping () -> Void,
        onRetry: @escaping () -> Void,
        onOpenWebHome: @escaping () -> Void,
        onToggleLaunchAtLogin: @escaping () -> Void,
        onSilentRefresh: @escaping () -> Void,
        onClosePopover: @escaping () -> Void,
        onQuit: @escaping () -> Void
    ) {
        contentView.configure(
            snapshot: snapshot,
            isRefreshing: isRefreshing,
            launchAtLoginEnabled: launchAtLoginEnabled,
            launchAtLoginAvailable: launchAtLoginAvailable,
            forceRebuild: forceRebuild,
            onCopyIP: onCopyIP,
            onOpenIPv6Check: onOpenIPv6Check,
            onRetry: onRetry,
            onOpenWebHome: onOpenWebHome,
            onToggleLaunchAtLogin: onToggleLaunchAtLogin,
            onSilentRefresh: onSilentRefresh,
            onClosePopover: onClosePopover,
            onQuit: onQuit
        )
    }
}

/// 极简两区弹窗视图：上半部分状态显示区 + 单一细分隔线 + 下半部分三行操作区。
@MainActor
final class PopoverContentView: NSView {
    static let panelWidth: CGFloat = 292

    private struct RenderSignature: Equatable {
        let snapshot: EgressSnapshot?
        let isRefreshingWhileNil: Bool
        let launchAtLoginEnabled: Bool
        let launchAtLoginAvailable: Bool
    }

    private let previewBackgroundColor: NSColor?
    private let displayZone = NSView()
    private let dividerView = HairlineDividerView()
    private let actionZone = NSView()
    private weak var copiedToastLabel: NSTextField?
    private var toastResetWorkItem: DispatchWorkItem?
    private var lastSignature: RenderSignature?

    private var onSilentRefresh: (() -> Void)?
    private var onClosePopover: (() -> Void)?
    private var onQuit: (() -> Void)?

    init(previewBackgroundColor: NSColor? = nil) {
        self.previewBackgroundColor = previewBackgroundColor
        super.init(frame: NSRect(x: 0, y: 0, width: Self.panelWidth, height: 168))
        translatesAutoresizingMaskIntoConstraints = false
        setupSkeleton()
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) {
        fatalError()
    }

    static func make(
        snapshot: EgressSnapshot?,
        isRefreshing: Bool,
        launchAtLoginEnabled: Bool,
        launchAtLoginAvailable: Bool,
        previewBackgroundColor: NSColor? = nil,
        onCopyIP: @escaping (String) -> Void = { _ in },
        onOpenIPv6Check: @escaping () -> Void = {},
        onRetry: @escaping () -> Void = {},
        onOpenWebHome: @escaping () -> Void = {},
        onToggleLaunchAtLogin: @escaping () -> Void = {},
        onSilentRefresh: @escaping () -> Void = {},
        onClosePopover: @escaping () -> Void = {},
        onQuit: @escaping () -> Void = {}
    ) -> PopoverContentView {
        let view = PopoverContentView(previewBackgroundColor: previewBackgroundColor)
        view.configure(
            snapshot: snapshot,
            isRefreshing: isRefreshing,
            launchAtLoginEnabled: launchAtLoginEnabled,
            launchAtLoginAvailable: launchAtLoginAvailable,
            forceRebuild: true,
            onCopyIP: onCopyIP,
            onOpenIPv6Check: onOpenIPv6Check,
            onRetry: onRetry,
            onOpenWebHome: onOpenWebHome,
            onToggleLaunchAtLogin: onToggleLaunchAtLogin,
            onSilentRefresh: onSilentRefresh,
            onClosePopover: onClosePopover,
            onQuit: onQuit
        )
        view.frame = NSRect(origin: .zero, size: view.fittingPanelSize())
        return view
    }

    private func setupSkeleton() {
        displayZone.translatesAutoresizingMaskIntoConstraints = false
        dividerView.translatesAutoresizingMaskIntoConstraints = false
        actionZone.translatesAutoresizingMaskIntoConstraints = false

        addSubview(displayZone)
        addSubview(dividerView)
        addSubview(actionZone)

        NSLayoutConstraint.activate([
            widthAnchor.constraint(equalToConstant: Self.panelWidth),

            displayZone.topAnchor.constraint(equalTo: topAnchor),
            displayZone.leadingAnchor.constraint(equalTo: leadingAnchor),
            displayZone.trailingAnchor.constraint(equalTo: trailingAnchor),

            dividerView.topAnchor.constraint(equalTo: displayZone.bottomAnchor),
            dividerView.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 14),
            dividerView.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -14),
            dividerView.heightAnchor.constraint(equalToConstant: 1),

            actionZone.topAnchor.constraint(equalTo: dividerView.bottomAnchor),
            actionZone.leadingAnchor.constraint(equalTo: leadingAnchor),
            actionZone.trailingAnchor.constraint(equalTo: trailingAnchor),
            actionZone.bottomAnchor.constraint(equalTo: bottomAnchor),
        ])
    }

    override func draw(_ dirtyRect: NSRect) {
        if let previewBackgroundColor {
            previewBackgroundColor.setFill()
            dirtyRect.fill()
        }
        super.draw(dirtyRect)
    }

    override var acceptsFirstResponder: Bool { true }

    override func performKeyEquivalent(with event: NSEvent) -> Bool {
        let flags = event.modifierFlags.intersection(.deviceIndependentFlagsMask)
        if flags == .command, let chars = event.charactersIgnoringModifiers?.lowercased() {
            if chars == "q" {
                onQuit?()
                return true
            }
            if chars == "r" {
                onSilentRefresh?()
                return true
            }
        }
        if event.keyCode == 53 {
            onClosePopover?()
            return true
        }
        return super.performKeyEquivalent(with: event)
    }

    override func cancelOperation(_ sender: Any?) {
        onClosePopover?()
    }

    func fittingPanelSize() -> NSSize {
        layoutSubtreeIfNeeded()
        return NSSize(width: Self.panelWidth, height: ceil(fittingSize.height))
    }

    func configure(
        snapshot: EgressSnapshot?,
        isRefreshing: Bool,
        launchAtLoginEnabled: Bool,
        launchAtLoginAvailable: Bool,
        forceRebuild: Bool,
        onCopyIP: @escaping (String) -> Void,
        onOpenIPv6Check: @escaping () -> Void,
        onRetry: @escaping () -> Void,
        onOpenWebHome: @escaping () -> Void,
        onToggleLaunchAtLogin: @escaping () -> Void,
        onSilentRefresh: @escaping () -> Void,
        onClosePopover: @escaping () -> Void,
        onQuit: @escaping () -> Void
    ) {
        self.onSilentRefresh = onSilentRefresh
        self.onClosePopover = onClosePopover
        self.onQuit = onQuit

        let sig = RenderSignature(
            snapshot: snapshot,
            isRefreshingWhileNil: snapshot == nil && isRefreshing,
            launchAtLoginEnabled: launchAtLoginEnabled,
            launchAtLoginAvailable: launchAtLoginAvailable
        )
        guard forceRebuild || sig != lastSignature else { return }
        lastSignature = sig

        rebuildDisplayZone(
            snapshot: snapshot,
            isRefreshing: isRefreshing,
            onCopyIP: onCopyIP,
            onOpenIPv6Check: onOpenIPv6Check,
            onRetry: onRetry
        )
        rebuildActionZone(
            launchAtLoginEnabled: launchAtLoginEnabled,
            launchAtLoginAvailable: launchAtLoginAvailable,
            onOpenWebHome: onOpenWebHome,
            onToggleLaunchAtLogin: onToggleLaunchAtLogin,
            onQuit: onQuit
        )
    }

    private func rebuildDisplayZone(
        snapshot: EgressSnapshot?,
        isRefreshing: Bool,
        onCopyIP: @escaping (String) -> Void,
        onOpenIPv6Check: @escaping () -> Void,
        onRetry: @escaping () -> Void
    ) {
        toastResetWorkItem?.cancel()
        toastResetWorkItem = nil
        copiedToastLabel = nil
        displayZone.subviews.forEach { $0.removeFromSuperview() }

        let primary = snapshot?.activeV4 ?? snapshot?.directV6

        guard let primary else {
            let titleField = makeLabel(
                snapshot == nil && isRefreshing ? "正在检测当前出口…" : "无法连接到 DetectRadar",
                font: .systemFont(ofSize: 13, weight: .semibold),
                color: snapshot == nil && isRefreshing ? .secondaryLabelColor : .labelColor
            )
            let subtitleField = makeLabel(
                snapshot == nil && isRefreshing ? "DetectRadar" : (snapshot?.errorMessage ?? "请检查网络连接"),
                font: .systemFont(ofSize: 11.5, weight: .regular),
                color: snapshot == nil && isRefreshing ? .tertiaryLabelColor : .secondaryLabelColor
            )
            displayZone.addSubview(titleField)
            displayZone.addSubview(subtitleField)

            var constraints = [
                titleField.topAnchor.constraint(equalTo: displayZone.topAnchor, constant: 13),
                titleField.leadingAnchor.constraint(equalTo: displayZone.leadingAnchor, constant: 14),
                subtitleField.topAnchor.constraint(equalTo: titleField.bottomAnchor, constant: 5),
                subtitleField.leadingAnchor.constraint(equalTo: displayZone.leadingAnchor, constant: 14),
                subtitleField.trailingAnchor.constraint(lessThanOrEqualTo: displayZone.trailingAnchor, constant: -14),
                subtitleField.bottomAnchor.constraint(equalTo: displayZone.bottomAnchor, constant: -12),
            ]

            if !(snapshot == nil && isRefreshing) {
                let retryButton = RetryPillButton(title: "重试", onAction: onRetry)
                displayZone.addSubview(retryButton)
                constraints.append(contentsOf: [
                    retryButton.trailingAnchor.constraint(equalTo: displayZone.trailingAnchor, constant: -14),
                    retryButton.centerYAnchor.constraint(equalTo: titleField.centerYAnchor),
                    titleField.trailingAnchor.constraint(lessThanOrEqualTo: retryButton.leadingAnchor, constant: -8),
                ])
            } else {
                constraints.append(
                    titleField.trailingAnchor.constraint(lessThanOrEqualTo: displayZone.trailingAnchor, constant: -14)
                )
            }
            NSLayoutConstraint.activate(constraints)
            return
        }

        let countryField = makeLabel(
            IPMasker.countryBadgeLine(for: primary),
            font: .systemFont(ofSize: 13.5, weight: .semibold),
            color: .labelColor
        )
        countryField.setContentHuggingPriority(.required, for: .horizontal)
        countryField.setContentCompressionResistancePriority(.defaultHigh, for: .horizontal)

        let usageBadge = UsageBadgeView(intel: primary)
        let isIPv6Primary = primary.ip.contains(":")
        let ipButton = IPCopyButton(ip: primary.ip, truncatesMiddle: isIPv6Primary) { [weak self] in
            onCopyIP(primary.ip)
            self?.flashCopiedToast()
        }

        let providerField = makeLabel(
            IPMasker.providerDetailLine(for: primary),
            font: .systemFont(ofSize: 11.5, weight: .regular),
            color: .secondaryLabelColor
        )
        providerField.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)

        let toastField = makeLabel(
            "✓ 已复制",
            font: .systemFont(ofSize: 11, weight: .medium),
            color: .systemGreen
        )
        toastField.alphaValue = 0
        toastField.setContentHuggingPriority(.required, for: .horizontal)
        toastField.setContentCompressionResistancePriority(.required, for: .horizontal)
        copiedToastLabel = toastField

        displayZone.addSubview(countryField)
        displayZone.addSubview(usageBadge)
        displayZone.addSubview(ipButton)
        displayZone.addSubview(providerField)
        displayZone.addSubview(toastField)

        var constraints: [NSLayoutConstraint] = [
            countryField.topAnchor.constraint(equalTo: displayZone.topAnchor, constant: 13),
            countryField.leadingAnchor.constraint(equalTo: displayZone.leadingAnchor, constant: 14),

            usageBadge.trailingAnchor.constraint(equalTo: displayZone.trailingAnchor, constant: -14),
            usageBadge.centerYAnchor.constraint(equalTo: countryField.centerYAnchor),
        ]

        let row2TopAnchor: NSLayoutYAxisAnchor
        if isIPv6Primary {
            constraints.append(contentsOf: [
                countryField.trailingAnchor.constraint(lessThanOrEqualTo: usageBadge.leadingAnchor, constant: -8),
                ipButton.topAnchor.constraint(equalTo: countryField.bottomAnchor, constant: 5),
                ipButton.leadingAnchor.constraint(equalTo: displayZone.leadingAnchor, constant: 10),
                ipButton.trailingAnchor.constraint(lessThanOrEqualTo: displayZone.trailingAnchor, constant: -10),
            ])
            row2TopAnchor = ipButton.bottomAnchor
        } else {
            constraints.append(contentsOf: [
                ipButton.leadingAnchor.constraint(equalTo: countryField.trailingAnchor, constant: 5),
                ipButton.centerYAnchor.constraint(equalTo: countryField.centerYAnchor),
                ipButton.trailingAnchor.constraint(lessThanOrEqualTo: usageBadge.leadingAnchor, constant: -8),
            ])
            row2TopAnchor = countryField.bottomAnchor
        }

        constraints.append(contentsOf: [
            providerField.topAnchor.constraint(equalTo: row2TopAnchor, constant: 5),
            providerField.leadingAnchor.constraint(equalTo: displayZone.leadingAnchor, constant: 14),
            providerField.trailingAnchor.constraint(lessThanOrEqualTo: toastField.leadingAnchor, constant: -8),

            toastField.trailingAnchor.constraint(equalTo: displayZone.trailingAnchor, constant: -14),
            toastField.centerYAnchor.constraint(equalTo: providerField.centerYAnchor),
        ])

        if let snap = snapshot, case .leaked(let carrier, _) = snap.ipv6Leak {
            let leakRow = IPv6LeakRowView(carrier: carrier, onAction: onOpenIPv6Check)
            displayZone.addSubview(leakRow)
            constraints.append(contentsOf: [
                leakRow.topAnchor.constraint(equalTo: providerField.bottomAnchor, constant: 6),
                leakRow.leadingAnchor.constraint(equalTo: displayZone.leadingAnchor, constant: 6),
                leakRow.trailingAnchor.constraint(equalTo: displayZone.trailingAnchor, constant: -6),
                leakRow.bottomAnchor.constraint(equalTo: displayZone.bottomAnchor, constant: -6),
            ])
        } else {
            constraints.append(
                providerField.bottomAnchor.constraint(equalTo: displayZone.bottomAnchor, constant: -12)
            )
        }

        NSLayoutConstraint.activate(constraints)
    }

    private func rebuildActionZone(
        launchAtLoginEnabled: Bool,
        launchAtLoginAvailable: Bool,
        onOpenWebHome: @escaping () -> Void,
        onToggleLaunchAtLogin: @escaping () -> Void,
        onQuit: @escaping () -> Void
    ) {
        actionZone.subviews.forEach { $0.removeFromSuperview() }

        let webRow = PopoverActionRow(
            title: "网页完整检测",
            shortcut: nil,
            isEnabled: true,
            onAction: onOpenWebHome
        )

        let loginRow = PopoverToggleRow(
            title: "开机自启",
            isOn: launchAtLoginEnabled,
            isEnabled: launchAtLoginAvailable,
            onToggle: onToggleLaunchAtLogin
        )
        if !launchAtLoginAvailable {
            loginRow.toolTip = "需以 DetectRadarBar.app 运行后支持开机自启"
        }

        let quitRow = PopoverActionRow(
            title: "退出",
            shortcut: "⌘Q",
            isEnabled: true,
            onAction: onQuit
        )

        actionZone.addSubview(webRow)
        actionZone.addSubview(loginRow)
        actionZone.addSubview(quitRow)

        NSLayoutConstraint.activate([
            webRow.topAnchor.constraint(equalTo: actionZone.topAnchor, constant: 6),
            webRow.leadingAnchor.constraint(equalTo: actionZone.leadingAnchor, constant: 6),
            webRow.trailingAnchor.constraint(equalTo: actionZone.trailingAnchor, constant: -6),

            loginRow.topAnchor.constraint(equalTo: webRow.bottomAnchor, constant: 2),
            loginRow.leadingAnchor.constraint(equalTo: actionZone.leadingAnchor, constant: 6),
            loginRow.trailingAnchor.constraint(equalTo: actionZone.trailingAnchor, constant: -6),

            quitRow.topAnchor.constraint(equalTo: loginRow.bottomAnchor, constant: 2),
            quitRow.leadingAnchor.constraint(equalTo: actionZone.leadingAnchor, constant: 6),
            quitRow.trailingAnchor.constraint(equalTo: actionZone.trailingAnchor, constant: -6),
            quitRow.bottomAnchor.constraint(equalTo: actionZone.bottomAnchor, constant: -6),
        ])
    }

    private func flashCopiedToast() {
        toastResetWorkItem?.cancel()
        copiedToastLabel?.alphaValue = 1

        let item = DispatchWorkItem { [weak self] in
            NSAnimationContext.runAnimationGroup { ctx in
                ctx.duration = 0.2
                self?.copiedToastLabel?.animator().alphaValue = 0
            }
        }
        toastResetWorkItem = item
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.4, execute: item)
    }

    private func makeLabel(_ text: String, font: NSFont, color: NSColor) -> NSTextField {
        let field = NSTextField(labelWithString: text)
        field.font = font
        field.textColor = color
        field.lineBreakMode = .byTruncatingTail
        field.translatesAutoresizingMaskIntoConstraints = false
        return field
    }
}

private final class HairlineDividerView: NSView {
    override func draw(_ dirtyRect: NSRect) {
        NSColor.labelColor.withAlphaComponent(0.11).setFill()
        bounds.fill()
    }
}

@MainActor
private final class UsageBadgeView: NSView {
    private let textField = NSTextField(labelWithString: "")
    private let fillColor: NSColor

    init(intel: IPIntel) {
        let kind = IPMasker.usageShortLabel(for: intel)
        let textColor: NSColor
        switch kind {
        case "宽带":
            self.fillColor = NSColor.systemGreen.withAlphaComponent(0.14)
            textColor = NSColor(name: nil) { appearance in
                let isDark = appearance.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua
                return isDark
                    ? NSColor(srgbRed: 0.45, green: 0.88, blue: 0.56, alpha: 1.0)
                    : NSColor(srgbRed: 0.11, green: 0.52, blue: 0.24, alpha: 1.0)
            }
        case "机房":
            self.fillColor = NSColor(name: nil) { appearance in
                let isDark = appearance.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua
                return isDark
                    ? NSColor.white.withAlphaComponent(0.11)
                    : NSColor.black.withAlphaComponent(0.07)
            }
            textColor = .secondaryLabelColor
        default:
            self.fillColor = NSColor.labelColor.withAlphaComponent(0.06)
            textColor = .tertiaryLabelColor
        }

        super.init(frame: .zero)
        translatesAutoresizingMaskIntoConstraints = false
        setContentHuggingPriority(.required, for: .horizontal)
        setContentCompressionResistancePriority(.required, for: .horizontal)

        textField.stringValue = kind
        textField.font = .systemFont(ofSize: 11, weight: .medium)
        textField.textColor = textColor
        textField.alignment = .center
        textField.setContentHuggingPriority(.required, for: .horizontal)
        textField.setContentCompressionResistancePriority(.required, for: .horizontal)
        textField.translatesAutoresizingMaskIntoConstraints = false
        addSubview(textField)

        NSLayoutConstraint.activate([
            heightAnchor.constraint(equalToConstant: 20),
            textField.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 7),
            textField.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -7),
            textField.centerYAnchor.constraint(equalTo: centerYAnchor),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) {
        fatalError()
    }

    override func draw(_ dirtyRect: NSRect) {
        effectiveAppearance.performAsCurrentDrawingAppearance {
            let path = NSBezierPath(roundedRect: bounds, xRadius: 5, yRadius: 5)
            fillColor.setFill()
            path.fill()
        }
    }
}

@MainActor
private final class SymbolIconView: NSView {
    private var symbolImage: NSImage?
    var tintColor: NSColor {
        didSet { needsDisplay = true }
    }

    init(symbolName: String, pointSize: CGFloat, weight: NSFont.Weight, tintColor: NSColor) {
        self.tintColor = tintColor
        super.init(frame: .zero)
        translatesAutoresizingMaskIntoConstraints = false
        setSymbol(symbolName, pointSize: pointSize, weight: weight)
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) {
        fatalError()
    }

    func setSymbol(_ name: String, pointSize: CGFloat, weight: NSFont.Weight) {
        let cfg = NSImage.SymbolConfiguration(pointSize: pointSize, weight: weight)
        symbolImage = NSImage(systemSymbolName: name, accessibilityDescription: nil)?
            .withSymbolConfiguration(cfg)
        needsDisplay = true
    }

    override func draw(_ dirtyRect: NSRect) {
        guard let symbolImage, let ctx = NSGraphicsContext.current?.cgContext else { return }
        let imgSize = symbolImage.size
        guard imgSize.width > 0, imgSize.height > 0 else { return }

        let scale = min(bounds.width / imgSize.width, bounds.height / imgSize.height, 1.0)
        let drawW = imgSize.width * scale
        let drawH = imgSize.height * scale
        let originX = ((bounds.width - drawW) * 0.5 * 2.0).rounded() * 0.5
        let originY = ((bounds.height - drawH) * 0.5 * 2.0).rounded() * 0.5
        let drawRect = NSRect(x: originX, y: originY, width: drawW, height: drawH)

        effectiveAppearance.performAsCurrentDrawingAppearance {
            NSGraphicsContext.saveGraphicsState()
            ctx.beginTransparencyLayer(auxiliaryInfo: nil)
            symbolImage.draw(in: drawRect, from: .zero, operation: .sourceOver, fraction: 1.0)
            tintColor.setFill()
            drawRect.fill(using: .sourceIn)
            ctx.endTransparencyLayer()
            NSGraphicsContext.restoreGraphicsState()
        }
    }
}

@MainActor
private final class IPCopyButton: NSView {
    private let ipField = NSTextField(labelWithString: "")
    private let iconView: SymbolIconView
    private var tracking: NSTrackingArea?
    private var isHovered = false
    private var resetIconWorkItem: DispatchWorkItem?
    private let onCopy: () -> Void

    init(ip: String, truncatesMiddle: Bool, onCopy: @escaping () -> Void) {
        self.onCopy = onCopy
        self.iconView = SymbolIconView(symbolName: "doc.on.doc", pointSize: 10.5, weight: .medium, tintColor: .tertiaryLabelColor)
        super.init(frame: .zero)
        translatesAutoresizingMaskIntoConstraints = false
        toolTip = "点击复制 IP"

        ipField.stringValue = ip
        ipField.font = .monospacedSystemFont(ofSize: 12, weight: .medium)
        ipField.textColor = .labelColor
        ipField.lineBreakMode = truncatesMiddle ? .byTruncatingMiddle : .byClipping
        ipField.setContentHuggingPriority(.required, for: .horizontal)
        ipField.setContentCompressionResistancePriority(truncatesMiddle ? .defaultLow : .required, for: .horizontal)
        ipField.translatesAutoresizingMaskIntoConstraints = false

        addSubview(ipField)
        addSubview(iconView)

        NSLayoutConstraint.activate([
            heightAnchor.constraint(equalToConstant: 21),

            ipField.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 4),
            ipField.centerYAnchor.constraint(equalTo: centerYAnchor),

            iconView.leadingAnchor.constraint(equalTo: ipField.trailingAnchor, constant: 4),
            iconView.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -4),
            iconView.centerYAnchor.constraint(equalTo: centerYAnchor),
            iconView.widthAnchor.constraint(equalToConstant: 14),
            iconView.heightAnchor.constraint(equalToConstant: 14),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) {
        fatalError()
    }

    override func draw(_ dirtyRect: NSRect) {
        if isHovered {
            NSColor.labelColor.withAlphaComponent(0.08).setFill()
            NSBezierPath(roundedRect: bounds, xRadius: 5, yRadius: 5).fill()
        }
    }

    override func resetCursorRects() {
        addCursorRect(bounds, cursor: .pointingHand)
    }

    override func updateTrackingAreas() {
        super.updateTrackingAreas()
        if let tracking { removeTrackingArea(tracking) }
        let area = NSTrackingArea(
            rect: bounds,
            options: [.mouseEnteredAndExited, .activeAlways, .inVisibleRect],
            owner: self,
            userInfo: nil
        )
        addTrackingArea(area)
        tracking = area
    }

    override func mouseEntered(with event: NSEvent) {
        isHovered = true
        if resetIconWorkItem == nil {
            iconView.tintColor = .secondaryLabelColor
        }
        needsDisplay = true
    }

    override func mouseExited(with event: NSEvent) {
        isHovered = false
        if resetIconWorkItem == nil {
            iconView.tintColor = .tertiaryLabelColor
        }
        needsDisplay = true
    }

    override func mouseUp(with event: NSEvent) {
        guard bounds.contains(convert(event.locationInWindow, from: nil)) else { return }
        onCopy()
        showCopiedFeedback()
    }

    private func showCopiedFeedback() {
        resetIconWorkItem?.cancel()
        setCopyIcon(copied: true)

        let item = DispatchWorkItem { [weak self] in
            guard let self else { return }
            self.resetIconWorkItem = nil
            self.setCopyIcon(copied: false)
        }
        resetIconWorkItem = item
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.4, execute: item)
    }

    private func setCopyIcon(copied: Bool) {
        iconView.setSymbol(
            copied ? "checkmark" : "doc.on.doc",
            pointSize: 10.5,
            weight: copied ? .semibold : .medium
        )
        iconView.tintColor = copied
            ? .systemGreen
            : (isHovered ? .secondaryLabelColor : .tertiaryLabelColor)
    }
}

@MainActor
private class HoverableRowView: NSView {
    private var tracking: NSTrackingArea?
    var isHovered = false {
        didSet { needsDisplay = true }
    }
    let rowEnabled: Bool

    init(isEnabled: Bool = true) {
        self.rowEnabled = isEnabled
        super.init(frame: .zero)
        translatesAutoresizingMaskIntoConstraints = false
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) {
        fatalError()
    }

    override func draw(_ dirtyRect: NSRect) {
        if rowEnabled && isHovered {
            NSColor.labelColor.withAlphaComponent(0.08).setFill()
            NSBezierPath(roundedRect: bounds, xRadius: 6, yRadius: 6).fill()
        }
    }

    override func updateTrackingAreas() {
        super.updateTrackingAreas()
        if let tracking { removeTrackingArea(tracking) }
        let area = NSTrackingArea(
            rect: bounds,
            options: [.mouseEnteredAndExited, .activeAlways, .inVisibleRect],
            owner: self,
            userInfo: nil
        )
        addTrackingArea(area)
        tracking = area
    }

    override func mouseEntered(with event: NSEvent) {
        guard rowEnabled else { return }
        isHovered = true
    }

    override func mouseExited(with event: NSEvent) {
        guard isHovered else { return }
        isHovered = false
    }
}

@MainActor
private final class IPv6LeakRowView: HoverableRowView {
    private let onAction: () -> Void

    init(carrier: String, onAction: @escaping () -> Void) {
        self.onAction = onAction
        super.init(isEnabled: true)
        toolTip = "前往 DetectRadar IPv6 泄露专项页面排查"

        let warnIcon = SymbolIconView(
            symbolName: "exclamationmark.triangle.fill",
            pointSize: 11.5,
            weight: .medium,
            tintColor: .systemOrange
        )

        let textField = NSTextField(labelWithString: "IPv6 泄露 · \(carrier)直连")
        textField.font = .systemFont(ofSize: 12, weight: .medium)
        textField.textColor = .labelColor
        textField.lineBreakMode = .byTruncatingTail
        textField.translatesAutoresizingMaskIntoConstraints = false

        let inspectLabel = NSTextField(labelWithString: "排查")
        inspectLabel.font = .systemFont(ofSize: 11.5, weight: .regular)
        inspectLabel.textColor = .secondaryLabelColor
        inspectLabel.setContentHuggingPriority(.required, for: .horizontal)
        inspectLabel.setContentCompressionResistancePriority(.required, for: .horizontal)
        inspectLabel.translatesAutoresizingMaskIntoConstraints = false

        let chevronIcon = SymbolIconView(
            symbolName: "chevron.right",
            pointSize: 9.5,
            weight: .semibold,
            tintColor: .tertiaryLabelColor
        )

        addSubview(warnIcon)
        addSubview(textField)
        addSubview(inspectLabel)
        addSubview(chevronIcon)

        NSLayoutConstraint.activate([
            heightAnchor.constraint(equalToConstant: 26),

            warnIcon.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 8),
            warnIcon.centerYAnchor.constraint(equalTo: centerYAnchor),
            warnIcon.widthAnchor.constraint(equalToConstant: 14),
            warnIcon.heightAnchor.constraint(equalToConstant: 14),

            textField.leadingAnchor.constraint(equalTo: warnIcon.trailingAnchor, constant: 6),
            textField.centerYAnchor.constraint(equalTo: centerYAnchor),
            textField.trailingAnchor.constraint(lessThanOrEqualTo: inspectLabel.leadingAnchor, constant: -8),

            chevronIcon.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -8),
            chevronIcon.centerYAnchor.constraint(equalTo: centerYAnchor),
            chevronIcon.widthAnchor.constraint(equalToConstant: 8),
            chevronIcon.heightAnchor.constraint(equalToConstant: 11),

            inspectLabel.trailingAnchor.constraint(equalTo: chevronIcon.leadingAnchor, constant: -3),
            inspectLabel.centerYAnchor.constraint(equalTo: centerYAnchor),
        ])
    }

    override func resetCursorRects() {
        addCursorRect(bounds, cursor: .pointingHand)
    }

    override func mouseUp(with event: NSEvent) {
        guard bounds.contains(convert(event.locationInWindow, from: nil)) else { return }
        isHovered = false
        onAction()
    }
}

@MainActor
private final class RetryPillButton: NSView {
    private var tracking: NSTrackingArea?
    private var isHovered = false
    private let onAction: () -> Void

    init(title: String, onAction: @escaping () -> Void) {
        self.onAction = onAction
        super.init(frame: .zero)
        translatesAutoresizingMaskIntoConstraints = false
        setContentHuggingPriority(.required, for: .horizontal)
        setContentCompressionResistancePriority(.required, for: .horizontal)

        let label = NSTextField(labelWithString: title)
        label.font = .systemFont(ofSize: 11, weight: .medium)
        label.textColor = .labelColor
        label.translatesAutoresizingMaskIntoConstraints = false
        addSubview(label)

        NSLayoutConstraint.activate([
            heightAnchor.constraint(equalToConstant: 21),
            label.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 8),
            label.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -8),
            label.centerYAnchor.constraint(equalTo: centerYAnchor),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) {
        fatalError()
    }

    override func draw(_ dirtyRect: NSRect) {
        let path = NSBezierPath(roundedRect: bounds.insetBy(dx: 0.25, dy: 0.25), xRadius: 5, yRadius: 5)
        NSColor.labelColor.withAlphaComponent(isHovered ? 0.14 : 0.08).setFill()
        path.fill()
        NSColor.labelColor.withAlphaComponent(0.12).setStroke()
        path.lineWidth = 0.5
        path.stroke()
    }

    override func resetCursorRects() {
        addCursorRect(bounds, cursor: .pointingHand)
    }

    override func updateTrackingAreas() {
        super.updateTrackingAreas()
        if let tracking { removeTrackingArea(tracking) }
        let area = NSTrackingArea(
            rect: bounds,
            options: [.mouseEnteredAndExited, .activeAlways, .inVisibleRect],
            owner: self,
            userInfo: nil
        )
        addTrackingArea(area)
        tracking = area
    }

    override func mouseEntered(with event: NSEvent) {
        isHovered = true
        needsDisplay = true
    }

    override func mouseExited(with event: NSEvent) {
        isHovered = false
        needsDisplay = true
    }

    override func mouseUp(with event: NSEvent) {
        guard bounds.contains(convert(event.locationInWindow, from: nil)) else { return }
        onAction()
    }
}

@MainActor
private final class PopoverActionRow: HoverableRowView {
    private let onAction: () -> Void

    init(
        title: String,
        shortcut: String?,
        isEnabled: Bool,
        onAction: @escaping () -> Void
    ) {
        self.onAction = onAction
        super.init(isEnabled: isEnabled)

        let titleField = NSTextField(labelWithString: title)
        titleField.font = .systemFont(ofSize: 13, weight: .regular)
        titleField.textColor = isEnabled ? .labelColor : .tertiaryLabelColor
        titleField.lineBreakMode = .byTruncatingTail
        titleField.translatesAutoresizingMaskIntoConstraints = false

        addSubview(titleField)

        var constraints = [
            heightAnchor.constraint(equalToConstant: 28),
            titleField.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 8),
            titleField.centerYAnchor.constraint(equalTo: centerYAnchor),
        ]

        if let shortcut {
            let shortcutField = NSTextField(labelWithString: shortcut)
            shortcutField.font = .systemFont(ofSize: 11.5, weight: .regular)
            shortcutField.textColor = .tertiaryLabelColor
            shortcutField.setContentHuggingPriority(.required, for: .horizontal)
            shortcutField.setContentCompressionResistancePriority(.required, for: .horizontal)
            shortcutField.translatesAutoresizingMaskIntoConstraints = false
            addSubview(shortcutField)
            constraints.append(contentsOf: [
                shortcutField.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -8),
                shortcutField.centerYAnchor.constraint(equalTo: centerYAnchor),
                titleField.trailingAnchor.constraint(lessThanOrEqualTo: shortcutField.leadingAnchor, constant: -8),
            ])
        } else {
            constraints.append(
                titleField.trailingAnchor.constraint(lessThanOrEqualTo: trailingAnchor, constant: -8)
            )
        }

        NSLayoutConstraint.activate(constraints)
    }

    override func mouseUp(with event: NSEvent) {
        guard rowEnabled else { return }
        guard bounds.contains(convert(event.locationInWindow, from: nil)) else { return }
        isHovered = false
        onAction()
    }
}

@MainActor
private final class MiniToggleSwitchView: NSView {
    var isOn: Bool {
        didSet { needsDisplay = true }
    }
    var isEnabled: Bool {
        didSet { needsDisplay = true }
    }

    init(isOn: Bool, isEnabled: Bool) {
        self.isOn = isOn
        self.isEnabled = isEnabled
        super.init(frame: NSRect(x: 0, y: 0, width: 28, height: 16))
        translatesAutoresizingMaskIntoConstraints = false
        setContentHuggingPriority(.required, for: .horizontal)
        setContentCompressionResistancePriority(.required, for: .horizontal)
        NSLayoutConstraint.activate([
            widthAnchor.constraint(equalToConstant: 28),
            heightAnchor.constraint(equalToConstant: 16),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) {
        fatalError()
    }

    override func draw(_ dirtyRect: NSRect) {
        effectiveAppearance.performAsCurrentDrawingAppearance {
            let isDark = effectiveAppearance.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua
            let trackPath = NSBezierPath(roundedRect: bounds, xRadius: 8, yRadius: 8)
            let trackColor: NSColor
            if isOn {
                trackColor = NSColor.controlAccentColor.withAlphaComponent(isEnabled ? 1.0 : 0.45)
            } else {
                let offBase = isDark ? NSColor.white.withAlphaComponent(0.20) : NSColor.black.withAlphaComponent(0.16)
                trackColor = offBase.withAlphaComponent(isEnabled ? (isDark ? 0.20 : 0.16) : 0.08)
            }
            trackColor.setFill()
            trackPath.fill()

            let knobX: CGFloat = isOn ? 14 : 2
            let knobRect = NSRect(x: knobX, y: 2, width: 12, height: 12)
            let shadow = NSShadow()
            shadow.shadowBlurRadius = 2.0
            shadow.shadowOffset = NSSize(width: 0, height: -0.5)
            shadow.shadowColor = NSColor(white: 0, alpha: 0.22)
            shadow.set()

            NSColor.white.withAlphaComponent(isEnabled ? 0.98 : 0.72).setFill()
            NSBezierPath(ovalIn: knobRect).fill()
            NSShadow().set()
        }
    }
}

@MainActor
private final class PopoverToggleRow: HoverableRowView {
    private let switchView: MiniToggleSwitchView
    private let onToggle: () -> Void

    init(
        title: String,
        isOn: Bool,
        isEnabled: Bool,
        onToggle: @escaping () -> Void
    ) {
        self.switchView = MiniToggleSwitchView(isOn: isOn, isEnabled: isEnabled)
        self.onToggle = onToggle
        super.init(isEnabled: isEnabled)

        let titleField = NSTextField(labelWithString: title)
        titleField.font = .systemFont(ofSize: 13, weight: .regular)
        titleField.textColor = isEnabled ? .labelColor : .tertiaryLabelColor
        titleField.lineBreakMode = .byTruncatingTail
        titleField.translatesAutoresizingMaskIntoConstraints = false

        addSubview(titleField)
        addSubview(switchView)

        NSLayoutConstraint.activate([
            heightAnchor.constraint(equalToConstant: 28),

            titleField.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 8),
            titleField.centerYAnchor.constraint(equalTo: centerYAnchor),
            titleField.trailingAnchor.constraint(lessThanOrEqualTo: switchView.leadingAnchor, constant: -8),

            switchView.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -8),
            switchView.centerYAnchor.constraint(equalTo: centerYAnchor),
        ])
    }

    override func mouseUp(with event: NSEvent) {
        guard rowEnabled else { return }
        guard bounds.contains(convert(event.locationInWindow, from: nil)) else { return }
        switchView.isOn.toggle()
        isHovered = false
        onToggle()
    }
}
