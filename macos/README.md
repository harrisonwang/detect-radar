# DetectRadarBar

[DetectRadar](https://detectradar.com/) 的原生 macOS 顶部状态栏伴侣（Pure Swift 6 + AppKit，零第三方依赖，安装即用）。

## 极简设计：状态栏一眼识别，两区面板即看即用

1. **状态栏常驻看出口**：顶部状态栏仅展示当前出口国旗与属性（`🇺🇸 机房` / `🇺🇸 宽带`）。
2. **上下两区极简弹窗**：
   - **上半显示区**：第 1 行同行呈现 `国家/地区` + `完整 IP（点击即复制）` + 无边框 `机房/宽带` 标签，第 2 行弱灰呈现 `主机商/运营商 · 城市`；仅在发生海外 IPv4 + 中国大陆运营商 IPv6 旁路泄露时，内联显示原生提示行 `IPv6 泄露 · <运营商>直连   排查 ›` 并可一键打开 `/ipv6-leak-test/` 排查（港澳台与海外分流不误报）。
   - **下半操作区**：无冗余左侧图标的纯文本操作行——「网页完整检测」「开机自启（右侧迷你开关）」「退出（`⌘Q`）」。

## 零配置 & 全自动静默刷新

- 无设置页、无后台定时轮询、无多余手动按钮：在应用启动、网卡切换（`NWPathMonitor`）、系统代理变动、休眠唤醒以及点开弹窗（距上次检测 ≥ 2 秒或处于异常态时）自动静默更新；支持隐藏快捷键 `⌘R` 手动重测。
- 本地 `getifaddrs` 预检存在全局 IPv6 地址时才探测 `6.detectradar.com`，纯 IPv4 网络零多余请求。
- 如需自托管后端域名，终端执行一条命令即可覆盖默认 `detectradar.com`：

```bash
defaults write com.detectradar.bar baseDomain example.com
```

## 本地构建与运行

需要 macOS 13+ 与 Xcode Command Line Tools（`swift` 6.0+）：

```bash
cd macos
./scripts/build-app.sh
open dist/DetectRadarBar.app
```

从 GitHub Releases 下载的未公证安装包如被 Gatekeeper 拦截，可执行以下命令移除隔离属性后运行：

```bash
xattr -cr /Applications/DetectRadarBar.app
```

## 相关链接

- DetectRadar 网页端：[https://detectradar.com/](https://detectradar.com/)
- IPv6 泄露专项检测：[https://detectradar.com/ipv6-leak-test/](https://detectradar.com/ipv6-leak-test/)
- 终端 / AI Agent 自检技能文件：[https://detectradar.com/SKILL.md](https://detectradar.com/SKILL.md)

## License

MIT © DetectRadar
