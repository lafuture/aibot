package handlers

import (
	"aibot/internal/models"
	"aibot/internal/state"
	"archive/zip"
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
						fileID := st.Data[stateKeyMediaFileID]
						if fileID == "" {
							log.Printf("ProcessKieTasks: empty media_file_id for chat_id=%d", id)
							continue
						}
						mediaURL, err := h.GetUrlFromFileId(ctx, fileID)
						if err != nil {
							log.Printf("GetUrlFromFileId chat_id=%d: %v", id, err)
							continue
						}
						prompt := st.Data[stateKeyPrompt]

						if h.KieAcquire != nil {
							h.KieAcquire()
						}

						sendErr := h.SendToNanoBananaPro(ctx, mediaURL, prompt, id)
						if sendErr != nil {
							log.Printf("SendToNanoBananaPro chat_id=%d: %v", id, sendErr)
							tryRequeueKieTask(h, ctx, key, st, id)
						} else {
							log.Printf("ProcessKieTasks: sent to kie chat_id=%d", id)
							delete(st.Data, stateKeyKieRetry)
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
						sendErr := h.SendToGeminiFlash(ctx, key, prompt, fileURL, history, id)
						if sendErr != nil {
							log.Printf("SendToGeminiFlash chat_id=%d: %v", id, sendErr)
							tryRequeueKieTask(h, ctx, key, st, id)
						} else {
							delete(st.Data, stateKeyKieRetry)
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
		{"role": "system", "content": "История переписки приведена только для контекста. Отвечай строго на последнее сообщение пользователя. Не перечисляй и не повторяй ответы на предыдущие вопросы. Пиши без звёздочек и маркеров, без выделения жирным, разделяй информацию на абзацы."},
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
		"messages":          messages,
		"stream":            false,
		"include_thoughts":  false,
		"reasoning_effort":  "high",
	}
	raw, err := json.Marshal(body)
	if err != nil {
		st.Step = stateStepAwaitText
		_ = h.Rd.Set(ctx, key, st, shortStateTTL)
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.kie.ai/gemini-3-flash/v1/chat/completions", bytes.NewReader(raw))
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
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		st.Step = stateStepAwaitText
		_ = h.Rd.Set(ctx, key, st, defaultStateTTL)
		return err
	}
	if resp.StatusCode != http.StatusOK {
		st.Step = stateStepAwaitText
		_ = h.Rd.Set(ctx, key, st, defaultStateTTL)
		log.Printf("SendToGeminiFlash: status %d body=%s", resp.StatusCode, string(respBody))
		return fmt.Errorf("gemini api: status %d", resp.StatusCode)
	}
	var out models.ChatCompletionResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		log.Printf("SendToGeminiFlash: unmarshal %v body=%s", err, string(respBody))
		st.Step = stateStepAwaitText
		_ = h.Rd.Set(ctx, key, st, defaultStateTTL)
		return err
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == "" {
		log.Printf("SendToGeminiFlash: no choices or empty content body=%s", string(respBody))
		st.Step = stateStepAwaitText
		_ = h.Rd.Set(ctx, key, st, defaultStateTTL)
		return fmt.Errorf("gemini api: no reply")
	}
	replyText := strings.TrimSpace(out.Choices[0].Message.Content)
	if err := h.SendMessage(ctx, chatID, replyText); err != nil {
		log.Printf("SendToGeminiFlash sendMessage: %v", err)
		st.Step = stateStepAwaitText
		_ = h.Rd.Set(ctx, key, st, defaultStateTTL)
		return err
	}

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
	_ = h.Rd.Set(ctx, key, st, defaultStateTTL)
	return nil
}

func (h *Handler) SendToNanoBananaPro(ctx context.Context, mediaURL, prompt string, id int64) error {
	if h.KieAPIKey == "" {
		return fmt.Errorf("KIE_API_KEY not set")
	}
	if h.KieCallbackURL == "" {
		return fmt.Errorf("KieCallbackURL not set")
	}

	body := map[string]any{
		"model":       "nano-banana-pro",
		"callBackUrl": h.KieCallbackURL,
		"input": map[string]any{
			"prompt":        prompt,
			"image_input":   []string{mediaURL},
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
