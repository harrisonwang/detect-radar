# DetectRadarBar

[DetectRadar](https://detectradar.com/) 的原生 macOS 顶部状态栏伴侣（Pure Swift 6 + AppKit，零第三方依赖，安装即用）。

## 极简设计：状态栏一眼识别，两区面板即看即用

1. **状态栏常驻看出口**：顶部状态栏仅展示当前出口国旗与属性（中文：`🇺🇸 机房` / `🇺🇸 宽带`；英文：`🇺🇸 DC` / `🇺🇸 ISP`）。
2. **上下两区极简弹窗**：
   - **上半显示区**：第 1 行同行呈现 `国家/地区` + `完整 IP（点击即复制）` + 无边框 `机房/宽带`（英文 `Datacenter/ISP`）标签，第 2 行弱灰呈现 `主机商/运营商 · 城市`；当 IPv6 经跨地区接入网运营商直连（大陆与港澳台分开计算），或 IPv4 走机房/代理而 IPv6 走不同 ASN 的同地区接入网运营商直连时，内联显示原生提示行并可一键打开 `/ipv6-leak-test/` 排查（非代理同地区双栈与机房双栈不误报）。
   - **下半操作区**：无冗余左侧图标的纯文本操作行——「网页完整检测 / Full Web Check」「开机自启 / Launch at Login（右侧迷你开关）」「退出 / Quit（`⌘Q`）」。

## 零配置 & 全自动静默刷新

- 界面与接口语言默认随系统语言自动切换（中文系统显示中文，非中文系统自动显示英文；也可通过 `defaults write com.detectradar.bar language en` 或 `zh` 手动指定）。
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

从 GitHub Releases 或 `dl.detectradar.com` 下载的未公证安装包如被 Gatekeeper 拦截，可执行以下命令移除隔离属性后运行：

```bash
xattr -cr /Applications/DetectRadarBar.app
```

## 发布与 Cloudflare R2 镜像

推送 `macos-v<版本号>` 标签（如 `macos-v0.1.0`）时，`.github/workflows/macos-release.yml` 会自动测试、构建 Universal 安装包、生成 `SHA256SUMS.txt` 并发布 GitHub Release，随后调用 `.github/workflows/mirror-macos-to-r2.yml` 将产物不可变镜像到 Cloudflare R2（存储桶 `detectradar-releases`，公开域名 `https://dl.detectradar.com`）。

- **对象路径规则**：`macos/releases/<tag>/`（包含 `DetectRadarBar-v<ver>-macos-universal.dmg`、`LICENSE`、`SHA256SUMS.txt`），不设可变的 `latest` 别名；写入使用原子 `If-None-Match: *`、MD5 校验、`max-age=31536000, immutable` 缓存策略，并在最后上传 `SHA256SUMS.txt` 后经公开域名回读全量核验 SHA-256。
- **仓库变量（Variables）**：`R2_PUBLIC_BASE_URL=https://dl.detectradar.com`、`R2_BUCKET=detectradar-releases`。
- **仓库密钥（Secrets）**：`R2_ACCOUNT_ID`、`R2_ACCESS_KEY_ID`、`R2_SECRET_ACCESS_KEY`（R2 Token 仅授予 `detectradar-releases` 桶的 Object Read & Write 权限）。
- **补传 / 恢复**：补齐 Secrets 或偶发网络中断后，可在 GitHub Actions 手动触发 `Mirror macOS Release to R2`（或 `gh workflow run mirror-macos-to-r2.yml -f tag=macos-v0.1.0`）安全重跑。

## 相关链接

- DetectRadar 网页端：[https://detectradar.com/](https://detectradar.com/)
- IPv6 泄露专项检测：[https://detectradar.com/ipv6-leak-test/](https://detectradar.com/ipv6-leak-test/)
- 终端 / AI Agent 自检技能文件：[https://detectradar.com/SKILL.md](https://detectradar.com/SKILL.md)

## License

MIT © DetectRadar
