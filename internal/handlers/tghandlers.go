package handlers

import (
	"aibot/internal/database"
	"aibot/internal/state"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"aibot/internal/models"

	yoopayment "github.com/rvinnie/yookassa-sdk-go/yookassa/payment"
)

const (
	// Команды
	startCommand = "/start"

	// Callback data
	callbackClaimGenerations   = "claim_gen"
	callbackVerifySubscription = "verify_sub"
	callbackChat               = "chat"
	callbackPhoto              = "photo"
	callbackProfile            = "profile"
	callbackSupport            = "support"
	callbackBackMain           = "back_main"
	callbackBackMainDelete     = "back_main_del"
	callbackSubscribtion       = "subscribtion"
	callbackSubPlanLite        = "sub_plan_lite"
	callbackSubPlanStandard    = "sub_plan_standard"
	callbackSubPlanPremium     = "sub_plan_premium"
	callbackSubPlanConfirm     = "sub_plan_confirm"
	callbackPaySbp             = "pay_sbp"
	callbackPayCard            = "pay_card"
	callbackPayConfirm         = "pay_confirm"
	callbackPayCancelPrefix    = "pay_cancel:"

	// TTL и таймеры
	defaultStateTTL = 24 * time.Hour
	shortStateTTL   = 1 * time.Hour
	kieRetryMax     = 2
	kieRetryDelay   = 15 * time.Second

	// Redis state keys
	stateKeyMediaFileID    = "media_file_id"
	stateKeyMediaFileIDs   = "media_file_ids"
	stateKeyFileID         = "file_id"
	stateKeyPrompt         = "prompt"
	stateKeyHistory        = "history"
	stateKeyDocumentFileID = "document_file_id"
	stateKeyKieRetry       = "kie_retry"

	// Redis state steps
	stateStepAwaitMedia     = "await_photo_prompt"
	stateStepGetPhotoPrompt = "get_photo_prompt"
	stateStepAwaitText      = "await_text"
	stateStepGetTextPrompt  = "GetTextPrompt"

	// Сообщения бота
	msgMainMenuGreeting    = "<tg-emoji emoji-id=\"4997289591011544358\">⚡️</tg-emoji> Главное меню\n\n <tg-emoji emoji-id=\"5370607250731718891\">📸</tg-emoji> Осталось генераций фото: %v\n<tg-emoji emoji-id=\"5386691718571646404\">💬</tg-emoji> Осталось генераций чата: %v"
	msgSubscribeToGetTrial = "<tg-emoji emoji-id=\"5420323339723881652\">‼️</tg-emoji> Чтобы получить бесплатные генерации необходимо подписаться на канал"
	msgSubscribeToUse      = "‼️ Для использования бота необходимо подписаться на канал"
	msgNotSubscribed       = "❌ Вы не подписались"
	msgSupport             = "<tg-emoji emoji-id=\"5226876951855113572\">😇</tg-emoji> Поддержка\n\n <tg-emoji emoji-id=\"5406745015365943482\">⏬</tg-emoji> Перейдите по кнопке ниже чтобы связаться с нами"
	msgInstructionPhoto    = "<tg-emoji emoji-id=\"5370607250731718891\">📸</tg-emoji> Для обработки или генерации изображения пришлите запрос с медиа или без"
	msgInstructionChat     = "<tg-emoji emoji-id=\"5386691718571646404\">💬</tg-emoji>  Для получения ответа на ваш запрос напишите его в чат"
	msgPaymentSuccess      = "<tg-emoji emoji-id=\"5298780919207844086\">✅</tg-emoji> Подписка успешно приобретена!"
	msgPayLink             = "<tg-emoji emoji-id=\"5386757680679377085\">💵</tg-emoji> Оплатите по ссылке ниже"
	msgPayMethodSelect     = "Нажмите «Оплатить» для перехода к оплате."
	msgPayCreateError      = "<tg-emoji emoji-id=\"5420323339723881652\">‼️</tg-emoji> Не удалось создать платёж. Попробуйте позже."
	msgPayLinkError        = "<tg-emoji emoji-id=\"5420323339723881652\">‼️</tg-emoji> Не удалось получить ссылку на оплату. Попробуйте позже."
	msgNoSubscription      = "отсутствует"
	msgDefaultPhotoPrompt  = "Что на изображении?"
	msgDefaultDocPrompt    = "Что в документе?"
	msgNoPhotoLeft         = "<tg-emoji emoji-id=\"5420323339723881652\">‼️</tg-emoji> У вас закончились генерации фото. Приобретите подписку для продолжения."
	msgNoChatLeft          = "<tg-emoji emoji-id=\"5420323339723881652\">‼️</tg-emoji> У вас закончились генерации чата. Приобретите подписку для продолжения."
	msgSubProfileTemplate     = "<tg-emoji emoji-id=\"5373012449597335010\">👤</tg-emoji> Профиль\n\n<tg-emoji emoji-id=\"5413887962591026442\">💎</tg-emoji> Подписка: %s\n<tg-emoji emoji-id=\"5386415655253730366\">⏳</tg-emoji> Осталось: %s"
	msgNoSubProfileTemplate     = "<tg-emoji emoji-id=\"5373012449597335010\">👤</tg-emoji> Профиль\n\n<tg-emoji emoji-id=\"5413887962591026442\">💎</tg-emoji> Подписка: %s"

	// Тексты кнопок
	btnMakePhoto       = "📸 Сделать фото"
	btnAIAssistant     = "💬 AI Ассистент"
	btnProfile         = "👤 Профиль"
	btnSupportBtn      = "😇 Поддержка"
	btnBuySubscription = "⭐️ Купить подписку"
	btnBack            = "🔙 Назад"
	btnSubscribe       = "🔗 Подписаться"
	btnClaimGens       = "🎁 Забрать генерации"
	btnCheckSub        = "🔍 Проверить подписку"
	btnChoose          = "➡️ Выбрал!"
	btnPay             = "💸 Оплатить"
	btnGoToPay         = "🔗 Перейти к оплате"
	btnCancelPay       = "❌ Отменить платёж"
	btnSupport         = "💬 Написать"

	// Суммы
	amountLite    = 99
	amountPremium = 359

	// Стикеры
	stickerProcessingFileID = "CAACAgEAAxkBAAFDzohpqBdOnUhQJ-6arIQOV9OK3GEdxwACLQIAAqcjIUQ9QDDJ7YO0tjoE"

	// URLs
	subscribeGifURL = "https://s13.gifyu.com/images/bvlVB.gif"
	balooAIimageURL = "https://i.postimg.cc/Nfh87p10/1772617963019-vmar016bf7r.png"
)

// Тексты тарифов подписки (описание в сообщении).
var subscriptionPlans = []struct {
	ID          string
	Title       string // для caption (HTML)
	BtnTitle    string // для inline-кнопок (plain text)
	Cost        string
	Duration    string
	Generations string
}{
	{"lite", "<tg-emoji emoji-id=\"5224607267797606837\">☄️</tg-emoji> Lite — Легкий старт", "🚀 Lite — Легкий старт", fmt.Sprintf("%d₽", amountLite), "1 неделя", fmt.Sprintf("%d фото и безлимитный ассистент", database.LitePhotoLimit)},
	{"premium", "<tg-emoji emoji-id=\"5431766464040283359\">🔥</tg-emoji> Premium — Лучший вариант", "🔥 Premium — Лучший выбор", fmt.Sprintf("%d₽", amountPremium), "1 месяц", fmt.Sprintf("%d фото и безлимитный ассистент", database.PremiumPhotoLimit)},
}

func subscriptionPlanMessage(planID string) string {
	for _, p := range subscriptionPlans {
		if p.ID == planID {
			return fmt.Sprintf("%s\n\n<tg-emoji emoji-id=\"5409048419211682843\">💲</tg-emoji> Стоимость: %s\n<tg-emoji emoji-id=\"5386415655253730366\">⌛️</tg-emoji> Длительность: %s\n<tg-emoji emoji-id=\"5440841102871517055\">💥</tg-emoji> Количество генераций в подписке: %s", p.Title, p.Cost, p.Duration, p.Generations)
		}
	}
	return subscriptionPlans[0].Title // fallback Mini
}

func subscriptionPlanMarkup(selectedPlanID string) models.InlineKeyboardMarkup {
	callbacks := map[string]string{
		"lite": callbackSubPlanLite, "standard": callbackSubPlanStandard,
		"premium": callbackSubPlanPremium,
	}
	rows := make([][]models.InlineKeyboardButton, 0, len(subscriptionPlans)+2)
	for _, p := range subscriptionPlans {
		text := p.BtnTitle
		if p.ID == selectedPlanID {
			text += " ✅"
		}
		rows = append(rows, []models.InlineKeyboardButton{{Text: text, CallbackData: callbacks[p.ID]}})
	}
	rows = append(rows,
		[]models.InlineKeyboardButton{{Text: btnChoose, CallbackData: callbackSubPlanConfirm}},
		[]models.InlineKeyboardButton{{Text: btnBack, CallbackData: callbackBackMain}},
	)
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// sendMessage отправляет текстовое сообщение в чат.
func (h *Handler) SendMessage(ctx context.Context, chatID int64, text string) (int, error) {
	return h.sendMessageWithInlineKeyboard(ctx, chatID, text, models.InlineKeyboardMarkup{})
}

// SendPhoto скачивает изображение по URL и отправляет в чат файлом (multipart).
func (h *Handler) SendPhoto(ctx context.Context, chatID int64, photoURL string) error {
	apiURL := "https://api.telegram.org/bot" + h.Token + "/sendPhoto"
	reqGet, err := http.NewRequestWithContext(ctx, http.MethodGet, photoURL, nil)
	if err != nil {
		return fmt.Errorf("sendPhoto request: %w", err)
	}
	reqGet.Header.Set("User-Agent", "TelegramBot (compatible)")
	clientGet := &http.Client{Timeout: 45 * time.Second}
	respGet, err := clientGet.Do(reqGet)
	if err != nil {
		return fmt.Errorf("sendPhoto fetch: %w", err)
	}
	defer respGet.Body.Close()
	if respGet.StatusCode != http.StatusOK {
		return fmt.Errorf("sendPhoto fetch: status %d", respGet.StatusCode)
	}
	const maxSize = 10 << 20 // 10 MB
	img, err := io.ReadAll(io.LimitReader(respGet.Body, maxSize+1))
	if err != nil {
		return fmt.Errorf("sendPhoto read: %w", err)
	}
	if int64(len(img)) > maxSize {
		return fmt.Errorf("sendPhoto: image too large")
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	part, err := w.CreateFormFile("photo", "image.png")
	if err != nil {
		return fmt.Errorf("sendPhoto multipart: %w", err)
	}
	if _, err := part.Write(img); err != nil {
		return fmt.Errorf("sendPhoto write part: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("sendPhoto close multipart: %w", err)
	}

	req2, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, &buf)
	if err != nil {
		return fmt.Errorf("sendPhoto api request: %w", err)
	}
	req2.Header.Set("Content-Type", w.FormDataContentType())

	clientUpload := &http.Client{Timeout: 90 * time.Second}
	resp2, err := clientUpload.Do(req2)
	if err != nil {
		return fmt.Errorf("sendPhoto api: %w", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		body2, _ := io.ReadAll(io.LimitReader(resp2.Body, 512))
		return fmt.Errorf("sendPhoto: status %d %s", resp2.StatusCode, string(body2))
	}
	// Удаляем стикер «обработка», если он был сохранён для этого чата
	if stickerMsgID, ok := h.Rd.GetAndClearStickerMessageID(ctx, chatID); ok && stickerMsgID > 0 {
		_ = h.deleteMessage(ctx, chatID, stickerMsgID)
	}

	st, _, _ := h.Rd.Get(ctx, "bot:state:"+strconv.FormatInt(chatID, 10))
	if st.Data == nil {
		st.Data = make(map[string]string)
	}
	st.Step = stateStepAwaitMedia
	_ = h.Rd.Set(ctx, "bot:state:"+strconv.FormatInt(chatID, 10), st, defaultStateTTL)

	return nil
}

// deleteMessage удаляет сообщение в чате (Telegram Bot API deleteMessage).
func (h *Handler) deleteMessage(ctx context.Context, chatID int64, messageID int) error {
	apiURL := "https://api.telegram.org/bot" + h.Token + "/deleteMessage"
	body := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("deleteMessage: status %d", resp.StatusCode)
	}
	return nil
}

// sendAnimationWithCaptionAndKeyboard отправляет GIF с подписью и inline-клавиатурой (для приглашения подписаться).
func (h *Handler) sendAnimationWithCaptionAndKeyboard(ctx context.Context, chatID int64, gifURL, caption string, markup models.InlineKeyboardMarkup) error {
	apiURL := "https://api.telegram.org/bot" + h.Token + "/sendAnimation"
	body := map[string]any{
		"chat_id":   chatID,
		"animation": gifURL,
		"caption":   caption,
		"parse_mode": "HTML",
	}
	if len(markup.InlineKeyboard) > 0 {
		body["reply_markup"] = markup
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("telegram sendAnimation: status %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var out struct {
		Ok     bool `json:"ok"`
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil || !out.Ok {
		return nil
	}

	st, _, _ := h.Rd.Get(ctx, "bot:state:"+strconv.FormatInt(chatID, 10))
	if st.Data == nil {
		st.Data = make(map[string]string)
	}
	st.Data["animation_message_id"] = strconv.Itoa(out.Result.MessageID)
	_ = h.Rd.Set(ctx, "bot:state:"+strconv.FormatInt(chatID, 10), st, defaultStateTTL)

	return nil
}

// sendMessageWithInlineKeyboard отправляет сообщение с inline-клавиатурой в чат.
func (h *Handler) sendMessageWithInlineKeyboard(ctx context.Context, chatID int64, text string, markup models.InlineKeyboardMarkup) (int, error) {
	apiURL := "https://api.telegram.org/bot" + h.Token + "/sendMessage"
	body := map[string]any{
		"chat_id": chatID,
		"text":    text,
		"parse_mode": "HTML",
	}
	if len(markup.InlineKeyboard) > 0 {
		body["reply_markup"] = markup
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("telegram sendMessage: status %d", resp.StatusCode)
		return 0, fmt.Errorf("sendMessage: status %d", resp.StatusCode)
	}

	if stickerMsgID, ok := h.Rd.GetAndClearStickerMessageID(ctx, chatID); ok && stickerMsgID > 0 {
		_ = h.deleteMessage(ctx, chatID, stickerMsgID)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	var out struct {
		Ok     bool `json:"ok"`
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil || !out.Ok {
		return 0, nil
	}
	return out.Result.MessageID, nil
}

// sendPhotoWithCaptionAndKeyboard отправляет фото с подписью и inline-клавиатурой.
func (h *Handler) sendPhotoWithCaptionAndKeyboard(ctx context.Context, chatID int64, photoURL, caption string, markup models.InlineKeyboardMarkup) (int, error) {
	apiURL := "https://api.telegram.org/bot" + h.Token + "/sendPhoto"
	body := map[string]any{
		"chat_id": chatID,
		"photo":   photoURL,
		"caption": caption,
		"parse_mode": "HTML",
	}
	if len(markup.InlineKeyboard) > 0 {
		body["reply_markup"] = markup
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		log.Printf("telegram sendPhoto: status %d body=%s", resp.StatusCode, string(respBody))
		return 0, fmt.Errorf("sendPhoto: status %d", resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	var out struct {
		Ok     bool `json:"ok"`
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil || !out.Ok {
		return 0, nil
	}
	return out.Result.MessageID, nil
}

// editMessageCaptionWithKeyboard редактирует подпись (caption) и клавиатуру фото-сообщения.
func (h *Handler) editMessageCaptionWithKeyboard(ctx context.Context, chatID int64, messageID int, caption string, markup models.InlineKeyboardMarkup) error {
	apiURL := "https://api.telegram.org/bot" + h.Token + "/editMessageCaption"
	body := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"caption":    caption,
		"parse_mode": "HTML",
	}
	if len(markup.InlineKeyboard) > 0 {
		body["reply_markup"] = markup
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("telegram editMessageCaption: status %d", resp.StatusCode)
	}
	return nil
}

// answerCallbackQuery убирает "часики" у кнопки и опционально показывает уведомление (text — всплывающее сообщение).
func (h *Handler) answerCallbackQuery(ctx context.Context, callbackQueryID string, text string) error {
	apiURL := "https://api.telegram.org/bot" + h.Token + "/answerCallbackQuery"
	body := map[string]any{"callback_query_id": callbackQueryID}
	if text != "" {
		body["text"] = text
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("telegram answerCallbackQuery: status %d", resp.StatusCode)
	}
	return nil
}

// isSubscribedToChannel возвращает true, если пользователь userID подписан на канал h.ChannelID. Бот должен быть админом канала.
func (h *Handler) isSubscribedToChannel(ctx context.Context, userID int64) bool {
	if h.ChannelID == "" {
		return true
	}
	apiURL := "https://api.telegram.org/bot" + h.Token + "/getChatMember?chat_id=" + url.QueryEscape(h.ChannelID) + "&user_id=" + strconv.FormatInt(userID, 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}
	var out struct {
		Ok  bool   `json:"ok"`
		Err string `json:"description,omitempty"`
		Res struct {
			Status string `json:"status"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return false
	}
	// status: left, kicked => не подписан; member, administrator, creator => подписан
	s := strings.ToLower(out.Res.Status)
	return out.Ok && (s == "member" || s == "administrator" || s == "creator")
}

// mainMenuMarkup возвращает клавиатуру главного меню (для отправки и для редактирования).
func (h *Handler) mainMenuMarkup() models.InlineKeyboardMarkup {
	return models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: btnMakePhoto, CallbackData: callbackPhoto}},
			{{Text: btnAIAssistant, CallbackData: callbackChat}},
			{
				{Text: btnProfile, CallbackData: callbackProfile},
				{Text: btnSupportBtn, CallbackData: callbackSupport},
			},
			{{Text: btnBuySubscription, CallbackData: callbackSubscribtion}},
		},
	}
}

// mainMenuGreetingText возвращает отформатированный текст главного меню с оставшимися лимитами.
func (h *Handler) mainMenuGreetingText(ctx context.Context, chatID int64) string {
	remainingPhoto, _ := h.Db.GetRemainingPhoto(ctx, chatID)

	var chatDisplay string
	user, _ := h.Db.GetUserByChatID(ctx, chatID)
	if user.Subscription == "lite" || user.Subscription == "premium" {
		chatDisplay = "♾️"
	} else {
		remainingChat, _ := h.Db.GetRemainingChat(ctx, chatID)
		chatDisplay = strconv.Itoa(remainingChat)
	}

	return fmt.Sprintf(msgMainMenuGreeting, remainingPhoto, chatDisplay)
}

// sendMainMenu отправляет главное меню с фото и кнопками.
func (h *Handler) sendMainMenu(ctx context.Context, chatID int64) {
	message := h.mainMenuGreetingText(ctx, chatID)
	_, _ = h.sendPhotoWithCaptionAndKeyboard(ctx, chatID, balooAIimageURL, message, h.mainMenuMarkup())
}

// HandleUpdate обрабатывает входящее обновление от Telegram (вызывается воркерами).
func (h *Handler) HandleUpdate(ctx context.Context, upd models.Update) {
	var chatID int64
	if upd.CallbackQuery != nil && upd.CallbackQuery.Message != nil {
		chatID = upd.CallbackQuery.Message.Chat.ID
	} else if upd.Message != nil {
		chatID = upd.Message.Chat.ID
	} else {
		return
	}

	key := "bot:state:" + strconv.FormatInt(chatID, 10)
	st, _, _ := h.Rd.Get(ctx, key)

	// Сначала обрабатываем callback и /start — для новых пользователей записи в users ещё нет, GetSubscription вернёт ошибку.
	if upd.CallbackQuery != nil {
		h.handleCallbackQuery(ctx, upd.CallbackQuery)
		return
	}
	if upd.Message == nil {
		return
	}

	text := strings.TrimSpace(upd.Message.Text)
	if text == startCommand {
		h.handleStart(ctx, chatID, upd.Message)
		st, _, _ := h.Rd.Get(ctx, "bot:state:"+strconv.FormatInt(chatID, 10))
		if st.Data == nil {
			st.Data = make(map[string]string)
		}
		st.Step = "start"
		_ = h.Rd.Set(ctx, "bot:state:"+strconv.FormatInt(chatID, 10), st, defaultStateTTL)
		return
	}

	sub, err := h.Db.GetSubscription(ctx, chatID)
	if err != nil {
		return
	}

	if sub != "" && sub != "End" {
		ended, err := h.Db.IsSubscriptionEnd(ctx, chatID)
		if err == nil && ended {
			_ = h.Db.ClearSubscription(ctx, chatID)
		}
	}

	if st.Step == stateStepAwaitMedia && h.saveMediaAndPromptIfPresent(ctx, key, chatID, upd.Message) {
		return
	}

	if st.Step == stateStepAwaitText && h.saveTextAndFileIfPresent(ctx, key, chatID, upd.Message) {
		return
	}

	log.Printf("handle update_id=%d chat_id=%d text=%q step=%s", upd.UpdateID, chatID, upd.Message.Text, st.Step)
}

// handleStart обрабатывает /start: проверка подписки и used_trial, одно из трёх сообщений.
func (h *Handler) handleStart(ctx context.Context, chatID int64, msg *models.MessageContent) {
	userID := chatID
	userName := ""
	if msg.From != nil {
		userID = msg.From.ID
		if msg.From.Username != "" {
			userName = "@" + msg.From.Username
		} else {
			userName = msg.From.FirstName
		}
	}

	err := h.Db.CreateUser(ctx, models.User{TelegramID: userID, CreatedAt: time.Now(), UserName: userName})
	if err != nil {
		log.Printf("createUser: %v", err)
		return
	}

	subscribed := h.isSubscribedToChannel(ctx, userID)
	usedTrial, err := h.Db.GetUsedTrial(ctx, chatID)
	if err != nil {
		log.Printf("getUsedTrial: %v", err)
		return
	}

	if subscribed {
		st, _, _ := h.Rd.Get(ctx, "bot:state:"+strconv.FormatInt(chatID, 10))
		if st.Data == nil {
			st.Data = make(map[string]string)
		}

		if animationMessageID, ok := st.Data["animation_message_id"]; ok {
			animationMessageIDInt, err := strconv.Atoi(animationMessageID)
			if err != nil {
				log.Printf("Atoi animationMessageID: %v", err)
				return
			}
			_ = h.deleteMessage(ctx, chatID, animationMessageIDInt)
		}
		
		h.sendMainMenu(ctx, chatID)
		return
	}
	if !usedTrial {
		// Не подписан, триал не использован: GIF + «Чтобы получить бесплатные генерации...» + [Подписаться, Забрать генерации]
		markup := h.subscribeKeyboard(callbackClaimGenerations)
		if err := h.sendAnimationWithCaptionAndKeyboard(ctx, chatID, subscribeGifURL, msgSubscribeToGetTrial, markup); err != nil {
			log.Printf("sendAnimation /start (trial): %v", err)
		}
		return
	}
	// Использовал триал, не подписан: GIF + «Для использования подпишитесь...» + [Подписаться, Проверить подписку]
	markup := h.subscribeKeyboard(callbackVerifySubscription)
	if err := h.sendAnimationWithCaptionAndKeyboard(ctx, chatID, subscribeGifURL, msgSubscribeToUse, markup); err != nil {
		log.Printf("sendAnimation /start (sub): %v", err)
	}
}

// subscribeKeyboard возвращает клавиатуру: [Подписаться (URL), Вторая кнопка по callbackData].
func (h *Handler) subscribeKeyboard(secondCallback string) models.InlineKeyboardMarkup {
	subscribeURL := h.ChannelLink
	if subscribeURL == "" && strings.HasPrefix(h.ChannelID, "@") {
		subscribeURL = "https://t.me/" + strings.TrimPrefix(h.ChannelID, "@")
	}
	row := []models.InlineKeyboardButton{
		{Text: btnSubscribe, URL: subscribeURL},
	}
	secondText := btnClaimGens
	if secondCallback == callbackVerifySubscription {
		secondText = btnCheckSub
	}
	row = append(row, models.InlineKeyboardButton{Text: secondText, CallbackData: secondCallback})
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{row}}
}

func (h *Handler) handleCallbackQuery(ctx context.Context, cq *models.CallbackQuery) {
	if cq.Message == nil {
		return
	}
	chatID := cq.Message.Chat.ID
	userID := chatID
	if cq.From != nil {
		userID = cq.From.ID
	}

	switch cq.Data {
	case callbackClaimGenerations:
		h.handleClaimGenerations(ctx, cq.ID, chatID, userID)
	case callbackVerifySubscription:
		h.handleVerifySubscription(ctx, cq.ID, chatID, userID)
	case callbackPhoto:
		h.handlePhoto(ctx, cq.ID, chatID, cq.Message.MessageID)
	case callbackChat:
		h.handleChat(ctx, cq.ID, chatID, cq.Message.MessageID)
	case callbackBackMain:
		h.handleBackMain(ctx, cq.ID, chatID, cq.Message.MessageID)
	case callbackBackMainDelete:
		h.handleBackMainDelete(ctx, cq.ID, chatID, cq.Message.MessageID)
	case callbackProfile:
		h.handleProfile(ctx, cq.ID, chatID, cq.Message.MessageID)
	case callbackSupport:
		h.handleSupport(ctx, cq.ID, chatID, cq.Message.MessageID)
	case callbackSubscribtion:
		h.handleSubscribtion(ctx, cq.ID, chatID, cq.Message.MessageID)
	case callbackSubPlanLite, callbackSubPlanStandard, callbackSubPlanPremium:
		h.handleSubPlanSelect(ctx, cq.ID, chatID, cq.Message.MessageID, cq.Data)
	case callbackPaySbp, callbackPayCard:
		h.handlePayMethodSelect(ctx, cq.ID, chatID, cq.Message.MessageID, cq.Data)
	case callbackSubPlanConfirm:
		h.handlePayConfirm(ctx, cq.ID, chatID, cq.Message.MessageID)
	default:
		if strings.HasPrefix(cq.Data, callbackPayCancelPrefix) {
			paymentID := strings.TrimPrefix(cq.Data, callbackPayCancelPrefix)
			h.handlePayCancel(ctx, cq.ID, chatID, cq.Message.MessageID, paymentID)
		} else {
			_ = h.answerCallbackQuery(ctx, cq.ID, "")
		}
	}
}

func (h *Handler) handleClaimGenerations(ctx context.Context, callbackID string, chatID, userID int64) {
	subscribed := h.isSubscribedToChannel(ctx, userID)
	if !subscribed {
		_ = h.answerCallbackQuery(ctx, callbackID, msgNotSubscribed)
		return
	}
	_ = h.answerCallbackQuery(ctx, callbackID, "")

	h.Db.SetUsedTrial(ctx, chatID)

	st, _, _ := h.Rd.Get(ctx, "bot:state:"+strconv.FormatInt(chatID, 10))
	if st.Data == nil {
		st.Data = make(map[string]string)
	}

	if animationMessageID, ok := st.Data["animation_message_id"]; ok {
		animationMessageIDInt, err := strconv.Atoi(animationMessageID)
		if err != nil {
			log.Printf("Atoi animationMessageID: %v", err)
			return
		}
		_ = h.deleteMessage(ctx, chatID, animationMessageIDInt)
	}

	h.sendMainMenu(ctx, chatID)
}

func (h *Handler) handleVerifySubscription(ctx context.Context, callbackID string, chatID, userID int64) {
	subscribed := h.isSubscribedToChannel(ctx, userID)
	if !subscribed {
		_ = h.answerCallbackQuery(ctx, callbackID, msgNotSubscribed)
		return
	}
	_ = h.answerCallbackQuery(ctx, callbackID, "")

	st, _, _ := h.Rd.Get(ctx, "bot:state:"+strconv.FormatInt(chatID, 10))
	if st.Data == nil {
		st.Data = make(map[string]string)
	}

	if animationMessageID, ok := st.Data["animation_message_id"]; ok {
		animationMessageIDInt, err := strconv.Atoi(animationMessageID)
		if err != nil {
			log.Printf("Atoi animationMessageID: %v", err)
			return
		}
		_ = h.deleteMessage(ctx, chatID, animationMessageIDInt)
	}

	h.sendMainMenu(ctx, chatID)
}

func (h *Handler) handlePhoto(ctx context.Context, callbackID string, chatID int64, messageID int) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")

	remaining, _ := h.Db.GetRemainingPhoto(ctx, chatID)
	if remaining <= 0 {
		buyMarkup := models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: btnBuySubscription, CallbackData: callbackSubscribtion}},
				{{Text: btnBack, CallbackData: callbackBackMain}},
			},
		}
		if err := h.editMessageCaptionWithKeyboard(ctx, chatID, messageID, msgNoPhotoLeft, buyMarkup); err != nil {
			log.Printf("editMessage no photo left: %v", err)
		}
		return
	}

	backMarkup := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: btnBack, CallbackData: callbackBackMainDelete}},
		},
	}
	if err := h.editMessageCaptionWithKeyboard(ctx, chatID, messageID, msgInstructionPhoto, backMarkup); err != nil {
		log.Printf("editMessage instruction photo: %v", err)
	}
	st, _, _ := h.Rd.Get(ctx, "bot:state:"+strconv.FormatInt(chatID, 10))
	if st.Data == nil {
		st.Data = make(map[string]string)
	}
	st.Step = stateStepAwaitMedia
	_ = h.Rd.Set(ctx, "bot:state:"+strconv.FormatInt(chatID, 10), st, defaultStateTTL)
}

func (h *Handler) handleChat(ctx context.Context, callbackID string, chatID int64, messageID int) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")

	remaining, _ := h.Db.GetRemainingChat(ctx, chatID)
	if remaining <= 0 {
		buyMarkup := models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: btnBuySubscription, CallbackData: callbackSubscribtion}},
				{{Text: btnBack, CallbackData: callbackBackMain}},
			},
		}
		if err := h.editMessageCaptionWithKeyboard(ctx, chatID, messageID, msgNoChatLeft, buyMarkup); err != nil {
			log.Printf("editMessage no chat left: %v", err)
		}
		return
	}

	backMarkup := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: btnBack, CallbackData: callbackBackMainDelete}},
		},
	}
	if err := h.editMessageCaptionWithKeyboard(ctx, chatID, messageID, msgInstructionChat, backMarkup); err != nil {
		log.Printf("editMessage instruction chat: %v", err)
	}
	st, _, _ := h.Rd.Get(ctx, "bot:state:"+strconv.FormatInt(chatID, 10))
	if st.Data == nil {
		st.Data = make(map[string]string)
	}
	st.Step = stateStepAwaitText
	_ = h.Rd.Set(ctx, "bot:state:"+strconv.FormatInt(chatID, 10), st, defaultStateTTL)
}

func (h *Handler) handleBackMain(ctx context.Context, callbackID string, chatID int64, messageID int) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")
	greeting := h.mainMenuGreetingText(ctx, chatID)
	if err := h.editMessageCaptionWithKeyboard(ctx, chatID, messageID, greeting, h.mainMenuMarkup()); err != nil {
		log.Printf("editMessage back main: %v", err)
	}
}

func (h *Handler) handleBackMainDelete(ctx context.Context, callbackID string, chatID int64, messageID int) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")
	_ = h.deleteMessage(ctx, chatID, messageID)
	st, _, _ := h.Rd.Get(ctx, "bot:state:"+strconv.FormatInt(chatID, 10))
	if st.Data == nil {
		st.Data = make(map[string]string)
	}
	st.Step = "main_menu"
	_ = h.Rd.Set(ctx, "bot:state:"+strconv.FormatInt(chatID, 10), st, defaultStateTTL)
	h.sendMainMenu(ctx, chatID)
}

func (h *Handler) handleProfile(ctx context.Context, callbackID string, chatID int64, messageID int) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")
	backMarkup := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: btnBack, CallbackData: callbackBackMain}},
		},
	}

	user, err := h.Db.GetUserByChatID(ctx, chatID)
	if err != nil {
		log.Printf("getUserByChatID: %v", err)
		return
	}

	remainingDays := int(time.Until(user.SubscriptionEndAt) / (24 * time.Hour))
	if remainingDays < 0 {
		remainingDays = 0
	}
	remainingDaysString := fmt.Sprintf("%d дней", remainingDays)

	subscription := user.Subscription

	var messageProfile string

	if subscription == "" {
		subscription = msgNoSubscription
		messageProfile = fmt.Sprintf(msgNoSubProfileTemplate, subscription)
	} else {
		messageProfile = fmt.Sprintf(msgSubProfileTemplate, subscription, remainingDaysString)
	}

	if err := h.editMessageCaptionWithKeyboard(ctx, chatID, messageID, messageProfile, backMarkup); err != nil {
		log.Printf("editMessage profile: %v", err)
	}
}

func (h *Handler) handleSupport(ctx context.Context, callbackID string, chatID int64, messageID int) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")
	backMarkup := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: btnSupport, URL: "https://t.me/BalooBoss"}},
			{{Text: btnBack, CallbackData: callbackBackMain}},
		},
	}
	if err := h.editMessageCaptionWithKeyboard(ctx, chatID, messageID, msgSupport, backMarkup); err != nil {
		log.Printf("editMessage support: %v", err)
	}
}

func (h *Handler) handleSubscribtion(ctx context.Context, callbackID string, chatID int64, messageID int) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")
	text := subscriptionPlanMessage("lite")
	markup := subscriptionPlanMarkup("lite")
	if err := h.editMessageCaptionWithKeyboard(ctx, chatID, messageID, text, markup); err != nil {
		log.Printf("editMessage subscribtion: %v", err)
	}
}

func (h *Handler) handleSubPlanSelect(ctx context.Context, callbackID string, chatID int64, messageID int, callbackData string) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")
	var planID string
	switch callbackData {
	case callbackSubPlanLite:
		planID = "lite"
	//case callbackSubPlanStandard:
	//	planID = "standard"
	case callbackSubPlanPremium:
		planID = "premium"
	default:
		planID = "lite"
	}
	text := subscriptionPlanMessage(planID)
	markup := subscriptionPlanMarkup(planID)
	if err := h.editMessageCaptionWithKeyboard(ctx, chatID, messageID, text, markup); err != nil {
		log.Printf("editMessage sub plan: %v", err)
	}

	key := "bot:state:" + strconv.FormatInt(chatID, 10)
	st, _, _ := h.Rd.Get(ctx, key)

	if st.Data == nil {
		st.Data = make(map[string]string)
	}

	st.Data["plan"] = planID
	_ = h.Rd.Set(ctx, key, st, defaultStateTTL)
}

func (h *Handler) handlePayMethodSelect(ctx context.Context, callbackID string, chatID int64, messageID int, callbackData string) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")
	var methodID string
	switch callbackData {
	case callbackPaySbp:
		methodID = "sbp"
	// case callbackPayCard:
	// 	methodID = "card"
	default:
		methodID = "sbp"
	}
	key := "bot:state:" + strconv.FormatInt(chatID, 10)
	st, _, _ := h.Rd.Get(ctx, key)
	if st.Data == nil {
		st.Data = make(map[string]string)
	}
	st.Data["payment_method"] = methodID
	_ = h.Rd.Set(ctx, key, st, defaultStateTTL)

	markup := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: btnPay, CallbackData: callbackPayConfirm}},
			{{Text: btnBack, CallbackData: callbackBackMain}},
		},
	}
	if err := h.editMessageCaptionWithKeyboard(ctx, chatID, messageID, msgPayMethodSelect, markup); err != nil {
		log.Printf("editMessage pay method: %v", err)
	}
}

func (h *Handler) handlePayConfirm(ctx context.Context, callbackID string, chatID int64, messageID int) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")

	keybot := "bot:state:" + strconv.FormatInt(chatID, 10)
	stbot, _, _ := h.Rd.Get(ctx, keybot)

	if stbot.Data == nil {
		stbot.Data = make(map[string]string)
	}

	if stbot.Data["plan"] == "" {
		stbot.Data["plan"] = "lite"
	}

	amount := 0

	switch stbot.Data["plan"] {
	case "lite":
		amount = amountLite
	case "premium":
		amount = amountPremium
	default:
		amount = amountLite
	}

	if amount == 0 {
		amount = amountLite
	}

	log.Printf("plan: %s, amount: %d", stbot.Data["plan"], amount)

	payment := h.CreatePayment(amount, stbot.Data["plan"])
	if payment == nil || payment.ID == "" {
		_ = h.editMessageCaptionWithKeyboard(ctx, chatID, messageID, msgPayCreateError, h.mainMenuMarkup())
		return
	}

	keypay := "ym:payment:" + payment.ID
	stpay := state.State{
		Data: map[string]string{
			"user_id":    strconv.FormatInt(chatID, 10),
			"message_id": strconv.Itoa(messageID),
		},
	}
	_ = h.Rd.Set(ctx, keypay, stpay, 24*time.Hour)

	log.Printf("almost get link")

	var payURL string
	if payment.Confirmation != nil {
		if m, ok := payment.Confirmation.(map[string]interface{}); ok {
			if u, _ := m["confirmation_url"].(string); u != "" {
				payURL = u
			}
		}
		if payURL == "" {
			switch c := payment.Confirmation.(type) {
			case *yoopayment.Redirect:
				if c != nil {
					payURL = c.ConfirmationURL
				}
			case yoopayment.Redirect:
				payURL = c.ConfirmationURL
			}
		}
	}

	if payURL == "" {
		_ = h.editMessageCaptionWithKeyboard(ctx, chatID, messageID, msgPayLinkError, h.mainMenuMarkup())
		return
	}

	log.Printf("Payment URL: %s", payURL)

	markup := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: btnGoToPay, URL: payURL}},
			{{Text: btnCancelPay, CallbackData: callbackPayCancelPrefix + payment.ID}},
		},
	}
	if err := h.editMessageCaptionWithKeyboard(ctx, chatID, messageID, msgPayLink, markup); err != nil {
		log.Printf("editMessage pay confirm: %v", err)
	}
}

func (h *Handler) handlePayCancel(ctx context.Context, callbackID string, chatID int64, messageID int, paymentID string) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")
	_ = h.CancelPayment(paymentID)
	greeting := h.mainMenuGreetingText(ctx, chatID)
	if err := h.editMessageCaptionWithKeyboard(ctx, chatID, messageID, greeting, h.mainMenuMarkup()); err != nil {
		log.Printf("editMessage pay cancel: %v", err)
	}
}

// sendSticker отправляет стикер по file_id и возвращает message_id (или 0 при ошибке).
func (h *Handler) sendSticker(ctx context.Context, chatID int64, stickerFileID string) (int, error) {
	apiURL := "https://api.telegram.org/bot" + h.Token + "/sendSticker"
	body := map[string]any{
		"chat_id": chatID,
		"sticker": stickerFileID,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Printf("telegram sendSticker: status %d", resp.StatusCode)
		return 0, fmt.Errorf("telegram sendSticker: status %d", resp.StatusCode)
	}
	var out struct {
		Ok     bool `json:"ok"`
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil || !out.Ok {
		return 0, nil
	}
	return out.Result.MessageID, nil
}

// saveMediaAndPromptIfPresent сохраняет file_id фото и промпт (caption) в Redis.
// Поддерживает media groups (альбомы): накапливает все фото группы и обрабатывает разом.
func (h *Handler) saveMediaAndPromptIfPresent(ctx context.Context, key string, chatID int64, msg *models.MessageContent) bool {
	if len(msg.Photo) == 0 {
		return false
	}

	remaining, err := h.Db.GetRemainingPhoto(ctx, chatID)
	if err != nil {
		log.Printf("GetRemainingPhoto chat_id=%d: %v", chatID, err)
	}
	if remaining <= 0 {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			msgID, _ := h.SendMessage(bgCtx, chatID, msgNoPhotoLeft)
			if msgID > 0 {
				time.Sleep(5 * time.Second)
				_ = h.deleteMessage(bgCtx, chatID, msgID)
			}
		}()
		return false
	}

	fileID := msg.Photo[len(msg.Photo)-1].FileID

	if msg.MediaGroupID != "" {
		return h.handleMediaGroup(ctx, key, chatID, msg.MediaGroupID, fileID, msg.Caption)
	}

	if msg.Caption == "" {
		return false
	}
	h.dispatchPhotoTask(ctx, key, chatID, []string{fileID}, msg.Caption)
	return true
}

// handleMediaGroup буферизирует фото из media group и запускает обработку после таймаута.
func (h *Handler) handleMediaGroup(ctx context.Context, key string, chatID int64, groupID, fileID, caption string) bool {
	h.mediaGroupsMu.Lock()
	entry, exists := h.mediaGroups[groupID]
	if !exists {
		entry = &mediaGroupEntry{
			ChatID: chatID,
			Timer:  &sync.Once{},
		}
		h.mediaGroups[groupID] = entry
	}
	entry.FileIDs = append(entry.FileIDs, fileID)
	if caption != "" {
		entry.Caption = caption
	}
	h.mediaGroupsMu.Unlock()

	entry.Timer.Do(func() {
		go func() {
			time.Sleep(2 * time.Second)

			h.mediaGroupsMu.Lock()
			final := h.mediaGroups[groupID]
			delete(h.mediaGroups, groupID)
			h.mediaGroupsMu.Unlock()

			if final == nil || len(final.FileIDs) == 0 {
				return
			}
			if final.Caption == "" {
				log.Printf("media group %s: no caption, skipping", groupID)
				return
			}

			bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			h.dispatchPhotoTask(bgCtx, key, chatID, final.FileIDs, final.Caption)
		}()
	})

	return true
}

// dispatchPhotoTask сохраняет file_id(s) и промпт в Redis, отправляет стикер и кладёт задачу в KieTasks.
func (h *Handler) dispatchPhotoTask(ctx context.Context, key string, chatID int64, fileIDs []string, caption string) {
	st, _, _ := h.Rd.Get(ctx, key)
	if st.Data == nil {
		st.Data = make(map[string]string)
	}
	st.Data[stateKeyMediaFileID] = fileIDs[0]
	if len(fileIDs) > 1 {
		joined, _ := json.Marshal(fileIDs)
		st.Data[stateKeyMediaFileIDs] = string(joined)
	} else {
		delete(st.Data, stateKeyMediaFileIDs)
	}
	st.Data[stateKeyPrompt] = caption
	st.Step = "GetPhotoPrompt"

	if err := h.Rd.Set(ctx, key, st, shortStateTTL); err != nil {
		log.Printf("Redis Set: %v", err)
		return
	}
	log.Printf("saved to Redis chat_id=%d file_ids=%v prompt_len=%d", chatID, fileIDs, len(caption))
	if msgID, err := h.sendSticker(ctx, chatID, stickerProcessingFileID); err != nil {
		log.Printf("sendSticker: %v", err)
	} else if msgID > 0 {
		_ = h.Rd.SetStickerMessageID(ctx, chatID, msgID, defaultStateTTL)
	}

	if h.KieTasks != nil {
		select {
		case h.KieTasks <- chatID:
			log.Printf("pushed to kieTasks chat_id=%d", chatID)
		default:
			log.Printf("kie queue full, chat_id=%d dropped", chatID)
		}
	}
}

// saveTextAndFileIfPresent обрабатывает сообщение в режиме AI ЧАТ: сохраняет текст, фото (file_id) или документ (file_id) в state. Возвращает true, если сообщение принято.
func (h *Handler) saveTextAndFileIfPresent(ctx context.Context, key string, chatID int64, msg *models.MessageContent) bool {
	remaining, err := h.Db.GetRemainingChat(ctx, chatID)
	if err != nil {
		log.Printf("GetRemainingChat chat_id=%d: %v", chatID, err)
	}
	if remaining <= 0 {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			msgID, _ := h.SendMessage(bgCtx, chatID, msgNoChatLeft)
			if msgID > 0 {
				time.Sleep(5 * time.Second)
				_ = h.deleteMessage(bgCtx, chatID, msgID)
			}
		}()
		return false
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		text = strings.TrimSpace(msg.Caption)
	}
	hasPhoto := len(msg.Photo) > 0
	hasDocument := msg.Document != nil && msg.Document.FileID != ""
	if text == "" && !hasPhoto && !hasDocument {
		return false
	}
	if text == "" && hasPhoto {
		text = msgDefaultPhotoPrompt
	}
	if text == "" && hasDocument {
		text = msgDefaultDocPrompt
	}

	st, _, _ := h.Rd.Get(ctx, key)
	if st.Data == nil {
		st.Data = make(map[string]string)
	}
	st.Data[stateKeyPrompt] = text
	if hasPhoto {
		st.Data[stateKeyFileID] = msg.Photo[len(msg.Photo)-1].FileID
		delete(st.Data, stateKeyDocumentFileID)
	} else if hasDocument {
		st.Data[stateKeyDocumentFileID] = msg.Document.FileID
		if msg.Document.FileName != "" {
			st.Data["document_file_name"] = msg.Document.FileName
		}
		delete(st.Data, stateKeyFileID)
	} else {
		delete(st.Data, stateKeyFileID)
		delete(st.Data, stateKeyDocumentFileID)
		delete(st.Data, "document_file_name")
	}

	var history []models.HistoryEntry
	historyJSON := st.Data[stateKeyHistory]
	if historyJSON == "" {
		history = []models.HistoryEntry{}
	} else {
		if err := json.Unmarshal([]byte(historyJSON), &history); err != nil {
			log.Printf("json.Unmarshal history: %v", err)
			history = []models.HistoryEntry{}
		}
	}
	history = append(history, models.HistoryEntry{Role: "user", Prompt: text, FileURL: st.Data[stateKeyFileID]})
	if n := len(history) - 10; n > 0 {
		history = history[n:]
	}
	b, errMarshal := json.Marshal(history)
	if errMarshal != nil {
		log.Printf("json.Marshal history: %v", errMarshal)
		return false
	}
	st.Data[stateKeyHistory] = string(b)
	st.Step = stateStepGetTextPrompt

	if err := h.Rd.Set(ctx, key, st, defaultStateTTL); err != nil {
		log.Printf("Redis Set: %v", err)
		return false
	}
	log.Printf("saved to Redis chat_id=%d prompt_len=%d (AI ЧАТ)", chatID, len(text))


	if msgID, err := h.sendSticker(ctx, chatID, stickerProcessingFileID); err != nil {
		log.Printf("sendSticker: %v", err)
	} else if msgID > 0 {
		_ = h.Rd.SetStickerMessageID(ctx, chatID, msgID, defaultStateTTL)
	}

	if h.KieTasks != nil {
		select {
		case h.KieTasks <- chatID:
			log.Printf("pushed to KieTasks chat_id=%d (AI ЧАТ)", chatID)
		default:
			log.Printf("KieTasks queue full, chat_id=%d dropped", chatID)
		}
	}
	return true
}

func (h *Handler) HandlePayment(ctx context.Context, payment models.YouMoneyResult) {
	if payment.Status == "succeeded" {
		if payment.MessageID > 0 {
			_ = h.deleteMessage(ctx, payment.ChatID, payment.MessageID)
		}

		if err := h.Db.EnsureUserExists(ctx, payment.ChatID); err != nil {
			log.Printf("EnsureUserExists chat_id=%d: %v", payment.ChatID, err)
		}
		subName := payment.Description
		log.Printf("subName: %s", subName)
		if err := h.Db.SetSubscription(ctx, payment.ChatID, subName); err != nil {
			log.Printf("SetSubscription chat_id=%d: %v", payment.ChatID, err)
		}

		msgID, err := h.SendMessage(ctx, payment.ChatID, msgPaymentSuccess)
		if err != nil {
			log.Printf("SendMessage chat_id=%d: %v", payment.ChatID, err)
		}

		if msgID > 0 {
			chatID := payment.ChatID
			go func() {
				delCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				h.sendMainMenu(delCtx, chatID)
				time.Sleep(5 * time.Second)
				_ = h.deleteMessage(delCtx, chatID, msgID)
			}()
		} else {
			h.sendMainMenu(ctx, payment.ChatID)
		}
	} else if payment.Status != "canceled" {
		text := fmt.Sprintf("❌ Произошла ошибка! %s", payment.Status)
		_, _ = h.SendMessage(ctx, payment.ChatID, text)
	}
}
