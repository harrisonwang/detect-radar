# DetectRadarBar

A minimal, zero-config native macOS menu bar companion for [DetectRadar](https://detectradar.com/) written in pure Swift 6 and AppKit (zero third-party dependencies).

[中文说明](./README.md)

## Design

- **At-a-Glance Menu Bar**: Displays flag + IP classification (`🇺🇸 DC` / `🇺🇸 ISP`, or `🇺🇸 机房` / `🇺🇸 宽带` in Chinese) directly in the menu bar.
- **2-Zone Popover**:
  - **Display Zone**: Country/region + click-to-copy full IP + borderless `Datacenter / ISP` badge on Row 1, muted provider & city on Row 2, plus a borderless inline IPv6 bypass alert row linking to `/en/ipv6-leak-test/` when IPv6 exits directly through a cross-region ISP (treating Mainland China, Hong Kong, Macao, and Taiwan distinctly) or via a different-ASN local ISP behind a datacenter/proxy IPv4 exit.
  - **Action Zone**: Pure-text rows focused strictly on `Full Web Check`, `Launch at Login` (with a native mini toggle switch), and `Quit (⌘Q)`.
- **Bilingual & Automatic Silent Refresh**: Switches UI and API queries automatically between English and Simplified Chinese based on `Locale.preferredLanguages` (override with `defaults write com.detectradar.bar language en` or `zh`). Updates automatically via `NWPathMonitor`, proxy config changes, wake events (3s debounce, 10s cooldown), and when opening the popover after ≥ 2s (`⌘R` also works). Probes `6.detectradar.com` only when `getifaddrs` finds a global IPv6 address.

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
