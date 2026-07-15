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
// 注意：此接口永不缓存，每次都返回最新结果
func (h *IPIntelHandler) GetMyIPIntel(c *fiber.Ctx) error {
	clientIP := c.IP()

	// 私有 IP 无需查询后端服务（本地开发环境常见）
	if util.IsPrivateIP(clientIP) {
		c.Set("Cache-Control", "no-store, no-cache, must-revalidate")
		c.Set("Pragma", "no-cache")
		c.Set("Expires", "0")
		return c.JSON(privateIPResponse(clientIP))
	}

	// 从 query 参数获取选项
	deepScan := c.QueryBool("deep", false)
	preferSource := c.Query("source", "")

	// /me 接口强制禁用缓存
	intel, err := h.svc.Lookup(c.Context(), clientIP, service.LookupOptions{
		DeepScan:     deepScan,
		PreferSource: preferSource,
		SkipCache:    true, // 永不缓存
	})
	if err != nil {
		return util.InternalError(c, err.Error())
	}

	// 禁止客户端缓存
	c.Set("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")

	return c.JSON(intel)
}
