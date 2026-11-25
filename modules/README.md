# DetectRadar 客户端面板模块（Surge / Stash / Loon）

在代理客户端内直接查看当前节点的真实出口，不用每次打开浏览器。

---

## 展示内容与边界

- **出口与线路**：IPv4 / IPv6 出口地址、归属地、运营商与 ASN、普通宽带 / 移动网络 / 机房 IP 判断。
- **网段背景**：所属 ASN 在 Cloudflare Radar 近 30 天的人类流量占比（网段参考，非单 IP 评分）。
- **IPv6 旁路检测**：同时请求纯 IPv4（`4.detectradar.com`）与纯 IPv6（`6.detectradar.com`），当 IPv4 在中国大陆以外且 IPv6 落在中国大陆宽带运营商时标红提醒泄露（已排除香港/澳门/台湾及机房双栈）。
- **各端交互差异**：
  - **Surge**：首页信息面板展示，默认仅手动点击刷新（`update-interval=-1`），避免后台轮询。
  - **Stash**：首页 Tile 磁贴展示，点击可跳转 [detectradar.com](https://detectradar.com/)。
  - **Loon**：在节点列表长按任意节点运行插件，只走该选中节点发起检测。
- **能力边界**：接口默认走 DetectRadar 服务端本地库（`LocalOnly`，不触发第三方深度扫描限额）；**客户端面板无法检测浏览器 DNS 泄露、WebRTC、时区/语言或指纹特征**，完整排查请访问 [https://detectradar.com/](https://detectradar.com/)。

---

## 安装链接与参数

### 1. 一键导入链接

| 客户端 | 配置文件 |
| --- | --- |
| **Surge**（`.sgmodule`） | `https://raw.githubusercontent.com/harrisonwang/detect-radar/main/modules/surge/detectradar.sgmodule` |
| **Stash**（`.stoverride`） | `https://raw.githubusercontent.com/harrisonwang/detect-radar/main/modules/stash/detectradar.stoverride` |
| **Loon**（`.plugin`） | `https://raw.githubusercontent.com/harrisonwang/detect-radar/main/modules/loon/detectradar.plugin` |

### 2. 可选参数（`$argument`）

Surge 与 Loon 可在模块/脚本参数中传 `key=value&...` 覆盖默认行为：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `title` | `DetectRadar 出口` | 自定义面板标题（Loon 未指定时自动显示当前节点名） |
| `ipv6` | `true` | 设为 `false` 可跳过 `6.detectradar.com` 探测 |
| `hide_ip` | `false` | 设为 `true` 对 IP 自动打码（`203.0.*.*` / `2001:db8:*`），方便截图分享 |

Stash 如需默认打码或关闭 IPv6，可下载覆盖文件后自定义对应脚本；Surge 若需定期刷新，可将 `[Panel]` 中的 `update-interval=-1` 改为秒数（如 `3600`）。
