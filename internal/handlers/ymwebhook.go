package handlers

import (
	"aibot/internal/models"
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	yoopayment "github.com/rvinnie/yookassa-sdk-go/yookassa/payment"
	yoowebhook "github.com/rvinnie/yookassa-sdk-go/yookassa/webhook"
)

func DoYoumoneyWebhook(allowedCIDRs []string, h *Handler, logger *log.Logger, YouMoneyResult chan models.YouMoneyResult) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		remoteIP := r.RemoteAddr
		log.Printf("Initial remote IP: %s", remoteIP)

		if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
			log.Printf("Using X-Real-IP header: %s", realIP)
			remoteIP = realIP
		}

		var host string
		if strings.Contains(remoteIP, ":") {
			var err error
			host, _, err = net.SplitHostPort(remoteIP)
			if err != nil {
				http.Error(w, "Invalid remote IP address", http.StatusBadRequest)
				return
			}
		} else {
			host = remoteIP
		}

		if !IsIPAllowed(host, allowedCIDRs) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		var webhookEvent yoowebhook.WebhookEvent[yoopayment.Payment]
		err := json.NewDecoder(r.Body).Decode(&webhookEvent)
		if err != nil {
			http.Error(w, "Invalid webhook data", http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)

		key := "ym:payment:" + webhookEvent.Object.ID
		st, ok, err := h.Rd.Get(context.Background(), key)
		if err != nil || !ok {
			logger.Printf("kie callback: redis get %s: ok=%v err=%v", key, ok, err)
			return
		}

		chatIDStr := st.Data["user_id"]
		if chatIDStr == "" {
			logger.Printf("kie callback: no user_id for taskId=%s", webhookEvent.Object.ID)
			return
		}
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			logger.Printf("kie callback: invalid user_id %q: %v", chatIDStr, err)
			return
		}

		var msgID int
		if v := st.Data["message_id"]; v != "" {
			msgID, _ = strconv.Atoi(v)
		}

		select {
		case YouMoneyResult <- models.YouMoneyResult{ChatID: chatID, Status: string(webhookEvent.Object.Status), Description: string(webhookEvent.Object.Description), MessageID: msgID}:
			log.Printf("Chatid: %d, status: %s", chatID, string(webhookEvent.Object.Status))
		default:
			logger.Printf("kie callback: results queue full, chat_id=%d dropped", chatID)
		}
	}
}

func IsIPAllowed(ip string, allowedCIDRs []string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	for _, cidr := range allowedCIDRs {
		_, allowedNet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if allowedNet.Contains(parsedIP) {
			return true
		}
	}
	return false
}
