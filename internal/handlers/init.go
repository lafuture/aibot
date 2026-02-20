package handlers

import (
	"aibot/internal/database"
	"aibot/internal/state"
)

type Handler struct {
	Rd             *state.RedisStore
	Db             *database.Postgres
	Token          string
	KieAPIKey      string
	KieCallbackURL string
	KieTasks       chan int64
	KieAcquire     func() // rate limit: вызывать перед SendToKie; может быть nil
	ChannelID   string // канал для проверки подписки (бот — админ)
	ChannelLink string // ссылка для кнопки «Подписаться»
}

func NewHandler(rd *state.RedisStore, db *database.Postgres, token, kieAPIKey, kieCallbackURL, channelID, channelLink string, kieTasks chan int64, kieAcquire func()) *Handler {
	return &Handler{
		Rd:             rd,
		Db:             db,
		Token:          token,
		KieAPIKey:      kieAPIKey,
		KieCallbackURL: kieCallbackURL,
		ChannelID:      channelID,
		ChannelLink:    channelLink,
		KieTasks:       kieTasks,
		KieAcquire:     kieAcquire,
	}
}
