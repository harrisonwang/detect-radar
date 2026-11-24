# DetectRadarBar

A minimal, zero-config native macOS menu bar companion for [DetectRadar](https://detectradar.com/) written in pure Swift 6 and AppKit (zero third-party dependencies).

[中文说明](./README.md)

## Design

- **At-a-Glance Menu Bar**: Displays flag + IP classification (`🇺🇸 机房` / `🇺🇸 宽带`) directly in the menu bar.
- **2-Zone Popover**:
  - **Display Zone**: Country/region + click-to-copy full IP + borderless `机房/宽带` badge on Row 1, muted provider & city on Row 2, plus a borderless inline IPv6 bypass alert row linking to `/ipv6-leak-test/` only when overseas IPv4 coexists with a Mainland China ISP IPv6 leak (HK/MO/TW excluded).
  - **Action Zone**: Pure-text rows focused strictly on `网页完整检测`, `开机自启` (with a native mini toggle switch), and `退出 (⌘Q)`.
- **Automatic Silent Refresh**: Updates automatically via `NWPathMonitor`, proxy config changes, wake events (3s debounce, 10s cooldown), and when opening the popover after ≥ 2s (hidden `⌘R` also works). Probes `6.detectradar.com` only when `getifaddrs` finds a global IPv6 address.

## Build & Run

Requires macOS 13+ with Xcode Command Line Tools (`swift` 6.0+):

```bash
cd macos
./scripts/build-app.sh
open dist/DetectRadarBar.app
```

For self-hosted DetectRadar instances:

```bash
defaults write com.detectradar.bar baseDomain example.com
```

If downloaded from GitHub Releases (ad-hoc signed):

```bash
xattr -cr /Applications/DetectRadarBar.app
```

## License

MIT © DetectRadar
