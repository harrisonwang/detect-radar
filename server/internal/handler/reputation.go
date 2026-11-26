package handler

import (
	"detect-radar/internal/service"
	"detect-radar/internal/util"

	"github.com/gofiber/fiber/v2"
)

type ReputationHandler struct {
	service *service.ReputationService
}

func NewReputationHandler(s *service.ReputationService) *ReputationHandler {
	return &ReputationHandler{service: s}
}

func (h *ReputationHandler) GetCurrentReputation(c *fiber.Ctx) error {
	ip := util.CanonicalIP(c.IP())
	result := h.service.Check(c.Context(), ip)
	cp := *result
	cp.Recommendation = service.ReputationRecommendation(&cp, util.ResolveLang(c))
	setNoStoreHeaders(c)
	return c.JSON(&cp)
}
