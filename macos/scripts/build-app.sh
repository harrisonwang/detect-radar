#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
APP_NAME="DetectRadarBar"
VERSION="${VERSION:-0.1.1}"
BUILD_NUMBER="${BUILD_NUMBER:-2}"
BUNDLE_ID="com.detectradar.bar"
ARCH="${ARCH:-$(uname -m)}"

cd "$ROOT_DIR"

echo "==> Building $APP_NAME ($ARCH)..."
if [[ "$ARCH" == "universal" ]]; then
  swift build -c release --arch arm64 --arch x86_64
  BIN_PATH="$(swift build -c release --arch arm64 --arch x86_64 --show-bin-path)/$APP_NAME"
  if [[ ! -x "$BIN_PATH" && -x "$ROOT_DIR/.build/apple/Products/Release/$APP_NAME" ]]; then
    BIN_PATH="$ROOT_DIR/.build/apple/Products/Release/$APP_NAME"
  fi
else
  swift build -c release
  BIN_PATH="$(swift build -c release --show-bin-path)/$APP_NAME"
fi

if [[ ! -x "$BIN_PATH" ]]; then
  echo "Error: executable not found at $BIN_PATH" >&2
  exit 1
fi

DIST_DIR="$ROOT_DIR/dist"
APP_DIR="$DIST_DIR/$APP_NAME.app"
CONTENTS_DIR="$APP_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"

rm -rf "$APP_DIR"
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR"
cp "$BIN_PATH" "$MACOS_DIR/$APP_NAME"
chmod +x "$MACOS_DIR/$APP_NAME"

SDK_VERSION="$(xcrun --show-sdk-version)"
xcrun vtool -set-build-version macos 13.0 "$SDK_VERSION" -replace -output "$MACOS_DIR/$APP_NAME" "$MACOS_DIR/$APP_NAME"

ICONSET_DIR="$(mktemp -d /tmp/detectradar-icon-XXXX).iconset"
mkdir -p "$ICONSET_DIR"
swift - "$ICONSET_DIR" <<'SWIFT'
import AppKit
import CoreGraphics

let outDir = CommandLine.arguments[1]
let sizes: [(String, Int)] = [
    ("icon_16x16.png", 16),
    ("icon_16x16@2x.png", 32),
    ("icon_32x32.png", 32),
    ("icon_32x32@2x.png", 64),
    ("icon_128x128.png", 128),
    ("icon_128x128@2x.png", 256),
    ("icon_256x256.png", 256),
    ("icon_256x256@2x.png", 512),
    ("icon_512x512.png", 512),
    ("icon_512x512@2x.png", 1024),
]

let cs = CGColorSpace(name: CGColorSpace.sRGB)!

for (name, px) in sizes {
    let s = CGFloat(px)
    let rep = NSBitmapImageRep(
        bitmapDataPlanes: nil, pixelsWide: px, pixelsHigh: px,
        bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
        colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0
    )!
    NSGraphicsContext.saveGraphicsState()
    let nsCtx = NSGraphicsContext(bitmapImageRep: rep)!
    NSGraphicsContext.current = nsCtx
    let ctx = nsCtx.cgContext
    ctx.setAllowsAntialiasing(true)
    ctx.setShouldAntialias(true)

    let inset = s * (100.0 / 1024.0)
    let tile = CGRect(x: inset, y: inset, width: s - inset * 2, height: s - inset * 2)
    let radius = tile.width * 0.2237
    let tilePath = CGPath(roundedRect: tile, cornerWidth: radius, cornerHeight: radius, transform: nil)

    // macOS HIG 底部轻投影
    ctx.saveGState()
    ctx.setShadow(
        offset: CGSize(width: 0, height: -s * 0.012),
        blur: s * 0.024,
        color: CGColor(colorSpace: cs, components: [0, 0, 0, 0.20])!
    )
    ctx.addPath(tilePath)
    ctx.setFillColor(CGColor(colorSpace: cs, components: [1, 1, 1, 1])!)
    ctx.fillPath()
    ctx.restoreGState()

    // 柔和白底渐变（与 macOS 原生白色系应用图标保持一致）
    ctx.saveGState()
    ctx.addPath(tilePath)
    ctx.clip()
    let bgColors = [
        CGColor(colorSpace: cs, components: [1.0, 1.0, 1.0, 1.0])!,
        CGColor(colorSpace: cs, components: [0.955, 0.965, 0.980, 1.0])!,
    ] as CFArray
    let bgGrad = CGGradient(colorsSpace: cs, colors: bgColors, locations: [0.0, 1.0])!
    ctx.drawLinearGradient(
        bgGrad,
        start: CGPoint(x: tile.midX, y: tile.maxY),
        end: CGPoint(x: tile.midX, y: tile.minY),
        options: []
    )
    ctx.restoreGState()

    // 浅细描边，确保在纯白 Finder 背景下边缘清晰
    ctx.saveGState()
    let borderW = max(1.0, s * 0.004)
    let borderPath = CGPath(
        roundedRect: tile.insetBy(dx: borderW * 0.5, dy: borderW * 0.5),
        cornerWidth: max(0, radius - borderW * 0.5),
        cornerHeight: max(0, radius - borderW * 0.5),
        transform: nil
    )
    ctx.addPath(borderPath)
    ctx.setStrokeColor(CGColor(colorSpace: cs, components: [0.06, 0.09, 0.16, 0.09])!)
    ctx.setLineWidth(borderW)
    ctx.strokePath()
    ctx.restoreGState()

    // 统一 DetectRadar 雷达十字靶图形（圆角端点 + 单层渐变，无叠加接缝）
    let center = CGPoint(x: tile.midX, y: tile.midY)
    let w = tile.width
    let ringR = w * 0.268
    let strokeW = max(1.5, w * 0.086)
    let innerR = w * 0.195
    let outerR = w * 0.342
    let dotR = w * 0.084

    let markPath = CGMutablePath()
    markPath.addEllipse(in: CGRect(x: center.x - ringR, y: center.y - ringR, width: ringR * 2, height: ringR * 2))
    for (dx, dy) in [(1.0, 0.0), (-1.0, 0.0), (0.0, 1.0), (0.0, -1.0)] {
        markPath.move(to: CGPoint(x: center.x + CGFloat(dx) * innerR, y: center.y + CGFloat(dy) * innerR))
        markPath.addLine(to: CGPoint(x: center.x + CGFloat(dx) * outerR, y: center.y + CGFloat(dy) * outerR))
    }
    let strokedMark = markPath.copy(strokingWithWidth: strokeW, lineCap: .round, lineJoin: .round, miterLimit: 4)
    let unified = CGMutablePath()
    unified.addPath(strokedMark)
    unified.addEllipse(in: CGRect(x: center.x - dotR, y: center.y - dotR, width: dotR * 2, height: dotR * 2))

    ctx.saveGState()
    ctx.addPath(unified)
    ctx.clip()
    let markColors = [
        CGColor(colorSpace: cs, components: [0.165, 0.820, 0.420, 1.0])!,
        CGColor(colorSpace: cs, components: [0.086, 0.639, 0.290, 1.0])!,
    ] as CFArray
    let markGrad = CGGradient(colorsSpace: cs, colors: markColors, locations: [0.0, 1.0])!
    ctx.drawLinearGradient(
        markGrad,
        start: CGPoint(x: center.x, y: center.y + outerR + strokeW),
        end: CGPoint(x: center.x, y: center.y - outerR - strokeW),
        options: []
    )
    ctx.restoreGState()

    NSGraphicsContext.restoreGraphicsState()

    let data = rep.representation(using: .png, properties: [:])!
    try! data.write(to: URL(fileURLWithPath: outDir).appendingPathComponent(name))
}
SWIFT

iconutil -c icns "$ICONSET_DIR" -o "$RESOURCES_DIR/AppIcon.icns"
rm -rf "$ICONSET_DIR"

cat > "$CONTENTS_DIR/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>zh_CN</string>
  <key>CFBundleExecutable</key>
  <string>$APP_NAME</string>
  <key>CFBundleIconFile</key>
  <string>AppIcon</string>
  <key>CFBundleIdentifier</key>
  <string>$BUNDLE_ID</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>DetectRadarBar</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>$VERSION</string>
  <key>CFBundleVersion</key>
  <string>$BUILD_NUMBER</string>
  <key>LSMinimumSystemVersion</key>
  <string>13.0</string>
  <key>LSUIElement</key>
  <true/>
  <key>NSHumanReadableCopyright</key>
  <string>MIT License · DetectRadar</string>
</dict>
</plist>
PLIST

echo "==> Ad-hoc signing $APP_DIR..."
codesign --force --deep --sign - "$APP_DIR"
codesign --verify --deep --strict "$APP_DIR"

if [[ "$ARCH" == "universal" ]]; then
  lipo "$MACOS_DIR/$APP_NAME" -verify_arch arm64
  lipo "$MACOS_DIR/$APP_NAME" -verify_arch x86_64
fi

DMG_NAME="${DMG_NAME:-$APP_NAME-v$VERSION-macos-$ARCH.dmg}"
DMG_STAGE="$(mktemp -d /tmp/detectradar-dmg-XXXX)"
trap 'rm -rf "$DMG_STAGE"' EXIT
cp -R "$APP_DIR" "$DMG_STAGE/$APP_NAME.app"
ln -s /Applications "$DMG_STAGE/Applications"

rm -f "$DIST_DIR/$DMG_NAME"
hdiutil create \
  -volname "$APP_NAME" \
  -srcfolder "$DMG_STAGE" \
  -ov \
  -format UDZO \
  "$DIST_DIR/$DMG_NAME" >/dev/null

echo "==> Done:"
echo "  App: $APP_DIR"
echo "  DMG: $DIST_DIR/$DMG_NAME"
