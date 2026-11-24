package handler

import (
	"errors"
	"strings"

	"detect-radar/dnsassessment"
	"detect-radar/internal/model"
	"detect-radar/internal/service"
	"detect-radar/internal/util"

	"github.com/gofiber/fiber/v2"
)

// ============================================================================
// DNS 泄露
// ============================================================================

type DNSHandler struct {
	service *service.DNSService
}

func NewDNSHandler(s *service.DNSService) *DNSHandler {
	return &DNSHandler{service: s}
}

// Assess runs the same read-only evaluator used by the homepage on all rounds
// collected by the DNS page. It does not change any stored scan.
func (h *DNSHandler) Assess(c *fiber.Ctx) error {
	var input dnsassessment.Input
	if err := c.BodyParser(&input); err != nil || !input.Valid() {
		return util.InvalidParameter(c, "DNS 检测样本无效")
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(dnsassessment.Evaluate(input))
}

// CreateLeakTest 创建 DNS 泄露测试（POST /leaks/dns）。
func (h *DNSHandler) CreateLeakTest(c *fiber.Ctx) error {
	// scan_id 可选：用于遥测流水把异步 DNS 结果关联回所属扫描
	var req model.DNSLeakTestRequest
	_ = c.BodyParser(&req)
	if req.ScanID != "" && !util.IsValidScanID(req.ScanID) {
		req.ScanID = ""
	}

	response, err := h.service.CreateLeakTest(c.Context(), req.ScanID)
	if errors.Is(err, service.ErrAmbiguousScan) {
		return ambiguousScan(c)
	}
	if err != nil {
		return storageUnavailable(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(response)
}

// GetLeakResult 查询 DNS 泄露结果（GET /leaks/dns/:id）。
func (h *DNSHandler) GetLeakResult(c *fiber.Ctx) error {
	testID := c.Params("id")
	if testID == "" {
		return util.MissingParameter(c, "缺少 DNS 泄露测试标识")
	}
	// 去掉 "leak_dns_" 前缀，得到真实短 ID
	actualID := strings.TrimPrefix(testID, "leak_dns_")

	result, err := h.service.GetLeakResult(c.Context(), actualID)
	if errors.Is(err, service.ErrDNSTestNotFound) {
		return util.ResourceNotFound(c, "DNS 泄露测试 '"+testID+"' 不存在或已过期")
	}
	if err != nil {
		return storageUnavailable(c, err)
	}
	return c.JSON(result)
}

// ============================================================================
// WebRTC 泄露归属评估
// ============================================================================

const maxWebRTCAssessInputIPs = 16

type WebRTCHandler struct {
	ipIntel *service.IPIntelService
}

func NewWebRTCHandler(ipIntel *service.IPIntelService) *WebRTCHandler {
	return &WebRTCHandler{ipIntel: ipIntel}
}

// Assess 使用与首页一致的纯本地归属逻辑评估 WebRTC 公网候选（POST /leaks/webrtc/assessment）。
// 出口 IP 严格取自当前连接 c.IP()，不接受客户端伪造出口，也不消耗 L3 远程额度。
func (h *WebRTCHandler) Assess(c *fiber.Ctx) error {
	var req service.WebRTCAssessmentRequest
	if err := c.BodyParser(&req); err != nil || len(req.PublicIPs) > maxWebRTCAssessInputIPs {
		return util.InvalidParameter(c, "WebRTC 检测样本无效")
	}
	clientIP := util.CanonicalIP(c.IP())
	result := service.AssessWebRTC(c.Context(), h.ipIntel, clientIP, nil, req.PublicIPs)
	c.Set("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")
	return c.JSON(result)
}
