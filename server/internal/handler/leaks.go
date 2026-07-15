package handler

import (
	"strings"

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

// CreateLeakTest POST /leaks/dns
func (h *DNSHandler) CreateLeakTest(c *fiber.Ctx) error {
	// scan_id 可选：用于遥测流水把异步 DNS 结果关联回所属扫描
	var req model.DNSLeakTestRequest
	_ = c.BodyParser(&req)
	if req.ScanID != "" && !util.IsValidScanID(req.ScanID) {
		req.ScanID = ""
	}

	response, err := h.service.CreateLeakTest(c.Context(), req.ScanID)
	if err != nil {
		return util.InternalError(c, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(response)
}

// GetLeakResult GET /leaks/dns/:id
func (h *DNSHandler) GetLeakResult(c *fiber.Ctx) error {
	testID := c.Params("id")
	if testID == "" {
		return util.MissingParameter(c, "DNS leak test ID is required")
	}
	// 去掉 "leak_dns_" 前缀，得到真实短 ID
	actualID := strings.TrimPrefix(testID, "leak_dns_")

	result, err := h.service.GetLeakResult(c.Context(), actualID)
	if err != nil {
		return util.ResourceNotFound(c, "DNS leak test '"+testID+"' not found or has expired.")
	}
	return c.JSON(result)
}
