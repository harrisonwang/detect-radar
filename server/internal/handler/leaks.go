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

type DNSHandler struct {
	service *service.DNSService
}

func NewDNSHandler(s *service.DNSService) *DNSHandler {
	return &DNSHandler{service: s}
}

func (h *DNSHandler) Assess(c *fiber.Ctx) error {
	var input dnsassessment.Input
	if err := c.BodyParser(&input); err != nil {
		return util.InvalidParameter(c, util.Localized(c, "DNS 检测样本无效", "Invalid DNS observation payload"))
	}
	if input.TotalRounds == 0 && len(input.Resolvers) > 0 {
		input.TotalRounds = 1
		if input.ObservedRounds == 0 {
			input.ObservedRounds = 1
		}
	}
	if !input.Valid() {
		return util.InvalidParameter(c, util.Localized(c, "DNS 检测样本无效", "Invalid DNS observation payload"))
	}
	setNoStoreHeaders(c)
	var cat dnsassessment.Catalog
	if h.service != nil {
		if nc := h.service.Catalog(); nc != nil {
			cat = nc
		}
	}
	if cat == nil {
		if nc := service.DefaultCatalog(); nc != nil {
			cat = nc
		}
	}
	return c.JSON(dnsassessment.EvaluateWithCatalog(input, cat, util.ResolveLang(c)))
}

func (h *DNSHandler) CreateLeakTest(c *fiber.Ctx) error {
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

func (h *DNSHandler) GetLeakResult(c *fiber.Ctx) error {
	testID := c.Params("id")
	if testID == "" {
		return util.MissingParameter(c, util.Localized(c, "缺少 DNS 泄露测试标识", "Missing DNS leak test ID"))
	}
	actualID := strings.TrimPrefix(testID, "leak_dns_")

	result, err := h.service.GetLeakResult(c.Context(), actualID)
	if errors.Is(err, service.ErrDNSTestNotFound) {
		return util.ResourceNotFound(c, util.Localized(c, "DNS 泄露测试 '"+testID+"' 不存在或已过期", "DNS leak test '"+testID+"' not found or expired"))
	}
	if err != nil {
		return storageUnavailable(c, err)
	}
	setNoStoreHeaders(c)
	return c.JSON(h.service.ProjectLeakResult(result, util.ResolveLang(c)))
}

const maxWebRTCAssessInputIPs = 16

type WebRTCHandler struct {
	ipIntel *service.IPIntelService
}

func NewWebRTCHandler(ipIntel *service.IPIntelService) *WebRTCHandler {
	return &WebRTCHandler{ipIntel: ipIntel}
}

func (h *WebRTCHandler) Assess(c *fiber.Ctx) error {
	var req service.WebRTCAssessmentRequest
	if err := c.BodyParser(&req); err != nil || len(req.PublicIPs) > maxWebRTCAssessInputIPs {
		return util.InvalidParameter(c, util.Localized(c, "WebRTC 检测样本无效", "Invalid WebRTC observation payload"))
	}
	clientIP := util.CanonicalIP(c.IP())
	result := service.AssessWebRTCWithLang(c.Context(), h.ipIntel, clientIP, nil, req.PublicIPs, util.ResolveLang(c))
	setNoStoreHeaders(c)
	return c.JSON(result)
}
