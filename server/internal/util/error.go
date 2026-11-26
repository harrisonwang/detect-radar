package util

import (
	"github.com/gofiber/fiber/v2"
)

// RFC 7807 Problem Details
type ProblemDetail struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

const (
	ProblemTypeInvalidParameter   = "https://detectradar.com/problems/invalid-parameter"
	ProblemTypeMissingParameter   = "https://detectradar.com/problems/missing-parameter"
	ProblemTypeResourceNotFound   = "https://detectradar.com/problems/resource-not-found"
	ProblemTypeInternalError      = "https://detectradar.com/problems/internal-error"
	ProblemTypeServiceUnavailable = "https://detectradar.com/problems/service-unavailable"
)

func Error(c *fiber.Ctx, status int, title, detail string) error {
	return c.Status(status).JSON(ProblemDetail{
		Type:     "about:blank",
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: c.Path(),
	})
}

// RFC7807Error returns an RFC 7807 formatted error response
func RFC7807Error(c *fiber.Ctx, problemType, title string, status int, detail string) error {
	return c.Status(status).JSON(ProblemDetail{
		Type:     problemType,
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: c.Path(),
	})
}

func InvalidParameter(c *fiber.Ctx, detail string) error {
	return RFC7807Error(c, ProblemTypeInvalidParameter, Localized(c, "参数无效", "Invalid Parameter"), fiber.StatusBadRequest, detail)
}

func MissingParameter(c *fiber.Ctx, detail string) error {
	return RFC7807Error(c, ProblemTypeMissingParameter, Localized(c, "缺少参数", "Missing Parameter"), fiber.StatusBadRequest, detail)
}

func ResourceNotFound(c *fiber.Ctx, detail string) error {
	return RFC7807Error(c, ProblemTypeResourceNotFound, Localized(c, "资源不存在", "Resource Not Found"), fiber.StatusNotFound, detail)
}

func InternalError(c *fiber.Ctx, detail string) error {
	return RFC7807Error(c, ProblemTypeInternalError, Localized(c, "服务器内部错误", "Internal Server Error"), fiber.StatusInternalServerError, detail)
}
