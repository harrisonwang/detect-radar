package service

import (
	"math"
	"sort"
	"strconv"

	"detect-radar/internal/model"
)

const (
	implausibleRTTMs = 60000.0

	fiberMSPerKM = 1.0 / 100.0

	rttGeoTolerance = 0.8

	cnCentroidSlackKM = 800.0

	ConnCharNoObviousAnomalies  = "no_obvious_anomalies"
	ConnCharPPPoE               = "pppoe"
	ConnCharLowMTU              = "low_mtu"
	ConnCharSuspectedTunneling  = "suspected_tunneling"
	ConnCharSuspectedProxyRelay = "suspected_proxy_relay"
	ConnCharInconclusive        = "inconclusive"

	proxyRelayDeltaMinMS = 30.0
	proxyRelayDeltaRatio = 0.2
	pppoeEstMTU          = 1492
)

type rttServerCoords struct {
	Lat float64
	Lon float64
}

func round1(x float64) float64 { return math.Round(x*10) / 10 }

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

func deriveRTT(client *model.RTTData, tcpRTT, tcpRTTVar string, intel *model.IPIntel, coords rttServerCoords) model.RTTAnalysis {
	return deriveRTTWithServerMin(client, tcpRTT, tcpRTTVar, 0, intel, coords)
}

func deriveRTTWithServerMin(client *model.RTTData, tcpRTT, tcpRTTVar string, serverTCPMinMS float64, intel *model.IPIntel, coords rttServerCoords) model.RTTAnalysis {
	return deriveRTTWithMeta(client, tcpRTT, tcpRTTVar, serverTCPMinMS, nil, intel, coords)
}

func deriveRTTWithMeta(client *model.RTTData, tcpRTT, tcpRTTVar string, serverTCPMinMS float64, connMeta *TCPConnMeta, intel *model.IPIntel, coords rttServerCoords) model.RTTAnalysis {
	var out model.RTTAnalysis

	if connMeta != nil && connMeta.PeerMSS > 0 && connMeta.EstMTU > 0 {
		out.PeerMSS = connMeta.PeerMSS
		out.EstMTU = connMeta.EstMTU
		out.ServerMTU = connMeta.ServerMTU
		out.MTUClamped = connMeta.MTUClamped
		out.IPVersion = connMeta.IPVersion
		ts := connMeta.Timestamps
		out.TCPTimestamps = &ts
		ws := connMeta.PeerWScale
		out.PeerWScale = &ws
	}

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
		out.ConnectMS = client.ConnectMS
		out.ConnReused = client.ConnReused
	}

	serverMS, haveServer := parseTCPInfoMS(tcpRTT)
	if haveServer {
		out.ServerTCPMS = serverMS
	}
	if varMS, ok := parseTCPInfoMS(tcpRTTVar); ok {
		out.ServerRTTVarMS = varMS
	}

	haveServerMin := serverTCPMinMS > 0 && serverTCPMinMS <= implausibleRTTMs
	if haveServerMin {
		out.ServerTCPMinMS = serverTCPMinMS
		if haveClient {
			dMin := round1(out.ClientMinMS - out.ServerTCPMinMS)
			out.DeltaMinMS = &dMin
		}
	}

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
		case intel.Country == "CN" || intel.Country == "HK" || intel.Country == "MO" || intel.Country == "TW":
			plat, plon, ok := lookupCNProvinceCoords(intel.Region)
			if !ok {
				switch intel.Country {
				case "HK":
					plat, plon, ok = lookupCNProvinceCoords("香港")
				case "MO":
					plat, plon, ok = lookupCNProvinceCoords("澳门")
				case "TW":
					plat, plon, ok = lookupCNProvinceCoords("台湾")
				}
			}
			if ok {
				lat, lon = plat, plon
				slackKM = cnCentroidSlackKM
				geoSource = "cn_province_centroid"
				haveGeo = true
			}
		}
		if haveGeo {
			dist := round1(haversineKM(lat, lon, coords.Lat, coords.Lon))
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

	haveServerAny := haveServer || haveServerMin
	haveMTU := out.EstMTU > 0
	switch {
	case haveClient && haveServerAny:
		out.Status = "ok"
	case haveClient || haveServerAny || haveMTU:
		out.Status = "partial"
	default:
		out.Status = "unavailable"
	}
	out.Characteristics = deriveConnectionCharacteristics(out, intel)
	return out
}

func deriveConnectionCharacteristics(out model.RTTAnalysis, intel *model.IPIntel) []string {
	haveDelta := out.DeltaMinMS != nil
	haveMTU := out.EstMTU > 0
	hasProxyRelay := false
	if haveDelta {
		threshold := math.Max(proxyRelayDeltaMinMS, out.ServerTCPMinMS*proxyRelayDeltaRatio)
		hasProxyRelay = *out.DeltaMinMS >= threshold
	}

	isMobile := intel != nil && (intel.IsMobile || intel.UsageTypeRaw == "mobile")
	isHosting := intel != nil && (intel.IsHosting || intel.InferUsageType() == "hosting")

	var tags []string
	if haveMTU && !out.MTUClamped {
		if out.EstMTU == pppoeEstMTU {
			tags = append(tags, ConnCharPPPoE)
		} else if out.EstMTU < pppoeEstMTU && !isMobile {
			if isHosting || hasProxyRelay {
				tags = append(tags, ConnCharSuspectedTunneling)
			} else {
				tags = append(tags, ConnCharLowMTU)
			}
		}
	}
	if hasProxyRelay {
		tags = append(tags, ConnCharSuspectedProxyRelay)
	}
	if len(tags) > 0 {
		return tags
	}
	if haveDelta && haveMTU {
		return []string{ConnCharNoObviousAnomalies}
	}
	if out.Status != "" && out.Status != "unavailable" {
		return []string{ConnCharInconclusive}
	}
	return nil
}

func haversineKM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	la1 := lat1 * math.Pi / 180
	la2 := lat2 * math.Pi / 180
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(la1)*math.Cos(la2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return r * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

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
