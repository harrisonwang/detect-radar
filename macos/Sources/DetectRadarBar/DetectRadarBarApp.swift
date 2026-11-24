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

        let liveSnap = await ProbeClient().runProbe()

        // 1. 干净常态（深色）— 从实际网络实时探测生成
        renderPopoverShot(
            snapshot: liveSnap,
            dark: true,
            url: outDir.appendingPathComponent("detectradar-bar-v4-clean-dark.png")
        )

        // 2. IPv6 国内运营商旁路泄露告警态（深色）
        let usDMIT = IPIntel(
            ip: "104.194.84.113", country: "US", countryName: "United States",
            region: "California", city: "Los Angeles", asn: "AS906",
            org: "DMIT Cloud Services", isp: "DMIT Cloud Services",
            usageType: "hosting", isHosting: true
        )
        let cnV6 = IPIntel(
            ip: "240e:390:100::1234", country: "CN", region: "上海市",
            isp: "China Telecom", usageType: "isp", isHosting: false
        )
        let alertSnap = EgressSnapshot(
            timestamp: Date(),
            activeV4: usDMIT,
            directV6: cnV6,
            systemProxy: ProxyEndpoint(scheme: .http, host: "127.0.0.1", port: 7890, companionSocksPort: 7891),
            ipv6Leak: DualStackEvaluator.evaluateIPv6Leak(activeV4: usDMIT, directV6: cnV6)
        )
        renderPopoverShot(
            snapshot: alertSnap,
            dark: true,
            url: outDir.appendingPathComponent("detectradar-bar-v4-alert-dark.png")
        )

        // 3. 干净常态（浅色）
        renderPopoverShot(
            snapshot: liveSnap,
            dark: false,
            url: outDir.appendingPathComponent("detectradar-bar-v4-clean-light.png")
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

    private static func renderPopoverShot(snapshot: EgressSnapshot, dark: Bool, url: URL) {
        let appearance = NSAppearance(named: dark ? .darkAqua : .aqua)!
        let panelFill = dark
            ? NSColor(srgbRed: 0.155, green: 0.165, blue: 0.185, alpha: 1.0)
            : NSColor(srgbRed: 0.965, green: 0.97, blue: 0.98, alpha: 1.0)

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
        let arrowH: CGFloat = 8
        let gapToBar: CGFloat = 4
        let bottomPad: CGFloat = 24
        let canvasW = panelW + padX * 2
        let canvasH = menuBarH + gapToBar + arrowH + panelH + bottomPad
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
            // 桌面壁纸底色 + 顶栏
            let desktopBg = dark
                ? NSColor(srgbRed: 0.055, green: 0.08, blue: 0.125, alpha: 1.0)
                : NSColor(srgbRed: 0.85, green: 0.885, blue: 0.935, alpha: 1.0)
            desktopBg.setFill()
            NSRect(x: 0, y: 0, width: canvasW, height: canvasH).fill()

            let barColor = dark ? NSColor(white: 0.12, alpha: 0.96) : NSColor(white: 0.96, alpha: 0.96)
            barColor.setFill()
            NSRect(x: 0, y: canvasH - menuBarH, width: canvasW, height: menuBarH).fill()

            NSColor(white: dark ? 1 : 0, alpha: dark ? 0.08 : 0.08).setFill()
            NSRect(x: 0, y: canvasH - menuBarH, width: canvasW, height: 0.5).fill()

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
            let pillX = canvasW - padX - pillW - 6
            let pillY = canvasH - menuBarH + (menuBarH - pillH) / 2
            (dark ? NSColor(white: 1, alpha: 0.18) : NSColor(white: 0, alpha: 0.10)).setFill()
            NSBezierPath(roundedRect: NSRect(x: pillX, y: pillY, width: pillW, height: pillH), xRadius: 5, yRadius: 5).fill()
            titleStr.draw(at: NSPoint(x: pillX + 8, y: pillY + (pillH - ts.height) / 2))

            let clockStr = NSAttributedString(
                string: "Tue 14:18",
                attributes: [
                    .font: NSFont.systemFont(ofSize: 12.5, weight: .medium),
                    .foregroundColor: dark ? NSColor(white: 0.84, alpha: 1) : NSColor(white: 0.25, alpha: 1),
                ]
            )
            clockStr.draw(at: NSPoint(x: pillX - clockStr.size().width - 14, y: canvasH - menuBarH + (menuBarH - clockStr.size().height) / 2))

            // NSPopover 气泡路径（带顶部指向状态栏按钮的尖角）
            let panelRect = NSRect(x: padX, y: bottomPad, width: panelW, height: panelH)
            let arrowCenterX = pillX + pillW / 2
            let popoverPath = makePopoverBubblePath(rect: panelRect, cornerRadius: 11, arrowTipX: arrowCenterX, arrowHeight: arrowH)

            let shadow = NSShadow()
            shadow.shadowBlurRadius = 20
            shadow.shadowOffset = NSSize(width: 0, height: -5)
            shadow.shadowColor = NSColor(white: 0, alpha: dark ? 0.56 : 0.22)
            shadow.set()
            panelFill.setFill()
            popoverPath.fill()
            NSShadow().set()

            NSGraphicsContext.saveGraphicsState()
            popoverPath.addClip()
            panelRep.draw(in: panelRect)
            NSGraphicsContext.restoreGraphicsState()

            NSColor(white: dark ? 1 : 0, alpha: dark ? 0.16 : 0.12).setStroke()
            popoverPath.lineWidth = 1
            popoverPath.stroke()
        }
        NSGraphicsContext.restoreGraphicsState()

        if let png = canvasRep.representation(using: .png, properties: [:]) {
            try? png.write(to: url)
            print("Saved preview: \(url.path) (\(Int(panelW))x\(Int(panelH))@2x)")
        }
    }

    private static func makePopoverBubblePath(rect: NSRect, cornerRadius r: CGFloat, arrowTipX: CGFloat, arrowHeight: CGFloat) -> NSBezierPath {
        let halfW: CGFloat = 9
        let minX = rect.minX
        let maxX = rect.maxX
        let minY = rect.minY
        let maxY = rect.maxY
        let clampedTipX = min(max(arrowTipX, minX + r + halfW + 4), maxX - r - halfW - 4)

        let path = NSBezierPath()
        path.move(to: NSPoint(x: minX + r, y: minY))
        path.line(to: NSPoint(x: maxX - r, y: minY))
        path.appendArc(withCenter: NSPoint(x: maxX - r, y: minY + r), radius: r, startAngle: -90, endAngle: 0, clockwise: false)
        path.line(to: NSPoint(x: maxX, y: maxY - r))
        path.appendArc(withCenter: NSPoint(x: maxX - r, y: maxY - r), radius: r, startAngle: 0, endAngle: 90, clockwise: false)
        path.line(to: NSPoint(x: clampedTipX + halfW, y: maxY))
        path.line(to: NSPoint(x: clampedTipX, y: maxY + arrowHeight))
        path.line(to: NSPoint(x: clampedTipX - halfW, y: maxY))
        path.line(to: NSPoint(x: minX + r, y: maxY))
        path.appendArc(withCenter: NSPoint(x: minX + r, y: maxY - r), radius: r, startAngle: 90, endAngle: 180, clockwise: false)
        path.line(to: NSPoint(x: minX, y: minY + r))
        path.appendArc(withCenter: NSPoint(x: minX + r, y: minY + r), radius: r, startAngle: 180, endAngle: 270, clockwise: false)
        path.close()
        return path
    }
}
#endif
