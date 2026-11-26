package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"detect-radar/internal/config"
)

type Message struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	ReplyTo string `json:"reply_to,omitempty"`
}

type Client struct {
	cfg  config.MailConfig
	http *http.Client
}

func NewClient(cfg config.MailConfig) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) Send(ctx context.Context, message Message) (bool, error) {
	body, err := json.Marshal(message)
	if err != nil {
		return false, err
	}
	endpoint := "https://api.cloudflare.com/client/v4/accounts/" + c.cfg.AccountID + "/email/sending/send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return true, fmt.Errorf("Cloudflare 暂不可用（HTTP %d）", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("Cloudflare 拒绝发送（HTTP %d）", resp.StatusCode)
	}
	var result struct {
		Success bool `json:"success"`
		Result  struct {
			Delivered            []string `json:"delivered"`
			Queued               []string `json:"queued"`
			PermanentBounces     []string `json:"permanent_bounces"`
			SuppressedRecipients []string `json:"suppressed_recipients"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return true, fmt.Errorf("Cloudflare 发送结果无效")
	}
	if len(result.Result.PermanentBounces) > 0 || len(result.Result.SuppressedRecipients) > 0 {
		return false, fmt.Errorf("收件地址被退信或抑制")
	}
	if result.Success {
		for _, address := range append(result.Result.Delivered, result.Result.Queued...) {
			if strings.EqualFold(address, message.To) {
				return false, nil
			}
		}
	}
	return false, fmt.Errorf("Cloudflare 未接收该邮件")
}
