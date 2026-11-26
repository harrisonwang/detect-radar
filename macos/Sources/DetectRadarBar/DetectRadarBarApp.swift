import AppKit
import DetectRadarCore
import Foundation

@main
@MainActor
struct DetectRadarBarMain {
    static func main() async {
        if CommandLine.arguments.contains("--smoke-probe") {
            let snap = await ProbeClient().runProbe()
            print("MenuBarTitle: \(IPMasker.menuBarTitle(snapshot: snap, isLoading: false))")
            if let primary = snap.activeV4 ?? snap.directV6 {
                print("Country: \(IPMasker.countryBadgeLine(for: primary))")
                print("Provider: \(IPMasker.providerDetailLine(for: primary))")
                print("Usage: \(IPMasker.usageShortLabel(for: primary))")
                print("IP: \(IPMasker.maskIP(primary.ip))")
                print("IPv6Leak: \(snap.ipv6Leak.isLeaked)")
            } else {
                print("Error: \(snap.errorMessage ?? "offline")")
            }
            return
        }

        #if DEBUG
        if let idx = CommandLine.arguments.firstIndex(of: "--render-preview"),
           idx + 1 < CommandLine.arguments.count {
            let outDir = URL(fileURLWithPath: CommandLine.arguments[idx + 1], isDirectory: true)
            await PopoverPreviewRenderer.renderAll(to: outDir)
            return
        }
        #endif

        let app = NSApplication.shared
        let delegate = AppDelegate()
        app.delegate = delegate
        app.setActivationPolicy(.accessory)
        app.run()
    }
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    private let controller = StatusBarController()

    func applicationDidFinishLaunching(_ notification: Notification) {
        controller.start()
    }
}

#if DEBUG
@MainActor
private enum PopoverPreviewRenderer {
    static func renderAll(to outDir: URL) async {
        let app = NSApplication.shared
        app.setActivationPolicy(.accessory)

        let sampleUS = IPIntel(
            ip: "203.0.113.42", country: "US", countryName: "United States",
            region: "California", city: "Los Angeles", asn: "AS14061",
            org: "DigitalOcean", isp: "DigitalOcean",
            usageType: "hosting", isHosting: true
        )
        let sampleCNv6 = IPIntel(
            ip: "2001:db8:390::1", country: "CN", region: "上海市",
            isp: "China Telecom", usageType: "isp", isHosting: false
        )
        let proxy = ProxyEndpoint(scheme: .http, host: "127.0.0.1", port: 7890, companionSocksPort: 7891)

        let cleanSnap = EgressSnapshot(
            timestamp: Date(),
            activeV4: sampleUS,
            directV6: nil,
            systemProxy: proxy,
            ipv6Leak: DualStackEvaluator.evaluateIPv6Leak(activeV4: sampleUS, directV6: nil)
        )
        let alertSnap = EgressSnapshot(
            timestamp: Date(),
            activeV4: sampleUS,
            directV6: sampleCNv6,
            systemProxy: proxy,
            ipv6Leak: DualStackEvaluator.evaluateIPv6Leak(activeV4: sampleUS, directV6: sampleCNv6)
        )

        let unifiedCanvasH: CGFloat = 251

        renderPopoverShot(
            snapshot: cleanSnap,
            dark: true,
            minCanvasHeight: unifiedCanvasH,
            url: outDir.appendingPathComponent("macos-bar-preview-dark-v3.png")
        )
        renderPopoverShot(
            snapshot: alertSnap,
            dark: true,
            minCanvasHeight: unifiedCanvasH,
            url: outDir.appendingPathComponent("macos-bar-preview-alert-v3.png")
        )
        renderPopoverShot(
            snapshot: cleanSnap,
            dark: false,
            minCanvasHeight: unifiedCanvasH,
            url: outDir.appendingPathComponent("macos-bar-preview-light-v3.png")
        )
    }

    private static func applyContentsScale(_ scale: CGFloat, to view: NSView) {
        if let layer = view.layer {
            layer.contentsScale = scale
            layer.sublayers?.forEach { $0.contentsScale = scale }
        }
        for sub in view.subviews {
            applyContentsScale(scale, to: sub)
        }
    }

    private static func captureRetinaView(_ view: NSView, size: NSSize, appearance: NSAppearance) -> NSBitmapImageRep? {
        let scale: CGFloat = 2.0
        guard let rep = NSBitmapImageRep(
            bitmapDataPlanes: nil,
            pixelsWide: Int(ceil(size.width * scale)),
            pixelsHigh: Int(ceil(size.height * scale)),
            bitsPerSample: 8,
            samplesPerPixel: 4,
            hasAlpha: true,
            isPlanar: false,
            colorSpaceName: .deviceRGB,
            bytesPerRow: 0,
            bitsPerPixel: 0
        ) else {
            return nil
        }
        rep.size = size

        view.appearance = appearance
        view.frame = NSRect(origin: .zero, size: size)

        let win = NSWindow(
            contentRect: NSRect(x: -4000, y: -4000, width: size.width, height: size.height),
            styleMask: [.borderless],
            backing: .buffered,
            defer: false
        )
        win.appearance = appearance
        win.isOpaque = true
        win.backgroundColor = .clear
        win.contentView = view
        win.orderFrontRegardless()
        RunLoop.main.run(until: Date().addingTimeInterval(0.06))
        view.layoutSubtreeIfNeeded()
        applyContentsScale(scale, to: view)

        appearance.performAsCurrentDrawingAppearance {
            view.cacheDisplay(in: view.bounds, to: rep)
        }
        win.close()
        return rep
    }

    private static func renderPopoverShot(snapshot: EgressSnapshot, dark: Bool, minCanvasHeight: CGFloat, url: URL) {
        let appearance = NSAppearance(named: dark ? .darkAqua : .aqua)!
        let panelFill = dark
            ? NSColor(srgbRed: 0.145, green: 0.155, blue: 0.175, alpha: 0.94)
            : NSColor(srgbRed: 0.985, green: 0.988, blue: 0.995, alpha: 0.95)

        let popoverView = PopoverContentView.make(
            snapshot: snapshot,
            isRefreshing: false,
            launchAtLoginEnabled: true,
            launchAtLoginAvailable: true,
            previewBackgroundColor: panelFill
        )
        let panelW = PopoverContentView.panelWidth
        let panelH = popoverView.frame.height
        guard let panelRep = captureRetinaView(
            popoverView,
            size: NSSize(width: panelW, height: panelH),
            appearance: appearance
        ) else {
            return
        }

        let padX: CGFloat = 22
        let menuBarH: CGFloat = 28
        let gapToBar: CGFloat = 6
        let bottomPad: CGFloat = 24
        let cornerRadius: CGFloat = 16
        let canvasW = panelW + padX * 2
        let canvasH = max(minCanvasHeight, menuBarH + gapToBar + panelH + bottomPad)
        let scale: CGFloat = 2.0

        guard let canvasRep = NSBitmapImageRep(
            bitmapDataPlanes: nil,
            pixelsWide: Int(ceil(canvasW * scale)),
            pixelsHigh: Int(ceil(canvasH * scale)),
            bitsPerSample: 8,
            samplesPerPixel: 4,
            hasAlpha: true,
            isPlanar: false,
            colorSpaceName: .deviceRGB,
            bytesPerRow: 0,
            bitsPerPixel: 0
        ) else {
            return
        }
        canvasRep.size = NSSize(width: canvasW, height: canvasH)

        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: canvasRep)
        appearance.performAsCurrentDrawingAppearance {
            let desktopBg = dark
                ? NSColor(srgbRed: 0.055, green: 0.08, blue: 0.125, alpha: 1.0)
                : NSColor(srgbRed: 0.86, green: 0.895, blue: 0.945, alpha: 1.0)
            desktopBg.setFill()
            NSRect(x: 0, y: 0, width: canvasW, height: canvasH).fill()

            let barColor = dark ? NSColor(white: 0.12, alpha: 0.92) : NSColor(white: 0.97, alpha: 0.92)
            barColor.setFill()
            NSRect(x: 0, y: canvasH - menuBarH, width: canvasW, height: menuBarH).fill()

            NSColor(white: dark ? 1 : 0, alpha: dark ? 0.08 : 0.06).setFill()
            NSRect(x: 0, y: canvasH - menuBarH, width: canvasW, height: 0.5).fill()

            let clockStr = NSAttributedString(
                string: "周二 14:18",
                attributes: [
                    .font: NSFont.systemFont(ofSize: 12.5, weight: .medium),
                    .foregroundColor: dark ? NSColor(white: 0.78, alpha: 1) : NSColor(white: 0.25, alpha: 1),
                ]
            )
            let clockSize = clockStr.size()
            let clockX = canvasW - padX - clockSize.width
            clockStr.draw(at: NSPoint(x: clockX, y: canvasH - menuBarH + (menuBarH - clockSize.height) / 2))

            let titleStr = NSAttributedString(
                string: IPMasker.menuBarTitle(snapshot: snapshot, isLoading: false),
                attributes: [
                    .font: NSFont.monospacedDigitSystemFont(ofSize: 13, weight: .medium),
                    .foregroundColor: dark ? NSColor.white : NSColor.black,
                ]
            )
            let ts = titleStr.size()
            let pillW = ts.width + 16
            let pillH: CGFloat = 20
            let pillX = clockX - pillW - 10
            let pillY = canvasH - menuBarH + (menuBarH - pillH) / 2
            (dark ? NSColor(white: 1, alpha: 0.18) : NSColor(white: 0, alpha: 0.10)).setFill()
            NSBezierPath(roundedRect: NSRect(x: pillX, y: pillY, width: pillW, height: pillH), xRadius: 5, yRadius: 5).fill()
            titleStr.draw(at: NSPoint(x: pillX + 8, y: pillY + (pillH - ts.height) / 2))

            let panelTopY = canvasH - menuBarH - gapToBar
            let panelRect = NSRect(x: padX, y: panelTopY - panelH, width: panelW, height: panelH)
            let panelPath = NSBezierPath(roundedRect: panelRect, xRadius: cornerRadius, yRadius: cornerRadius)

            let shadow = NSShadow()
            shadow.shadowBlurRadius = 22
            shadow.shadowOffset = NSSize(width: 0, height: -6)
            shadow.shadowColor = NSColor(white: 0, alpha: dark ? 0.52 : 0.16)
            shadow.set()
            panelFill.setFill()
            panelPath.fill()
            NSShadow().set()

            NSGraphicsContext.saveGraphicsState()
            panelPath.addClip()
            panelRep.draw(in: panelRect)
            NSGraphicsContext.restoreGraphicsState()

            NSColor(white: dark ? 1 : 0, alpha: dark ? 0.18 : 0.08).setStroke()
            panelPath.lineWidth = 0.75
            panelPath.stroke()
            if !dark {
                let innerRect = panelRect.insetBy(dx: 0.75, dy: 0.75)
                let innerPath = NSBezierPath(roundedRect: innerRect, xRadius: cornerRadius - 0.75, yRadius: cornerRadius - 0.75)
                NSColor.white.withAlphaComponent(0.75).setStroke()
                innerPath.lineWidth = 0.75
                innerPath.stroke()
            }
        }
        NSGraphicsContext.restoreGraphicsState()

        if let png = canvasRep.representation(using: .png, properties: [:]) {
            try? png.write(to: url)
            print("Saved preview: \(url.path) (\(Int(panelW))x\(Int(panelH))@2x)")
        }
    }
}
#endif
