#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
APP_NAME="DetectRadarBar"
VERSION="${VERSION:-0.1.0}"
BUILD_NUMBER="${BUILD_NUMBER:-1}"
BUNDLE_ID="com.detectradar.bar"
ARCH="${ARCH:-$(uname -m)}"

cd "$ROOT_DIR"

echo "==> Building $APP_NAME ($ARCH)..."
if [[ "$ARCH" == "universal" ]]; then
  swift build -c release --arch arm64 --arch x86_64
  BIN_PATH="$ROOT_DIR/.build/apple/Products/Release/$APP_NAME"
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

# 程序化生成深空绿雷达 .icns 图标
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

for (name, px) in sizes {
    let s = CGFloat(px)
    let rep = NSBitmapImageRep(
        bitmapDataPlanes: nil, pixelsWide: px, pixelsHigh: px,
        bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
        colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0
    )!
    NSGraphicsContext.saveGraphicsState()
    let ctx = NSGraphicsContext(bitmapImageRep: rep)!.cgContext
    let rect = CGRect(x: 0, y: 0, width: s, height: s)
    let path = CGPath(roundedRect: rect.insetBy(dx: s * 0.06, dy: s * 0.06), cornerWidth: s * 0.22, cornerHeight: s * 0.22, transform: nil)
    ctx.addPath(path)
    ctx.setFillColor(CGColor(red: 0.02, green: 0.05, blue: 0.12, alpha: 1.0))
    ctx.fillPath()

    ctx.setStrokeColor(CGColor(red: 0.29, green: 0.87, blue: 0.50, alpha: 0.92))
    ctx.setLineWidth(max(1.2, s * 0.065))
    let center = CGPoint(x: s / 2, y: s / 2)
    ctx.strokeEllipse(in: CGRect(x: center.x - s * 0.28, y: center.y - s * 0.28, width: s * 0.56, height: s * 0.56))

    let crossR = s * 0.36
    let innerR = s * 0.17
    for (dx, dy) in [(1.0, 0.0), (-1.0, 0.0), (0.0, 1.0), (0.0, -1.0)] {
        ctx.move(to: CGPoint(x: center.x + CGFloat(dx) * innerR, y: center.y + CGFloat(dy) * innerR))
        ctx.addLine(to: CGPoint(x: center.x + CGFloat(dx) * crossR, y: center.y + CGFloat(dy) * crossR))
        ctx.strokePath()
    }

    ctx.setFillColor(CGColor(red: 0.29, green: 0.87, blue: 0.50, alpha: 1.0))
    ctx.fillEllipse(in: CGRect(x: center.x - s * 0.08, y: center.y - s * 0.08, width: s * 0.16, height: s * 0.16))
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
  lipo "$MACOS_DIR/$APP_NAME" -verify_arch arm64 x86_64
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
