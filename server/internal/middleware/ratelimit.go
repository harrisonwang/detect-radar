package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	"detect-radar/internal/config"
)

// RateLimit 全局配额，按真实客户端 IP 计数。
//
// 豁免 RTT 应用层探针 GET /api/v1/ping：它是零工作量的 204（不查 IP、不落库、无放大
// 价值），而客户端要循环打 N 次来测往返时延。若计入全局配额，一次正常「~7 次探测 + 扫描 +
// 泄露测试 + DNS 轮询」的会话（约 13~18 次请求）叠加探测就会逼近 100/60s 上限，用户重跑
// 两次即 429。豁免后探测不再侵蚀扫描预算；探测本身零成本，不需要配额兜底。
func RateLimit(cfg config.RateLimitConfig) fiber.Handler {
	return newLimiter(cfg.Max, cfg.Expiration, func(c *fiber.Ctx) bool {
		return c.Path() == "/api/v1/ping"
	})
}

// newLimiter skip 为 nil 时对所有请求计数；skip 返回 true 表示该请求不计入本配额。
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
