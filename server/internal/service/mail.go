package service

import (
	"context"
	"log"
	"time"

	"detect-radar/internal/config"
	"detect-radar/internal/mail"
	"detect-radar/internal/repository"
)

const mailMaxAttempts = 8

type Mailer struct {
	outbox   *repository.MailOutbox
	from, to string
	send     func(context.Context, mail.Message) (bool, error)
}

func NewMailer(outbox *repository.MailOutbox, cfg config.MailConfig) *Mailer {
	return &Mailer{outbox: outbox, from: cfg.From, to: cfg.To, send: mail.NewClient(cfg).Send}
}

func (m *Mailer) Run(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		for i := 0; i < 10 && ctx.Err() == nil; i++ {
			worked, err := m.processNext(ctx)
			if err != nil && ctx.Err() == nil {
				log.Printf("[邮件] 处理任务失败：%v", err)
			}
			if err != nil || !worked {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Mailer) processNext(ctx context.Context) (bool, error) {
	job, err := m.outbox.Next(ctx, time.Now())
	if err != nil || job == nil {
		return false, err
	}
	retry, sendErr := m.send(ctx, job.Message)
	// 关闭时保留任务，重启后继续发送。
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	var retryAt time.Time
	var message string
	if sendErr != nil {
		message = sendErr.Error()
		if retry && job.Attempts+1 < mailMaxAttempts {
			retryAt = time.Now().Add(30 * time.Second * time.Duration(1<<job.Attempts))
		}
		log.Printf("[邮件] %s 第 %d 次失败，重试=%t：%s", job.EventID, job.Attempts+1, !retryAt.IsZero(), message)
	}
	return true, m.outbox.Finish(ctx, job.EventID, retryAt, message)
}
