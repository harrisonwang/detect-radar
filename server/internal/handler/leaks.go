package handler

import (
	"errors"
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
