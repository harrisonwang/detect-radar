package handler

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"detect-radar/internal/model"
	"detect-radar/internal/service"
	"detect-radar/internal/util"
)

type IPIntelHandler struct {
	svc *service.IPIntelService
}

func NewIPIntelHandler(svc *service.IPIntelService) *IPIntelHandler {
	return &IPIntelHandler{svc: svc}
}

func privateIPResponse(ip string) *model.IPIntel {
	return &model.IPIntel{
		IP:           ip,
		Country:      "LOCAL",
		CountryName:  "Private Network",
		City:         "Local",
		UsageType:    "private",
		UsageTypeRaw: "private",
		Source:       "local",
		Tier:         "L0",
		Confidence:   100,
		DetectMethod: "rfc1918",
		Tip:          "内网 IP",
		FetchedAt:    time.Now(),
	}
}

func setNoStoreHeaders(c *fiber.Ctx) {
	c.Set("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")
}

func (h *IPIntelHandler) resolveClientIPIntel(c *fiber.Ctx) (*model.IPIntel, error) {
	clientIP := util.CanonicalIP(c.IP())

	if util.IsPrivateIP(clientIP) {
		return privateIPResponse(clientIP), nil
	}

	deepScan := c.QueryBool("deep", false)
	opts := service.LookupOptions{
		DeepScan:  deepScan,
		LocalOnly: !deepScan,
	}
	if deepScan {
		opts.PreferSource = c.Query("source", "")
	}

	return h.svc.Lookup(c.Context(), clientIP, opts)
}

func (h *IPIntelHandler) GetMyIPIntel(c *fiber.Ctx) error {
	intel, err := h.resolveClientIPIntel(c)
	if err != nil {
		return util.InternalError(c, err.Error())
	}

	setNoStoreHeaders(c)

	return c.JSON(intel)
}

func (h *IPIntelHandler) GetMyIPText(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/plain; charset=utf-8")
	setNoStoreHeaders(c)
	c.Set("X-Robots-Tag", "noindex, nofollow")
	c.Set("Vary", "User-Agent, Accept")

	intel, err := h.resolveClientIPIntel(c)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("查询失败，请稍后重试\n")
	}

	return c.SendString(formatIPIntelText(intel))
}

func formatIPIntelText(intel *model.IPIntel) string {
	var b strings.Builder
	b.WriteString("DetectRadar · 终端出口信息\n")
	fmt.Fprintf(&b, "IP:       %s\n", intel.IP)
	fmt.Fprintf(&b, "地区:     %s\n", formatIntelLocation(intel))

	operator := strings.TrimSpace(intel.ISP)
	if operator == "" {
		operator = strings.TrimSpace(intel.Org)
	}
	if operator == "" {
		operator = "未知"
	}
	if asn := normalizeASN(intel.ASN); asn != "" {
		fmt.Fprintf(&b, "运营商:   %s (%s)\n", operator, asn)
	} else {
		fmt.Fprintf(&b, "运营商:   %s\n", operator)
	}

	fmt.Fprintf(&b, "类型:     %s\n", formatIntelTypeLine(intel))

	if intel.HumanRatio != nil {
		pct := math.Max(0, math.Min(100, *intel.HumanRatio))
		fmt.Fprintf(&b, "人类流量: %.1f%%（Cloudflare Radar 近 30 天 ASN 统计）\n", pct)
	}

	b.WriteString("更多检测: https://detectradar.com/\n")
	return b.String()
}

func formatIntelLocation(intel *model.IPIntel) string {
	if intel.UsageType == "private" || intel.Country == "LOCAL" {
		return "内网地址 (LOCAL)"
	}
	var parts []string
	seen := make(map[string]struct{}, 3)
	for _, raw := range []string{intel.CountryName, intel.Region, intel.City} {
		part := strings.TrimSpace(raw)
		if part == "" {
			continue
		}
		key := strings.ToLower(part)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		parts = append(parts, part)
	}
	loc := strings.Join(parts, " · ")
	if loc == "" {
		if cc := strings.TrimSpace(intel.Country); cc != "" {
			return cc
		}
		return "未知"
	}
	if cc := strings.TrimSpace(intel.Country); cc != "" && cc != "LOCAL" {
		return fmt.Sprintf("%s (%s)", loc, cc)
	}
	return loc
}

func normalizeASN(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	allDigits := true
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		return "AS" + s
	}
	if len(s) > 2 && strings.EqualFold(s[:2], "as") {
		return "AS" + s[2:]
	}
	return s
}

func formatIntelTypeLine(intel *model.IPIntel) string {
	var base string
	switch intel.UsageType {
	case "private":
		base = "内网地址 (private)"
	case "hosting":
		base = "机房 IP (hosting)"
	case "isp":
		if intel.IsMobile {
			base = "移动网络 (isp)"
		} else {
			base = "普通宽带 (isp)"
		}
	default:
		base = "类型未定 (unknown)"
	}

	var tags []string
	if intel.IsVPN {
		tags = append(tags, "[VPN]")
	}
	if intel.IsProxy {
		tags = append(tags, "[代理]")
	}
	if intel.IsTor {
		tags = append(tags, "[Tor]")
	}
	if intel.IsRelay {
		tags = append(tags, "[Relay]")
	}
	if len(tags) == 0 {
		return base
	}
	return base + " " + strings.Join(tags, " ")
}
