package handlers

import (
	"aibot/internal/models"
	"aibot/internal/state"
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (h *Handler) ProcessKieTasks(ctx context.Context) {
	var wg sync.WaitGroup
	const workers = 32
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case id, ok := <-h.KieTasks:
					if !ok {
						return
					}
					log.Printf("ProcessKieTasks: received chat_id=%d", id)
					key := "bot:state:" + strconv.FormatInt(id, 10)
					st, ok, _ := h.Rd.Get(ctx, key)
					if !ok {
						log.Printf("ProcessKieTasks: no state in Redis for chat_id=%d key=%s", id, key)
						continue
					}
					if st.Data == nil {
						log.Printf("ProcessKieTasks: state.Data is nil for chat_id=%d", id)
						continue
					}

					if st.Step == "GetPhotoPrompt" {
						var mediaURLs []string
						m := loadPhotoModel(st)
						if st.Data[stateKeyPhotoMode] == photoModeValueNormal {
							m = nil
						}
						modelPromptOnly := st.Data[stateKeyModelPromptOnly] == "1"

						// Только промпт при модели — используем только фото человека (модель)
						if modelPromptOnly && m != nil {
							for _, fid := range modelRefFileIDs(m) {
								if fid == "" {
									continue
								}
								modelURL, errModel := h.GetUrlFromFileId(ctx, fid)
								if errModel != nil {
									log.Printf("ProcessKieTasks model ref chat_id=%d fid=%s: %v", id, fid, errModel)
									continue
								}
								if modelURL != "" {
									mediaURLs = append(mediaURLs, modelURL)
								}
							}
							if len(mediaURLs) == 0 {
								log.Printf("ProcessKieTasks: model prompt only but no model refs chat_id=%d", id)
								continue
							}
						} else if raw := st.Data[stateKeyMediaFileIDs]; raw != "" {
							var fileIDs []string
							if err := json.Unmarshal([]byte(raw), &fileIDs); err != nil {
								log.Printf("ProcessKieTasks: bad media_file_ids json chat_id=%d: %v", id, err)
								continue
							}
							for _, fid := range fileIDs {
								u, err := h.GetUrlFromFileId(ctx, fid)
								if err != nil {
									log.Printf("GetUrlFromFileId chat_id=%d fid=%s: %v", id, fid, err)
									continue
								}
								mediaURLs = append(mediaURLs, u)
							}
						} else {
							fileID := st.Data[stateKeyMediaFileID]
							if fileID == "" && !modelPromptOnly {
								log.Printf("ProcessKieTasks: empty media_file_id for chat_id=%d", id)
								continue
							}
							if fileID != "" {
								u, err := h.GetUrlFromFileId(ctx, fileID)
								if err != nil {
									log.Printf("GetUrlFromFileId chat_id=%d: %v", id, err)
									continue
								}
								mediaURLs = []string{u}
							}
						}

						if len(mediaURLs) == 0 {
							log.Printf("ProcessKieTasks: no media URLs for chat_id=%d", id)
							continue
						}

						if st.Data[stateKeyPhotoMode] == photoModeValueReference && loadPhotoModel(st) == nil {
							log.Printf("ProcessKieTasks: reference mode without model chat_id=%d", id)
							continue
						}

						// Модель = человек. Референс = сцена. Порядок: [референс, человек...]. При modelPromptOnly — только человек.
						if !modelPromptOnly && m != nil {
							var suffix []string
							for _, fid := range modelRefFileIDs(m) {
								if fid == "" {
									continue
								}
								modelURL, errModel := h.GetUrlFromFileId(ctx, fid)
								if errModel != nil {
									log.Printf("ProcessKieTasks model ref chat_id=%d fid=%s: %v", id, fid, errModel)
									continue
								}
								if modelURL != "" {
									suffix = append(suffix, modelURL)
								}
							}
							if len(suffix) > 0 {
								mediaURLs = append(mediaURLs, suffix...)
							}
						}

						prompt := st.Data[stateKeyPrompt]
						// Для nano-banana: объясняем порядок изображений
						if m != nil {
							if modelPromptOnly {
								prompt = fmt.Sprintf(`На входе изображение человека (референс внешности).

Задача: %s.

Инструкции:
- Используй изображение как ЕДИНСТВЕННЫЙ источник внешности человека
- Максимально точно воспроизведи лицо: форма лица, глаза, нос, губы, расстояния между чертами
- Сохрани полную узнаваемость человека
- НЕ изменяй идентичность и не "улучшай" лицо

Разрешено:
- Менять одежду, позу, фон, освещение
- Добавлять детали окружения

Вариативность (обязательно):
- Итог не должен быть точной копией референса
- Добавь небольшие естественные изменения: другой ракурс, микро-изменение позы, вариации освещения или окружения
- Сохрани реалистичность — как будто это новая фотография того же человека

Важно:
- Лицо должно быть тем же самым человеком, а не просто похожим
- Вариативность НЕ должна влиять на узнаваемость лица

Требования к результату:
- Фотореализм, высокая детализация
- Без искажений лица и артефактов
- Итог — тот же человек, но в немного изменённой, естественной ситуации`, prompt)
							} else if len(mediaURLs) > 1 {
								nPerson := len(mediaURLs) - 1
prompt = fmt.Sprintf(`Первое изображение — референс сцены (поза, композиция, освещение, стиль).
Следующие %d изображений — один и тот же человек (разные ракурсы лица).

Задача: %s.

Инструкции:
- Используй ВСЕ изображения человека для точного восстановления его внешности
- НЕ усредняй лицо и НЕ смешивай черты
- Определи ключевые, устойчивые черты лица и строго их сохрани
- Лицо должно быть узнаваемо как тот же человек

Критически важно:
- Лицо — абсолютный приоритет
- Запрещено менять идентичность или добавлять новые черты
- Запрещено "улучшение" лица или стилизация

Композиция:
- Используй первое изображение как ОСНОВУ сцены
- Воспроизведи общий стиль, позу и освещение
- НО не копируй сцену 1-в-1

Вариативность (обязательно):
- Внеси небольшие естественные изменения:
  - слегка другой ракурс или положение камеры
  - небольшое изменение позы
  - вариации освещения или фона
- Итог должен выглядеть как новая фотография, вдохновлённая референсом, а не его копия

Одежда:
- Следуй описанию задачи
- Должна выглядеть реалистично и естественно

Приоритет:
1. Идентичность лица (максимально точно)
2. Геометрия и пропорции лица
3. Общая композиция сцены
4. Вариативность и естественность

Требования:
- Фотореализм, высокая детализация
- Без искажений лица и артефактов
- Без эффекта "другого человека"
- Итог — тот же человек, но в слегка изменённой, новой сцене`, nPerson, prompt)
							}
						}

						if h.KieAcquire != nil {
							h.KieAcquire()
						}

						sendErr := h.SendToNanoBananaPro(ctx, mediaURLs, prompt, id)
						if sendErr != nil {
							log.Printf("SendToNanoBananaPro chat_id=%d: %v", id, sendErr)
							tryRequeueKieTask(h, ctx, key, st, id)
						} else {
							log.Printf("ProcessKieTasks: sent to kie chat_id=%d (%d images)", id, len(mediaURLs))
							delete(st.Data, stateKeyKieRetry)
							delete(st.Data, stateKeyModelPromptOnly)
							_ = h.Rd.Set(ctx, key, st, defaultStateTTL)
						}
					}
					if st.Step == "GetTextPrompt" {
						prompt := st.Data[stateKeyPrompt]
						fileID := st.Data[stateKeyFileID]
						docFileID := st.Data[stateKeyDocumentFileID]
						docFileName := st.Data["document_file_name"]
						var fileURL string
						if fileID != "" {
							var errURL error
							fileURL, errURL = h.GetUrlFromFileId(ctx, fileID)
							if errURL != nil {
								log.Printf("GetUrlFromFileId chat_id=%d: %v", id, errURL)
							}
						}
						if docFileID != "" {
							docBytes, errDoc := h.GetFileBytesByFileID(ctx, docFileID)
							if errDoc != nil {
								log.Printf("GetFileBytesByFileID chat_id=%d: %v", id, errDoc)
							} else {
								docText, errExtract := extractTextFromDocument(docBytes, docFileName)
								if errExtract != nil {
									log.Printf("extractTextFromDocument chat_id=%d: %v", id, errExtract)
								} else if docText != "" {
									prompt = prompt + "\n\nСодержимое приложенного документа:\n" + docText
								}
							}
							delete(st.Data, stateKeyDocumentFileID)
							delete(st.Data, "document_file_name")
							_ = h.Rd.Set(ctx, key, st, defaultStateTTL)
						}
						var history []models.HistoryEntry
						err := json.Unmarshal([]byte(st.Data[stateKeyHistory]), &history)
						if err != nil {
							log.Printf("json.Unmarshal: %v", err)
							continue
						}

						if prompt == "" {
							log.Printf("ProcessKieTasks: empty prompt for chat_id=%d", id)
							continue
						}
						if h.KieAcquire != nil {
							h.KieAcquire()
						}
						timeNow := time.Now()
						sendErr := h.SendToGeminiFlash(ctx, key, prompt, fileURL, history, id)
						log.Printf("SendToGeminiFlash chat_id=%d: %v", id, time.Since(timeNow))
						if sendErr != nil {
							log.Printf("SendToGeminiFlash chat_id=%d: %v", id, sendErr)
							tryRequeueKieTask(h, ctx, key, st, id)
						} else {
							delete(st.Data, stateKeyKieRetry)
							if _, err := h.Db.AddUsedChat(ctx, id); err != nil {
								log.Printf("AddUsedChat chat_id=%d: %v", id, err)
							}
						}
					}
				}
			}
		}()
	}
	wg.Wait()
}

// SendToGeminiFlash отправляет запрос в Gemini Flash (API kie.ai): текст или текст+картинка (fileURL). Получает ответ и шлёт его пользователю в Telegram; обновляет историю в state.
func (h *Handler) SendToGeminiFlash(ctx context.Context, key, prompt, fileURL string, history []models.HistoryEntry, chatID int64) error {
	st, _, _ := h.Rd.Get(ctx, key)
	if st.Data == nil {
		st.Data = make(map[string]string)
	}

	if h.KieAPIKey == "" {
		return fmt.Errorf("KIE_API_KEY not set")
	}
	messages := []map[string]any{
		{"role": "system", "content": "Строго запрещено: эмодзи, смайлики, Unicode-символы кроме букв и цифр. Используй только переносы строк для разделения абзацев. Не представляйся и не называй себя (Gemini, ассистент и т.п.). Не заканчивай ответ вопросом к пользователю. Отвечай только по существу. История переписки приведена только для контекста. Отвечай строго на последнее сообщение пользователя. Не перечисляй и не повторяй ответы на предыдущие вопросы."},
	}
	// История уже содержит текущее сообщение пользователя; последнее добавляем отдельно с разрешённым fileURL
	n := len(history)
	if n > 1 {
		for _, m := range history[:n-1] {
			role := m.Role
			if role != "user" && role != "assistant" && role != "system" {
				role = "user"
			}
			messages = append(messages, map[string]any{"role": role, "content": m.Prompt})
		}
	}
	if fileURL != "" {
		messages = append(messages, map[string]any{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": prompt},
				{"type": "image_url", "image_url": map[string]any{"url": fileURL}},
			},
		})
	} else {
		messages = append(messages, map[string]any{"role": "user", "content": prompt})
	}
	body := map[string]any{
		"messages":         messages,
		"stream":           true,
		"include_thoughts": false,
		"reasoning_effort": "low",
	}
	raw, err := json.Marshal(body)
	if err != nil {
		st.Step = stateStepAwaitText
		_ = h.Rd.Set(ctx, key, st, shortStateTTL)
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.kie.ai/gpt-5-2/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		st.Step = stateStepAwaitText
		_ = h.Rd.Set(ctx, key, st, shortStateTTL)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.KieAPIKey)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		st.Step = stateStepAwaitText
		_ = h.Rd.Set(ctx, key, st, defaultStateTTL)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		st.Step = stateStepAwaitText
		_ = h.Rd.Set(ctx, key, st, defaultStateTTL)
		log.Printf("SendToGeminiFlash: status %d", resp.StatusCode)
		return fmt.Errorf("gemini api: status %d", resp.StatusCode)
	}

	t0 := time.Now()
	replyText := ""
	var MessageID int
	var lastEditAt time.Time
	var ttft, tFirstMsg time.Duration
	assistantBackMarkup := photoNormalOnlyBackMarkup()

	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		json.Unmarshal([]byte(data), &chunk)

		if replyText == "" {
			if len(chunk.Choices) > 0 && len(chunk.Choices[0].Delta.Content) != 0 {
				if ttft == 0 {
					ttft = time.Since(t0)
				}
				m, err := h.sendMessageWithInlineKeyboard(ctx, chatID, chunk.Choices[0].Delta.Content, assistantBackMarkup)
				if err != nil {
					log.Printf("SendToGeminiFlash sendMessage: %v", err)
					st.Step = stateStepAwaitText
					_ = h.Rd.Set(ctx, key, st, defaultStateTTL)
					return err
				}
				tFirstMsg = time.Since(t0)
				replyText += chunk.Choices[0].Delta.Content
				MessageID = m
				lastEditAt = time.Now()
			}
		} else {
			oldReplytext := replyText
			if len(chunk.Choices) > 0 && len(chunk.Choices[0].Delta.Content) != 0 {
				replyText += chunk.Choices[0].Delta.Content
			}

			if time.Since(lastEditAt) > 500*time.Millisecond && oldReplytext != replyText {
				err := h.editMessageWithKeyboard(ctx, chatID, MessageID, replyText, assistantBackMarkup)
				if err != nil {
					log.Printf("SendToGeminiFlash sendMessage: %v", err)
					st.Step = stateStepAwaitText
					_ = h.Rd.Set(ctx, key, st, defaultStateTTL)
				}
				lastEditAt = time.Now()
			}
		}
	}

	if err := h.editMessageWithKeyboard(ctx, chatID, MessageID, replyText, assistantBackMarkup); err != nil {
		log.Printf("SendToGeminiFlash sendMessage: %v", err)
		st.Step = stateStepAwaitText
		_ = h.Rd.Set(ctx, key, st, defaultStateTTL)
	}

	tStreamEnd := time.Since(t0)
	log.Printf("SendToGeminiFlash chat_id=%d: TTFT=%v firstMsg=%v total=%v len=%d",
		chatID, ttft, tFirstMsg, tStreamEnd, len(replyText))

	historyJSON := st.Data[stateKeyHistory]
	var hist []models.HistoryEntry
	if historyJSON != "" {
		_ = json.Unmarshal([]byte(historyJSON), &hist)
	}
	hist = append(hist, models.HistoryEntry{Role: "assistant", Prompt: replyText, FileURL: ""})
	if n := len(hist) - 10; n > 0 {
		hist = hist[n:]
	}
	b, _ := json.Marshal(hist)
	st.Data[stateKeyHistory] = string(b)
	st.Step = stateStepAwaitText
	delete(st.Data, stateKeyDocumentFileID)
	delete(st.Data, "document_file_name")
	_ = h.Rd.Set(ctx, key, st, shortStateTTL)
	return nil
}

func (h *Handler) SendToNanoBananaPro(ctx context.Context, mediaURLs []string, prompt string, id int64) error {
	if h.KieAPIKey == "" {
		return fmt.Errorf("KIE_API_KEY not set")
	}
	if h.KieCallbackURL == "" {
		return fmt.Errorf("KieCallbackURL not set")
	}

	body := map[string]any{
		"model":       "nano-banana-2",
		"callBackUrl": h.KieCallbackURL,
		"input": map[string]any{
			"prompt":        prompt,
			"image_input":   mediaURLs,
			"aspect_ratio":  "auto",
			"resolution":    "2K",
			"output_format": "png",
		},
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.kie.ai/api/v1/jobs/createTask", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.KieAPIKey)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("kie createTask: status %d", resp.StatusCode)
	}

	kiebody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var out models.KieBody
	if err := json.Unmarshal(kiebody, &out); err != nil {
		return err
	}
	if out.Code != 200 {
		log.Printf("SendToKie: kie API code=%d msg=%q body=%s", out.Code, out.Msg, string(kiebody))
		return fmt.Errorf("kie createTask: code=%d msg=%s", out.Code, out.Msg)
	}
	if out.Data.TaskID == "" {
		log.Printf("SendToKie: kie API 200 but no taskId body=%s", string(kiebody))
		return fmt.Errorf("kie createTask: no taskId in response")
	}

	key := "kie:task:" + out.Data.TaskID

	st, _, _ := h.Rd.Get(ctx, key)
	if st.Data == nil {
		st.Data = make(map[string]string)
	}

	st.Step = "SentKieTask"
	st.Data["user_id"] = strconv.FormatInt(id, 10)

	h.Rd.Set(ctx, key, st, defaultStateTTL)

	return nil
}

func (h *Handler) GetUrlFromFileId(ctx context.Context, fileID string) (string, error) {
	apiURL := "https://api.telegram.org/bot" + h.Token + "/getFile?file_id=" + fileID
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
		return "", fmt.Errorf("getFile: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var out models.FileOut
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if !out.Ok {
		return "", fmt.Errorf("getFile: not ok")
	}
	if out.Result.FilePath == "" {
		return "", fmt.Errorf("getFile: empty file_path")
	}
	fileURL := "https://api.telegram.org/file/bot" + h.Token + "/" + out.Result.FilePath
	return fileURL, nil
}

// GetFileBytesByFileID получает файл по file_id из Telegram и возвращает его содержимое.
func (h *Handler) GetFileBytesByFileID(ctx context.Context, fileID string) ([]byte, error) {
	fileURL, err := h.GetUrlFromFileId(ctx, fileID)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download file: status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// extractTextFromDocument извлекает текст из .txt или .docx. Для docx читает word/document.xml из zip.
func extractTextFromDocument(data []byte, fileName string) (string, error) {
	ext := strings.ToLower(path.Ext(fileName))
	switch ext {
	case ".txt":
		return string(data), nil
	case ".docx":
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return "", fmt.Errorf("docx zip: %w", err)
		}
		var docXML []byte
		for _, f := range zr.File {
			if f.Name == "word/document.xml" {
				rc, err := f.Open()
				if err != nil {
					return "", err
				}
				docXML, err = io.ReadAll(rc)
				rc.Close()
				if err != nil {
					return "", err
				}
				break
			}
		}
		if len(docXML) == 0 {
			return "", fmt.Errorf("docx: word/document.xml not found")
		}
		// Извлекаем текст из элементов <w:t>...</w:t>
		re := regexp.MustCompile(`<w:t[^>]*>([^<]*)</w:t>`)
		matches := re.FindAllStringSubmatch(string(docXML), -1)
		var parts []string
		for _, m := range matches {
			if len(m) > 1 && m[1] != "" {
				parts = append(parts, m[1])
			}
		}
		return strings.Join(parts, ""), nil
	default:
		return "", fmt.Errorf("неподдерживаемый формат документа: %s (поддерживаются .txt, .docx)", ext)
	}
}

// tryRequeueKieTask при ошибке SendToKie ставит задачу обратно в очередь с задержкой (до kieRetryMax попыток).
func tryRequeueKieTask(h *Handler, ctx context.Context, key string, st state.State, id int64) {
	n := 0
	if st.Data != nil {
		if s := st.Data[stateKeyKieRetry]; s != "" {
			n, _ = strconv.Atoi(s)
		}
	}
	if n >= kieRetryMax {
		log.Printf("ProcessKieTasks: max retries reached for chat_id=%d", id)
		return
	}
	if st.Data == nil {
		st.Data = make(map[string]string)
	}
	st.Data[stateKeyKieRetry] = strconv.Itoa(n + 1)
	_ = h.Rd.Set(ctx, key, st, defaultStateTTL)
	chatID := id
	retryNum := n + 1
	go func() {
		time.Sleep(kieRetryDelay)
		select {
		case h.KieTasks <- chatID:
			log.Printf("ProcessKieTasks: requeued chat_id=%d for retry (%d/%d)", chatID, retryNum, kieRetryMax)
		default:
			log.Printf("ProcessKieTasks: requeue full, chat_id=%d dropped", chatID)
		}
	}()
}
