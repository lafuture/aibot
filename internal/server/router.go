package server

import (
	"aibot/internal/models"
	"log"
	"net/http"

	"aibot/internal/handlers"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func newRouter(secret string, logger *log.Logger, updates chan models.Update, handler *handlers.Handler, kieResults chan models.KieResult) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Post("/tg/webhook", handlers.DoTgWebhook(secret, logger, updates))
	r.Post("/kie/callback", handlers.DoKieCallback(logger, handler, kieResults))

	return r
}
