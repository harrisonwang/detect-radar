package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"detect-radar/internal/adapter"
	"detect-radar/internal/model"
	"detect-radar/internal/service"
)

type mapIntelAdapter struct {
	byIP map[string]*model.IPIntel
}

func (m *mapIntelAdapter) Name() string                       { return "local" }
func (m *mapIntelAdapter) Tier() string                       { return "L1" }
func (m *mapIntelAdapter) Capabilities() adapter.Capabilities { return adapter.Capabilities{} }
func (m *mapIntelAdapter) Lookup(_ context.Context, ip string) (*model.IPIntel, error) {
	if v, ok := m.byIP[ip]; ok {
		cp := *v
		return &cp, nil
	}
	return &model.IPIntel{IP: ip}, nil
}

func TestWebRTCHandler_Assess(t *testing.T) {
	local := &mapIntelAdapter{
		byIP: map[string]*model.IPIntel{
			"240e:469:e692:1dbe:b0f0:bafe:4e4c:329": {
				IP: "240e:0469:e692:1dbe:b0f0:bafe:4e4c:0329", Country: "CN",
				ASN: "4134", Org: "China Telecom", ISP: "电信", UsageType: "isp", UsageTypeRaw: "residential",
			},
			"106.16.168.167": {
				IP: "106.16.168.167", Country: "CN",
				ASN: "AS4134", Org: "China Telecom", ISP: "电信", UsageType: "isp", UsageTypeRaw: "residential",
			},
		},
	}
	reg := adapter.NewRegistry()
	reg.SetLocal(local)
	ipSvc := service.NewIPIntelService(reg, nil, nil, nil, nil)
	h := NewWebRTCHandler(ipSvc)

	app := fiber.New(fiber.Config{
		EnableTrustedProxyCheck: true,
		TrustedProxies:          []string{"0.0.0.0", "127.0.0.1"},
		ProxyHeader:             "X-Real-IP",
	})
	app.Post("/api/v1/leaks/webrtc/assessment", h.Assess)

	t.Run("dual_stack_china_telecom_clear", func(t *testing.T) {
		body := `{"public_ips":["240e:469:e692:1dbe:b0f0:bafe:4e4c:329","106.16.168.167"]}`
		req := httptest.NewRequest("POST", "/api/v1/leaks/webrtc/assessment", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Real-IP", "240e:0469:e692:1dbe:b0f0:bafe:4e4c:0329")
		req.RemoteAddr = "127.0.0.1:12345"

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
			t.Fatalf("Cache-Control = %q, want no-store", cc)
		}

		var got service.WebRTCAssessmentResult
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Verdict.State != "clear" || got.Verdict.Reason != "dual-stack" {
			t.Fatalf("verdict = %+v, want clear/dual-stack", got.Verdict)
		}
		if got.Exit == nil || got.Exit.IP != "240e:469:e692:1dbe:b0f0:bafe:4e4c:329" {
			t.Fatalf("exit = %+v, want canonical IPv6", got.Exit)
		}
		if len(got.Candidates) != 2 || got.Candidates[1].ASN != "AS4134" {
			t.Fatalf("candidates = %+v", got.Candidates)
		}
	})

	t.Run("rejects_excessive_candidates", func(t *testing.T) {
		ips := make([]string, 17)
		for i := range ips {
			ips[i] = "1.1.1.1"
		}
		raw, _ := json.Marshal(map[string]any{"public_ips": ips})
		req := httptest.NewRequest("POST", "/api/v1/leaks/webrtc/assessment", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12345"

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		if resp.StatusCode != 400 {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})
}
