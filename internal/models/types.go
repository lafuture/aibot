package models

import "time"

// MessageChat — чат в сообщении.
type MessageChat struct {
	ID int64 `json:"id"`
}

// PhotoSize — размер фото (Telegram API); у фото берём последний — самый большой.
type PhotoSize struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
}

// Document — документ в сообщении (Telegram API).
type Document struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

// MessageContent — содержимое сообщения (текст, фото, документ, подпись).
type MessageContent struct {
	Text     string        `json:"text"`
	Chat     MessageChat   `json:"chat"`
	From     *TelegramUser `json:"from,omitempty"`
	Photo    []PhotoSize   `json:"photo,omitempty"`
	Document *Document     `json:"document,omitempty"`
	Caption  string        `json:"caption,omitempty"`
}

// CallbackQuery — нажатие на inline-кнопку.
type CallbackQuery struct {
	ID      string        `json:"id"`
	Data    string        `json:"data"`
	From    *TelegramUser `json:"from,omitempty"`
	Message *struct {
		MessageID int         `json:"message_id"`
		Chat      MessageChat `json:"chat"`
	} `json:"message,omitempty"`
}

type Update struct {
	UpdateID      int             `json:"update_id"`
	Message       *MessageContent `json:"message,omitempty"`
	CallbackQuery *CallbackQuery  `json:"callback_query,omitempty"`
}

type Response struct {
	Ok          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
}

// InlineKeyboardButton — кнопка inline-клавиатуры (Telegram API).
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

// InlineKeyboardMarkup — разметка inline-клавиатуры для reply_markup.
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type FileOut struct {
	Ok     bool `json:"ok"`
	Result struct {
		FilePath string `json:"file_path"`
	} `json:"result"`
}

// KieBody — ответ API kie.ai на createTask (успех и ошибки при HTTP 200).
type KieBody struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		TaskID string `json:"taskId"`
	} `json:"data"`
}

// KieResult — внутренняя структура для канала: отправить фото в Telegram.
type KieResult struct {
	ChatID   int64  `json:"-"`
	ImageURL string `json:"-"`
	Text     string `json:"-"`
}

// KieCallbackResp — тело POST callback от kie.ai при завершении генерации.
type KieCallbackResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		CompleteTime int64  `json:"completeTime"`
		CostTime     int    `json:"costTime"`
		CreateTime   int64  `json:"createTime"`
		Model        string `json:"model"`
		Param        string `json:"param"`
		ResultJSON   string `json:"resultJson"`
		State        string `json:"state"`
		TaskID       string `json:"taskId"`
		UpdateTime   int64  `json:"updateTime"`
	} `json:"data"`
}

type KieResultURLs struct {
	ResultUrls []string `json:"resultUrls"`
}

type TelegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name,omitempty"`
	Username  string `json:"username,omitempty"` // @username в Telegram (без @)
}

type User struct {
	TelegramID int64     `json:"telegram_id"`
	UserName   string    `json:"user_name"`
	CreatedAt  time.Time `json:"created_at"`

	Statuses string `json:"statuses"`

	Subscription        string    `json:"subscription"`
	SubscriptionStatus  string    `json:"subscription_status"`
	SubscriptionEndAt   time.Time `json:"subscription_end_at"`
	SubscriptionStartAt time.Time `json:"subscription_start_at"`

	LimitsPhoto      int `json:"limits_photo"`
	LimitsChat      int `json:"limits_chat"`
	RemainingPhoto   int `json:"remaining_photo"`
	RemainingChat   int `json:"remaining_chat"`
	UsedPhoto int `json:"used_photo"`
	UsedChat int `json:"used_chat"`
	UsedTrial   int `json:"used_trial"`
}

type HistoryEntry struct {
	Role    string `json:"Role"`
	Prompt  string `json:"Prompt"`
	FileURL string `json:"FileURL"`
}

// ChatCompletionResponse — ответ API gemini (kie.ai), текст в choices[0].message.content.
type ChatCompletionResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Index        int    `json:"index"`
		Message      struct {
			Content string `json:"content"`
			Role    string `json:"role"`
		} `json:"message"`
	} `json:"choices"`
}