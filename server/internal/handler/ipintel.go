package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"detect-radar/internal/model"
	"detect-radar/internal/service"
	"detect-radar/internal/util"
)

// IPIntelHandler IP 信息处理器
type IPIntelHandler struct {
	svc *service.IPIntelService
}

// NewIPIntelHandler 创建 IP 信息处理器
func NewIPIntelHandler(svc *service.IPIntelService) *IPIntelHandler {
	return &IPIntelHandler{svc: svc}
}

// privateIPResponse 返回私有 IP 的响应（无需查询后端服务）
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

// GetMyIPIntel 获取客户端 IP 信息
// GET /api/v1/ip
// HTTP 响应禁止客户端/代理缓存（避免切换代理后读到旧 IP）；
// 服务端按真实 clientIP 复用缓存，默认仅跑本地 L1+L2（或复用已有深扫缓存），不主动消耗 L3 额度。
func (h *IPIntelHandler) GetMyIPIntel(c *fiber.Ctx) error {
	clientIP := util.CanonicalIP(c.IP())

	// 私有 IP 无需查询后端服务（本地开发环境常见）
	if util.IsPrivateIP(clientIP) {
		c.Set("Cache-Control", "no-store, no-cache, must-revalidate")
		c.Set("Pragma", "no-cache")
		c.Set("Expires", "0")
		return c.JSON(privateIPResponse(clientIP))
	}

	// 从 query 参数获取选项：默认 LocalOnly（不触发 L3 远程 API），显式 ?deep=true 才允许深扫
	deepScan := c.QueryBool("deep", false)
	opts := service.LookupOptions{
		DeepScan:  deepScan,
		LocalOnly: !deepScan,
	}
	if deepScan {
		opts.PreferSource = c.Query("source", "")
	}

	intel, err := h.svc.Lookup(c.Context(), clientIP, opts)
	if err != nil {
		return util.InternalError(c, err.Error())
	}

	// 禁止客户端与中间代理缓存，切换代理后可立即看到新出口 IP
	c.Set("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")

	return c.JSON(intel)
}
