package handlers

import (
	"aibot/internal/models"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// DoKieCallback обрабатывает POST callback от kie.ai: парсит ответ, достаёт chat_id из Redis, кладёт в канал для отправки фото.
func DoKieCallback(logger *log.Logger, h *Handler, kieResults chan models.KieResult) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		defer r.Body.Close()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		var resp models.KieCallbackResp
		if err := json.Unmarshal(body, &resp); err != nil {
			logger.Printf("kie callback: bad json: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)

		key := "kie:task:" + resp.Data.TaskID
		st, ok, err := h.Rd.Get(context.Background(), key)
		if err != nil || !ok {
			logger.Printf("kie callback: redis get %s: ok=%v err=%v", key, ok, err)
			return
		}

		chatIDStr := st.Data["user_id"]
		if chatIDStr == "" {
			logger.Printf("kie callback: no user_id for taskId=%s", resp.Data.TaskID)
			return
		}
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			logger.Printf("kie callback: invalid user_id %q: %v", chatIDStr, err)
			return
		}

		if st.Data["try"] == "" {
			st.Data["try"] = "1"
		}

		try, err := strconv.Atoi(st.Data["try"])
		if err != nil {
			logger.Printf("kie callback: redis get %s: try=%v err=%v", key, st, err)
		}

		if resp.Code != 200 || resp.Data.TaskID == "" {
			logger.Printf("kie callback: code=%d or empty taskId", resp.Code)
			if try < 3 {
				try = try + 1
				st.Data["try"] = strconv.Itoa(try)
				h.Rd.Set(context.Background(), key, st, defaultStateTTL)
				if h.KieTasks != nil {
					select {
					case h.KieTasks <- chatID:
						log.Printf("pushed to kieTasks chat_id=%d", chatID)
					default:
						log.Printf("kie queue full, chat_id=%d dropped", chatID)
					}
				}
				h.SendMessage(context.Background(), chatID, "Не удалось обработать изображение, попробуйте позже")
			}
			return
		}

		var resultURLs models.KieResultURLs
		if err := json.Unmarshal([]byte(resp.Data.ResultJSON), &resultURLs); err != nil {
			logger.Printf("kie callback: bad json: %v", err)
			return
		}

		if len(resultURLs.ResultUrls) == 0 {
			logger.Printf("kie callback: no resultUrls taskId=%s", resp.Data.TaskID)
			return
		}

		imageURL := resultURLs.ResultUrls[0]
		imageURL = strings.ReplaceAll(imageURL, "\\/", "/")
		imageURL = strings.TrimSpace(imageURL)
		logger.Printf("kie callback: imageURL=%s", imageURL)

		select {
		case kieResults <- models.KieResult{ChatID: chatID, ImageURL: imageURL}:
		default:
			logger.Printf("kie callback: results queue full, chat_id=%d dropped", chatID)
		}
	}
}
