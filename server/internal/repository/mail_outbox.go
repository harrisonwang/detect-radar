package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"detect-radar/internal/mail"
)

type MailOutbox struct{ db *sql.DB }

type QueuedMail struct {
	EventID  string
	Attempts int
	Message  mail.Message
}

func NewMailOutbox(db *sql.DB) (*MailOutbox, error) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS mail_outbox (
 event_id TEXT PRIMARY KEY NOT NULL REFERENCES scan_events(event_id) ON DELETE CASCADE,
 message_json TEXT NOT NULL CHECK(json_valid(message_json)),
 attempts INTEGER NOT NULL DEFAULT 0,
 next_attempt_at INTEGER,
 last_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_mail_outbox_due ON mail_outbox(next_attempt_at);`)
	if err != nil {
		return nil, err
	}
	return &MailOutbox{db: db}, nil
}

// 使用调用方事务，确保业务记录与通知一起提交。
func (r *MailOutbox) Enqueue(ctx context.Context, tx *sql.Tx, eventID string, message mail.Message) error {
	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO mail_outbox(event_id,message_json,next_attempt_at) VALUES(?,?,?)`, eventID, string(raw), time.Now().UnixMilli())
	return err
}

// 单个发送循环串行读取；过期业务记录不再发送。
func (r *MailOutbox) Next(ctx context.Context, now time.Time) (*QueuedMail, error) {
	var job QueuedMail
	var raw []byte
	err := r.db.QueryRowContext(ctx, `SELECT q.event_id,q.attempts,q.message_json
 FROM mail_outbox q JOIN scan_events e ON e.event_id=q.event_id
 WHERE q.next_attempt_at<=? AND (e.expires_at IS NULL OR e.expires_at>?)
 ORDER BY q.next_attempt_at,e.seq LIMIT 1`, now.UnixMilli(), now.UnixMilli()).Scan(&job.EventID, &job.Attempts, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &job.Message); err != nil {
		return nil, err
	}
	return &job, nil
}

// 成功后删除；retryAt 为零表示停止自动重试。
func (r *MailOutbox) Finish(ctx context.Context, eventID string, retryAt time.Time, sendError string) error {
	if sendError == "" {
		_, err := r.db.ExecContext(ctx, `DELETE FROM mail_outbox WHERE event_id=?`, eventID)
		return err
	}
	var next any
	if !retryAt.IsZero() {
		next = retryAt.UnixMilli()
	}
	_, err := r.db.ExecContext(ctx, `UPDATE mail_outbox SET attempts=attempts+1,next_attempt_at=?,last_error=? WHERE event_id=?`, next, sendError, eventID)
	return err
}
