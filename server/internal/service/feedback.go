package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"detect-radar/internal/mail"
	"detect-radar/internal/model"
	"detect-radar/internal/telemetry"
)

const (
	FeedbackFalsePositive   = "false_positive"
	FeedbackMissedDetection = "missed_detection"
	FeedbackDataWrong       = "data_wrong"
	FeedbackOther           = "other"
)

const feedbackNoteMaxRunes = 500

func ValidFeedbackCategory(category string) bool {
	switch category {
	case FeedbackFalsePositive, FeedbackMissedDetection, FeedbackDataWrong, FeedbackOther:
		return true
	}
	return false
}

func NormalizeFeedbackNote(note string) (string, bool) {
	var b strings.Builder
	for _, r := range note {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteRune(' ')
		case unicode.IsControl(r):
		default:
			b.WriteRune(r)
		}
	}
	cleaned := strings.TrimSpace(b.String())
	if utf8.RuneCountInString(cleaned) > feedbackNoteMaxRunes {
		return "", false
	}
	return cleaned, true
}

func (j *Journal) FeedbackEvent(scanID, category, note, replyEmail string, resp *model.ScanResponse) error {
	if j == nil || j.store == nil {
		return ErrStorageUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	instanceID, binding, err := j.store.ResolveInstance(ctx, scanID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	ev := map[string]any{
		"event": "feedback", "ts": timestamp,
		"scan_id": scanID, "category": category, "binding_status": binding,
	}
	if note != "" {
		ev["note"] = note
	}
	if replyEmail != "" {
		ev["reply_email"] = replyEmail
	}
	if resp != nil && instanceID != "" {
		ev["context"] = map[string]any{
			"score": resp.Score, "risk_level": resp.RiskLevel,
			"fired": firedRules(resp.Diagnosis), "analysis": resp.Analysis,
		}
	}
	if j.mailer == nil {
		return j.append(instanceID, "", ev)
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	tx, err := j.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	defer tx.Rollback()
	eventID, err := j.store.AppendTx(ctx, tx, telemetry.Event{InstanceID: instanceID, Payload: payload})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	message := feedbackMessage(scanID, category, note, replyEmail, timestamp)
	message.From, message.To = j.mailer.from, j.mailer.to
	if err = j.mailer.outbox.Enqueue(ctx, tx, eventID, message); err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	return nil
}

func feedbackMessage(scanID, category, note, replyEmail, timestamp string) mail.Message {
	label := map[string]string{
		FeedbackFalsePositive:   "正常网络被误判",
		FeedbackMissedDetection: "代理/VPN 未检出",
		FeedbackDataWrong:       "位置/运营商/ASN 数据错误",
		FeedbackOther:           "其他问题",
	}[category]
	message := mail.Message{
		Subject: fmt.Sprintf("[DetectRadar][%s] %s", label, scanID),
		Text:    fmt.Sprintf("分类：%s\n描述：%s\n检测编号：%s\n时间（UTC）：%s\n", label, note, scanID, timestamp),
		ReplyTo: replyEmail,
	}
	if replyEmail != "" {
		message.Text += "联系邮箱：" + replyEmail + "\n"
	}
	return message
}
