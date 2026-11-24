# cloud-ip-crawler

一条命令，把主流云厂商、大机房和两千多家托管商的 IP 段抓下来，汇总成一个 SQLite 数据库，用来判断某个 IP 是不是机房 IP。

[![CI](https://github.com/harrisonwang/detect-radar/actions/workflows/crawler-ci.yml/badge.svg)](https://github.com/harrisonwang/detect-radar/actions/workflows/crawler-ci.yml)
[![Daily dataset](https://github.com/harrisonwang/detect-radar/actions/workflows/crawler-daily.yml/badge.svg)](https://github.com/harrisonwang/detect-radar/actions/workflows/crawler-daily.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](../LICENSE)

English: [README.en.md](README.en.md)

## 为什么做这个

判断一个 IP 是机房的还是家庭宽带的，是很多事情的第一步。

数据分三层，置信度从高到低：

1. **官方文件**（AWS、GCP、Azure、Cloudflare、Fastly 等 11 家）。各家自己发布的服务网段清单，最准，但格式各不相同：AWS 给 JSON，DigitalOcean 给 CSV，Linode 和 Vultr 用 [Geofeed](https://www.rfc-editor.org/rfc/rfc8805.html)，Bunny CDN 干脆给一串单个 IP，Azure 则藏在一个每周换网址的下载页后面。
2. **命名 ASN**（Hetzner、OVH、Contabo、Akamai、Rackspace、阿里云、腾讯云、华为云 等 25 家）。它们不发官方清单，改从各自的 ASN 取「当前对外宣告的前缀」。
3. **hosting 兜底层**。一家一家列举托管商永远列不完，这层换思路：对全球 BGP 表里**每个 ASN 的名称**做关键词匹配（HOSTING / VPS / CLOUD / SERVER / DATACENTER / IDC…），把两千多个「名字里就写着自己是托管商」的 ASN 一次性收进来。大厂骨干 AS（AS16509 AMAZON-02、AS13335 CLOUDFLARENET 等）也在这层显式收录——官方清单只列「对外服务的网段」，`1.1.1.1`、`8.8.8.8` 这类著名漏网之鱼就靠这层兜住。

第 2、3 层共用 [iptoasn.com](https://iptoasn.com/) 的全量 BGP 表（一个 6.8 MB 的文件，每日更新，源头是 RouteViews），一次下载覆盖全部，不用逐 ASN 打接口。

纯 Go，不需要 CGO，一个二进制文件跑完就得到一个 25 MB 的 SQLite 数据库，IPv4、IPv6 都有。

## 快速开始

不想自己跑，直接下载每天自动构建的数据库（SQLite / CSV / MMDB 三种格式）：

```bash
curl -LO https://github.com/harrisonwang/detect-radar/releases/download/dataset-latest/cloud-ip.db.gz
gunzip cloud-ip.db.gz

# nginx / HAProxy 用户直接拿 MMDB
curl -LO https://github.com/harrisonwang/detect-radar/releases/download/dataset-latest/cloud-ip.mmdb.gz
gunzip cloud-ip.mmdb.gz
```

MMDB 配 nginx 的 [geoip2 模块](https://github.com/leev/ngx_http_geoip2_module)，三行就能拿到判定结果：

```nginx
geoip2 /etc/nginx/cloud-ip.mmdb {
    $cloud_provider provider;   # 非空即机房 IP，值形如 aws / hetzner / hosting
    $cloud_tier     tier;       # official / asn / hosting，置信度从高到低
}
```

自己跑：

```bash
go install github.com/harrisonwang/detect-radar/cloud-ip-crawler/cmd/cloud-ip-crawler@latest
cloud-ip-crawler --db cloud-ip.db
```

数据库文件不存在会自动建好。全量抓一次大约 30 秒，Azure 占了大半时间。

查一个 IP 是谁的，不用写 SQL：

```bash
$ cloud-ip-crawler lookup 1.1.1.1
hosting  1.1.1.0/24  region=US  service=AS13335 CLOUDFLARENET

$ cloud-ip-crawler lookup 52.94.76.10
aws  52.94.76.0/22  region=us-west-2  service=AMAZON
hosting  52.94.72.0/21  region=US  service=AS16509 AMAZON-02
```

命中退出码为 0，未命中为 1，脚本里可以直接当判断用。同一个 IP 可能列出多条归属（官方清单一条 + hosting 兜底层一条），置信度取前者。

## 用法

```
cloud-ip-crawler [flags]           抓取数据
cloud-ip-crawler lookup <ip>       查询一个 IP 的归属
cloud-ip-crawler export [flags]    导出 MMDB（--out cloud-ip.mmdb）

  --db string          SQLite 文件路径（默认 cloud-ip.db，抓取时不存在则自动创建）
  --providers string   逗号分隔的厂商，或 all（默认 all）
  --dry-run            只抓取并打印样例，不写数据库
  --version            打印版本
```

MMDB 每个 IP 只存置信度最高的一条归属（official > asn > hosting，`tier` 字段标明来源层）；要看全部归属用 SQLite + `lookup`。

```bash
# 只更新 AWS 和 GCP，其它厂商的数据原样保留
cloud-ip-crawler --providers=aws,gcp

# 先看看能抓到什么，不落库
cloud-ip-crawler --providers=cloudflare --dry-run

# 不想要启发式兜底层，只要点名的厂商
cloud-ip-crawler --providers=aws,gcp,azure,cloudflare,oracle,digitalocean,linode,vultr,fastly,bunny,zscaler,hetzner,ovh,contabo,leaseweb,gcore,scaleway,netcup,softlayer,buyvm,hosthatch,alibaba,tencent
```

每家厂商的数据在一个事务里先删后插、整体替换，所以厂商下架的网段不会留在库里。

只要有一个数据源抓失败，程序就以非零状态退出。每天的自动构建靠这个判断要不要发布——宁可今天不更新，也不发一份缺了 Azure 的库。

## 数据源

都是公开来源，不需要任何 API key。

**一、各家自己发布的官方地址**

| 厂商 | 数据源 | 格式 |
|------|--------|------|
| AWS | `ip-ranges.amazonaws.com/ip-ranges.json` | JSON |
| GCP | `gstatic.com/ipranges/cloud.json` | JSON |
| Azure | Microsoft 下载中心 | JSON |
| Cloudflare | `cloudflare.com/ips-v4` | 纯文本 |
| Oracle | `docs.oracle.com/.../public_ip_ranges.json` | JSON |
| DigitalOcean | `digitalocean.com/geo/google.csv` | CSV |
| Linode | `geoip.linode.com` | Geofeed |
| Vultr | `geofeed.constant.com` | Geofeed |
| Fastly | `api.fastly.com/public-ip-list` | JSON |
| Bunny CDN | `bunnycdn.com/api/system/edgeserverlist` | JSON（单个 IP 列表） |
| Zscaler | `config.zscaler.com/api/<cloud>/cenr/json` | JSON |

**二、命名 ASN 的大机房 / VPS 商家**

这些厂商不发官方 IP 文件，改从它们的 ASN 取「当前对外宣告的前缀」。ASN 清单见 [`internal/crawler/asn.go`](internal/crawler/asn.go)，每个号都用 RIPEstat 的 as-overview 逐个核对过 holder。

| 厂商 | ASN |
|------|-----|
| Hetzner | 24940, 213230, 212317 |
| OVH（含 Kimsufi / SoYouStart 品牌） | 16276 |
| Contabo | 51167 |
| Leaseweb | 60781, 30633, 7203, 19148, 396190 |
| G-Core | 199524 |
| Scaleway | 12876 |
| netcup | 197540 |
| IBM SoftLayer | 36351 |
| Frantech / BuyVM | 53667 |
| HostHatch | 63473 |
| 阿里云 | 45102, 37963 |
| 腾讯云 | 132203, 45090 |
| 华为云 | 136907, 55990 |
| Akamai（含原 Linode 骨干） | 20940, 16625, 63949 |
| Rackspace | 27357, 33070, 12200, 19994 |
| GoDaddy | 26496 |
| Namecheap | 22612 |
| M247 | 9009 |
| Selectel | 49505 |
| Sakura Internet | 9370, 7684 |
| Fly.io | 40509 |
| DataCamp / CDN77 | 60068 |
| Kamatera | 36007 |
| Aeza | 210644 |
| Timeweb | 9123 |

留意语义差别：ASN 拿到的是「该 AS 宣告的所有前缀」，比官方服务清单**粗**——可能含厂商自用甚至客户自带（BYOIP）网段。对「判断是不是机房 IP」这个目标，宁可全一点反而合适。

**三、hosting 兜底层（启发式）**

对 iptoasn 全量表里每个 ASN 的名称做关键词匹配（HOSTING / VPS / VDS / CLOUD / SERVER / DATACENTER / DEDICATED / COLOCATION / IDC），两千多个托管商 ASN 一次性收进来，`provider` 统一记 `hosting`，`service` 记「AS 号 + 名称」便于溯源。中国电信 / 移动的 IDC 分支（`CHINANET-IDC-BJ` 这类）也会被正确抓到。

大厂骨干 AS 关键词抓不到（AMAZON-02 里没有关键词），在 [`internal/crawler/iptoasn.go`](internal/crawler/iptoasn.go) 里显式收录：AS16509 / AS14618（Amazon）、AS8075（Microsoft）、AS15169 / AS396982（Google）、AS13335（Cloudflare）、AS31898（Oracle）、AS14061（DigitalOcean）、AS63949（Linode/Akamai）、AS20473（Vultr）、AS54113（Fastly），同样逐个核对过 holder。

这层是启发式，置信度低于前两层：有少量误报（名字里带 VPS/CLOUD 的小 ISP，实测 2452 个 ASN 里有 3 个可疑），也有漏报（用创始人名字命名、名字里不带行业词的机房）。要纯净数据就 `--providers` 里不选 `hosting`。

抓一次的量（2026 年 7 月实测）：

```
                 v4     v6
hosting       47632  21801
azure         43310  16067
akamai         6365  12372
aws            7750   2899
linode         5313     98
m247           2584    400
alibaba        1127    280
digitalocean   1078    148
leaseweb        636    555
oracle         1089      0
gcp             991     48
bunny           623    355
ovh             847     69
tencent         827     85
softlayer       733     72
zscaler         572    105
selectel        550     20
huawei          415    113
gcore           445     53
vultr           436     71
timeweb         354     84
godaddy         323     17
rackspace       252      4
contabo         250      2
datacamp        184     33
aeza            177     38
netcup          129     12
hetzner         126      7
scaleway         84     27
kamatera         81     18
namecheap        52     34
hosthatch        75     12
buyvm            25     61
sakura           75      3
flyio            34     39
cloudflare       15      7
fastly           19      2
──────────────────────────
共 181589 条（其中 v6 56011），29 MB，全量抓一次约 27 秒
```

Azure 的原始数据里有三万多条重复网段（同一个 CIDR 挂在好几个 service / region 标签下），入库时按 `(provider, cidr)` 去重，只留一条。Zscaler 也一样——同一个节点网段会在几十个城市、几个 cloud 下反复出现，抓到的 3282 条去重后只剩 572 条。ASN 路线的条数看起来比别处少（Hetzner 只有 126 条），是因为 iptoasn 把相邻的同 ASN 网段聚合过，覆盖的地址总量不变。

上游数据不能全信：Vultr 的 geofeed 里混着三个 RFC 5737 测试网段（`192.0.2.0/24` 等）。特殊用途网段（私网、TEST-NET、组播……）在入库前统一丢弃。

## 数据表和查询

```sql
CREATE TABLE cloud_ip_ranges (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    provider   VARCHAR(100) NOT NULL,   -- aws / gcp / azure / ...
    cidr       VARCHAR(50)  NOT NULL,   -- 52.94.76.0/22
    ip_start   BLOB NOT NULL,           -- 定长存储，可以直接比大小
    ip_end     BLOB NOT NULL,
    region     VARCHAR(100),
    service    VARCHAR(100),
    ip_version TINYINT DEFAULT 4,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(provider, cidr)
);
```

`ip_start` 和 `ip_end` 按定长存，所以判断一个 IP 在不在某个网段里，就是一次走索引的范围比较：

```python
import sqlite3, ipaddress

db = sqlite3.connect("cloud-ip.db")
key = ipaddress.ip_address("52.94.76.10").packed

row = db.execute(
    "SELECT provider, cidr FROM cloud_ip_ranges "
    "WHERE ip_version = 4 AND ip_start <= ? AND ip_end >= ? LIMIT 1",
    (key, key),
).fetchone()

print(row)   # ('aws', '52.94.76.0/22')
```

## 它能告诉你什么，不能告诉你什么

这一节比上面所有内容都重要。

**查到了**，看 provider 判断置信度：

- 命中**官方文件厂商**（`aws`、`gcp`…）：厂商自己说这网段是它的，最准，还带 region / service 标签，直接用。
- 命中**命名 ASN 厂商**（`hetzner`、`ovh`…）：BGP 表说这网段是它宣告的，很准，但可能含厂商自用或客户自带的 IP。
- 只命中 **`hosting`**：一个名字像托管商的 ASN 宣告了它。作为「机房 IP」信号足够强，但 ASN 归类是启发式的，当厂商归属用之前先看看 `service` 里的 AS 名称。

以前官方清单的著名漏网之鱼现在能兜住了：`1.1.1.1`（Cloudflare 公共 DNS，不在 `ips-v4` 里）和 `8.8.8.8`（Google 公共 DNS）都能从 hosting 层查到归属。

**查不到，不等于它不是机房 IP**，但概率已经不大了。剩下的漏网主要是：名字里不带行业词的小机房（用创始人名字命名的那种）、企业自建机房、以及躲在住宅代理后面的流量——最后这类本来就不该由 IP 段清单来解决。

**v4、v6 都收。** 各家 v6 覆盖见上面的表，唯一的空缺是 Oracle——它的官方文件里就没有 v6。查询时按 `ip_version` 区分：v4 的 key 是 4 字节，v6 是 16 字节，`lookup` 子命令和 MMDB 对两者一视同仁。

## 后续计划

欢迎 PR，尤其是前两项：

- [x] ~~支持 IPv6~~：v4 / v6 都收，Oracle（官方文件没有 v6）是唯一的空缺。
- [ ] **打磨 hosting 关键词**：误报 / 漏报清单欢迎提 issue，关键词和显式收录 / 排除清单都在 `iptoasn.go`，改一行的事。
- [ ] **更多命名厂商**：往 `asn.go` 的清单里加一行就行，核对 holder 后提 PR。注意有些牌子没有自有 ASN：Kimsufi / SoYouStart 是 OVH 品牌（同一批 ASN），RackNerd 的 IP 挂在 ColoCrossing（AS36352）名下宣告，hosting 层已覆盖，无法单独打标签。
- [x] ~~导出成 MMDB~~：`export` 子命令，每日 Release 里有现成的 `cloud-ip.mmdb.gz`。
- [ ] 导出成 JSON。

## 它的来历

原本是 [DetectRadar](https://detectradar.com) 里的一个组件，用来识别机房 IP。DetectRadar 是个网页工具，打开就能看到自己的浏览器有没有在泄露真实 IP，免费、不用注册。

## 许可证

[MIT](LICENSE)
