package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"aibot/internal/models"
)

func setTgWebhookForm(webhookURL, secret string, dropPending bool) url.Values {
	form := url.Values{}
	form.Set("url", webhookURL)
	if secret != "" {
		form.Set("secret_token", secret)
	}
	if dropPending {
		form.Set("drop_pending_updates", "true")
	}
	return form
}

func checkSetTgWebhookResponse(body []byte, statusCode int) error {
	if statusCode == http.StatusOK {
		var tr models.Response
		_ = json.Unmarshal(body, &tr)
		if tr.Ok {
			return nil
		}
	}
	if len(body) == 0 {
		return errors.New("telegram setWebhook failed: empty response")
	}
	return errors.New(string(body))
}

func SetTgWebhook(ctx context.Context, botToken, webhookURL, secret string, dropPending bool) error {
	apiURL := "https://api.telegram.org/bot" + botToken + "/setWebhook"
	form := setTgWebhookForm(webhookURL, secret, dropPending)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return checkSetTgWebhookResponse(body, resp.StatusCode)
}

func DoTgWebhook(secret string, logger *log.Logger, updates chan models.Update) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if secret != "" && r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != secret {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		defer r.Body.Close()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		var upd models.Update
		if err := json.Unmarshal(body, &upd); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if upd.Message != nil {
			logger.Printf("[webhook] update_id=%d chat_id=%d photo_len=%d caption_len=%d",
				upd.UpdateID, upd.Message.Chat.ID, len(upd.Message.Photo), len(upd.Message.Caption))
		}

		w.WriteHeader(http.StatusOK)

		select {
		case updates <- upd:
		default:
			logger.Printf("drop update_id=%d: queue full", upd.UpdateID)
		}
	}
}
