package config

import (
	"os"
	"github.com/joho/godotenv"
)

type Config struct {
	BotToken       string
	WebhookURL     string
	SecretToken    string
	ListenAddr     string
	RedisAddr      string
	RedisPassword  string
	RedisDB        string
	Migra          string
	DbUrl          string
	KieAPIKey      string
	KieCallbackURL string
	ChannelID      string // ID или @username канала для проверки подписки (бот должен быть админом)
	ChannelLink    string // URL кнопки «Подписаться», например https://t.me/channel; если пусто и ChannelID начинается с @ — подставляется t.me/username
}

func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		BotToken:       os.Getenv("BOT_TOKEN"),
		WebhookURL:     os.Getenv("WEBHOOK_URL"),
		SecretToken:    os.Getenv("SECRET_TOKEN"),
		ListenAddr:     os.Getenv("LISTEN_ADDR"),
		RedisAddr:      os.Getenv("REDIS_ADDR"),
		RedisPassword:  os.Getenv("REDIS_PASSWORD"),
		RedisDB:        os.Getenv("REDIS_DB"),
		Migra:          os.Getenv("MIGRATIONSURL"),
		DbUrl:          os.Getenv("DATABASE_URL"),
		KieAPIKey:      os.Getenv("KIE_API_KEY"),
		KieCallbackURL: os.Getenv("CALLBACK_URL"),
		ChannelID:      os.Getenv("CHANNEL_ID"),
		ChannelLink:     os.Getenv("CHANNEL_LINK"),
	}

	return cfg, nil
}
