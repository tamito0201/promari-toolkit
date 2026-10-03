package usecase

import (
	"context"
	"fmt"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
)

// CloudUseCase relays messages from this terminal to a Claude Code cloud
// session (`pmr cloud`). Sending only queues the message: the work, and the
// tokens it costs, happen in the cloud session. Reading the reply is left to
// the /cloud command, which reads the session's log inside Claude Code; the
// relay itself never holds a Claude.ai credential.
type CloudUseCase struct {
	Messenger repository.CloudMessenger
	Links     repository.CloudLinkStore
	Clock     repository.Clock
}

// Use sets the session later messages go to.
func (u CloudUseCase) Use(ctx context.Context, raw string) (model.CloudLink, error) {
	id, err := model.ParseCloudSessionID(raw)
	if err != nil {
		return model.CloudLink{}, err
	}
	link := model.CloudLink{Session: id}
	if err := u.Links.Save(ctx, link); err != nil {
		return model.CloudLink{}, fmt.Errorf("save the cloud session: %w", err)
	}
	return link, nil
}

// Status returns the session in use and when the last message went.
func (u CloudUseCase) Status(ctx context.Context) (model.CloudLink, error) {
	link, ok, err := u.Links.Load(ctx)
	switch {
	case err != nil:
		return model.CloudLink{}, fmt.Errorf("read the cloud session: %w", err)
	case !ok || link.Session.IsZero():
		return model.CloudLink{}, model.ErrNoCloudSession
	}
	return link, nil
}

// Send queues text into the session in use. The time is taken before the
// send, so everything the session writes in answer comes after SentAt.
func (u CloudUseCase) Send(ctx context.Context, text string) (model.CloudReceipt, error) {
	msg, err := model.NewCloudMessage(text)
	if err != nil {
		return model.CloudReceipt{}, err
	}
	link, err := u.Status(ctx)
	if err != nil {
		return model.CloudReceipt{}, err
	}
	sentAt := u.Clock.Now()
	page, err := u.Messenger.Send(ctx, link.Session, msg)
	if err != nil {
		return model.CloudReceipt{}, fmt.Errorf("send to %s: %w", link.Session, err)
	}
	link.SentAt = sentAt
	if err := u.Links.Save(ctx, link); err != nil {
		// The message is in the session already; only the reading window is lost.
		return model.CloudReceipt{}, fmt.Errorf("sent, but could not record the time: %w", err)
	}
	return model.CloudReceipt{Session: link.Session, URL: page, SentAt: sentAt}, nil
}
