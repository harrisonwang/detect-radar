package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	"detect-radar/internal/config"
)

func RateLimit(cfg config.RateLimitConfig) fiber.Handler {
	return newLimiter(cfg.Max, cfg.Expiration, func(c *fiber.Ctx) bool {
		return c.Path() == "/api/v1/ping"
	})
}

func newLimiter(max int, expiration time.Duration, skip func(*fiber.Ctx) bool) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        max,
		Expiration: expiration,
		Next:       skip,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"success": false,
				"error": fiber.Map{
					"code":    "RATE_LIMIT_EXCEEDED",
					"message": "Too many requests",
				},
				"timestamp": fiber.Map{},
			})
		},
	})
}
