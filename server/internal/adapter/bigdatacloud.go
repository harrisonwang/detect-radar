package adapter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"detect-radar/internal/model"
)

type BigDataCloudAdapter struct {
	RemoteBase
}

type bdcResponse struct {
	IP      string `json:"ip"`
	Country struct {
		IsoAlpha2 string `json:"isoAlpha2"`
		Name      string `json:"name"`
	} `json:"country"`
	Location struct {
		City                 string  `json:"city"`
		PrincipalSubdivision string  `json:"principalSubdivision"`
		Latitude             float64 `json:"latitude"`
		Longitude            float64 `json:"longitude"`
		TimeZone             struct {
			IanaTimeId string `json:"ianaTimeId"`
		} `json:"timeZone"`
	} `json:"location"`
	Network struct {
		Organisation string `json:"organisation"`
		Carriers     []struct {
			ASN string `json:"asn"`
		} `json:"carriers"`
	} `json:"network"`
	SecurityThreat string `json:"securityThreat"`
	HazardReport   struct {
		IsKnownAsTorServer bool `json:"isKnownAsTorServer"`
		IsKnownAsVpn       bool `json:"isKnownAsVpn"`
		IsKnownAsProxy     bool `json:"isKnownAsProxy"`
		IsSpamhausDrop     bool `json:"isSpamhausDrop"`
		HostingLikelihood  int  `json:"hostingLikelihood"`
		IsHostingAsn       bool `json:"isHostingAsn"`
		IsCellular         bool `json:"isCellular"`
		ICloudPrivateRelay bool `json:"iCloudPrivateRelay"`
	} `json:"hazardReport"`
}

func NewBigDataCloudAdapter(apiKey string) *BigDataCloudAdapter {
	return &BigDataCloudAdapter{
		RemoteBase: NewRemoteBase(
			"bigdatacloud",
			"https://api.bigdatacloud.net/data/ip-geolocation-full",
			apiKey,
		),
	}
}

func (a *BigDataCloudAdapter) Lookup(ctx context.Context, ip string) (*model.IPIntel, error) {
	url := fmt.Sprintf("%s?ip=%s&key=%s", a.BaseURL(), ip, a.APIKey())

	var resp bdcResponse
	if err := a.DoGet(ctx, url, nil, &resp); err != nil {
		return nil, fmt.Errorf("bigdatacloud lookup failed: %w", err)
	}

	return a.transform(&resp), nil
}

func (a *BigDataCloudAdapter) transform(resp *bdcResponse) *model.IPIntel {
	h := resp.HazardReport

	intel := &model.IPIntel{
		IP:          resp.IP,
		Country:     resp.Country.IsoAlpha2,
		CountryName: resp.Country.Name,
		City:        resp.Location.City,
		Region:      resp.Location.PrincipalSubdivision,
		Timezone:    resp.Location.TimeZone.IanaTimeId,
		Latitude:    resp.Location.Latitude,
		Longitude:   resp.Location.Longitude,
		Org:         resp.Network.Organisation,
		Source:      "bigdatacloud",
		Tier:        "L3",
		FetchedAt:   time.Now(),

		IsVPN:     h.IsKnownAsVpn,
		IsProxy:   h.IsKnownAsProxy,
		IsTor:     h.IsKnownAsTorServer,
		IsRelay:   h.ICloudPrivateRelay,
		IsHosting: h.IsHostingAsn || h.HostingLikelihood >= 8,
		IsMobile:  h.IsCellular,
	}

	if len(resp.Network.Carriers) > 0 {
		intel.ASN = resp.Network.Carriers[0].ASN
	}

	intel.UsageType, intel.UsageTypeRaw = a.inferUsageType(resp)

	intel.IsAnonymous = intel.IsVPN || intel.IsProxy || intel.IsTor || intel.IsRelay

	intel.UsageType = intel.InferUsageType()

	intel.Confidence = h.HostingLikelihood * 10
	if intel.Confidence < 80 && (intel.IsHosting || intel.UsageTypeRaw != "") {
		intel.Confidence = 80
	}
	intel.DetectMethod = "api_bigdatacloud"

	return intel
}

func (a *BigDataCloudAdapter) inferUsageType(resp *bdcResponse) (string, string) {
	h := resp.HazardReport
	if h.IsHostingAsn || h.HostingLikelihood >= 8 {
		return "hosting", "hosting"
	}
	if h.IsCellular {
		return "isp", "mobile"
	}
	if strings.Contains(strings.ToLower(resp.SecurityThreat), "hosting") {
		return "hosting", "hosting"
	}
	if h.HostingLikelihood <= 2 {
		return "isp", ""
	}
	return "unknown", ""
}
