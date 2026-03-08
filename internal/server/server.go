package server

import (
	"aibot/internal/handlers"
	"aibot/internal/models"
	"context"
	"log"
	"net/http"
	"time"
)

type Server struct {
	srv *http.Server
}

func NewServer(addr, secret string, logger *log.Logger, updates chan models.Update, handler *handlers.Handler, kieResults chan models.KieResult, YouMoneyResult chan models.YouMoneyResult) *Server {
	return &Server{
		srv: &http.Server{
			Addr:              addr,
			Handler:           newRouter(secret, logger, updates, handler, kieResults, YouMoneyResult),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
	}
}

func (s *Server) Start() error {
	return s.srv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
