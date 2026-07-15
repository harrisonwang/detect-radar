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
	ProblemTypeRateLimitExceeded  = "https://detectradar.com/problems/rate-limit-exceeded"
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

// InvalidParameter returns an invalid parameter error
func InvalidParameter(c *fiber.Ctx, detail string) error {
	return RFC7807Error(c, ProblemTypeInvalidParameter, "Invalid Parameter", fiber.StatusBadRequest, detail)
}

// MissingParameter returns a missing parameter error
func MissingParameter(c *fiber.Ctx, detail string) error {
	return RFC7807Error(c, ProblemTypeMissingParameter, "Missing Parameter", fiber.StatusBadRequest, detail)
}

// ResourceNotFound returns a resource not found error
func ResourceNotFound(c *fiber.Ctx, detail string) error {
	return RFC7807Error(c, ProblemTypeResourceNotFound, "Resource Not Found", fiber.StatusNotFound, detail)
}

// InternalError returns an internal server error
func InternalError(c *fiber.Ctx, detail string) error {
	return RFC7807Error(c, ProblemTypeInternalError, "Internal Server Error", fiber.StatusInternalServerError, detail)
}
