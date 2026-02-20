package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"aibot/internal/config"
	"aibot/internal/database"
	"aibot/internal/handlers"
	"aibot/internal/models"
	"aibot/internal/ratelimit"
	"aibot/internal/server"
	"aibot/internal/state"

	"github.com/redis/go-redis/v9"
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal(err)
	}

	rDB, err := strconv.Atoi(cfg.RedisDB)
	if err != nil {
		logger.Fatal(err)
	}

	rd := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DB:           rDB,
		PoolSize:     32,
		MinIdleConns: 4,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
	if err := rd.Ping(context.Background()).Err(); err != nil {
		logger.Fatalf("Redis ping failed: %v", err)
	}
	defer rd.Close()
	store := state.NewRedisStore(rd)

	db, err := database.NewDatabase(cfg.DbUrl, cfg.Migra)
	if err != nil {
		logger.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	kieTasks := make(chan int64, 10000)
	kieLimiter := ratelimit.NewFixedWindow(20, 10*time.Second)
	kieCtx, kieCancel := context.WithCancel(context.Background())
	defer kieCancel()
	kieLimiter.StartRefill(kieCtx)

	handler := handlers.NewHandler(store, db, cfg.BotToken, cfg.KieAPIKey, cfg.KieCallbackURL, cfg.ChannelID, cfg.ChannelLink, kieTasks, kieLimiter.Acquire)

	var updateBuf = 10000
	var numUpdateWorkers = 32
	updates := make(chan models.Update, updateBuf)
	var wg sync.WaitGroup
	for i := 0; i < numUpdateWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for upd := range updates {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				handler.HandleUpdate(ctx, upd)
				cancel()
			}
		}()
	}

	var kieWg sync.WaitGroup
	kieWg.Add(1)
	go func() {
		defer kieWg.Done()
		handler.ProcessKieTasks(kieCtx)
	}()

	var KieResultsBuf = 10000
	var numKieResultsWorkers = 32
	kieResults := make(chan models.KieResult, KieResultsBuf)

	for i := 0; i < numKieResultsWorkers; i++ {
		kieWg.Add(1)
		go func() {
			defer kieWg.Done()
			for res := range kieResults {
				ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
				if res.ImageURL == "" {
					if err := handler.SendMessage(ctx, res.ChatID, res.Text); err != nil {
						logger.Printf("SendMessage chat_id=%d: %v", res.ChatID, err)
					}
				} else {
					if err := handler.SendPhoto(ctx, res.ChatID, res.ImageURL); err != nil {
						logger.Printf("SendPhoto chat_id=%d: %v", res.ChatID, err)
					}
				}
				cancel()
			}
		}()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := handlers.SetTgWebhook(ctx, cfg.BotToken, cfg.WebhookURL, cfg.SecretToken, true); err != nil {
		logger.Fatal(err)
	}

	srv := server.NewServer(cfg.ListenAddr, cfg.SecretToken, logger, updates, handler, kieResults)

	go func() {
		logger.Printf("listening on %s", cfg.ListenAddr)
		if err := srv.Start(); err != nil {
			logger.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Printf("server shutdown: %v", err)
	}
	kieCancel()
	close(kieTasks)
	close(kieResults)
	kieWg.Wait()
	close(updates)
	wg.Wait()
	logger.Println("stopped")
}
