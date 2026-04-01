package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"aibot/internal/models"
	"aibot/internal/state"
)

const maxPhotoModelNameLen = 40

// photoModelEntry — одна пользовательская модель (имя + file_id эталонов в Telegram).
type photoModelEntry struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	FileIDs []string `json:"file_ids"`
	FileID  string   `json:"file_id,omitempty"` // legacy: одно фото
}

func modelRefFileIDs(m *photoModelEntry) []string {
	if m == nil {
		return nil
	}
	if len(m.FileIDs) > 0 {
		return m.FileIDs
	}
	if m.FileID != "" {
		return []string{m.FileID}
	}
	return nil
}

func normalizePhotoModelFileIDs(m *photoModelEntry) {
	if m == nil {
		return
	}
	if len(m.FileIDs) == 0 && m.FileID != "" {
		m.FileIDs = []string{m.FileID}
	}
}

func newPhotoModelID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// loadPhotoModel читает одну модель из state. Поддерживает legacy JSON-массив (берётся первый элемент).
func loadPhotoModel(st state.State) *photoModelEntry {
	if st.Data == nil {
		return nil
	}
	raw := st.Data[stateKeyPhotoModelsJSON]
	if raw == "" {
		return nil
	}
	// legacy: массив моделей
	var arr []photoModelEntry
	if err := json.Unmarshal([]byte(raw), &arr); err == nil && len(arr) > 0 {
		p := arr[0]
		normalizePhotoModelFileIDs(&p)
		return &p
	}
	var one photoModelEntry
	if err := json.Unmarshal([]byte(raw), &one); err != nil {
		log.Printf("loadPhotoModel: %v", err)
		return nil
	}
	normalizePhotoModelFileIDs(&one)
	if one.ID == "" && one.Name == "" && len(one.FileIDs) == 0 && one.FileID == "" {
		return nil
	}
	return &one
}

// savePhotoModel сохраняет ровно одну модель (объект JSON).
func savePhotoModel(st *state.State, m *photoModelEntry) {
	if st.Data == nil {
		st.Data = make(map[string]string)
	}
	if m == nil {
		delete(st.Data, stateKeyPhotoModelsJSON)
		return
	}
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	st.Data[stateKeyPhotoModelsJSON] = string(b)
}

// // photoInstructionCaption — инструкция: с моделью — другая (референс + подпись).
// func (h *Handler) photoInstructionCaption(ctx context.Context, chatID int64) string {
// 	key := "bot:state:" + strconv.FormatInt(chatID, 10)
// 	st, _, _ := h.Rd.Get(ctx, key)
// 	if loadPhotoModel(st) != nil {
// 		return msgInstructionPhotoModel
// 	}
// 	return msgInstructionPhoto
// }

// modelStatusButtonText — подпись нижней кнопки «Модель: …» (до 64 символов).
func (h *Handler) modelStatusButtonText(ctx context.Context, chatID int64) string {
	key := "bot:state:" + strconv.FormatInt(chatID, 10)
	st, _, _ := h.Rd.Get(ctx, key)
	m := loadPhotoModel(st)
	if m == nil {
		return btnModelNotSet
	}
	const prefix = "👤 Модель: "
	prefixR := []rune(prefix)
	nameR := []rune(strings.TrimSpace(m.Name))
	const maxRunes = 64
	maxName := maxRunes - len(prefixR)
	if maxName < 1 {
		maxName = 1
	}
	if len(nameR) > maxName {
		nameR = append(append([]rune{}, nameR[:maxName-1]...), '…')
	}
	return prefix + string(nameR)
}

// photoInstructionKeyboard — экран «Сделать фото»: строка «Модель:» сверху, затем действия, назад.
func (h *Handler) photoInstructionKeyboard(ctx context.Context, chatID int64) models.InlineKeyboardMarkup {
	key := "bot:state:" + strconv.FormatInt(chatID, 10)
	st, _, _ := h.Rd.Get(ctx, key)
	hasModel := loadPhotoModel(st) != nil

	statusText := h.modelStatusButtonText(ctx, chatID)
	rows := [][]models.InlineKeyboardButton{
		{{Text: statusText, CallbackData: callbackModelStatus}},
		{{Text: btnSetNewModel, CallbackData: callbackMakeModel}},
	}
	if hasModel {
		rows = append(rows, []models.InlineKeyboardButton{{Text: btnResetModel, CallbackData: callbackResetModel}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: btnBack, CallbackData: callbackBackMainDelete}})
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// handlePhotoModelNameInput обрабатывает текст с названием модели (шаг создания).
func (h *Handler) handlePhotoModelNameInput(ctx context.Context, key string, chatID int64, msg *models.MessageContent) bool {
	name := strings.TrimSpace(msg.Text)
	if name == "" {
		if len(msg.Photo) > 0 {
			go func() {
				bctx, cancel := context.WithTimeout(ctx, 15*time.Second)
				defer cancel()
				msgId, _ := h.SendMessage(bctx, chatID, msgPhotoModelPickName)
				time.Sleep(5 * time.Second)
				h.deleteMessage(bctx, chatID, msgId)
			}()
		}
		return true
	}
	if len([]rune(name)) > maxPhotoModelNameLen {
		_, _ = h.SendMessage(ctx, chatID, fmt.Sprintf("Слишком длинное имя. Максимум %d символов.", maxPhotoModelNameLen))
		return true
	}
	st, _, _ := h.Rd.Get(ctx, key)
	if st.Data == nil {
		st.Data = make(map[string]string)
	}
	st.Data[stateKeyPendingModelName] = name
	st.Step = stateStepAwaitModelPhoto

	// Удаляем «Введите имя модели» и сообщение пользователя с именем (только после ответа пользователя)
	if enterNameMsgID, ok := st.Data[stateKeyModelCreateEnterNameMsgID]; ok {
		if mid, err := strconv.Atoi(enterNameMsgID); err == nil && mid > 0 {
			_ = h.deleteMessage(ctx, chatID, mid)
		}
		delete(st.Data, stateKeyModelCreateEnterNameMsgID)
	}
	_ = h.deleteMessage(ctx, chatID, msg.MessageID)

	msgID, err := h.SendMessage(ctx, chatID, msgPhotoModelSendPhoto)
	if err != nil {
		log.Printf("handlePhotoModelNameInput SendMessage: %v", err)
		return true
	}
	if msgID > 0 {
		st.Data[stateKeyModelCreateSendPhotoMsgID] = strconv.Itoa(msgID)
	}
	if err := h.Rd.Set(ctx, key, st, defaultStateTTL); err != nil {
		log.Printf("handlePhotoModelNameInput Set: %v", err)
	}

	return true
}

// finalizePhotoModelCreation сохраняет одну модель (новая заменяет предыдущую).
// userMessageIDs — message_id сообщений пользователя (фото) для удаления.
func (h *Handler) finalizePhotoModelCreation(ctx context.Context, key string, chatID int64, fileIDs []string, userMessageIDs ...int) {
	seen := make(map[string]struct{})
	var uniq []string
	for _, fid := range fileIDs {
		fid = strings.TrimSpace(fid)
		if fid == "" {
			continue
		}
		if _, ok := seen[fid]; ok {
			continue
		}
		seen[fid] = struct{}{}
		uniq = append(uniq, fid)
	}
	if len(uniq) == 0 {
		return
	}

	st, _, _ := h.Rd.Get(ctx, key)
	if st.Data == nil {
		st.Data = make(map[string]string)
	}
	pending := strings.TrimSpace(st.Data[stateKeyPendingModelName])
	if pending == "" {
		st.Step = stateStepAwaitModelName
		_ = h.Rd.Set(ctx, key, st, defaultStateTTL)
		_, _ = h.SendMessage(ctx, chatID, msgPhotoModelEnterName)
		return
	}

	// Удаляем «Пришлите фото модели» и сообщения пользователя с фото (только после ответа пользователя)
	if sendPhotoMsgID, ok := st.Data[stateKeyModelCreateSendPhotoMsgID]; ok {
		if mid, err := strconv.Atoi(sendPhotoMsgID); err == nil && mid > 0 {
			_ = h.deleteMessage(ctx, chatID, mid)
		}
		delete(st.Data, stateKeyModelCreateSendPhotoMsgID)
	}
	for _, mid := range userMessageIDs {
		_ = h.deleteMessage(ctx, chatID, mid)
	}

	m := &photoModelEntry{
		ID:      newPhotoModelID(),
		Name:    pending,
		FileIDs: uniq,
	}
	savePhotoModel(&st, m)
	delete(st.Data, stateKeyPendingModelName)
	delete(st.Data, stateKeySelectedPhotoModel) // legacy id не используем
	st.Step = stateStepAwaitMedia
	if err := h.Rd.Set(ctx, key, st, defaultStateTTL); err != nil {
		log.Printf("finalizePhotoModelCreation Set: %v", err)
	}

	// «Модель сохранена» — финальное сообщение, удаляем через 5 сек
	go func() {
		bctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		msgId, _ := h.SendMessage(bctx, chatID, msgPhotoModelSaved)
		time.Sleep(5 * time.Second)
		if msgId > 0 {
			_ = h.deleteMessage(bctx, chatID, msgId)
		}
	}()
	h.refreshPhotoInstructionMessage(ctx, chatID)
}

// handlePhotoModelPhotoInput — одно фото (не альбом); альбом обрабатывается в handleModelPhotoMediaGroup.
func (h *Handler) handlePhotoModelPhotoInput(ctx context.Context, key string, chatID int64, msg *models.MessageContent) bool {
	if len(msg.Photo) == 0 {
		
		_, _ = h.SendMessage(ctx, chatID, msgPhotoModelSendPhoto)
		
		return true
	}
	if msg.MediaGroupID != "" {
		return false
	}
	fileID := msg.Photo[len(msg.Photo)-1].FileID
	h.finalizePhotoModelCreation(ctx, key, chatID, []string{fileID}, msg.MessageID)
	return true
}

func (h *Handler) refreshPhotoInstructionMessage(ctx context.Context, chatID int64) {
	key := "bot:state:" + strconv.FormatInt(chatID, 10)
	st, _, _ := h.Rd.Get(ctx, key)
	if st.Data == nil {
		return
	}
	msgIDStr := st.Data[stateKeyPhotoInstructionMsgID]
	if msgIDStr == "" {
		return
	}
	msgID, err := strconv.Atoi(msgIDStr)
	if err != nil || msgID <= 0 {
		return
	}
	if st.Data[stateKeyPhotoMode] == photoModeValueNormal {
		markup := photoNormalOnlyBackMarkup()
		if err := h.editMessageCaptionWithKeyboard(ctx, chatID, msgID, msgInstructionPhotoNormal, markup); err != nil {
			log.Printf("refreshPhotoInstructionMessage chat_id=%d: %v", chatID, err)
		}
		return
	}
	caption := msgInstructionPhoto
	if loadPhotoModel(st) != nil {
		caption = msgInstructionPhotoModel
	}
	markup := h.photoInstructionKeyboard(ctx, chatID)
	if err := h.editMessageCaptionWithKeyboard(ctx, chatID, msgID, caption, markup); err != nil {
		log.Printf("refreshPhotoInstructionMessage chat_id=%d: %v", chatID, err)
	}
}
