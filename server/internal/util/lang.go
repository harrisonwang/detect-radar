package util

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

const (
	LangZH = "zh"
	LangEN = "en"
)

func NormalizeLang(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "en") {
		return LangEN
	}
	if strings.HasPrefix(s, "zh") {
		return LangZH
	}
	return ""
}

func ResolveLang(c *fiber.Ctx) string {
	if c == nil {
		return LangZH
	}
	if q := NormalizeLang(c.Query("lang")); q != "" {
		return q
	}
	accept := c.Get("Accept-Language")
	for _, part := range strings.Split(accept, ",") {
		tag := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if l := NormalizeLang(tag); l != "" {
			return l
		}
	}
	return LangZH
}

func Localized(c *fiber.Ctx, zh, en string) string {
	if ResolveLang(c) == LangEN {
		return en
	}
	return zh
}
