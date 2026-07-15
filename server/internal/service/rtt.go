package service

import (
	"math"
	"sort"
	"strconv"

	"detect-radar/internal/model"
)

// RTT 物理探测派生（Phase 1 只采集不计分）。
//
// 三个测量拼成一次会话的物理画像：
//  1. 客户端应用层 RTT——浏览器循环打 GET /api/v1/ping 测得，含「浏览器→出口」这一腿；
//  2. nginx 出口↔服务器 TCP RTT——nginx 环回反代到 127.0.0.1:8080，Go 侧读 TCP_INFO
//     只会得到 ~0（环回），真正的「出口 IP↔服务器」RTT 只有 nginx 的 $tcpinfo_rtt 看得到，
//     经 X-TCP-RTT / X-TCP-RTTVAR 头（µs 整数）传进来；
//  3. 地理-延迟下限校验——IP 声称位置到本机的大圆距离换算出物理最小往返时延，
//     若实测 TCP RTT 低于下限，说明 IP 位置与延迟自相矛盾（geo violation）。
//
// Δ = client_min - server_tcp ≈ 「浏览器→出口」这一腿：HTTP/SOCKS 代理下显著，
// L3 VPN（改路由不改 TCP 终点）下接近 0，故 VPN 主要靠 geo violation 抓。
//
// 本文件全是纯函数，绝不被 rules.go 的 score() 消费——RTT 在 Phase 1 不参与任何评分。

const (
	// implausibleRTTMs 超过 60s 的 RTT 视为坏值/无效头，按缺失处理。
	implausibleRTTMs = 60000.0

	// fiberMSPerKM 光在光纤里约 200,000 km/s（≈2/3 c）。
	// 大圆距离 D km 的往返最小时延 = 2*D/200000 秒 = D/100 毫秒，即每公里 0.01 ms。
	fiberMSPerKM = 1.0 / 100.0

	// rttGeoTolerance geo violation 判据的容差因子：只有实测 TCP RTT 低于
	// 「物理下限 × 0.8」才判违背。0.8 用来吸收测量噪声与「光纤=2/3 c」这一近似的不确定性
	// （实际骨干路径可能更直、CDN 可能更近），宁可漏判也不误判物理上勉强可能的边界值。
	rttGeoTolerance = 0.8

	// cnCentroidSlackKM 用省会质心代表全省时，从质心距离里预扣的保守余量（km）。
	// 省会与省内最远角可差数百公里（新疆/西藏/内蒙古尤甚），取 800km 覆盖绝大多数省。
	// 方向性保守：高估 D 会抬高 geo_min、制造假 violation；低估 D 只让判据更宽松、少报。
	// 故先扣 slack 再算下限，宁可低估距离、少报 violation，也不制造假阳性
	// ——与本仓库「宁漏勿误」的一致哲学。仅省会质心来源用它；intel 自带精确坐标时 slack=0。
	cnCentroidSlackKM = 800.0
)

// rttServerCoords 服务器自身坐标（配置注入）。Lat/Lon 全 0 视为未配置，跳过 geo 校验。
type rttServerCoords struct {
	Lat float64
	Lon float64
}

// round1 把 ms/km 值收敛到 0.1 精度。三路输入精度参差（客户端 performance.now 已被隐私
// 粗化到 ~1ms、nginx 头是 µs 整数、haversine 是全精度 float64），相减/换算会带出
// 152.82299999999998 这类浮点尾噪，落 journal 与 API 都难读。0.1 对「几十毫秒级路径差异」
// 的标定与呈现绰绰有余；统一在派生出口收敛，geo violation 也用收敛后的值判，
// 保证从落库字段就能逐字重放判定。
// server_tcp/rtt_var 刻意不收敛：µs/1000 的最短十进制表示本就 ≤3 位小数（journal 实证干净），
// 且 0.1 收敛会把 <0.05ms 的极小值归零、被 omitempty 吞掉（值在、字段消失）。
func round1(x float64) float64 { return math.Round(x*10) / 10 }

// parseTCPInfoMS 把 nginx 传的 $tcpinfo_rtt / $tcpinfo_rttvar（µs 整数字符串）转成 ms。
// 返回 (ms, ok)。以下一律按「缺失」处理（ok=false），使下游优雅降级：
//   - 空串/缺头（本地开发无 nginx）；
//   - 非整数（坏头/被篡改）；
//   - 负数（不可能的值）；
//   - 0（内核尚未测得 RTT——出口↔服务器跨网 RTT 不可能真为 0，当作无数据，
//     否则 geo violation 会因 0 < 任意下限而恒真）；
//   - 超过 implausibleRTTMs（60s）的离谱值。
func parseTCPInfoMS(raw string) (float64, bool) {
	if raw == "" {
		return 0, false
	}
	us, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || us <= 0 {
		return 0, false
	}
	ms := float64(us) / 1000.0
	if ms > implausibleRTTMs {
		return 0, false
	}
	return ms, true
}

// deriveRTT 从客户端 RTTData、nginx 头和 IP 信息派生出 journal-ready 的 RTTAnalysis。
// 全部输入都当不可信/可选：client 为 nil、头缺失、intel 无坐标时都优雅降级为 nil 指针，
// 绝不编造。Status：ok=两侧都有；partial=只有一侧；unavailable=两侧都无。
func deriveRTT(client *model.RTTData, tcpRTT, tcpRTTVar string, intel *model.IPIntel, coords rttServerCoords) model.RTTAnalysis {
	var out model.RTTAnalysis

	// —— 客户端应用层 RTT（含代理腿）——
	// 优先用 samples 服务端重算 min/median/jitter（不信任客户端上报的聚合），
	// samples 缺失时才退回客户端 min/median/jitter 聚合值。
	haveClient := false
	if client != nil {
		valid := make([]float64, 0, len(client.Samples))
		for _, s := range client.Samples {
			if s >= 0 && s <= implausibleRTTMs {
				valid = append(valid, s)
			}
		}
		switch {
		case len(valid) > 0:
			out.ClientMinMS = round1(sliceMin(valid))
			out.ClientMedianMS = round1(sliceMedian(valid))
			out.ClientJitterMS = round1(sliceJitter(valid))
			haveClient = true
		case client.Count > 0 && client.MinMS > 0:
			out.ClientMinMS = round1(client.MinMS)
			out.ClientMedianMS = round1(client.MedianMS)
			out.ClientJitterMS = round1(client.JitterMS)
			haveClient = true
		}
		// connect_ms / conn_reused 是 Resource Timing 旁证，原样透传（H2 复用时不可得）
		out.ConnectMS = client.ConnectMS
		out.ConnReused = client.ConnReused
	}

	// —— nginx 出口 IP ↔ 服务器 TCP RTT ——
	serverMS, haveServer := parseTCPInfoMS(tcpRTT)
	if haveServer {
		out.ServerTCPMS = serverMS
	}
	if varMS, ok := parseTCPInfoMS(tcpRTTVar); ok {
		out.ServerRTTVarMS = varMS
	}

	// —— Δ = client_min - server_tcp ≈ 浏览器→出口 这一腿 ——
	if haveClient && haveServer {
		d := round1(out.ClientMinMS - out.ServerTCPMS)
		out.DeltaMS = &d
	}

	// —— 地理-延迟下限校验 ——
	// 仅当 nginx TCP RTT 在手、本机坐标已配、且拿得到 IP 声称坐标时才算；任一缺失整块跳过（nil）。
	// 坐标两路来源：
	//   1. intel 自带精确坐标（geo_source=intel，slack=0）；
	//   2. CN 权威层清空坐标（applyCNGeo 刻意为之）后，用省会质心兜底
	//      （geo_source=cn_province_centroid，slack=800km）——否则所有中国 IP 的 geo 校验全线失效，
	//      恰好放过「卖中国住宅 IP、实际托管在别处」的代理商。质心绝不回写 intel、不经 API 暴露。
	if haveServer && (coords.Lat != 0 || coords.Lon != 0) {
		var lat, lon, slackKM float64
		var geoSource string
		haveGeo := false
		switch {
		case intel.Latitude != 0 || intel.Longitude != 0:
			lat, lon = intel.Latitude, intel.Longitude
			slackKM = 0
			geoSource = "intel"
			haveGeo = true
		case intel.Country == "CN":
			if plat, plon, ok := lookupCNProvinceCoords(intel.Region); ok {
				lat, lon = plat, plon
				slackKM = cnCentroidSlackKM
				geoSource = "cn_province_centroid"
				haveGeo = true
			}
		}
		if haveGeo {
			// 先收敛距离再派生下限：落库的 geo_min 恒等于 round1((geo_distance_km-slack)/100)，
			// Phase 2 从 journal 字段即可逐字复算，无需回溯全精度中间量。
			dist := round1(haversineKM(lat, lon, coords.Lat, coords.Lon))
			// D_eff = max(0, D - slack)：质心近似最多高估距离 ~slack，先扣掉再算物理下限，
			// 方向性保守（宁可低估距离、少报 violation，也不制造假阳性）。geo_distance_km 仍报
			// 真实大圆距离（诚实），slack 只体现在 geo_min_ms 上，geo_source 自描述其近似性。
			effDist := dist - slackKM
			if effDist < 0 {
				effDist = 0
			}
			geoMin := round1(effDist * fiberMSPerKM)
			out.GeoDistanceKM = &dist
			out.GeoMinMS = &geoMin
			out.GeoSource = geoSource
			violation := out.ServerTCPMS < geoMin*rttGeoTolerance
			out.GeoViolation = &violation
		}
	}

	// —— Status ——
	switch {
	case haveClient && haveServer:
		out.Status = "ok"
	case haveClient || haveServer:
		out.Status = "partial"
	default:
		out.Status = "unavailable"
	}
	return out
}

// haversineKM 计算两点间的大圆距离（km），用于 RTT 地理延迟下限校验。
func haversineKM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0 // 地球平均半径 km
	la1 := lat1 * math.Pi / 180
	la2 := lat2 * math.Pi / 180
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(la1)*math.Cos(la2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return r * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// sliceMin / sliceMedian / sliceJitter 服务端重算聚合，均要求 xs 非空（调用方已保证）。
func sliceMin(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

func sliceMedian(xs []float64) float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func sliceJitter(xs []float64) float64 {
	mn, mx := xs[0], xs[0]
	for _, x := range xs[1:] {
		if x < mn {
			mn = x
		}
		if x > mx {
			mx = x
		}
	}
	return mx - mn
}
