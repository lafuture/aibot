package handlers

import (
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
	"time"


	"aibot/internal/models"
)

const (
	startCommand               = "/start"
	callbackClaimGenerations   = "claim_gen"  // забрать генерации (проверка подписки)
	callbackSubscribe          = "sub"        // подписаться (URL к каналу)
	callbackVerifySubscription = "verify_sub" // проверить подписку
	callbackChat               = "chat"
	callbackPhoto              = "photo"
	callbackProfile            = "profile"
	callbackSupport            = "support"
	callbackBackMain           = "back_main"
	instructionPhotoText            = "Для обработки или генерации изображения пришлите медиа и промпт"
	instructionChatText            = "Для получения ответа на ваш запрос напишите его в чат"
	defaultStateTTL            = 24 * time.Hour
	stateKeyMediaFileID        = "media_file_id"
	stateKeyFileID             = "file_id"
	stateKeyPrompt             = "prompt"
	stateStepAwaitMedia        = "await_photo_prompt"
	stateStepGetPhotoPrompt    = "get_photo_prompt"
	stateStepAwaitText         = "await_text"
	stateStepGetTextPrompt     = "GetTextPrompt"
	startProcessing            = "Медиа и промпт сохранены, начинаем обработку..."
	stateKeyKieRetry           = "kie_retry"
	kieRetryMax                = 2
	kieRetryDelay              = 15 * time.Second
	msgSubscribeToGetTrial     = "Чтобы получить бесплатные генерации необходимо подписаться на канал"
	msgSubscribeToUse          = "Для использования бота необходимо подписаться на канал"
	msgNotSubscribed           = "Вы не подписались"
	msgMainMenuGreeting        = "Главное меню"
	msgProfile                 = "Профиль: здесь будет информация о вашем аккаунте."
	msgSupport                 = "Поддержка: напишите нам @support или перейдите по ссылке из описания бота."
	msgSubscribtionPlaceholder  = "Раздел «Купить подписку» в разработке."
	stateKeyHistory            = "history"
	stateKeyDocumentFileID     = "document_file_id"
	shortStateTTL              = 1 * time.Hour
	callbackSubscribtion       = "subscribtion"
	callbackSubPlanLite        = "sub_plan_lite"
	callbackSubPlanStandard    = "sub_plan_standard"
	callbackSubPlanPremium     = "sub_plan_premium"
	callbackSubPlanConfirm     = "sub_plan_confirm"
	callbackPaySbp             = "pay_sbp"
	callbackPayCard            = "pay_card"
	callbackPayConfirm         = "pay_confirm"
	msgSubPlanConfirm          = "Выберите способ оплаты:"
	subscribeGifURL            = "https://s13.gifyu.com/images/bvlVB.gif" // GIF для неподписанных (при invite)
)

var paymentMethods = []struct {
	ID    string
	Title string
}{
	{"sbp", "СБП"},
	{"card", "Карта"},
}

func paymentMethodMarkup(selectedID string) models.InlineKeyboardMarkup {
	callbacks := map[string]string{
		"sbp":  callbackPaySbp,
		"card": callbackPayCard,
	}
	rows := make([][]models.InlineKeyboardButton, 0, len(paymentMethods)+2)
	for _, m := range paymentMethods {
		text := m.Title
		if m.ID == selectedID {
			text += " ✅"
		}
		rows = append(rows, []models.InlineKeyboardButton{{Text: text, CallbackData: callbacks[m.ID]}})
	}
	rows = append(rows,
		[]models.InlineKeyboardButton{{Text: "➡️ Оплатить", CallbackData: callbackPayConfirm}},
		[]models.InlineKeyboardButton{{Text: "Назад", CallbackData: callbackBackMain}},
	)
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// Тексты тарифов подписки (описание в сообщении).
var subscriptionPlans = []struct {
	ID           string
	Title        string
	Cost         string
	Duration     string
	Generations  string
}{
	{"lite", "Lite — Легкий старт", "99₽ — влюбиться в себя заново", "1 неделя", "9 фото + безлимитный ассистент"},
	{"standard", "Standard — Золотая середина", "299₽ — больше генераций и свободы.", "1 месяц", "50 фото + безлимитный ассистент"},
	{"premium", "Premium — Для требовательных.", "599₽ — максимум возможностей.", "1 месяц", "120 фото + безлимитный ассистент"},
}

func subscriptionPlanMessage(planID string) string {
	for _, p := range subscriptionPlans {
		if p.ID == planID {
			return fmt.Sprintf("%s\n\n• Стоимость: %s\n• Длительность: %s\n• Количество генераций в подписке: %s", p.Title, p.Cost, p.Duration, p.Generations)
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
		text := p.Title
		if p.ID == selectedPlanID {
			text += " ✅"
		}
		rows = append(rows, []models.InlineKeyboardButton{{Text: text, CallbackData: callbacks[p.ID]}})
	}
	rows = append(rows,
		[]models.InlineKeyboardButton{{Text: "➡️ Выбрал!", CallbackData: callbackSubPlanConfirm}},
		[]models.InlineKeyboardButton{{Text: "Назад", CallbackData: callbackBackMain}},
	)
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// sendMessage отправляет текстовое сообщение в чат.
func (h *Handler) SendMessage(ctx context.Context, chatID int64, text string) error {
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
	return nil
}

// sendMessageWithInlineKeyboard отправляет сообщение с inline-клавиатурой в чат.
func (h *Handler) sendMessageWithInlineKeyboard(ctx context.Context, chatID int64, text string, markup models.InlineKeyboardMarkup) error {
	apiURL := "https://api.telegram.org/bot" + h.Token + "/sendMessage"
	body := map[string]any{
		"chat_id": chatID,
		"text":    text,
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
		log.Printf("telegram sendMessage: status %d", resp.StatusCode)
	}
	return nil
}

// editMessageTextWithKeyboard редактирует текст и клавиатуру существующего сообщения.
func (h *Handler) editMessageTextWithKeyboard(ctx context.Context, chatID int64, messageID int, text string, markup models.InlineKeyboardMarkup) error {
	apiURL := "https://api.telegram.org/bot" + h.Token + "/editMessageText"
	body := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
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
		log.Printf("telegram editMessageText: status %d", resp.StatusCode)
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
			{{Text: "Сделать фото", CallbackData: callbackPhoto}},
			{{Text: "AI Ассистент", CallbackData: callbackChat}},
			{
				{Text: "Профиль", CallbackData: callbackProfile},
				{Text: "Поддержка", CallbackData: callbackSupport},
			},
			{{Text: "Купить подписку", CallbackData: callbackSubscribtion}},
		},
	}
}

// sendMainMenu отправляет главное меню с кнопками.
func (h *Handler) sendMainMenu(ctx context.Context, chatID int64) {
	_ = h.sendMessageWithInlineKeyboard(ctx, chatID, msgMainMenuGreeting, h.mainMenuMarkup())
}

// HandleUpdate обрабатывает входящее обновление от Telegram (вызывается воркерами).
func (h *Handler) HandleUpdate(ctx context.Context, upd models.Update) {
	if upd.CallbackQuery != nil {
		h.handleCallbackQuery(ctx, upd.CallbackQuery)
		return
	}
	if upd.Message == nil {
		return
	}

	chatID := upd.Message.Chat.ID
	key := "bot:state:" + strconv.FormatInt(chatID, 10)
	st, _, _ := h.Rd.Get(ctx, key)
	text := strings.TrimSpace(upd.Message.Text)

	if text == startCommand {
		h.handleStart(ctx, chatID, upd.Message)
		return
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
		{Text: "Подписаться", URL: subscribeURL},
	}
	secondText := "Забрать генерации"
	if secondCallback == callbackVerifySubscription {
		secondText = "Проверить подписку"
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
	case callbackProfile:
		h.handleProfile(ctx, cq.ID, chatID, cq.Message.MessageID)
	case callbackSupport:
		h.handleSupport(ctx, cq.ID, chatID, cq.Message.MessageID)
	case callbackSubscribtion:
		h.handleSubscribtion(ctx, cq.ID, chatID, cq.Message.MessageID)
	case callbackSubPlanLite, callbackSubPlanStandard, callbackSubPlanPremium:
		h.handleSubPlanSelect(ctx, cq.ID, chatID, cq.Message.MessageID, cq.Data)
	case callbackSubPlanConfirm:
		h.handleSubPlanConfirm(ctx, cq.ID, chatID, cq.Message.MessageID)
	case callbackPaySbp, callbackPayCard:
		h.handlePayMethodSelect(ctx, cq.ID, chatID, cq.Message.MessageID, cq.Data)
	case callbackPayConfirm:
		h.handlePayConfirm(ctx, cq.ID, chatID, cq.Message.MessageID)
	default:
		_ = h.answerCallbackQuery(ctx, cq.ID, "")
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

	h.sendMainMenu(ctx, chatID)
}

func (h *Handler) handleVerifySubscription(ctx context.Context, callbackID string, chatID, userID int64) {
	subscribed := h.isSubscribedToChannel(ctx, userID)
	if !subscribed {
		_ = h.answerCallbackQuery(ctx, callbackID, msgNotSubscribed)
		return
	}
	_ = h.answerCallbackQuery(ctx, callbackID, "")
	h.sendMainMenu(ctx, chatID)
}

func (h *Handler) handlePhoto(ctx context.Context, callbackID string, chatID int64, messageID int) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")
	backMarkup := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "Назад", CallbackData: callbackBackMain}},
		},
	}
	if err := h.editMessageTextWithKeyboard(ctx, chatID, messageID, instructionPhotoText, backMarkup); err != nil {
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
	backMarkup := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "Назад", CallbackData: callbackBackMain}},
		},
	}
	if err := h.editMessageTextWithKeyboard(ctx, chatID, messageID, instructionChatText, backMarkup); err != nil {
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
	if err := h.editMessageTextWithKeyboard(ctx, chatID, messageID, msgMainMenuGreeting, h.mainMenuMarkup()); err != nil {
		log.Printf("editMessage back main: %v", err)
	}
}

func (h *Handler) handleProfile(ctx context.Context, callbackID string, chatID int64, messageID int) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")
	backMarkup := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "Назад", CallbackData: callbackBackMain}},
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

	messageProfile := fmt.Sprintf(`Профиль

Имя пользователя: %s

Подписка: %s
Осталось: %s

Использовано генераций фото: %d
Использовано генераций чата: %d`, user.UserName, user.Subscription, remainingDaysString, user.UsedPhoto, user.UsedChat)

	if err := h.editMessageTextWithKeyboard(ctx, chatID, messageID, messageProfile, backMarkup); err != nil {
		log.Printf("editMessage profile: %v", err)
	}
}

func (h *Handler) handleSupport(ctx context.Context, callbackID string, chatID int64, messageID int) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")
	backMarkup := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "Назад", CallbackData: callbackBackMain}},
		},
	}
	if err := h.editMessageTextWithKeyboard(ctx, chatID, messageID, msgSupport, backMarkup); err != nil {
		log.Printf("editMessage support: %v", err)
	}
}

func (h *Handler) handleSubscribtion(ctx context.Context, callbackID string, chatID int64, messageID int) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")
	text := subscriptionPlanMessage("lite")
	markup := subscriptionPlanMarkup("lite")
	if err := h.editMessageTextWithKeyboard(ctx, chatID, messageID, text, markup); err != nil {
		log.Printf("editMessage subscribtion: %v", err)
	}
}

func (h *Handler) handleSubPlanSelect(ctx context.Context, callbackID string, chatID int64, messageID int, callbackData string) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")
	var planID string
	switch callbackData {
	case callbackSubPlanLite:
		planID = "lite"
	case callbackSubPlanStandard:
		planID = "standard"
	case callbackSubPlanPremium:
		planID = "premium"
	default:
		planID = "lite"
	}
	text := subscriptionPlanMessage(planID)
	markup := subscriptionPlanMarkup(planID)
	if err := h.editMessageTextWithKeyboard(ctx, chatID, messageID, text, markup); err != nil {
		log.Printf("editMessage sub plan: %v", err)
	}
}

func (h *Handler) handleSubPlanConfirm(ctx context.Context, callbackID string, chatID int64, messageID int) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")
	markup := paymentMethodMarkup("sbp")
	if err := h.editMessageTextWithKeyboard(ctx, chatID, messageID, msgSubPlanConfirm, markup); err != nil {
		log.Printf("editMessage handleSubPlanConfirm: %v", err)
	}
}

func (h *Handler) handlePayMethodSelect(ctx context.Context, callbackID string, chatID int64, messageID int, callbackData string) {
	_ = h.answerCallbackQuery(ctx, callbackID, "")
	var methodID string
	switch callbackData {
	case callbackPaySbp:
		methodID = "sbp"
	case callbackPayCard:
		methodID = "card"
	default:
		methodID = "sbp"
	}
	markup := paymentMethodMarkup(methodID)
	if err := h.editMessageTextWithKeyboard(ctx, chatID, messageID, msgSubPlanConfirm, markup); err != nil {
		log.Printf("editMessage pay method: %v", err)
	}
}

func (h *Handler) handlePayConfirm(ctx context.Context, callbackID string, chatID int64, messageID int) {
	_ = h.answerCallbackQuery(ctx, callbackID, "Скоро отправим ссылку на оплату.")
	if err := h.editMessageTextWithKeyboard(ctx, chatID, messageID, msgMainMenuGreeting, h.mainMenuMarkup()); err != nil {
		log.Printf("editMessage pay confirm: %v", err)
	}
}

func (h *Handler) GetStickerFileId(ctx context.Context, stickerNumber int) (string, error) {
	apiURL := "https://api.telegram.org/bot" + h.Token + "/getStickerSet?name=balooAItest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("telegram sendSticker: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var out struct {
		Ok     bool `json:"ok"`
		Result struct {
			Name     string `json:"name"`
			Stickers []struct {
				FileID string `json:"file_id"`
			} `json:"stickers"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if !out.Ok || stickerNumber < 0 || stickerNumber >= len(out.Result.Stickers) {
		return "", fmt.Errorf("getStickerSet: invalid sticker number or response")
	}
	return out.Result.Stickers[stickerNumber].FileID, nil
}

// sendSticker отправляет стикер и возвращает message_id отправленного сообщения (или 0 при ошибке).
func (h *Handler) sendSticker(ctx context.Context, chatID int64, stickerNumber int) (messageID int, err error) {
	stickerFileId, err := h.GetStickerFileId(ctx, stickerNumber)
	if err != nil {
		return 0, err
	}

	apiURL := "https://api.telegram.org/bot" + h.Token + "/sendSticker"

	body := map[string]any{
		"chat_id": chatID,
		"sticker": stickerFileId,
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

// saveMediaAndPromptIfPresent сохраняет file_id фото и промпт (caption) в Redis. Возвращает true, если сообщение содержало фото и подпись.
func (h *Handler) saveMediaAndPromptIfPresent(ctx context.Context, key string, chatID int64, msg *models.MessageContent) bool {
	if len(msg.Photo) == 0 || msg.Caption == "" {
		return false
	}
	fileID := msg.Photo[len(msg.Photo)-1].FileID

	st, _, _ := h.Rd.Get(ctx, key)
	if st.Data == nil {
		st.Data = make(map[string]string)
	}
	st.Data[stateKeyMediaFileID] = fileID
	st.Data[stateKeyPrompt] = msg.Caption
	st.Step = "GetPhotoPrompt"

	if err := h.Rd.Set(ctx, key, st, shortStateTTL); err != nil {
		log.Printf("Redis Set: %v", err)
		return false
	}
	log.Printf("saved to Redis chat_id=%d file_id=%s prompt_len=%d", chatID, fileID, len(msg.Caption))
	// if err := h.SendMessage(ctx, chatID, startProcessing); err != nil {
	// 	log.Printf("sendMessage: %v", err)
	// }
	if msgID, err := h.sendSticker(ctx, chatID, 0); err != nil {
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
	return true
}

// saveTextAndFileIfPresent обрабатывает сообщение в режиме AI ЧАТ: сохраняет текст, фото (file_id) или документ (file_id) в state. Возвращает true, если сообщение принято.
func (h *Handler) saveTextAndFileIfPresent(ctx context.Context, key string, chatID int64, msg *models.MessageContent) bool {
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
		text = "Что на изображении?"
	}
	if text == "" && hasDocument {
		text = "Что в документе?"
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