package server

import (
	"aibot/internal/models"
	"log"
	"net/http"

	"aibot/internal/handlers"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var AllowedCIDRs = []string{
	"185.71.76.0/27",
	"185.71.77.0/27",
	"77.75.153.0/25",
	"77.75.156.11/32",
	"77.75.156.35/32",
	"77.75.154.128/25",
	"2a02:5180::/32",
}

func newRouter(secret string, logger *log.Logger, updates chan models.Update, handler *handlers.Handler, kieResults chan models.KieResult, YouMoneyResult chan models.YouMoneyResult) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Post("/tg/webhook", handlers.DoTgWebhook(secret, logger, updates))
	r.Post("/kie/callback/yWiJwdBfHLxqbtEd9wixxZc9", handlers.DoKieCallback(logger, handler, kieResults))
	r.Post("/payments/youmoney", handlers.DoYoumoneyWebhook(AllowedCIDRs, handler, logger, YouMoneyResult))

	return r
}
