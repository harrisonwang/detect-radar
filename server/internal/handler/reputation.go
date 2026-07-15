package handler

import (
	"detect-radar/internal/service"

	"github.com/gofiber/fiber/v2"
)

type ReputationHandler struct {
	service *service.ReputationService
}

func NewReputationHandler(s *service.ReputationService) *ReputationHandler {
	return &ReputationHandler{service: s}
}

// GetCurrentReputation GET /ip/reputation （检测当前请求出口 IP）
func (h *ReputationHandler) GetCurrentReputation(c *fiber.Ctx) error {
	ip := c.IP()
	result := h.service.Check(c.Context(), ip)
	return c.JSON(result)
}
