package handlers

import (
	"aibot/internal/database"
	"aibot/internal/state"
	"sync"
)

// mediaGroupEntry хранит накопленные file_id и caption для media group.
type mediaGroupEntry struct {
	FileIDs []string
	Caption string
	ChatID  int64
	Timer   *sync.Once
}

// modelPhotoGroupEntry — альбом фото при создании модели (без подписи).
type modelPhotoGroupEntry struct {
	Key        string // bot:state:<chat_id>
	ChatID     int64
	FileIDs    []string
	MessageIDs []int // для удаления сообщений пользователя после сохранения
	Timer      *sync.Once
}

type Handler struct {
	Rd                *state.RedisStore
	Db                *database.Postgres
	Token             string
	KieAPIKey         string
	KieCallbackURL    string
	KieTasks          chan int64
	KieAcquire        func() // rate limit: вызывать перед SendToKie; может быть nil
	ChannelID         string // канал для проверки подписки (бот — админ)
	ChannelLink       string // ссылка для кнопки «Подписаться»
	YouKassaID        string
	YouKassaSecretKey string
	YouKassaReturnURL string // URL возврата после оплаты (ReturnURL в платёж ЮKassa)

	mediaGroupsMu sync.Mutex
	mediaGroups   map[string]*mediaGroupEntry

	modelPhotoGroupsMu sync.Mutex
	modelPhotoGroups   map[string]*modelPhotoGroupEntry
	LogsChannelID      string
}

func NewHandler(rd *state.RedisStore, db *database.Postgres, token, kieAPIKey, kieCallbackURL, channelID, channelLink string, kieTasks chan int64, kieAcquire func(), youKassaID, youKassaSecretKey, youKassaReturnURL string, LogsChannelID string) *Handler {
	return &Handler{
		Rd:                rd,
		Db:                db,
		Token:             token,
		KieAPIKey:         kieAPIKey,
		KieCallbackURL:    kieCallbackURL,
		ChannelID:         channelID,
		ChannelLink:       channelLink,
		KieTasks:          kieTasks,
		KieAcquire:        kieAcquire,
		YouKassaID:        youKassaID,
		YouKassaSecretKey: youKassaSecretKey,
		YouKassaReturnURL: youKassaReturnURL,
		mediaGroups:       make(map[string]*mediaGroupEntry),
		modelPhotoGroups:  make(map[string]*modelPhotoGroupEntry),
		LogsChannelID:     LogsChannelID,
	}
}
